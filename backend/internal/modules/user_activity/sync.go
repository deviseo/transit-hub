package user_activity

import (
	"context"
	"errors"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
)

const (
	// 首次同步拉取最近 62 天：足够计算「近 30 天 vs 前 30 天」的月环比。
	initialSyncDays = 62
	// 之后每次同步再向前回补 31 天历史，直到平台能提供的最早日期（分摊对站点的压力）。
	backfillChunkDays = 31
	// sub2api 使用记录默认保留 90 天；new-api 的 quota_data 不会自动清理，最多回补约 400 天（用于同比）。
	sub2apiHistoryDays = 90
	newAPIHistoryDays  = 400
	// 本地每日用量保留天数（去年同期对比需要 365 + 30 天）。
	retentionDays = 400

	maxSiteUsers        = 50000
	usersPageSize       = 100
	maxUserPages        = maxSiteUsers/usersPageSize + 1
	trendLimitMargin    = 1000
	defaultRequestPause = 150 * time.Millisecond

	syncInterval     = time.Hour
	syncStartDelay   = 2 * time.Minute
	syncTimeout      = 30 * time.Minute
	minScheduledGap  = 45 * time.Minute
	stateSaveTimeout = 10 * time.Second

	// new-api 的用量按用户名聚合；用户名对不上当前用户（已删除或改名）时用该前缀保留历史用量，
	// 只计入整体统计，不出现在用户列表中。
	unmatchedUserPrefix = "~"
)

// historyLimitDays 返回平台最多能回补的天数。
func historyLimitDays(platform upstream.Platform) int {
	if platform == upstream.PlatformNewAPI {
		return newAPIHistoryDays
	}
	return sub2apiHistoryDays
}

func supportedPlatform(platform upstream.Platform) bool {
	return platform == upstream.PlatformSub2API || platform == upstream.PlatformNewAPI
}

// TriggerSync 手动触发同步：先校验管理员会话（失败立即返回），再在后台拉取数据。
// 已有同步在进行时直接返回当前状态。
func (s *Service) TriggerSync(ctx context.Context, ws Workspace) (SyncStatus, error) {
	if !s.acquire(ws) {
		state, ok, err := s.store.GetSyncState(ctx, ws)
		if err != nil {
			return SyncStatus{}, err
		}
		status := s.syncStatus(state, ok)
		status.Status = SyncStatusRunning
		return status, nil
	}
	session, err := s.sessions.RequireSession(ctx, ws.UserID, ws.AdminAccountID)
	if err != nil {
		s.release(ws)
		return SyncStatus{}, mapSessionError(err)
	}
	if !supportedPlatform(session.Platform) {
		s.release(ws)
		return SyncStatus{}, requestError(ErrorPlatform)
	}
	state, err := s.markRunning(ctx, ws, session.Platform, StartedByManual)
	if err != nil {
		s.release(ws)
		return SyncStatus{}, err
	}
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		defer s.release(ws)
		runCtx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()
		_ = s.runSync(runCtx, ws, session, state)
	}()
	return s.syncStatus(state, true), nil
}

// SyncWorkspace 同步执行一次例行同步（定时任务使用）。
func (s *Service) SyncWorkspace(ctx context.Context, ws Workspace) error {
	return s.SyncWorkspaceAs(ctx, ws, StartedBySchedule)
}

// SyncWorkspaceAs 同步执行一次，startedBy 记录这次同步是怎么开始的。还没同步过的工作区会话不可用时不创建状态。
func (s *Service) SyncWorkspaceAs(ctx context.Context, ws Workspace, startedBy string) error {
	if !s.acquire(ws) {
		return requestError(ErrorSyncInProgress)
	}
	defer s.release(ws)
	session, err := s.sessions.RequireSession(ctx, ws.UserID, ws.AdminAccountID)
	if err != nil {
		mapped := mapSessionError(err)
		s.saveFailure(ws, mapped, startedBy)
		return mapped
	}
	if !supportedPlatform(session.Platform) {
		s.saveFailure(ws, requestError(ErrorPlatform), startedBy)
		return requestError(ErrorPlatform)
	}
	state, err := s.markRunning(ctx, ws, session.Platform, startedBy)
	if err != nil {
		return err
	}
	return s.runSync(ctx, ws, session, state)
}

