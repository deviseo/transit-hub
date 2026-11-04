package upstream

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

// 仪表盘「今日成本」相关：缓存里的「今日消费」是站点上一次同步成功时的值，跨天后在重新同步前仍是前一天的数，
// 不能当作今天的成本。这里提供两件事：
//   - RefreshStaleSites：打开仪表盘时补同步今天还没有同步成功的站点；
//   - ConsumeOnDate：直接按日期向上游查询某一天的消费，用于历史快照定稿。

const (
	staleRefreshConcurrency = 4
	// staleRefreshCooldown 内不重复为同一站点触发补同步（避免同步一直失败的站点在每次打开仪表盘时都被请求）。
	staleRefreshCooldown  = 2 * time.Minute
	consumeOnDateParallel = 4
)

type staleRefreshState struct {
	mu        sync.Mutex
	inFlight  map[string]struct{}
	attempted map[string]time.Time
}

// SyncedSince 判断站点是否在 since 之后同步成功过（LastSyncedAt 只在同步成功时更新）。
func SyncedSince(lastSyncedAt *int64, since time.Time) bool {
	return lastSyncedAt != nil && *lastSyncedAt >= since.UnixMilli()
}

// RefreshStaleSites 同步工作区内今天（since 之后）还没有同步成功的站点（有会话、充值倍率 > 0）。
// 并发上限 4，最多等待 wait；超时未完成的同步在后台继续，完成后更新缓存，下次打开仪表盘即可看到。
func (s *Service) RefreshStaleSites(ctx context.Context, userID string, adminAccountID string, since time.Time, wait time.Duration) {
	sites, err := s.cache.ListByUser(ctx, userID)
	if err != nil {
		log.Printf("[upstream] stale refresh: list sites failed user_id=%s err=%v", userID, err)
		return
	}
	now := time.Now()
	targets := make([]string, 0)
	for _, site := range sites {
		if site.AdminAccountID != adminAccountID || site.Session == nil || site.RechargeRate <= 0 || SyncedSince(site.LastSyncedAt, since) {
			continue
		}
		if s.beginStaleRefresh(site.ID, now) {
			targets = append(targets, site.ID)
		}
	}
	if len(targets) == 0 {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sem := make(chan struct{}, staleRefreshConcurrency)
		var wg sync.WaitGroup
		for _, id := range targets {
			wg.Add(1)
			sem <- struct{}{}
			go func(id string) {
				defer wg.Done()
				defer func() { <-sem }()
				defer s.endStaleRefresh(id)
				// 使用独立的 context：请求返回后同步仍要完成并写回缓存。
				if _, err := s.sync(context.Background(), id); err != nil {
					log.Printf("[upstream] stale refresh failed site_id=%s err=%v", id, err)
				}
			}(id)
		}
		wg.Wait()
	}()
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	case <-ctx.Done():
	}
}

func (s *Service) beginStaleRefresh(siteID string, now time.Time) bool {
	s.staleRefresh.mu.Lock()
	defer s.staleRefresh.mu.Unlock()
	if s.staleRefresh.inFlight == nil {
		s.staleRefresh.inFlight = map[string]struct{}{}
		s.staleRefresh.attempted = map[string]time.Time{}
	}
	if _, running := s.staleRefresh.inFlight[siteID]; running {
		return false
	}
	if last, ok := s.staleRefresh.attempted[siteID]; ok && now.Sub(last) < staleRefreshCooldown {
		return false
	}
	s.staleRefresh.inFlight[siteID] = struct{}{}
	s.staleRefresh.attempted[siteID] = now
	return true
}

func (s *Service) endStaleRefresh(siteID string) {
	s.staleRefresh.mu.Lock()
	defer s.staleRefresh.mu.Unlock()
	delete(s.staleRefresh.inFlight, siteID)
}

// DailyConsume 是工作区内各上游站点某一天的消费合计（已乘充值倍率，人民币）。
type DailyConsume struct {
	Total float64
	// Sites 是计入的站点数；Approximated 是查询失败、改用当天最后一次同步缓存值的站点数；Failed 是无法得到数据的站点数。
	Sites        int
	Approximated int
	Failed       int
}

// 站点某一天消费的状态。
const (
	ConsumeStatusOK           = "ok"
	ConsumeStatusApproximated = "approximated"
	ConsumeStatusFailed       = "failed"
	ConsumeStatusNoRate       = "no_rate"
)

