package vip

import (
	"context"
	"errors"
	"log"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"transithub/backend/internal/shared/bizclock"
)

// store 是 Repository 的接口，便于用内存实现测试服务层逻辑。
type store interface {
	SyncState(ctx context.Context, ws Workspace) (syncState, bool, error)
	Watchlist(ctx context.Context, ws Workspace, win windows, dailyTop, lifetimeTop int) ([]statRow, error)
	DismissedCount(ctx context.Context, ws Workspace) (int, error)
	Search(ctx context.Context, ws Workspace, win windows, query string, limit int) ([]Candidate, []float64, error)
	Watch(ctx context.Context, ws Workspace, siteUserID string, note string) (bool, error)
	Unwatch(ctx context.Context, ws Workspace, siteUserID string) error
	RestoreDismissed(ctx context.Context, ws Workspace) (int, error)
	Settings(ctx context.Context, ws Workspace) (Settings, error)
	SaveSettings(ctx context.Context, ws Workspace, settings Settings) error
	AlertWorkspaces(ctx context.Context) ([]Workspace, error)
	AlertStates(ctx context.Context, ws Workspace) (map[string]alertState, error)
	SaveAlertStates(ctx context.Context, ws Workspace, notified map[string]string, at time.Time, recovered []string) error
}

// PriceProvider 返回工作区「我的站点售价」（站点余额 1 个单位卖多少元），和仪表盘、销售分析一致。
type PriceProvider interface {
	SiteUnitPrice(ctx context.Context, userID string, adminAccountID string) (float64, error)
}

// AccountResolver 解析用户当前选中的工作区。
type AccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

// Names 提供工作区名称（提醒消息的标题）和站点的品牌名（复制给用户的话术里用）。
type Names interface {
	WorkspaceName(ctx context.Context, userID string, adminAccountID string) string
	BrandName(ctx context.Context, userID string, adminAccountID string) string
}

// Service 用「用户分析」同步到本地的数据挑出大户、算余额还能用几天，快用完时提醒站长。不访问站点。
type Service struct {
	store    store
	prices   PriceProvider
	accounts AccountResolver
	names    Names
	notifier Notifier
	now      func() time.Time
	// checked 记下每个工作区上次检查提醒时的同步时间：数据没变就不重复算（余额每小时才同步一次）。
	checked sync.Map
}

func NewService(repository *Repository, prices PriceProvider, accounts AccountResolver, names Names, notifier Notifier) *Service {
	return &Service{store: repository, prices: prices, accounts: accounts, names: names, notifier: notifier, now: time.Now}
}

// CurrentWorkspace 返回用户当前选中的工作区。
func (s *Service) CurrentWorkspace(ctx context.Context, userID string) (Workspace, error) {
	if s.accounts == nil {
		return Workspace{}, requestError(ErrorNoCurrentAccount)
	}
	adminAccountID, err := s.accounts.RequireCurrentID(ctx, userID)
	if err != nil || strings.TrimSpace(adminAccountID) == "" {
		return Workspace{}, requestError(ErrorNoCurrentAccount)
	}
	return Workspace{UserID: userID, AdminAccountID: adminAccountID}, nil
}

// currentWindows 是近 7 天和再往前 7 天：截至昨天的完整自然日（业务时区），和用户分析一致。
func (s *Service) currentWindows() windows {
	local := s.now().In(bizclock.Location())
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	return windows{
		From7:     today.AddDate(0, 0, -7),
		To7:       today.AddDate(0, 0, -1),
		PrevFrom7: today.AddDate(0, 0, -14),
		PrevTo7:   today.AddDate(0, 0, -8),
	}
}

func (s *Service) unitPrice(ctx context.Context, ws Workspace) float64 {
	if s.prices == nil {
		return 1
	}
	price, err := s.prices.SiteUnitPrice(ctx, ws.UserID, ws.AdminAccountID)
	if err != nil || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		if err != nil {
			log.Printf("[vip] load site unit price failed admin_account_id=%s err=%v", ws.AdminAccountID, err)
		}
		return 1
	}
	return price
}