// WaitBackground 等待后台同步结束（测试与优雅退出使用）。
func (s *Service) WaitBackground() {
	s.background.Wait()
}

func (s *Service) acquire(ws Workspace) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.running[ws]; exists {
		return false
	}
	s.running[ws] = struct{}{}
	return true
}

func (s *Service) release(ws Workspace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, ws)
}

func (s *Service) markRunning(ctx context.Context, ws Workspace, platform upstream.Platform, startedBy string) (SyncState, error) {
	state, _, err := s.store.GetSyncState(ctx, ws)
	if err != nil {
		return SyncState{}, err
	}
	started := s.now()
	state.Platform = string(platform)
	state.Status = SyncStatusRunning
	state.ErrorKey = ""
	state.LastStartedAt = &started
	state.StartedBy = startedBy
	if err := s.store.SaveSyncState(ctx, ws, state); err != nil {
		return SyncState{}, err
	}
	return state, nil
}

// saveFailure 记录一次没能开始的同步（会话不可用、平台不支持）：也算一次尝试，记下尝试时间和方式，页面据此提醒。
func (s *Service) saveFailure(ws Workspace, err error, startedBy string) {
	ctx, cancel := context.WithTimeout(context.Background(), stateSaveTimeout)
	defer cancel()
	state, ok, getErr := s.store.GetSyncState(ctx, ws)
	if getErr != nil || !ok {
		// 从未同步过的工作区不因定时任务失败而创建状态。
		return
	}
	attemptedAt := s.now()
	state.LastStartedAt = &attemptedAt
	state.StartedBy = startedBy
	state.Status = SyncStatusFailed
	state.ErrorKey = errorKeyOf(err)
	if saveErr := s.store.SaveSyncState(ctx, ws, state); saveErr != nil {
		log.Printf("[user-activity] save sync failure failed user_id=%s admin_account_id=%s err=%v", ws.UserID, ws.AdminAccountID, saveErr)
	}
}

func (s *Service) runSync(ctx context.Context, ws Workspace, session upstream.Session, state SyncState) error {
	started := s.now()
	err := s.collect(ctx, ws, session, &state)
	if err != nil {
		state.Status = SyncStatusFailed
		state.ErrorKey = errorKeyOf(err)
		log.Printf("[user-activity] sync failed user_id=%s admin_account_id=%s platform=%s err=%v", ws.UserID, ws.AdminAccountID, session.Platform, err)
	} else {
		state.Status = SyncStatusSuccess
		state.ErrorKey = ""
		log.Printf("[user-activity] sync done user_id=%s admin_account_id=%s platform=%s users=%d duration=%s", ws.UserID, ws.AdminAccountID, session.Platform, state.UserCount, s.now().Sub(started).Round(time.Millisecond))
	}
	saveCtx, cancel := context.WithTimeout(context.Background(), stateSaveTimeout)
	defer cancel()
	if saveErr := s.store.SaveSyncState(saveCtx, ws, state); saveErr != nil {
		log.Printf("[user-activity] save sync state failed user_id=%s admin_account_id=%s err=%v", ws.UserID, ws.AdminAccountID, saveErr)
		if err == nil {
			err = saveErr
		}
	}
	return err
}