// SiteDailyConsume 是一个上游站点在某一天的消费（上游额度）；Cost = Consume × RechargeRate（人民币）。
type SiteDailyConsume struct {
	SiteID       string
	SiteName     string
	BaseURL      string
	RechargeRate float64
	Consume      float64
	Cost         float64
	Status       string
}

// SiteConsumesOnDate 逐个查询工作区内上游站点在 date（YYYY-MM-DD，按 timezone 切分）这一天的消费。
// 刷新会话后直接按日期查询；查询失败时，如果缓存是在这一天内同步的，用缓存中的「今日消费」近似
// （即当天最后一次同步的值），否则标记为失败，不当作 0。没有设置充值倍率的站点不查询、不计成本。
func (s *Service) SiteConsumesOnDate(ctx context.Context, userID string, adminAccountID string, date string, timezone string) ([]SiteDailyConsume, error) {
	sites, err := s.cache.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}
	dayStart, err := time.ParseInLocation("2006-01-02", date, loc)
	if err != nil {
		return nil, err
	}
	dayEnd := dayStart.AddDate(0, 0, 1)

	results := make([]SiteDailyConsume, 0, len(sites))
	var mu sync.Mutex
	sem := make(chan struct{}, consumeOnDateParallel)
	var wg sync.WaitGroup
	for _, site := range sites {
		if site.AdminAccountID != adminAccountID {
			continue
		}
		base := SiteDailyConsume{SiteID: site.ID, SiteName: site.Name, BaseURL: site.BaseURL, RechargeRate: site.RechargeRate}
		if site.RechargeRate <= 0 {
			base.Status = ConsumeStatusNoRate
			results = append(results, base)
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(site *Site, item SiteDailyConsume) {
			defer wg.Done()
			defer func() { <-sem }()
			amount, ok := s.siteConsumeOnDate(ctx, site, date, timezone)
			switch {
			case ok:
				item.Consume = amount
				item.Status = ConsumeStatusOK
			// 查询失败：只有当缓存是在这一天里同步的，「今日消费」才属于这一天。
			case site.Metrics.TodayConsume.Value != nil && site.LastSyncedAt != nil &&
				*site.LastSyncedAt >= dayStart.UnixMilli() && *site.LastSyncedAt < dayEnd.UnixMilli():
				item.Consume = *site.Metrics.TodayConsume.Value
				item.Status = ConsumeStatusApproximated
			default:
				item.Status = ConsumeStatusFailed
			}
			item.Cost = item.Consume * site.RechargeRate
			mu.Lock()
			results = append(results, item)
			mu.Unlock()
		}(site, base)
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].SiteID < results[j].SiteID })
	return results, nil
}

// ConsumeOnDate 汇总工作区内各上游站点在 date 这一天的消费 × 充值倍率（见 SiteConsumesOnDate）。
func (s *Service) ConsumeOnDate(ctx context.Context, userID string, adminAccountID string, date string, timezone string) (DailyConsume, error) {
	sites, err := s.SiteConsumesOnDate(ctx, userID, adminAccountID, date, timezone)
	if err != nil {
		return DailyConsume{}, err
	}
	result := DailyConsume{}
	for _, site := range sites {
		switch site.Status {
		case ConsumeStatusOK:
			result.Total += site.Cost
			result.Sites++
		case ConsumeStatusApproximated:
			result.Total += site.Cost
			result.Sites++
			result.Approximated++
		case ConsumeStatusFailed:
			result.Failed++
		}
	}
	return result, nil
}

func (s *Service) siteConsumeOnDate(ctx context.Context, site *Site, date string, timezone string) (float64, bool) {
	if site.Session == nil {
		return 0, false
	}
	session := *site.Session
	refreshed, err := s.platformService.RefreshSession(session)
	if err != nil {
		log.Printf("[upstream] consume on date: refresh session failed site_id=%s err=%v", site.ID, err)
		return 0, false
	}
	// 刷新成功后先写回会话，再查询：查询失败也不能丢掉已轮换的 refresh token。
	s.writeBackRefreshedSession(ctx, site.ID, site.UserID, session, refreshed)
	amount, err := s.platformService.FetchSelfConsumeOnDate(refreshed, date, timezone)
	if err != nil {
		log.Printf("[upstream] consume on date failed site_id=%s date=%s err=%v", site.ID, date, err)
		return 0, false
	}
	return amount, true
}
