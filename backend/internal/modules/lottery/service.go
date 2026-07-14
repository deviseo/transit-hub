package lottery

import (
	"context"
	"errors"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
)

type AdminAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}
type AdminSessionProvider interface {
	RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error)
}
type sub2APIFetcher interface {
	FetchCurrentUser(srcHost string, token string) (Sub2APIUser, error)
}
type rewardRedeemer interface {
	Redeem(ctx context.Context, session upstream.Session, job RewardJob) RewardResult
	CleanupDedicatedRate(ctx context.Context, session upstream.Session, job RateCleanupJob, replacement *RateCleanupReplacement) RewardResult
}
type subscriptionGroupProvider interface {
	FetchSub2APIAdminAllGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error)
}

type Service struct {
	repository    *Repository
	sessions      *EmbedSessionStore
	sub2api       sub2APIFetcher
	rewards       rewardRedeemer
	accounts      AdminAccountResolver
	adminSessions AdminSessionProvider
	groupProvider subscriptionGroupProvider
	// allowPrivateTargets 只能由本地调试配置开启；默认 false，保持公开嵌入的 SSRF 防护。
	allowPrivateTargets bool
	newToken            func() (string, error)
	now                 func() time.Time
}

func NewService(repository *Repository, sessions *EmbedSessionStore, sub2api sub2APIFetcher, rewards rewardRedeemer, adminSessions AdminSessionProvider) *Service {
	return &Service{repository: repository, sessions: sessions, sub2api: sub2api, rewards: rewards, adminSessions: adminSessions, newToken: func() (string, error) { return randomHex(32) }, now: time.Now}
}

func (s *Service) EnsureSchema(ctx context.Context) error                { return s.repository.EnsureSchema(ctx) }
func (s *Service) SetAdminAccountResolver(accounts AdminAccountResolver) { s.accounts = accounts }
func (s *Service) SetSubscriptionGroupProvider(provider subscriptionGroupProvider) {
	s.groupProvider = provider
}
func (s *Service) SetAllowPrivateTargets(allow bool) { s.allowPrivateTargets = allow }

func (s *Service) GetEmbedConfig(ctx context.Context, userID string) (EmbedConfigResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	origin, err := s.currentWorkspaceSourceOrigin(ctx, userID, adminAccountID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	config, err := s.autoBindEmbedConfig(ctx, userID, adminAccountID, origin)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	return embedConfigResponse(*config), nil
}

func (s *Service) RotateEmbedToken(ctx context.Context, userID string) (EmbedConfigResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	origin, err := s.currentWorkspaceSourceOrigin(ctx, userID, adminAccountID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	config, err := s.autoBindEmbedConfig(ctx, userID, adminAccountID, origin)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	token, err := s.newToken()
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	if err := s.repository.RotateEmbedToken(ctx, userID, adminAccountID, token); err != nil {
		return EmbedConfigResponse{}, err
	}
	if err := s.sessions.DeleteWorkspace(ctx, userID, adminAccountID); err != nil {
		return EmbedConfigResponse{}, err
	}
	config.EmbedToken = token
	config.UpdatedAt = s.now()
	return embedConfigResponse(*config), nil
}

// ListSubscriptionGroups 从当前工作区的实时管理员会话读取分组，而不是使用可能包含其他关联站点的倍率快照。
// 只返回当前可用、ID 可用于 Sub2API 发奖接口且带有有效倍率的分组，供管理员明确选择奖励目标。
func (s *Service) ListSubscriptionGroups(ctx context.Context, userID string) (ListSubscriptionGroupsResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return ListSubscriptionGroupsResponse{}, err
	}
	if s.adminSessions == nil {
		return ListSubscriptionGroupsResponse{}, requestError(ErrorRewardAdminSession)
	}
	session, err := s.adminSessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return ListSubscriptionGroupsResponse{}, requestError(ErrorRewardAdminSession)
	}
	if session.Platform != upstream.PlatformSub2API || s.groupProvider == nil {
		return ListSubscriptionGroupsResponse{}, requestError(ErrorRewardUnsupported)
	}
	groups, err := s.groupProvider.FetchSub2APIAdminAllGroups(session)
	if err != nil {
		return ListSubscriptionGroupsResponse{}, requestError(ErrorSubscriptionGroups)
	}

	itemsByID := make(map[string]SubscriptionGroupResponse, len(groups))
	for _, group := range groups {
		id := strings.TrimSpace(group.ID)
		name := strings.TrimSpace(group.Name)
		numericID, parseErr := strconv.ParseInt(id, 10, 64)
		if parseErr != nil || numericID <= 0 || name == "" || group.Multiplier == nil {
			continue
		}
		status := strings.TrimSpace(group.Status)
		if status != "" && !strings.EqualFold(status, "active") {
			continue
		}
		multiplier := *group.Multiplier
		if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
			continue
		}
		itemsByID[id] = SubscriptionGroupResponse{
			ID:         id,
			Name:       name,
			Multiplier: strconv.FormatFloat(multiplier, 'f', -1, 64),
		}
	}
	items := make([]SubscriptionGroupResponse, 0, len(itemsByID))
	for _, item := range itemsByID {
		items = append(items, item)
	}
	sort.Slice(items, func(left, right int) bool {
		if items[left].Name == items[right].Name {
			return items[left].ID < items[right].ID
		}
		return items[left].Name < items[right].Name
	})
	return ListSubscriptionGroupsResponse{Items: items}, nil
}