// collect 拉取用户、用量与入账并写入本地。分两步：先写入用户与最近的数据（让页面尽快有数据），
// 再回补一段更早的历史；回补失败不影响本次同步结果，下次继续。
func (s *Service) collect(ctx context.Context, ws Workspace, session upstream.Session, state *SyncState) error {
	now := s.now()
	today := s.dayOf(now)
	retainFrom := today.AddDate(0, 0, -(retentionDays - 1))
	earliest := today.AddDate(0, 0, -(historyLimitDays(session.Platform) - 1))
	if earliest.Before(retainFrom) {
		earliest = retainFrom
	}
	state.WarningKey = ""

	users, complete, err := s.fetchUsers(ctx, session)
	if err != nil {
		return err
	}

	recentFrom := resumeFrom(today, state.SyncedFrom, state.SyncedTo, earliest)
	fetcher := &dayFetcher{service: s, session: session, now: now, trendLimit: len(users.profiles) + trendLimitMargin, usernameToID: users.usernameToID}
	recentDays := dayList(recentFrom, today)
	recent, err := fetcher.fetchDays(ctx, recentDays)
	if err != nil {
		return err
	}
	snapshot := Snapshot{
		Profiles:             users.profiles,
		Days:                 recentDays,
		Daily:                recent.daily,
		LastUsed:             recent.lastUsed,
		SyncStamp:            now,
		PruneMissingProfiles: complete,
		RetainFrom:           retainFrom,
		SiteDaily:            siteDailyOf(today, users.profiles),
	}

	// 入账：sub2api 读取充值记录（从入账覆盖的最后一天开始补到今天）；new-api 按「剩余额度 + 累计已用」的快照差值累加到今天。
	rechargeFrom := resumeFrom(today, state.RechargeSyncedFrom, state.RechargeSyncedTo, retainFrom)
	rechargeFetched := false
	switch session.Platform {
	case upstream.PlatformSub2API:
		scan, err := s.fetchSub2APIRecharges(ctx, session, s.localMidnight(rechargeFrom), s.localMidnight(today.AddDate(0, 0, 1)), retainFrom)
		if err != nil {
			state.WarningKey = rechargeWarningKey(err)
			log.Printf("[user-activity] recharge fetch failed user_id=%s admin_account_id=%s err=%v", ws.UserID, ws.AdminAccountID, err)
		} else {
			snapshot.RechargeDays = dayList(rechargeFrom, today)
			snapshot.Recharges = scan.Recharges
			rechargeFetched = true
		}
	case upstream.PlatformNewAPI:
		snapshot.FundedDeltaDay = today
	}
	if err := s.store.ReplaceSnapshot(ctx, ws, snapshot); err != nil {
		return err
	}

	// 本地连续覆盖的区间：本次拉取的区间与上次的区间相接时合并，否则从本次开始重新累计。
	syncedFrom := mergeCoverage(recentFrom, state.SyncedFrom, state.SyncedTo, retainFrom)
	syncedTo := today
	syncedAt := now
	state.SyncedFrom = &syncedFrom
	state.SyncedTo = &syncedTo
	state.UserCount = len(users.profiles)
	state.LastSyncedAt = &syncedAt
	switch session.Platform {
	case upstream.PlatformSub2API:
		if rechargeFetched {
			from := mergeCoverage(rechargeFrom, state.RechargeSyncedFrom, state.RechargeSyncedTo, retainFrom)
			to := today
			state.RechargeSyncedFrom, state.RechargeSyncedTo = &from, &to
		}
	case upstream.PlatformNewAPI:
		if state.RechargeSince == nil {
			// 首次同步只建立基准，之后每次同步把差值记为入账。
			since, from := now, today
			state.RechargeSince, state.RechargeSyncedFrom = &since, &from
		}
		to := today
		state.RechargeSyncedTo = &to
	}
	// 状态仍为 running，但先保存覆盖区间，让页面在回补历史期间就能展示最新数据。
	if err := s.store.SaveSyncState(ctx, ws, *state); err != nil {
		return err
	}

	if err := s.backfillUsage(ctx, ws, fetcher, state, earliest, now); err != nil {
		return err
	}
	if session.Platform == upstream.PlatformSub2API && rechargeFetched {
		if err := s.backfillSub2APIRecharges(ctx, ws, session, state, retainFrom, now); err != nil {
			return err
		}
	}
	return nil
}