// Summary 返回名单：系统推荐加手动关注，每人的余额、日均消耗、预计还能用几天和状态。
func (s *Service) Summary(ctx context.Context, ws Workspace) (Summary, error) {
	settings, err := s.store.Settings(ctx, ws)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Settings: settings, UnitPrice: s.unitPrice(ctx, ws), Items: []Row{}}
	if s.names != nil {
		summary.SiteName = s.names.BrandName(ctx, ws.UserID, ws.AdminAccountID)
	}
	state, synced, err := s.store.SyncState(ctx, ws)
	if err != nil {
		return Summary{}, err
	}
	if !synced {
		return summary, nil
	}
	summary.Synced = true
	summary.Platform = state.Platform
	summary.LastSyncedAt = state.LastSyncedAt
	stats, err := s.store.Watchlist(ctx, ws, s.currentWindows(), settings.DailyTop, settings.LifetimeTop)
	if err != nil {
		return Summary{}, err
	}
	now := s.now()
	for _, stat := range stats {
		row := buildRow(stat, settings, summary.UnitPrice, now)
		summary.Items = append(summary.Items, row)
		summary.Counts.All++
		switch row.Status {
		case StatusLow:
			summary.Counts.Low++
		case StatusEmpty:
			summary.Counts.Empty++
		case StatusDeclining:
			summary.Counts.Declining++
		}
	}
	if summary.DismissedCount, err = s.store.DismissedCount(ctx, ws); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

// buildRow 把原始数据换算成人民币，算出日均、趋势、预计可用天数和状态。
func buildRow(stat statRow, settings Settings, price float64, now time.Time) Row {
	row := Row{
		SiteUserID:      stat.SiteUserID,
		Email:           stat.Email,
		Username:        stat.Username,
		Source:          Source{Manual: stat.Kind == kindManual, Note: stat.Note, DailyRank: stat.DailyRank, LifetimeRank: stat.LifetimeRank},
		DailyAverage:    stat.Cost7 / 7 * price,
		PreviousAverage: stat.CostPrev7 / 7 * price,
		Lifetime:        stat.Lifetime * price,
		LastUsedAt:      stat.LastUsedAt,
	}
	if stat.Balance != nil {
		balance := *stat.Balance * price
		row.Balance = &balance
	}
	if row.PreviousAverage > 0 {
		trend := (row.DailyAverage - row.PreviousAverage) / row.PreviousAverage
		row.Trend = &trend
	}
	if row.DailyAverage > 0 && row.Balance != nil {
		days := math.Max(*row.Balance, 0) / row.DailyAverage
		row.DaysLeft = &days
	}
	if stat.CreditDay != nil {
		row.LastCreditDay = stat.CreditDay.Format(bizclock.DateLayout)
		row.LastCreditAmount = stat.CreditAmount * price
	}
	row.Status = classify(row, stat.UserStatus, settings, now)
	return row
}

// active：近 7 天用过（有消耗，或者最近使用时间在 7 天内）。不在用的人余额再少也不提醒。
func active(row Row, now time.Time) bool {
	return row.DailyAverage > 0 || (row.LastUsedAt != nil && row.LastUsedAt.After(now.AddDate(0, 0, -activeDays)))
}

// classify 判断状态：站点停用了；余额用完了；在用、并且预计几天内用完或余额低于提醒线（快用完）；
// 近 7 天日均不到前 7 天的一半（用量下降）；否则正常。
func classify(row Row, userStatus string, settings Settings, now time.Time) string {
	if userStatus == StatusDisabled {
		return StatusDisabled
	}
	if row.Balance != nil && *row.Balance <= 0 {
		return StatusEmpty
	}
	if row.Balance != nil && active(row, now) {
		if settings.AlertBalance > 0 && *row.Balance < settings.AlertBalance {
			return StatusLow
		}
		if settings.AlertDays > 0 && row.DaysLeft != nil && *row.DaysLeft <= settings.AlertDays {
			return StatusLow
		}
	}
	if row.PreviousAverage >= decliningMinimum && row.DailyAverage < row.PreviousAverage*decliningRatio {
		return StatusDeclining
	}
	return StatusNormal
}