func (s *Service) CreateCampaign(ctx context.Context, userID string, req CreateCampaignRequest) (CampaignResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return CampaignResponse{}, err
	}
	campaign, prizes, err := s.buildDraft(userID, adminAccountID, "", req)
	if err != nil {
		return CampaignResponse{}, err
	}
	if err := s.repository.CreateCampaign(ctx, campaign, prizes); err != nil {
		return CampaignResponse{}, err
	}
	s.audit(ctx, campaign, "admin", userID, "create", nil)
	return s.campaignResponse(ctx, campaign, true)
}

func (s *Service) UpdateCampaign(ctx context.Context, userID, id string, req UpdateCampaignRequest) (CampaignResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return CampaignResponse{}, err
	}
	campaign, prizes, err := s.buildDraft(userID, adminAccountID, id, req)
	if err != nil {
		return CampaignResponse{}, err
	}
	if err := s.repository.UpdateDraftCampaign(ctx, campaign, prizes); err != nil {
		return CampaignResponse{}, err
	}
	s.audit(ctx, campaign, "admin", userID, "update", nil)
	return s.GetCampaign(ctx, userID, id)
}

func (s *Service) ListCampaigns(ctx context.Context, userID string) (ListCampaignsResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return ListCampaignsResponse{}, err
	}
	campaigns, err := s.repository.ListCampaigns(ctx, userID, adminAccountID)
	if err != nil {
		return ListCampaignsResponse{}, err
	}
	items := make([]CampaignResponse, 0, len(campaigns))
	for _, campaign := range campaigns {
		resp, err := s.campaignResponse(ctx, campaign, true)
		if err != nil {
			return ListCampaignsResponse{}, err
		}
		items = append(items, resp)
	}
	return ListCampaignsResponse{Items: items}, nil
}

func (s *Service) GetCampaign(ctx context.Context, userID, id string) (CampaignResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return CampaignResponse{}, err
	}
	campaign, err := s.repository.GetCampaign(ctx, userID, adminAccountID, id)
	if err != nil || campaign == nil {
		if err != nil {
			return CampaignResponse{}, err
		}
		return CampaignResponse{}, requestError(ErrorNotFound)
	}
	return s.campaignResponse(ctx, *campaign, true)
}

func (s *Service) ListEntries(ctx context.Context, userID, id string) (ListEntriesResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return ListEntriesResponse{}, err
	}
	campaign, err := s.repository.GetCampaign(ctx, userID, adminAccountID, id)
	if err != nil || campaign == nil {
		if err != nil {
			return ListEntriesResponse{}, err
		}
		return ListEntriesResponse{}, requestError(ErrorNotFound)
	}
	entries, err := s.repository.ListEntries(ctx, id, false)
	if err != nil {
		return ListEntriesResponse{}, err
	}
	items := make([]EntryResponse, 0, len(entries))
	for _, entry := range entries {
		items = append(items, entryResponse(entry))
	}
	return ListEntriesResponse{Items: items}, nil
}

func (s *Service) ListAudit(ctx context.Context, userID, id string) (AuditResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return AuditResponse{}, err
	}
	campaign, err := s.repository.GetCampaign(ctx, userID, adminAccountID, id)
	if err != nil || campaign == nil {
		if err != nil {
			return AuditResponse{}, err
		}
		return AuditResponse{}, requestError(ErrorNotFound)
	}
	items, err := s.repository.ListAuditLogs(ctx, id)
	if err != nil {
		return AuditResponse{}, err
	}
	return AuditResponse{Items: items}, nil
}

func (s *Service) PublishCampaign(ctx context.Context, userID, id string) (CampaignResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return CampaignResponse{}, err
	}
	campaign, err := s.repository.GetCampaign(ctx, userID, adminAccountID, id)
	if err != nil || campaign == nil {
		if err != nil {
			return CampaignResponse{}, err
		}
		return CampaignResponse{}, requestError(ErrorNotFound)
	}
	secret, err := randomHex(32)
	if err != nil {
		return CampaignResponse{}, err
	}
	status := StatusOpen
	if campaign.RegistrationStart != nil && campaign.RegistrationStart.After(s.now()) {
		status = StatusScheduled
	}
	if err := s.repository.PublishCampaign(ctx, userID, adminAccountID, id, status, secret, seedCommitment(secret)); err != nil {
		return CampaignResponse{}, err
	}
	campaign.Status, campaign.SeedSecret, campaign.SeedCommitment = status, secret, seedCommitment(secret)
	s.audit(ctx, *campaign, "admin", userID, "publish", map[string]any{"status": status})
	return s.GetCampaign(ctx, userID, id)
}