// backfillUsage 向前回补一段（backfillChunkDays 天）用量，从近到远，失败时保留已拉到的连续区间。
func (s *Service) backfillUsage(ctx context.Context, ws Workspace, fetcher *dayFetcher, state *SyncState, earliest time.Time, now time.Time) error {
	backfillTo := state.SyncedFrom.AddDate(0, 0, -1)
	backfillFrom := backfillTo.AddDate(0, 0, -(backfillChunkDays - 1))
	if backfillFrom.Before(earliest) {
		backfillFrom = earliest
	}
	if backfillFrom.After(backfillTo) {
		return nil
	}
	days := dayList(backfillFrom, backfillTo)
	fetched := make([]time.Time, 0, len(days))
	older := collectedUsage{lastUsed: map[string]time.Time{}}
	for i := len(days) - 1; i >= 0; i-- {
		if err := s.sleep(ctx); err != nil {
			break
		}
		usage, err := fetcher.fetchDay(ctx, days[i])
		if err != nil {
			log.Printf("[user-activity] backfill stopped user_id=%s admin_account_id=%s day=%s err=%v", ws.UserID, ws.AdminAccountID, formatDay(days[i]), err)
			break
		}
		fetched = append(fetched, days[i])
		older.merge(usage)
	}
	if len(fetched) == 0 {
		return nil
	}
	if err := s.store.ReplaceSnapshot(ctx, ws, Snapshot{Days: fetched, Daily: older.daily, LastUsed: older.lastUsed, SyncStamp: now}); err != nil {
		return err
	}
	earliestFetched := fetched[len(fetched)-1]
	state.SyncedFrom = &earliestFetched
	return nil
}

// backfillSub2APIRecharges 向前回补一段（backfillChunkDays 天）入账记录。sub2api 的充值记录不会过期，
// 可以一直回补到本地保留期起点；站点上已没有更早的记录时直接把覆盖区间延伸到保留期起点。
func (s *Service) backfillSub2APIRecharges(ctx context.Context, ws Workspace, session upstream.Session, state *SyncState, retainFrom time.Time, now time.Time) error {
	if state.RechargeSyncedFrom == nil {
		return nil
	}
	backfillTo := state.RechargeSyncedFrom.AddDate(0, 0, -1)
	backfillFrom := backfillTo.AddDate(0, 0, -(backfillChunkDays - 1))
	if backfillFrom.Before(retainFrom) {
		backfillFrom = retainFrom
	}
	if backfillFrom.After(backfillTo) {
		return nil
	}
	scan, err := s.fetchSub2APIRecharges(ctx, session, s.localMidnight(backfillFrom), s.localMidnight(backfillTo.AddDate(0, 0, 1)), retainFrom)
	if err != nil {
		log.Printf("[user-activity] recharge backfill stopped user_id=%s admin_account_id=%s err=%v", ws.UserID, ws.AdminAccountID, err)
		return nil
	}
	if err := s.store.ReplaceSnapshot(ctx, ws, Snapshot{RechargeDays: dayList(backfillFrom, backfillTo), Recharges: scan.Recharges, SyncStamp: now}); err != nil {
		return err
	}
	from := backfillFrom
	if scan.NoneBefore {
		from = retainFrom
	}
	state.RechargeSyncedFrom = &from
	return nil
}

// resumeFrom 返回本次需要重新拉取的起始日：首次同步拉取最近 initialSyncDays 天；之后从上次覆盖到的那天
// （当时那天的数据还不完整，且至少重拉昨天）开始，最多回看 initialSyncDays 天，且不早于 floor。
func resumeFrom(today time.Time, syncedFrom *time.Time, syncedTo *time.Time, floor time.Time) time.Time {
	from := today.AddDate(0, 0, -(initialSyncDays - 1))
	if syncedFrom != nil && syncedTo != nil {
		resume := *syncedTo
		if yesterday := today.AddDate(0, 0, -1); resume.After(yesterday) {
			resume = yesterday
		}
		if resume.After(from) {
			from = resume
		}
	}
	if from.Before(floor) {
		from = floor
	}
	return from
}

