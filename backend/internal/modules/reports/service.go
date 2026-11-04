package reports

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/shared/bizclock"
)

// 错误 key 直接作为 i18n key 返回给前端。
const (
	ErrorNoCurrentAccount = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorNoBots           = "admin.settings.dailyReport.errors.noBots"
	ErrorRequest          = "admin.settings.dailyReport.errors.request"
)

const (
	schedulerInterval = time.Minute
	// sentMarkerTTL 覆盖到第二天发送时间之后，保证同一天只发一次（重启、多实例都不会重复）。
	sentMarkerTTL = 36 * time.Hour
)

// StrategySource 提供各工作区的日报设置。
type StrategySource interface {
	ListStrategies(ctx context.Context) ([]settings.WorkspaceStrategy, error)
	GetStrategyForWorkspace(ctx context.Context, userID string, adminAccountID string) (settings.StrategySettings, error)
}

// Notifier 用指定工作区的机器人发送通知。
type Notifier interface {
	SendFormattedToWorkspaceBots(ctx context.Context, userID string, adminAccountID string, botIDs []string, message string, format settings.NotificationTemplateFormat)
}

// AccountResolver 解析用户当前选中的工作区（预览、测试发送使用）。
type AccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

// OnceStore 保证每个工作区每天只发送一次日报。
type OnceStore interface {
	Claim(ctx context.Context, key string, ttl time.Duration) bool
	Release(ctx context.Context, key string)
}

type reportBuilder interface {
	Build(ctx context.Context, userID string, adminAccountID string, now time.Time) (DailyReport, error)
}

// Service 按各工作区设置的时间发送经营日报，并提供预览与测试发送。
type Service struct {
	builder    reportBuilder
	strategies StrategySource
	notifier   Notifier
	accounts   AccountResolver
	once       OnceStore
	now        func() time.Time
}

func NewService(builder *Builder, strategies StrategySource, notifier Notifier, accounts AccountResolver, once OnceStore) *Service {
	return &Service{builder: builder, strategies: strategies, notifier: notifier, accounts: accounts, once: once, now: time.Now}
}

type requestError string

func (e requestError) Error() string { return string(e) }

func (s *Service) currentWorkspace(ctx context.Context, userID string) (string, error) {
	if s.accounts == nil {
		return "", requestError(ErrorNoCurrentAccount)
	}
	adminAccountID, err := s.accounts.RequireCurrentID(ctx, userID)
	if err != nil || strings.TrimSpace(adminAccountID) == "" {
		return "", requestError(ErrorNoCurrentAccount)
	}
	return adminAccountID, nil
}

// Preview 生成当前工作区昨天的日报（不发送）。
func (s *Service) Preview(ctx context.Context, userID string) (DailyReport, error) {
	adminAccountID, err := s.currentWorkspace(ctx, userID)
	if err != nil {
		return DailyReport{}, err
	}
	return s.builder.Build(ctx, userID, adminAccountID, s.now())
}

// SendTest 立即把当前工作区的日报发给指定机器人；botIDs 为空时使用已保存的日报机器人。
func (s *Service) SendTest(ctx context.Context, userID string, botIDs []string) (DailyReport, int, error) {
	adminAccountID, err := s.currentWorkspace(ctx, userID)
	if err != nil {
		return DailyReport{}, 0, err
	}
	targets := cleanBotIDs(botIDs)
	if len(targets) == 0 {
		strategy, err := s.strategies.GetStrategyForWorkspace(ctx, userID, adminAccountID)
		if err != nil {
			return DailyReport{}, 0, err
		}
		targets = cleanBotIDs(strategy.DailyReportBotIDs)
	}
	if len(targets) == 0 {
		return DailyReport{}, 0, requestError(ErrorNoBots)
	}
	report, err := s.builder.Build(ctx, userID, adminAccountID, s.now())
	if err != nil {
		return DailyReport{}, 0, err
	}
	s.notifier.SendFormattedToWorkspaceBots(ctx, userID, adminAccountID, targets, report.Text, settings.NotificationTemplateFormatText)
	return report, len(targets), nil
}