func (s *Service) CloseCampaign(ctx context.Context, userID, id string) (CampaignResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return CampaignResponse{}, err
	}
	if err := s.repository.SetCampaignStatus(ctx, userID, adminAccountID, id, StatusOpen, StatusClosed); err != nil {
		return CampaignResponse{}, err
	}
	c, _ := s.repository.GetCampaign(ctx, userID, adminAccountID, id)
	if c != nil {
		s.audit(ctx, *c, "admin", userID, "close", nil)
	}
	return s.GetCampaign(ctx, userID, id)
}

func (s *Service) CancelCampaign(ctx context.Context, userID, id string) (CampaignResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return CampaignResponse{}, err
	}
	for _, from := range []string{StatusDraft, StatusScheduled, StatusOpen, StatusClosed} {
		if err := s.repository.SetCampaignStatus(ctx, userID, adminAccountID, id, from, StatusCancelled); err == nil {
			return s.GetCampaign(ctx, userID, id)
		}
	}
	return CampaignResponse{}, requestError(ErrorInvalidState)
}

func (s *Service) DrawCampaign(ctx context.Context, userID, id string) (CampaignResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return CampaignResponse{}, err
	}
	campaign, _, err := s.repository.DrawCampaign(ctx, userID, adminAccountID, id)
	if err != nil {
		return CampaignResponse{}, err
	}
	campaign, err = requireDrawnCampaign(campaign)
	if err != nil {
		return CampaignResponse{}, err
	}
	s.audit(ctx, *campaign, "admin", userID, "draw", map[string]any{"winnerCount": campaign.WinnerCount})
	return s.GetCampaign(ctx, userID, id)
}

func requireDrawnCampaign(campaign *Campaign) (*Campaign, error) {
	if campaign == nil {
		return nil, requestError(ErrorNotFound)
	}
	return campaign, nil
}

func (s *Service) CreateEmbedSession(ctx context.Context, req CreateSessionRequest) (CreateSessionResponse, error) {
	viewerToken := strings.TrimSpace(req.Sub2apiToken)
	if viewerToken == "" {
		viewerToken = strings.TrimSpace(req.ViewerToken)
	}
	if strings.TrimSpace(req.EmbedToken) == "" || viewerToken == "" {
		return CreateSessionResponse{}, requestError(ErrorEmbedRequest)
	}
	config, err := s.repository.GetEmbedConfigByToken(ctx, strings.TrimSpace(req.EmbedToken))
	if err != nil {
		return CreateSessionResponse{}, err
	}
	if config == nil || strings.TrimSpace(config.Sub2apiSourceOrigin) == "" {
		return CreateSessionResponse{}, requestError(ErrorEmbedConfigNotFound)
	}
	normalizedSrcHost, err := normalizeSrcHostWithPrivateTargets(req.SrcHost, s.allowPrivateTargets)
	if err != nil {
		return CreateSessionResponse{}, err
	}
	if normalizedSrcHost != config.Sub2apiSourceOrigin {
		return CreateSessionResponse{}, requestError(ErrorEmbedSrcHostMismatch)
	}
	if _, err := s.validateCurrentSourceBinding(ctx, config.UserID, config.AdminAccountID, config.Sub2apiSourceOrigin, normalizedSrcHost); err != nil {
		return CreateSessionResponse{}, err
	}
	user, err := s.sub2api.FetchCurrentUser(normalizedSrcHost, viewerToken)
	if err != nil {
		var subErr *sub2APIError
		if errors.As(err, &subErr) && subErr.unauthorized {
			return CreateSessionResponse{}, requestError(ErrorEmbedSub2apiAuth)
		}
		return CreateSessionResponse{}, requestError(ErrorEmbedSub2apiRequest)
	}
	if strings.TrimSpace(req.UrlUserID) != "" && strings.TrimSpace(req.UrlUserID) != user.ID {
		return CreateSessionResponse{}, requestError(ErrorEmbedUserMismatch)
	}
	if !viewerActive(user.Status) {
		return CreateSessionResponse{}, requestError(ErrorEmbedUserInactive)
	}
	token, err := s.newToken()
	if err != nil {
		return CreateSessionResponse{}, err
	}
	session := EmbedSession{UserID: config.UserID, AdminAccountID: config.AdminAccountID, EmbedToken: config.EmbedToken, SrcHost: normalizedSrcHost, SrcURL: strings.TrimSpace(req.SrcURL), Sub2apiUserID: user.ID, Sub2apiEmailMasked: maskEmail(user.Email), Sub2apiRole: user.Role, CreatedAt: s.now()}
	if err := s.sessions.Save(ctx, token, session); err != nil {
		return CreateSessionResponse{}, err
	}
	return CreateSessionResponse{SessionToken: token}, nil
}

func (s *Service) ListEmbedCampaigns(ctx context.Context, sessionToken string) (ListCampaignsResponse, error) {
	session, err := s.requireEmbedSession(ctx, sessionToken)
	if err != nil {
		return ListCampaignsResponse{}, err
	}
	campaigns, err := s.repository.ListEmbedCampaigns(ctx, session.UserID, session.AdminAccountID)
	if err != nil {
		return ListCampaignsResponse{}, err
	}
	items := make([]CampaignResponse, 0, len(campaigns))
	for _, campaign := range campaigns {
		resp, err := s.embedCampaignResponse(ctx, campaign, session.Sub2apiUserID, false)
		if err != nil {
			return ListCampaignsResponse{}, err
		}
		items = append(items, resp)
	}
	return ListCampaignsResponse{Items: items}, nil
}