// mergeCoverage 合并覆盖区间：本次拉取的区间与上次的区间相接时沿用更早的起点，否则从本次开始重新累计。
func mergeCoverage(fetchedFrom time.Time, previousFrom *time.Time, previousTo *time.Time, floor time.Time) time.Time {
	from := fetchedFrom
	if previousFrom != nil && previousTo != nil && !fetchedFrom.After(previousTo.AddDate(0, 0, 1)) && previousFrom.Before(from) {
		from = *previousFrom
	}
	if from.Before(floor) {
		from = floor
	}
	return from
}

func (s *Service) sleep(ctx context.Context) error {
	if s.pause <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(s.pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type siteUsers struct {
	profiles     []Profile
	usernameToID map[string]string
}

// fetchUsers 拉取站点全部用户。complete 表示确认拿到了完整列表（用于清理站点上已删除的用户）。
func (s *Service) fetchUsers(ctx context.Context, session upstream.Session) (siteUsers, bool, error) {
	result := siteUsers{usernameToID: map[string]string{}}
	seen := map[string]struct{}{}
	total := -1
	ended := false
	for page := 1; page <= maxUserPages; page++ {
		if page > 1 {
			if err := s.sleep(ctx); err != nil {
				return siteUsers{}, false, err
			}
		}
		var (
			profiles   []Profile
			pageTotal  int
			totalKnown bool
			last       bool
		)
		switch session.Platform {
		case upstream.PlatformSub2API:
			response, err := s.platform.FetchSub2APIAdminUsersPage(session, upstream.Sub2APIAdminUsersQuery{
				Page: page, PageSize: usersPageSize, SortBy: "created_at", SortOrder: "desc", Timezone: s.tzName,
			})
			if err != nil {
				return siteUsers{}, false, usersError(err)
			}
			for _, user := range response.Items {
				profiles = append(profiles, sub2apiProfile(user))
			}
			pageTotal, totalKnown = response.Total, response.TotalKnown
			last = len(response.Items) == 0 || (response.PagesKnown && page >= response.Pages) || (!response.PagesKnown && len(response.Items) < usersPageSize)
		case upstream.PlatformNewAPI:
			response, err := s.platform.FetchNewAPIAdminUsersPage(session, page, usersPageSize)
			if err != nil {
				return siteUsers{}, false, usersError(err)
			}
			for _, user := range response.Items {
				profiles = append(profiles, newAPIProfile(user))
				if name := strings.TrimSpace(user.Username); name != "" {
					result.usernameToID[name] = user.ID
				}
			}
			pageTotal, totalKnown = response.Total, response.TotalKnown
			last = len(response.Items) < usersPageSize
		default:
			return siteUsers{}, false, requestError(ErrorPlatform)
		}
		if totalKnown {
			total = pageTotal
			if total > maxSiteUsers {
				return siteUsers{}, false, requestError(ErrorTooManyUsers)
			}
		}
		added := 0
		for _, profile := range profiles {
			if profile.SiteUserID == "" {
				continue
			}
			if _, exists := seen[profile.SiteUserID]; exists {
				continue
			}
			seen[profile.SiteUserID] = struct{}{}
			result.profiles = append(result.profiles, profile)
			added++
		}
		if len(result.profiles) > maxSiteUsers {
			return siteUsers{}, false, requestError(ErrorTooManyUsers)
		}
		if total >= 0 && len(result.profiles) >= total {
			ended = true
			break
		}
		if last || added == 0 {
			// added == 0：站点忽略了分页参数或只返回重复数据，停止翻页避免死循环。
			ended = last
			break
		}
	}
	complete := ended && (total < 0 || len(result.profiles) >= total)
	return result, complete, nil
}

func sub2apiProfile(user upstream.Sub2APIAdminUser) Profile {
	everUsed := user.LastUsedAt != nil || (user.TotalRecharged != nil && *user.TotalRecharged > 0)
	return Profile{
		SiteUserID:     strings.TrimSpace(user.ID),
		Email:          strings.TrimSpace(user.Email),
		Username:       strings.TrimSpace(user.Username),
		Role:           normalizeRole(user.Role),
		Status:         normalizeStatus(user.Status),
		Balance:        user.Balance,
		TotalRecharged: user.TotalRecharged,
		EverUsed:       everUsed,
		SiteCreatedAt:  user.CreatedAt,
		LastUsedAt:     user.LastUsedAt,
	}
}

func newAPIProfile(user upstream.NewAPIAdminUser) Profile {
	balance := user.Balance
	used := user.UsedQuota
	// 消费只是把额度从 quota 挪到 used_quota，二者之和不变；充值、兑换、管理员调整额度会改变这个和。
	funded := user.Balance + user.UsedQuota
	role := RoleUser
	switch {
	case user.Role >= 100:
		role = RoleRoot
	case user.Role >= 10:
		role = RoleAdmin
	}
	status := strconv.Itoa(user.Status)
	switch user.Status {
	case 1:
		status = StatusActive
	case 2:
		status = StatusDisabled
	}
	return Profile{
		SiteUserID:    strings.TrimSpace(user.ID),
		Email:         strings.TrimSpace(user.Email),
		Username:      strings.TrimSpace(user.Username),
		Role:          role,
		Status:        status,
		Group:         strings.TrimSpace(user.Group),
		Balance:       &balance,
		TotalUsed:     &used,
		FundedTotal:   &funded,
		EverUsed:      user.EverUsed,
		SiteCreatedAt: user.CreatedAt,
		// new-api 没有「最近使用时间」字段，由用量数据推算（见 dayFetcher）。
	}
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "admin":
		return RoleAdmin
	case "root", "super_admin", "superadmin":
		return RoleRoot
	case "":
		return RoleUser
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "enabled", "1", "normal":
		return StatusActive
	case "disabled", "banned", "inactive", "2":
		return StatusDisabled
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

type collectedUsage struct {
	daily    []DailyUsage
	lastUsed map[string]time.Time
}

func (c *collectedUsage) merge(other collectedUsage) {
	c.daily = append(c.daily, other.daily...)
	if c.lastUsed == nil {
		c.lastUsed = map[string]time.Time{}
	}
	for id, at := range other.lastUsed {
		if current, ok := c.lastUsed[id]; !ok || at.After(current) {
			c.lastUsed[id] = at
		}
	}
}

// dayFetcher 按「一次一天」拉取用量：每天的数据完全按业务时区切分，不依赖站点数据库时区对日期的标注。
type dayFetcher struct {
	service      *Service
	session      upstream.Session
	now          time.Time
	trendLimit   int
	usernameToID map[string]string
}

func (f *dayFetcher) fetchDays(ctx context.Context, days []time.Time) (collectedUsage, error) {
	result := collectedUsage{lastUsed: map[string]time.Time{}}
	for i, day := range days {
		if i > 0 {
			if err := f.service.sleep(ctx); err != nil {
				return collectedUsage{}, err
			}
		}
		usage, err := f.fetchDay(ctx, day)
		if err != nil {
			return collectedUsage{}, err
		}
		result.merge(usage)
	}
	return result, nil
}

func (f *dayFetcher) fetchDay(ctx context.Context, day time.Time) (collectedUsage, error) {
	if err := ctx.Err(); err != nil {
		return collectedUsage{}, err
	}
	byUser := map[string]*DailyUsage{}
	lastUsed := map[string]time.Time{}
	add := func(id string, requests int64, tokens int64, cost float64) {
		usage, ok := byUser[id]
		if !ok {
			usage = &DailyUsage{SiteUserID: id, Day: day}
			byUser[id] = usage
		}
		usage.Requests += requests
		usage.Tokens += tokens
		usage.Cost += cost
	}
	switch f.session.Platform {
	case upstream.PlatformSub2API:
		date := formatDay(day)
		points, err := f.service.platform.FetchSub2APIUserUsageTrend(f.session, date, date, f.service.tzName, f.trendLimit)
		if err != nil {
			return collectedUsage{}, usageError(err)
		}
		// 请求的区间就是这一天（按业务时区），所有返回行都归到这一天；用户实付金额 actual_cost 作为消费。
		// sub2api 用户列表中的 last_used_at 精确到秒，这里不再推算最近使用时间。
		for _, point := range points {
			add(point.UserID, point.Requests, point.Tokens, point.ActualCost)
		}
	case upstream.PlatformNewAPI:
		start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, f.service.loc)
		end := start.AddDate(0, 0, 1).Add(-time.Second)
		buckets, err := f.service.platform.FetchNewAPIUserQuotaData(f.session, start, end)
		if err != nil {
			return collectedUsage{}, usageError(err)
		}
		for _, bucket := range buckets {
			if bucket.Requests <= 0 && bucket.Tokens <= 0 && bucket.Cost <= 0 {
				continue
			}
			id, matched := f.usernameToID[bucket.Username]
			if !matched {
				id = unmatchedUserPrefix + bucket.Username
			}
			add(id, bucket.Requests, bucket.Tokens, bucket.Cost)
			if matched && bucket.Requests > 0 {
				// quota_data 按小时聚合：取该小时结束（且不晚于现在）作为「最近使用时间」的上界，
				// 不会把用户算得比实际更久没用。
				at := bucket.HourStart.Add(time.Hour)
				if at.After(f.now) {
					at = f.now
				}
				if current, ok := lastUsed[id]; !ok || at.After(current) {
					lastUsed[id] = at
				}
			}
		}
	default:
		return collectedUsage{}, requestError(ErrorPlatform)
	}
	ids := make([]string, 0, len(byUser))
	for id := range byUser {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	daily := make([]DailyUsage, 0, len(ids))
	for _, id := range ids {
		daily = append(daily, *byUser[id])
	}
	return collectedUsage{daily: daily, lastUsed: lastUsed}, nil
}

// dayList 返回 [from, to]（含两端）的每一天，升序。
func dayList(from time.Time, to time.Time) []time.Time {
	days := make([]time.Time, 0)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		days = append(days, day)
	}
	return days
}

// syncError 记录同步失败的原因（i18n key）以及原始错误（只写日志）。
type syncError struct {
	key   string
	cause error
}

func (e *syncError) Error() string {
	if e.cause == nil {
		return e.key
	}
	return e.key + ": " + e.cause.Error()
}

func (e *syncError) Unwrap() error { return e.cause }

func usersError(err error) error {
	return &syncError{key: upstreamErrorKey(err, ErrorUsersFetch), cause: err}
}

func usageError(err error) error {
	return &syncError{key: upstreamErrorKey(err, ErrorUsageFetch), cause: err}
}

// upstreamErrorKey 把站点请求错误转换为前端文案：网络错误沿用上游模块的提示，鉴权失败提示重新登录，其它用 fallback。
func upstreamErrorKey(err error, fallback string) string {
	var requestErr *upstream.RequestError
	if errors.As(err, &requestErr) {
		switch requestErr.MessageKey {
		case upstream.ErrorNetwork:
			return ErrorNetwork
		case upstream.ErrorAuth, upstream.ErrorVerificationRequired, upstream.ErrorNewAPITokenRejected:
			return ErrorAuthRequired
		}
		if requestErr.StatusCode == 401 {
			return ErrorAuthRequired
		}
	}
	return fallback
}

func mapSessionError(err error) error {
	message := err.Error()
	if strings.Contains(message, "authRequired") || strings.Contains(message, "adminOnly") || strings.Contains(message, upstream.ErrorAuth) {
		return requestError(ErrorAuthRequired)
	}
	if strings.Contains(message, upstream.ErrorNetwork) {
		return requestError(ErrorNetwork)
	}
	return requestError(ErrorRequest)
}

func errorKeyOf(err error) string {
	var syncErr *syncError
	if errors.As(err, &syncErr) {
		return syncErr.key
	}
	var keyErr requestError
	if errors.As(err, &keyErr) {
		return string(keyErr)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrorInterrupted
	}
	return ErrorRequest
}

// StartScheduler 启动后台同步：重启前未完成的同步标记为被打断；服务启动 syncStartDelay 后在后台继续这些同步，
// 并为新工作区开始首次同步；此后每小时（hh:00:30）同步所有同步过的工作区。
func (s *Service) StartScheduler(ctx context.Context) {
	if err := s.store.ResetRunningSyncs(ctx, ErrorInterrupted); err != nil {
		log.Printf("[user-activity] reset interrupted syncs failed err=%v", err)
	}
	go func() {
		timer := time.NewTimer(syncStartDelay)
		defer timer.Stop()
		startup := true
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			s.syncDueWorkspaces(ctx, startup)
			startup = false
			timer.Reset(s.untilNextSlot(s.now()))
		}
	}()
}

// untilNextSlot 返回到下一个整点（业务时区，hh:00:30）的时长。按整点同步让 new-api 的快照差值
// 与自然日边界对齐：每天第一次同步发生在 00:00 之后几十秒内。
func (s *Service) untilNextSlot(now time.Time) time.Duration {
	local := now.In(s.loc)
	next := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 30, 0, s.loc).Add(syncInterval)
	wait := next.Sub(now)
	if wait < time.Minute {
		wait += syncInterval
	}
	return wait
}