func cleanBotIDs(botIDs []string) []string {
	result := make([]string, 0, len(botIDs))
	seen := map[string]struct{}{}
	for _, id := range botIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// StartScheduler 每分钟检查一次：到了工作区设置的发送时间（北京时间）且当天还没发过，就生成并发送日报。
// 进程在发送时间之后才启动时，当天仍会补发一次。
func (s *Service) StartScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(schedulerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runDue(ctx)
			}
		}
	}()
}

func (s *Service) runDue(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[daily-report] scheduler panic recovered: %v", r)
		}
	}()
	strategies, err := s.strategies.ListStrategies(ctx)
	if err != nil {
		log.Printf("[daily-report] load strategies failed: %v", err)
		return
	}
	now := s.now().In(bizclock.Location())
	minuteOfDay := now.Hour()*60 + now.Minute()
	date := now.Format(bizclock.DateLayout)
	for _, strategy := range strategies {
		config := strategy.Settings
		bots := cleanBotIDs(config.DailyReportBotIDs)
		if !config.EnableDailyReport || len(bots) == 0 {
			continue
		}
		due, ok := settings.ParseDailyReportTime(config.DailyReportTime)
		if !ok {
			due, _ = settings.ParseDailyReportTime(settings.DefaultDailyReportTime)
		}
		if minuteOfDay < due {
			continue
		}
		key := "transithub:daily-report:" + strategy.UserID + ":" + strategy.AdminAccountID + ":" + date
		if !s.once.Claim(ctx, key, sentMarkerTTL) {
			continue
		}
		report, err := s.builder.Build(ctx, strategy.UserID, strategy.AdminAccountID, now)
		if err != nil {
			// 生成失败（例如数据库暂时不可用）：释放标记，下一分钟重试。
			s.once.Release(ctx, key)
			log.Printf("[daily-report] build failed admin_account_id=%s err=%v", strategy.AdminAccountID, err)
			continue
		}
		s.notifier.SendFormattedToWorkspaceBots(ctx, strategy.UserID, strategy.AdminAccountID, bots, report.Text, settings.NotificationTemplateFormatText)
		log.Printf("[daily-report] sent admin_account_id=%s date=%s bots=%d", strategy.AdminAccountID, report.Date, len(bots))
	}
}

// IsRequestError 判断是否为可直接返回给前端的业务错误。
func IsRequestError(err error) bool {
	var target requestError
	return errors.As(err, &target)
}

// redisOnceStore 用 Redis SET NX EX 记录「今天已发送」；Redis 不可用时退回进程内记录。
type redisOnceStore struct {
	client   *redis.Client
	fallback *memoryOnceStore
}

func NewRedisOnceStore(client *redis.Client) OnceStore {
	return redisOnceStore{client: client, fallback: newMemoryOnceStore()}
}

func (s redisOnceStore) Claim(ctx context.Context, key string, ttl time.Duration) bool {
	if s.client == nil {
		return s.fallback.Claim(ctx, key, ttl)
	}
	claimed, err := s.client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		log.Printf("[daily-report] redis unavailable, using in-process marker: %v", err)
		return s.fallback.Claim(ctx, key, ttl)
	}
	return claimed
}

func (s redisOnceStore) Release(ctx context.Context, key string) {
	s.fallback.Release(ctx, key)
	if s.client == nil {
		return
	}
	if err := s.client.Del(ctx, key).Err(); err != nil {
		log.Printf("[daily-report] redis release failed: %v", err)
	}
}

type memoryOnceStore struct {
	mu      sync.Mutex
	now     func() time.Time
	expires map[string]time.Time
}

func newMemoryOnceStore() *memoryOnceStore {
	return &memoryOnceStore{now: time.Now, expires: map[string]time.Time{}}
}

func (s *memoryOnceStore) Claim(ctx context.Context, key string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if expiry, ok := s.expires[key]; ok && now.Before(expiry) {
		return false
	}
	s.expires[key] = now.Add(ttl)
	return true
}

func (s *memoryOnceStore) Release(ctx context.Context, key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.expires, key)
}