func (s *Service) GetEmbedCampaign(ctx context.Context, sessionToken, id string) (CampaignResponse, error) {
	session, err := s.requireEmbedSession(ctx, sessionToken)
	if err != nil {
		return CampaignResponse{}, err
	}
	campaign, err := s.repository.GetCampaign(ctx, session.UserID, session.AdminAccountID, id)
	if err != nil || campaign == nil {
		if err != nil {
			return CampaignResponse{}, err
		}
		return CampaignResponse{}, requestError(ErrorNotFound)
	}
	return s.embedCampaignResponse(ctx, *campaign, session.Sub2apiUserID, true)
}

func (s *Service) EnterCampaign(ctx context.Context, sessionToken, id string) (EntryResponse, error) {
	session, err := s.requireEmbedSession(ctx, sessionToken)
	if err != nil {
		return EntryResponse{}, err
	}
	campaign, err := s.repository.GetCampaign(ctx, session.UserID, session.AdminAccountID, id)
	if err != nil || campaign == nil {
		if err != nil {
			return EntryResponse{}, err
		}
		return EntryResponse{}, requestError(ErrorNotFound)
	}
	if err := s.syncRegistrationState(ctx, campaign); err != nil {
		return EntryResponse{}, err
	}
	if campaign.Status != StatusOpen {
		return EntryResponse{}, requestError(ErrorEmbedCampaignNotOpen)
	}
	receipt, err := randomHex(24)
	if err != nil {
		return EntryResponse{}, err
	}
	receiptSum := seedCommitment(receipt)
	entryID, err := newID("lent")
	if err != nil {
		return EntryResponse{}, err
	}
	entry, err := s.repository.InsertEntry(ctx, Entry{ID: entryID, CampaignID: id, UserID: session.UserID, AdminAccountID: session.AdminAccountID, Sub2apiUserID: session.Sub2apiUserID, MaskedEmail: session.Sub2apiEmailMasked, ReceiptToken: receipt, ReceiptHash: receiptSum, Status: EntryStatusActive})
	if err != nil {
		return EntryResponse{}, err
	}
	s.audit(ctx, *campaign, "embed", session.Sub2apiUserID, "entry", map[string]any{"entryId": entry.ID})
	return entryResponse(*entry), nil
}

func (s *Service) WithdrawEntry(ctx context.Context, sessionToken, id string) error {
	session, err := s.requireEmbedSession(ctx, sessionToken)
	if err != nil {
		return err
	}
	campaign, err := s.repository.GetCampaign(ctx, session.UserID, session.AdminAccountID, id)
	if err != nil || campaign == nil {
		if err != nil {
			return err
		}
		return requestError(ErrorNotFound)
	}
	if err := s.syncRegistrationState(ctx, campaign); err != nil {
		return err
	}
	if campaign.Status != StatusOpen {
		return requestError(ErrorEmbedCampaignNotOpen)
	}
	if err := s.repository.WithdrawEntry(ctx, id, session.Sub2apiUserID); err != nil {
		return err
	}
	s.audit(ctx, *campaign, "embed", session.Sub2apiUserID, "withdraw", nil)
	return nil
}

// syncRegistrationState 在报名写操作前按活动时间校正状态，消除定时调度与用户点击之间的竞争窗口。
// 即使请求恰好落在报名开始或截止的同一秒，也会先推进 scheduled -> open -> closed，再执行权限判断。
func (s *Service) syncRegistrationState(ctx context.Context, campaign *Campaign) error {
	now := s.now()
	if campaign.Status == StatusScheduled && campaign.RegistrationStart != nil && !campaign.RegistrationStart.After(now) {
		err := s.repository.SetCampaignStatus(ctx, campaign.UserID, campaign.AdminAccountID, campaign.ID, StatusScheduled, StatusOpen)
		if err == nil {
			campaign.Status = StatusOpen
		} else {
			latest, getErr := s.repository.GetCampaignByID(ctx, campaign.ID)
			if getErr != nil {
				return getErr
			}
			if latest == nil {
				return requestError(ErrorNotFound)
			}
			*campaign = *latest
		}
	}
	if campaign.Status == StatusOpen && campaign.RegistrationEnd != nil && !campaign.RegistrationEnd.After(now) {
		err := s.repository.SetCampaignStatus(ctx, campaign.UserID, campaign.AdminAccountID, campaign.ID, StatusOpen, StatusClosed)
		if err == nil {
			campaign.Status = StatusClosed
			return nil
		}
		latest, getErr := s.repository.GetCampaignByID(ctx, campaign.ID)
		if getErr != nil {
			return getErr
		}
		if latest == nil {
			return requestError(ErrorNotFound)
		}
		*campaign = *latest
	}
	return nil
}

func (s *Service) ProcessRewardJobs(ctx context.Context, limit int) {
	jobs, err := s.repository.ClaimRewardJobs(ctx, limit, 10*time.Minute)
	if err != nil {
		log.Printf("[lottery] claim reward jobs failed err=%v", err)
		return
	}
	for _, job := range jobs {
		s.processRewardJob(ctx, job)
	}
}