// Search 按邮箱、用户名或 ID 找用户（添加关注用），金额换算成人民币。
func (s *Service) Search(ctx context.Context, ws Workspace, query string) ([]Candidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Candidate{}, nil
	}
	if utf8.RuneCountInString(query) > 100 {
		query = string([]rune(query)[:100])
	}
	candidates, costs, err := s.store.Search(ctx, ws, s.currentWindows(), query, searchLimit)
	if err != nil {
		return nil, err
	}
	price := s.unitPrice(ctx, ws)
	for i := range candidates {
		candidates[i].DailyAverage = costs[i] / 7 * price
		if candidates[i].Balance != nil {
			balance := *candidates[i].Balance * price
			candidates[i].Balance = &balance
		}
	}
	return candidates, nil
}

func cleanSiteUserID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 100 {
		return "", requestError(ErrorInvalidUser)
	}
	return value, nil
}

// Watch 手动关注一个用户（已经关注的更新备注）。
func (s *Service) Watch(ctx context.Context, ws Workspace, siteUserID string, note string) error {
	id, err := cleanSiteUserID(siteUserID)
	if err != nil {
		return err
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > maxNoteRunes {
		note = string([]rune(note)[:maxNoteRunes])
	}
	found, err := s.store.Watch(ctx, ws, id, note)
	if err != nil {
		return err
	}
	if !found {
		return requestError(ErrorUserNotFound)
	}
	return nil
}

// Unwatch 把一个人移出名单（系统也不再推荐他）。
func (s *Service) Unwatch(ctx context.Context, ws Workspace, siteUserID string) error {
	id, err := cleanSiteUserID(siteUserID)
	if err != nil {
		return err
	}
	return s.store.Unwatch(ctx, ws, id)
}

// RestoreDismissed 让移出过的人重新参加推荐。
func (s *Service) RestoreDismissed(ctx context.Context, ws Workspace) (int, error) {
	return s.store.RestoreDismissed(ctx, ws)
}

// SaveSettings 校验并保存设置，返回保存后的值。
func (s *Service) SaveSettings(ctx context.Context, ws Workspace, settings Settings) (Settings, error) {
	cleaned, err := cleanSettings(settings)
	if err != nil {
		return Settings{}, err
	}
	if err := s.store.SaveSettings(ctx, ws, cleaned); err != nil {
		return Settings{}, err
	}
	return cleaned, nil
}

func cleanSettings(settings Settings) (Settings, error) {
	finite := func(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
	if settings.DailyTop < 0 || settings.DailyTop > 100 || settings.LifetimeTop < 0 || settings.LifetimeTop > 100 ||
		!finite(settings.AlertDays) || settings.AlertDays < 0 || settings.AlertDays > 30 ||
		!finite(settings.AlertBalance) || settings.AlertBalance < 0 || settings.AlertBalance > 1_000_000 {
		return Settings{}, requestError(ErrorInvalidSettings)
	}
	bots := make([]string, 0, len(settings.BotIDs))
	for _, id := range settings.BotIDs {
		id = strings.TrimSpace(id)
		if id == "" || containsString(bots, id) {
			continue
		}
		bots = append(bots, id)
	}
	if len(bots) > maxBots {
		return Settings{}, requestError(ErrorInvalidSettings)
	}
	settings.BotIDs = bots
	settings.ReminderText = strings.TrimSpace(settings.ReminderText)
	settings.EmptyReminderText = strings.TrimSpace(settings.EmptyReminderText)
	if utf8.RuneCountInString(settings.ReminderText) > maxReminderRunes || utf8.RuneCountInString(settings.EmptyReminderText) > maxReminderRunes {
		return Settings{}, requestError(ErrorInvalidSettings)
	}
	return settings, nil
}

func containsString(values []string, value string) bool {
	for _, known := range values {
		if known == value {
			return true
		}
	}
	return false
}

// IsRequestError 判断是否为可直接返回给前端的业务错误。
func IsRequestError(err error) bool {
	var target requestError
	return errors.As(err, &target)
}