// syncDueWorkspaces 是一轮后台同步：被重启打断的同步马上继续（resume）；其余同步过的工作区例行同步（schedule，
// 服务刚启动的那一轮跳过，留给整点）；保存了管理员会话、还没同步过的工作区自动开始首次同步（auto）。
func (s *Service) syncDueWorkspaces(ctx context.Context, startup bool) {
	workspaces, err := s.store.ListSyncedWorkspaces(ctx)
	if err != nil {
		log.Printf("[user-activity] list workspaces failed err=%v", err)
		return
	}
	known := make(map[Workspace]bool, len(workspaces))
	for _, ws := range workspaces {
		known[ws] = true
		if ctx.Err() != nil {
			return
		}
		state, ok, err := s.store.GetSyncState(ctx, ws)
		if err != nil || !ok {
			continue
		}
		startedBy := StartedBySchedule
		if state.Status == SyncStatusFailed && state.ErrorKey == ErrorInterrupted {
			startedBy = StartedByResume
		} else if startup {
			continue
		} else if state.LastStartedAt != nil && s.now().Sub(*state.LastStartedAt) < minScheduledGap && s.dayOf(*state.LastStartedAt).Equal(s.dayOf(s.now())) {
			// 刚手动同步过的工作区跳过本轮；但每天的第一次同步不跳过，保证日界附近的入账归到正确的日期。
			continue
		}
		s.runScheduled(ctx, ws, startedBy)
	}
	if s.workspaces == nil {
		return
	}
	candidates, err := s.workspaces.SessionWorkspaces(ctx)
	if err != nil {
		log.Printf("[user-activity] list session workspaces failed err=%v", err)
		return
	}
	for _, ws := range candidates {
		if known[ws] || ctx.Err() != nil {
			continue
		}
		s.runScheduled(ctx, ws, StartedByAuto)
	}
}

func (s *Service) runScheduled(ctx context.Context, ws Workspace, startedBy string) {
	runCtx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	err := s.SyncWorkspaceAs(runCtx, ws, startedBy)
	if err == nil || errors.Is(err, requestError(ErrorSyncInProgress)) {
		return
	}
	// 还没同步过的工作区会话失效或平台不支持时不记日志（每小时都会再试），避免刷屏。
	if startedBy == StartedByAuto && (errors.Is(err, requestError(ErrorAuthRequired)) || errors.Is(err, requestError(ErrorPlatform))) {
		return
	}
	log.Printf("[user-activity] background sync failed user_id=%s admin_account_id=%s started_by=%s err=%v", ws.UserID, ws.AdminAccountID, startedBy, err)
}