func (s *Service) processRewardJob(ctx context.Context, job RewardJob) {
	session, err := s.adminSessions.RequireSession(ctx, job.UserID, job.AdminAccountID)
	if err != nil {
		s.finishReward(ctx, job, RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorRewardAdminSession, Detail: err.Error()})
		return
	}
	result := s.rewards.Redeem(ctx, session, job)
	s.finishReward(ctx, job, result)
}

func (s *Service) finishReward(ctx context.Context, job RewardJob, result RewardResult) {
	now := s.now()
	next := now.Add(time.Duration(1<<min(job.AttemptCount, 6)) * time.Minute)
	if result.Status == RewardFulfilled || result.Status == RewardManualAttention || result.Status == RewardFailed {
		next = now.AddDate(10, 0, 0)
	}
	cleanupAt := rateCleanupAtForReward(job, result, now)
	if err := s.repository.CompleteRewardJob(ctx, job.ID, result.Status, result.ErrorKey, result.Detail, result.RemoteRef, next, cleanupAt); err != nil {
		log.Printf("[lottery] complete reward job failed id=%s err=%v", job.ID, err)
	}
	_ = s.repository.FinalizeCampaignRewards(ctx, job.CampaignID)
}

// rateCleanupAtForReward 只为新版、成功设置了专属倍率的订阅奖品生成清理时间。
// multiplier 为空的历史奖品仍由 Sub2API 订阅自身管理到期，不创建 TransitHub 清理任务。
func rateCleanupAtForReward(job RewardJob, result RewardResult, now time.Time) *time.Time {
	if result.Status != RewardFulfilled || result.SkipRateCleanup || job.Prize.Type != PrizeTypeSubscription || strings.TrimSpace(job.Prize.Multiplier) == "" || job.Prize.ValidityDays == nil {
		return nil
	}
	if *job.Prize.ValidityDays < 1 || *job.Prize.ValidityDays > 36500 {
		return nil
	}
	if _, err := positiveFiniteMultiplier(job.Prize.Multiplier); err != nil {
		return nil
	}
	cleanupAt := now.AddDate(0, 0, *job.Prize.ValidityDays)
	return &cleanupAt
}

// ProcessRateCleanupJobs 处理到期专属倍率。任务与奖励发放状态分离，因此清理重试不会
// 把已经完成并对外公开的抽奖活动重新改回“发奖中”或“部分完成”。
func (s *Service) ProcessRateCleanupJobs(ctx context.Context, limit int) {
	jobs, err := s.repository.ClaimRateCleanupJobs(ctx, limit, 10*time.Minute)
	if err != nil {
		log.Printf("[lottery] claim rate cleanup jobs failed err=%v", err)
		return
	}
	for _, job := range jobs {
		s.processRateCleanupJob(ctx, job)
	}
}

func (s *Service) processRateCleanupJob(ctx context.Context, job RateCleanupJob) {
	session, err := s.adminSessions.RequireSession(ctx, job.UserID, job.AdminAccountID)
	if err != nil {
		s.finishRateCleanup(ctx, job, RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorRewardAdminSession, Detail: err.Error()})
		return
	}
	replacement, err := s.repository.FindRateCleanupReplacement(ctx, job, s.now())
	if err != nil {
		s.finishRateCleanup(ctx, job, RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorUnknown, Detail: err.Error()})
		return
	}
	result := s.rewards.CleanupDedicatedRate(ctx, session, job, replacement)
	s.finishRateCleanup(ctx, job, result)
}

func (s *Service) finishRateCleanup(ctx context.Context, job RateCleanupJob, result RewardResult) {
	now := s.now()
	status := RateCleanupRetryable
	next := now.Add(time.Duration(1<<min(job.CleanupAttemptCount, 6)) * time.Minute)
	// 清理阶段的 404、已不存在倍率和不可恢复的 4xx 都属于终态。错误详情仅留作内部
	// 审计，不传播到中奖者，也不改变已完成活动的奖励状态。
	if result.Status != RewardRetryableFailed {
		status = RateCleanupCompleted
		next = now.AddDate(10, 0, 0)
	}
	if err := s.repository.CompleteRateCleanup(ctx, job.ID, status, result.Detail, next); err != nil {
		log.Printf("[lottery] complete rate cleanup failed id=%s err=%v", job.ID, err)
	}
}

func (s *Service) RetryReward(ctx context.Context, userID, jobID string) error {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return err
	}
	return s.repository.RetryRewardJob(ctx, userID, adminAccountID, jobID)
}

func (s *Service) CompleteManualReward(ctx context.Context, userID, jobID string) error {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return err
	}
	campaignID, err := s.repository.CompleteManualRewardJob(ctx, userID, adminAccountID, jobID)
	if err != nil {
		return err
	}
	return s.repository.FinalizeCampaignRewards(ctx, campaignID)
}

func (s *Service) RunSchedulerTick(ctx context.Context) {
	now := s.now()
	if ids, err := s.repository.ListDueOpen(ctx, now); err == nil {
		for _, id := range ids {
			c, _ := s.repository.GetCampaignByID(ctx, id)
			if c != nil {
				_ = s.repository.SetCampaignStatus(ctx, c.UserID, c.AdminAccountID, id, StatusScheduled, StatusOpen)
			}
		}
	}
	if ids, err := s.repository.ListDueClose(ctx, now); err == nil {
		for _, id := range ids {
			c, _ := s.repository.GetCampaignByID(ctx, id)
			if c != nil {
				_ = s.repository.SetCampaignStatus(ctx, c.UserID, c.AdminAccountID, id, StatusOpen, StatusClosed)
			}
		}
	}
	if ids, err := s.repository.ListDueDraw(ctx, now); err == nil {
		for _, id := range ids {
			c, _ := s.repository.GetCampaignByID(ctx, id)
			if c != nil {
				_, _, _ = s.repository.DrawCampaign(ctx, c.UserID, c.AdminAccountID, id)
			}
		}
	}
	s.ProcessRewardJobs(ctx, 5)
	s.ProcessRateCleanupJobs(ctx, 5)
	if err := s.repository.ReconcileRewardCampaignStatuses(ctx); err != nil {
		log.Printf("[lottery] reconcile reward campaign statuses failed err=%v", err)
	}
}

func (s *Service) FrameAncestorOrigin(ctx context.Context, embedToken string) (string, bool) {
	config, err := s.repository.GetEmbedConfigByToken(ctx, strings.TrimSpace(embedToken))
	if err != nil || config == nil || config.Sub2apiSourceOrigin == "" {
		return "", false
	}
	if _, err := s.validateCurrentSourceBinding(ctx, config.UserID, config.AdminAccountID, config.Sub2apiSourceOrigin, config.Sub2apiSourceOrigin); err != nil {
		if errors.Is(err, requestError(ErrorEmbedAdminSession)) {
			return config.Sub2apiSourceOrigin, true
		}
		return "", false
	}
	return config.Sub2apiSourceOrigin, true
}

func (s *Service) buildDraft(userID, adminAccountID, id string, req CreateCampaignRequest) (Campaign, []Prize, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len(req.Prizes) == 0 {
		return Campaign{}, nil, requestError(ErrorValidation)
	}
	if id == "" {
		var err error
		id, err = newID("lcmp")
		if err != nil {
			return Campaign{}, nil, err
		}
	}
	start, err := parseShanghaiTime(req.RegistrationStart)
	if err != nil {
		return Campaign{}, nil, err
	}
	end, err := parseShanghaiTime(req.RegistrationEnd)
	if err != nil {
		return Campaign{}, nil, err
	}
	drawAt, err := parseShanghaiTime(req.DrawAt)
	if err != nil {
		return Campaign{}, nil, err
	}
	drawMode := strings.TrimSpace(req.DrawMode)
	if drawMode == "" {
		drawMode = DrawModeManual
	}
	if drawMode != DrawModeManual && drawMode != DrawModeScheduled {
		return Campaign{}, nil, requestError(ErrorValidation)
	}
	if err := validateCampaignTiming(start, end, drawAt, drawMode); err != nil {
		return Campaign{}, nil, err
	}
	prizes := make([]Prize, 0, len(req.Prizes))
	for _, input := range req.Prizes {
		prizeID, err := newID("lprz")
		if err != nil {
			return Campaign{}, nil, err
		}
		deliveryMode := strings.TrimSpace(input.DeliveryMode)
		if deliveryMode == "" {
			deliveryMode = DeliverySub2APIAuto
		}
		voucherCodes := make([]string, 0, len(input.VoucherCodes))
		for _, code := range input.VoucherCodes {
			voucherCodes = append(voucherCodes, strings.TrimSpace(code))
		}
		p := Prize{ID: prizeID, CampaignID: id, UserID: userID, AdminAccountID: adminAccountID, Type: strings.TrimSpace(input.Type), Name: strings.TrimSpace(input.Name), Quantity: input.Quantity, SortOrder: input.SortOrder, BalanceAmount: strings.TrimSpace(input.BalanceAmount), GroupID: strings.TrimSpace(input.GroupID), GroupName: strings.TrimSpace(input.GroupName), Multiplier: strings.TrimSpace(input.Multiplier), ValidityDays: input.ValidityDays, DeliveryMode: deliveryMode, ManualContact: strings.TrimSpace(input.ManualContact), VoucherCodes: voucherCodes, ValueMarker: 1}
		if err := validatePrizeDraft(p); err != nil {
			return Campaign{}, nil, requestError(ErrorValidation)
		}
		prizes = append(prizes, p)
	}
	return Campaign{ID: id, UserID: userID, AdminAccountID: adminAccountID, Name: name, Description: strings.TrimSpace(req.Description), Status: StatusDraft, RegistrationStart: start, RegistrationEnd: end, DrawAt: drawAt, DrawMode: drawMode, PublicWinners: req.PublicWinners, AlgorithmVersion: AlgorithmVersion}, prizes, nil
}

func validateCampaignTiming(start, end, drawAt *time.Time, drawMode string) error {
	if start != nil && end != nil && !end.After(*start) {
		return requestError(ErrorValidation)
	}
	if drawMode == DrawModeScheduled && drawAt == nil {
		return requestError(ErrorValidation)
	}
	if end != nil && drawAt != nil && drawAt.Before(*end) {
		return requestError(ErrorValidation)
	}
	return nil
}

func validatePrizeDraft(prize Prize) error {
	if prize.Name == "" || prize.Quantity <= 0 || prize.ValueMarker != 1 {
		return requestError(ErrorValidation)
	}
	if prize.Multiplier != "" {
		if err := validatePositiveDecimal(prize.Multiplier); err != nil {
			return requestError(ErrorValidation)
		}
	}
	switch prize.Type {
	case PrizeTypeBalance:
		if prize.BalanceAmount == "" || prize.GroupID != "" || prize.Multiplier != "" || prize.ValidityDays != nil {
			return requestError(ErrorValidation)
		}
		if err := validatePositiveDecimal(prize.BalanceAmount); err != nil {
			return err
		}
		return validatePrizeDelivery(prize)
	case PrizeTypeSubscription:
		if prize.BalanceAmount != "" || prize.GroupID == "" || prize.ValidityDays == nil || *prize.ValidityDays < 1 || *prize.ValidityDays > 36500 || prize.DeliveryMode != DeliverySub2APIAuto || prize.ManualContact != "" || len(prize.VoucherCodes) != 0 {
			return requestError(ErrorValidation)
		}
		return nil
	default:
		return requestError(ErrorValidation)
	}
}

func validatePrizeDelivery(prize Prize) error {
	switch prize.DeliveryMode {
	case DeliverySub2APIAuto:
		if prize.ManualContact != "" || len(prize.VoucherCodes) != 0 {
			return requestError(ErrorValidation)
		}
	case DeliveryVoucher:
		if prize.ManualContact != "" || len(prize.VoucherCodes) != prize.Quantity {
			return requestError(ErrorValidation)
		}
		seen := make(map[string]struct{}, len(prize.VoucherCodes))
		for _, code := range prize.VoucherCodes {
			if code == "" {
				return requestError(ErrorValidation)
			}
			if _, exists := seen[code]; exists {
				return requestError(ErrorValidation)
			}
			seen[code] = struct{}{}
		}
	case DeliveryManual:
		if prize.ManualContact == "" || len(prize.VoucherCodes) != 0 {
			return requestError(ErrorValidation)
		}
	default:
		return requestError(ErrorValidation)
	}
	return nil
}

func validatePositiveDecimal(value string) error {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed <= 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return requestError(ErrorValidation)
	}
	return nil
}

func (s *Service) requireCurrentAdminAccountID(ctx context.Context, userID string) (string, error) {
	if s.accounts == nil {
		return "", requestError(ErrorNoCurrentAccount)
	}
	return s.accounts.RequireCurrentID(ctx, userID)
}
func (s *Service) currentWorkspaceSourceOrigin(ctx context.Context, userID, adminAccountID string) (string, error) {
	session, err := s.adminSessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return "", requestError(ErrorAdminOnly)
	}
	origin, err := normalizeSrcHostWithPrivateTargets(session.BaseURL, s.allowPrivateTargets)
	if err != nil || session.Platform != upstream.PlatformSub2API {
		return "", requestError(ErrorInvalidSourceOrigin)
	}
	return origin, nil
}
func (s *Service) validateCurrentSourceBinding(ctx context.Context, userID, adminAccountID, storedOrigin, sessionSrcHost string) (upstream.Session, error) {
	session, err := s.adminSessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return upstream.Session{}, requestError(ErrorEmbedAdminSession)
	}
	origin, err := normalizeSrcHostWithPrivateTargets(session.BaseURL, s.allowPrivateTargets)
	if err != nil || session.Platform != upstream.PlatformSub2API || origin != storedOrigin || origin != sessionSrcHost {
		return upstream.Session{}, requestError(ErrorEmbedSourceBinding)
	}
	return session, nil
}
func (s *Service) autoBindEmbedConfig(ctx context.Context, userID, adminAccountID, origin string) (*EmbedConfig, error) {
	config, err := s.repository.GetEmbedConfigByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	if config == nil {
		token, err := s.newToken()
		if err != nil {
			return nil, err
		}
		config = &EmbedConfig{UserID: userID, AdminAccountID: adminAccountID, EmbedToken: token, Sub2apiSourceOrigin: origin, CreatedAt: s.now(), UpdatedAt: s.now()}
		if err := s.repository.InsertEmbedConfig(ctx, *config); err != nil {
			return nil, err
		}
		return s.repository.GetEmbedConfigByWorkspace(ctx, userID, adminAccountID)
	}
	if config.Sub2apiSourceOrigin != origin {
		if err := s.repository.UpdateEmbedConfig(ctx, userID, adminAccountID, origin); err != nil {
			return nil, err
		}
		_ = s.sessions.DeleteWorkspace(ctx, userID, adminAccountID)
		config.Sub2apiSourceOrigin = origin
	}
	return config, nil
}
func (s *Service) requireEmbedSession(ctx context.Context, token string) (*EmbedSession, error) {
	session, err := s.sessions.Get(ctx, strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, requestError(ErrorEmbedSessionInvalid)
	}
	return session, nil
}
func (s *Service) campaignResponse(ctx context.Context, c Campaign, admin bool) (CampaignResponse, error) {
	prizes, err := s.repository.ListPrizes(ctx, c.ID)
	if err != nil {
		return CampaignResponse{}, err
	}
	if !admin {
		prizes = redactPrizeDeliverySecrets(prizes)
	}
	winners := []WinnerResponse{}
	if admin || c.PublicWinners {
		rows, err := s.repository.ListWinners(ctx, c.ID)
		if err != nil {
			return CampaignResponse{}, err
		}
		for _, winner := range rows {
			winners = append(winners, WinnerResponse{ID: winner.ID, PrizeID: winner.PrizeID, EntryID: winner.EntryID, MaskedEmail: winner.MaskedEmail, PrizeSlot: winner.PrizeSlot})
		}
	}
	rewardStatuses := []RewardStatus{}
	if admin {
		rewardStatuses, err = s.repository.ListRewardStatuses(ctx, c.ID)
		if err != nil {
			return CampaignResponse{}, err
		}
	}
	return CampaignResponse{ID: c.ID, Name: c.Name, Description: c.Description, Status: c.Status, RegistrationStart: formatOptionalTime(c.RegistrationStart), RegistrationEnd: formatOptionalTime(c.RegistrationEnd), DrawAt: formatOptionalTime(c.DrawAt), DrawMode: c.DrawMode, PublicWinners: c.PublicWinners, SeedCommitment: c.SeedCommitment, EntrySnapshotHash: c.EntrySnapshotHash, RevealedSeed: visibleSeed(c, admin), AlgorithmVersion: c.AlgorithmVersion, EntryCount: c.EntryCount, WinnerCount: c.WinnerCount, Prizes: prizes, Winners: winners, RewardStatuses: rewardStatuses, CreatedAt: formatTime(c.CreatedAt), UpdatedAt: formatTime(c.UpdatedAt)}, nil
}

// redactPrizeDeliverySecrets keeps the public prize catalogue useful while
// preventing unassigned voucher codes or the organizer's private contact path
// from leaking to every embedded viewer.
func redactPrizeDeliverySecrets(prizes []Prize) []Prize {
	for i := range prizes {
		prizes[i].ManualContact = ""
		prizes[i].VoucherCodes = nil
	}
	return prizes
}

func (s *Service) embedCampaignResponse(ctx context.Context, campaign Campaign, sub2apiUserID string, includePublicEntries bool) (CampaignResponse, error) {
	response, err := s.campaignResponse(ctx, campaign, false)
	if err != nil {
		return CampaignResponse{}, err
	}
	if includePublicEntries {
		entries, err := s.repository.ListEntries(ctx, campaign.ID, false)
		if err != nil {
			return CampaignResponse{}, err
		}
		response.Entries = make([]EntryResponse, 0, len(entries))
		for _, entry := range entries {
			response.Entries = append(response.Entries, entryResponse(entry))
		}
	}
	entry, winner, reward, err := s.repository.GetViewerCampaignState(ctx, campaign.ID, sub2apiUserID)
	if err != nil {
		return CampaignResponse{}, err
	}
	if entry != nil {
		entryResp := entryResponse(*entry)
		response.MyEntry = &entryResp
	}
	if winner != nil {
		winnerResp := WinnerResponse{ID: winner.ID, PrizeID: winner.PrizeID, EntryID: winner.EntryID, MaskedEmail: winner.MaskedEmail, PrizeSlot: winner.PrizeSlot}
		response.MyWinner = &winnerResp
	}
	response.MyRewardStatus = reward
	return response, nil
}

func visibleSeed(c Campaign, admin bool) string {
	if admin || c.Status == StatusDrawn || c.Status == StatusFulfilling || c.Status == StatusCompleted || c.Status == StatusPartial {
		return c.RevealedSeed
	}
	return ""
}
func entryResponse(e Entry) EntryResponse {
	return EntryResponse{ID: e.ID, CampaignID: e.CampaignID, MaskedEmail: e.MaskedEmail, ReceiptHash: e.ReceiptHash, Status: e.Status, CreatedAt: formatTime(e.CreatedAt)}
}
func embedConfigResponse(config EmbedConfig) EmbedConfigResponse {
	return EmbedConfigResponse{EmbedToken: config.EmbedToken, Sub2apiSourceOrigin: config.Sub2apiSourceOrigin, CreatedAt: formatTime(config.CreatedAt), UpdatedAt: formatTime(config.UpdatedAt)}
}
func (s *Service) audit(ctx context.Context, c Campaign, actorType, actorID, event string, detail map[string]any) {
	id, err := newID("laud")
	if err != nil {
		return
	}
	if detail == nil {
		detail = map[string]any{}
	}
	_ = s.repository.AppendAudit(ctx, AuditLog{ID: id, CampaignID: c.ID, UserID: c.UserID, AdminAccountID: c.AdminAccountID, ActorType: actorType, ActorID: actorID, Event: event, Detail: detail})
}
