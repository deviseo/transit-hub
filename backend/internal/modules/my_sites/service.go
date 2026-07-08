package my_sites

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// StateRepository 分组映射状态的持久化接口，由 Repository 实现。
type StateRepository interface {
	Get(ctx context.Context, userID string, adminAccountID string) (*State, error)
	Save(ctx context.Context, state State) error
}

// RealConnectionRepository 真实对接绑定记录的持久化接口。
type RealConnectionRepository interface {
	SaveRealConnection(ctx context.Context, conn RealConnection) error
	ListRealConnections(ctx context.Context, userID string, adminAccountID string) ([]RealConnection, error)
	GetRealConnection(ctx context.Context, id string, userID string, adminAccountID string) (*RealConnection, error)
	DeleteRealConnection(ctx context.Context, id string, userID string, adminAccountID string) error
}

// UpstreamSiteLookup 根据 ID 获取上游站点信息（含 Session），供真实对接流程使用。
type UpstreamSiteLookup interface {
	GetSite(ctx context.Context, siteID string) (*upstream.Site, error)
}

// BotNotifier 机器人通知发送接口，由 settings.Service 实现。
// 自动调价成功后通过此接口向用户配置的机器人发送通知。
type BotNotifier interface {
	SendToBots(ctx context.Context, userID string, botIDs []string, message string)
}

// Service 负责分组映射的查询与保存，以及真实对接的编排。
// 供仪表盘分组弹窗和分组倍率页面复用。
type Service struct {
	repository      StateRepository
	connRepository  RealConnectionRepository
	platformService *upstream.PlatformService
	upstreamLookup  UpstreamSiteLookup
	botNotifier     BotNotifier
	accounts        AdminAccountResolver
}

type AdminAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

func NewService(repository StateRepository, platformService *upstream.PlatformService, upstreamLookup UpstreamSiteLookup) *Service {
	return &Service{repository: repository, platformService: platformService, upstreamLookup: upstreamLookup}
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	if repo, ok := s.repository.(*Repository); ok {
		s.connRepository = repo
		return repo.EnsureSchema(ctx)
	}
	return nil
}

// SetBotNotifier 注入机器人通知发送能力，供自动调价成功后发送通知。
func (s *Service) SetBotNotifier(notifier BotNotifier) {
	s.botNotifier = notifier
}

func (s *Service) SetAdminAccountResolver(accounts AdminAccountResolver) {
	s.accounts = accounts
}

// MappingOptions 获取分组映射选项：自有分组（通过 admin 接口拉取全量）与上游分组（从缓存读取）。
// 每次调用会刷新自有分组列表并清理引用了已不存在分组的映射关系。
func (s *Service) MappingOptions(ctx context.Context, userID string) (MappingOptionsResponse, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return MappingOptionsResponse{}, err
	}
	state, err := s.authenticatedState(ctx, userID, adminAccountID)
	if err != nil {
		return MappingOptionsResponse{}, err
	}
	adminGroups, err := s.platformService.FetchAdminAllGroups(state.Session)
	if err != nil {
		return MappingOptionsResponse{}, err
	}
	// 用最新拉取的分组列表更新缓存，并将历史真实对接记录补偿回 mappings。
	freshOwnGroups := make([]GroupOption, 0, len(adminGroups))
	idToName := make(map[string]string, len(adminGroups))
	for _, g := range adminGroups {
		name := strings.TrimSpace(g.Name)
		if name != "" {
			idToName[g.ID] = name
		}
		multiplier := 0.0
		if g.Multiplier != nil {
			multiplier = *g.Multiplier
		}
		freshOwnGroups = append(freshOwnGroups, GroupOption{Name: name, Multiplier: multiplier})
	}
	if err := s.backfillMappingsFromRealConnections(ctx, state, idToName); err != nil {
		return MappingOptionsResponse{}, err
	}
	// 自有分组变化时，自动清理引用了已不存在分组的映射关系
	freshGroupSet := make(map[string]struct{}, len(freshOwnGroups))
	for _, g := range freshOwnGroups {
		freshGroupSet[strings.TrimSpace(g.Name)] = struct{}{}
	}
	cleanedMappings := make([]GroupMapping, 0, len(state.Mappings))
	for _, m := range state.Mappings {
		if _, exists := freshGroupSet[strings.TrimSpace(m.OwnGroup)]; exists {
			cleanedMappings = append(cleanedMappings, m)
		}
	}
	state.OwnGroups = freshOwnGroups
	state.Mappings = cleanedMappings
	if err := s.repository.Save(ctx, *state); err != nil {
		return MappingOptionsResponse{}, err
	}

	groups := make([]MappingOwnGroupOption, 0, len(adminGroups))
	for _, g := range adminGroups {
		name := strings.TrimSpace(g.Name)
		if name != "" {
			multiplier := 0.0
			if g.Multiplier != nil {
				multiplier = *g.Multiplier
			}
			groups = append(groups, MappingOwnGroupOption{
				ID:               g.ID,
				SiteName:         state.Email,
				GroupName:        name,
				Multiplier:       multiplier,
				Platform:         g.Platform,
				Status:           g.Status,
				IsExclusive:      g.IsExclusive,
				SubscriptionType: g.SubscriptionType,
			})
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].SiteName == groups[j].SiteName {
			return groups[i].GroupName < groups[j].GroupName
		}
		return groups[i].SiteName < groups[j].SiteName
	})
	return MappingOptionsResponse{OwnGroups: groups, Mappings: state.Mappings}, nil
}

// SaveMappings 保存用户的分组映射关系，包含自动调价配置。
// 对自动调价字段做基础归一化和校验：
//   - AutoPricingSource 为空时默认 primary_upstream
//   - AutoPricingStrategy 为空时默认 percentage
//   - EnableAutoPricing=true 且 source=primary_upstream 时，主上游必须在 UpstreamTargets 中
//   - MinMultiplier 和 MaxMultiplier 同时设置时必须 min <= max
func (s *Service) SaveMappings(ctx context.Context, userID string, mappings []MappingRequest) (StatusResponse, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return StatusResponse{}, err
	}
	state, err := s.authenticatedState(ctx, userID, adminAccountID)
	if err != nil {
		return StatusResponse{}, err
	}
	next := make([]GroupMapping, 0, len(mappings))
	for _, mapping := range mappings {
		ownGroup := strings.TrimSpace(mapping.OwnGroup)
		if ownGroup == "" {
			continue
		}
		targets := make([]UpstreamGroupRef, 0, len(mapping.UpstreamTargets))
		for _, target := range mapping.UpstreamTargets {
			if strings.TrimSpace(target.SiteID) == "" || strings.TrimSpace(target.GroupName) == "" {
				continue
			}
			targets = append(targets, UpstreamGroupRef{SiteID: strings.TrimSpace(target.SiteID), GroupName: strings.TrimSpace(target.GroupName)})
		}

		// 归一化自动调价配置默认值（指针 nil 表示未传，此时用默认值；显式传 0 保留 0）
		source := strings.TrimSpace(mapping.AutoPricingSource)
		if source == "" {
			source = "primary_upstream"
		}
		strategy := strings.TrimSpace(mapping.AutoPricingStrategy)
		if strategy == "" {
			strategy = "percentage"
		}
		fixedIncrease := floatOrDefault(mapping.FixedIncrease, 0.1)
		percentageIncrease := floatOrDefault(mapping.PercentageIncrease, 10)
		thresholdPercent := floatOrDefault(mapping.AdjustThresholdPercent, 10)

		// 校验：数值字段不能为负
		if fixedIncrease < 0 || percentageIncrease < 0 || thresholdPercent < 0 {
			return StatusResponse{}, requestError(ErrorInvalidAutoPricingConf)
		}
		if mapping.MinMultiplier != nil && *mapping.MinMultiplier < 0 {
			return StatusResponse{}, requestError(ErrorInvalidAutoPricingConf)
		}
		if mapping.MaxMultiplier != nil && *mapping.MaxMultiplier < 0 {
			return StatusResponse{}, requestError(ErrorInvalidAutoPricingConf)
		}

		// 校验：开启自动调价且来源为 primary_upstream 时，主上游必须在 targets 中
		if mapping.EnableAutoPricing && source == "primary_upstream" {
			primarySiteID := strings.TrimSpace(mapping.PrimaryUpstreamSiteID)
			primaryGroupName := strings.TrimSpace(mapping.PrimaryUpstreamGroupName)
			if primarySiteID == "" || primaryGroupName == "" {
				return StatusResponse{}, requestError(ErrorInvalidAutoPricingConf)
			}
			found := false
			for _, t := range targets {
				if t.SiteID == primarySiteID && t.GroupName == primaryGroupName {
					found = true
					break
				}
			}
			if !found {
				return StatusResponse{}, requestError(ErrorInvalidAutoPricingConf)
			}
		}

		// 校验：min <= max
		if mapping.MinMultiplier != nil && mapping.MaxMultiplier != nil && *mapping.MinMultiplier > *mapping.MaxMultiplier {
			return StatusResponse{}, requestError(ErrorInvalidAutoPricingConf)
		}

		// 归一化自动调价通知配置
		notifyBotIDs := filterEmptyStrings(mapping.AutoPricingNotifyBotIDs)
		notifyTemplate := strings.TrimSpace(mapping.AutoPricingNotifyTemplate)

		// 校验：开启通知时必须至少选择一个机器人
		if mapping.EnableAutoPricingNotify && len(notifyBotIDs) == 0 {
			return StatusResponse{}, requestError(ErrorInvalidAutoPricingConf)
		}

		gm := GroupMapping{
			OwnGroup:                  ownGroup,
			UpstreamTargets:           targets,
			EnableAutoPricing:         mapping.EnableAutoPricing,
			AutoPricingSource:         source,
			PrimaryUpstreamSiteID:     strings.TrimSpace(mapping.PrimaryUpstreamSiteID),
			PrimaryUpstreamGroupName:  strings.TrimSpace(mapping.PrimaryUpstreamGroupName),
			AutoPricingStrategy:       strategy,
			FixedIncrease:             fixedIncrease,
			PercentageIncrease:        percentageIncrease,
			AdjustThresholdPercent:    thresholdPercent,
			MinMultiplier:             mapping.MinMultiplier,
			MaxMultiplier:             mapping.MaxMultiplier,
			EnableAutoPricingNotify:   mapping.EnableAutoPricingNotify,
			AutoPricingNotifyBotIDs:   notifyBotIDs,
			AutoPricingNotifyTemplate: notifyTemplate,
		}
		next = append(next, gm)
	}
	state.Mappings = next
	if err := s.repository.Save(ctx, *state); err != nil {
		return StatusResponse{}, err
	}
	return StatusResponse{Authenticated: true, BaseURL: state.BaseURL, Email: state.Email, Mappings: state.Mappings}, nil
}

// RealConnect 执行真实对接流程：按平台分支创建上游 Key/Token 和 admin 端转发目标（账号/Channel），最后持久化绑定记录。
func (s *Service) RealConnect(ctx context.Context, userID string, req RealConnectRequest) (RealConnectResponse, error) {
	if strings.TrimSpace(req.UpstreamSiteID) == "" || strings.TrimSpace(req.UpstreamGroupID) == "" ||
		len(req.OwnGroupIDs) == 0 {
		return RealConnectResponse{}, requestError(ErrorRequest)
	}

	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return RealConnectResponse{}, err
	}
	state, err := s.authenticatedState(ctx, userID, adminAccountID)
	if err != nil {
		return RealConnectResponse{}, err
	}

	upstreamSite, err := s.upstreamLookup.GetSite(ctx, req.UpstreamSiteID)
	if err != nil || upstreamSite == nil || upstreamSite.UserID != userID || upstreamSite.AdminAccountID != adminAccountID || upstreamSite.Session == nil {
		return RealConnectResponse{}, requestError(ErrorRequest)
	}

	groupType, multiplierDisplay := resolveGroupInfo(upstreamSite.Metrics.Groups, req.UpstreamGroupID)
	if groupType == "" && strings.TrimSpace(req.GroupType) != "" {
		groupType = req.GroupType
	}
	// new-api 的分组是纯名称，不强制要求分组平台类型；sub2api 仍然必填。
	if groupType == "" && upstreamSite.Platform != upstream.PlatformNewAPI {
		log.Printf("[real-connect] 无法识别分组平台类型 site=%s group_id=%s", upstreamSite.Name, req.UpstreamGroupID)
		return RealConnectResponse{}, requestError(ErrorRequest)
	}

	groupName := strings.TrimSpace(req.UpstreamGroupName)
	if groupName == "" {
		groupName = req.UpstreamGroupID
	}

	var conn RealConnection
	switch upstreamSite.Platform {
	case upstream.PlatformNewAPI:
		conn, err = s.realConnectNewAPI(ctx, userID, req, state, upstreamSite, groupType, groupName)
	default:
		conn, err = s.realConnectSub2API(ctx, userID, req, state, upstreamSite, groupType, groupName, multiplierDisplay)
	}
	if err != nil {
		return RealConnectResponse{}, err
	}

	if s.connRepository != nil {
		if err := s.connRepository.SaveRealConnection(ctx, conn); err != nil {
			log.Printf("[real-connect] 保存绑定记录失败 conn_id=%s err=%v", conn.ID, err)
			return RealConnectResponse{}, err
		}
	}

	s.addUpstreamMapping(ctx, userID, adminAccountID, req.OwnGroupIDs, req.UpstreamSiteID, groupName)

	log.Printf("[real-connect] 真实对接完成 conn_id=%s site=%s group=%s type=%s platform=%s", conn.ID, upstreamSite.Name, groupName, groupType, upstreamSite.Platform)
	return RealConnectResponse{Connection: conn}, nil
}

// realConnectSub2API 原有的 Sub2API 对接流程：创建 API Key + 创建转发账号。
func (s *Service) realConnectSub2API(ctx context.Context, userID string, req RealConnectRequest, state *State, upstreamSite *upstream.Site, groupType, groupName, multiplierDisplay string) (RealConnection, error) {
	groupIDInt, err := strconv.Atoi(req.UpstreamGroupID)
	if err != nil {
		return RealConnection{}, requestError(ErrorRequest)
	}

	keyName := fmt.Sprintf("%s-%s-%s", randomKeyPrefix(), upstreamSite.Name, groupName)
	typePrefix := groupTypePrefix(groupType)
	rateLabel := multiplierDisplay
	if rateLabel == "" {
		rateLabel = groupName
	}
	accountName := fmt.Sprintf("%s-【%s】-%s", typePrefix, upstreamSite.Name, rateLabel)

	log.Printf("[real-connect] sub2api 开始创建上游 key site=%s group=%s type=%s", upstreamSite.Name, groupName, groupType)
	upstreamSession := *upstreamSite.Session
	keyID, key, err := s.platformService.CreateSub2APIKey(upstreamSession, keyName, groupIDInt)
	if err != nil {
		log.Printf("[real-connect] 创建上游 key 失败 site=%s err=%v", upstreamSite.Name, err)
		return RealConnection{}, err
	}
	log.Printf("[real-connect] 上游 key 创建成功 key_id=%s", keyID)

	ownGroupIDInts, err := stringsToInts(req.OwnGroupIDs)
	if err != nil {
		log.Printf("[real-connect] 自有分组 ID 转换失败 err=%v", err)
		return RealConnection{}, requestError(ErrorRequest)
	}

	payload := buildAccountPayload(groupType, upstreamSite.BaseURL, key, ownGroupIDInts, accountName)
	accountID, err := s.platformService.CreateSub2APIAdminAccount(state.Session, payload)
	if err != nil {
		log.Printf("[real-connect] 创建 admin 账号失败 key_id=%s err=%v", keyID, err)
		return RealConnection{}, err
	}
	log.Printf("[real-connect] admin 账号创建成功 account_id=%s name=%s", accountID, accountName)

	connID, err := randomConnID()
	if err != nil {
		return RealConnection{}, err
	}
	return RealConnection{
		ID:                      connID,
		UserID:                  userID,
		WorkspaceAdminAccountID: state.AdminAccountID,
		UpstreamSiteID:          req.UpstreamSiteID,
		UpstreamGroupID:         req.UpstreamGroupID,
		UpstreamGroupName:       groupName,
		UpstreamKeyID:           keyID,
		UpstreamKey:             key,
		AdminAccountID:          accountID,
		AdminAccountName:        accountName,
		OwnGroupIDs:             req.OwnGroupIDs,
		GroupType:               groupType,
		CreatedAt:               time.Now().Format(time.RFC3339),
	}, nil
}

// realConnectNewAPI new-api 对接流程：创建 Token → 回查 Token ID → 获取完整 Key → 创建 Channel → 回查 Channel ID。
func (s *Service) realConnectNewAPI(ctx context.Context, userID string, req RealConnectRequest, state *State, upstreamSite *upstream.Site, groupType, groupName string) (RealConnection, error) {
	log.Printf("[real-connect] new-api 开始对接 site=%s group=%s type=%s", upstreamSite.Name, groupName, groupType)
	upstreamSession := *upstreamSite.Session

	// 步骤 1：在上游站点创建 Token
	tokenName := fmt.Sprintf("%s-%s-%s", randomKeyPrefix(), upstreamSite.Name, groupName)
	tokenID, tokenKey, err := s.platformService.CreateNewAPIToken(upstreamSession, tokenName, req.UpstreamGroupID)
	if err != nil {
		log.Printf("[real-connect] new-api 创建 token 失败 site=%s err=%v", upstreamSite.Name, err)
		return RealConnection{}, err
	}
	log.Printf("[real-connect] new-api token 创建成功 token_id=%s key=%s...", tokenID, safeKeyPreview(tokenKey))

	// 步骤 2：在 admin 端创建 Channel
	// 优先使用前端传入的 channelType（new-api 渠道类型 ID），否则按 groupType 字符串映射
	channelType := req.ChannelType
	if channelType <= 0 {
		channelType = groupTypeToNewAPIChannelType(groupType)
	}
	channelTypeName := newAPIChannelTypeName(channelType)
	channelName := fmt.Sprintf("%s-【%s】-%s", channelTypeName, upstreamSite.Name, groupName)
	channelID, err := s.platformService.CreateNewAPIChannel(state.Session, channelName, upstreamSite.BaseURL, tokenKey, channelType, req.OwnGroupIDs)
	if err != nil {
		log.Printf("[real-connect] new-api 创建 channel 失败 token_id=%s err=%v", tokenID, err)
		return RealConnection{}, err
	}
	log.Printf("[real-connect] new-api channel 创建成功 channel_id=%s name=%s", channelID, channelName)

	connID, err := randomConnID()
	if err != nil {
		return RealConnection{}, err
	}
	return RealConnection{
		ID:                      connID,
		UserID:                  userID,
		WorkspaceAdminAccountID: state.AdminAccountID,
		UpstreamSiteID:          req.UpstreamSiteID,
		UpstreamGroupID:         req.UpstreamGroupID,
		UpstreamGroupName:       groupName,
		UpstreamKeyID:           tokenID,
		UpstreamKey:             tokenKey,
		AdminAccountID:          channelID,
		AdminAccountName:        channelName,
		OwnGroupIDs:             req.OwnGroupIDs,
		GroupType:               groupType,
		CreatedAt:               time.Now().Format(time.RFC3339),
	}, nil
}

// groupTypeToNewAPIChannelType 将分组平台类型映射为 new-api channel type 数字（回退用）。
func groupTypeToNewAPIChannelType(groupType string) int {
	switch strings.ToLower(groupType) {
	case "openai":
		return 1
	case "anthropic":
		return 14
	case "gemini":
		return 24
	case "deepseek":
		return 43
	default:
		return 1
	}
}

// newAPIChannelTypeName 返回 new-api channel type ID 对应的短名称，用于 channel 命名前缀。
func newAPIChannelTypeName(channelType int) string {
	names := map[int]string{
		1: "OpenAI", 2: "Midjourney", 3: "Azure", 4: "Ollama",
		5: "MJ+", 6: "OpenAIMax", 7: "OhMyGPT", 8: "Custom",
		9: "AILS", 10: "AIProxy", 11: "PaLM", 12: "API2GPT",
		13: "AIGC2D", 14: "Anthropic", 15: "Baidu", 16: "Zhipu",
		17: "Ali", 18: "Xunfei", 19: "360", 20: "OpenRouter",
		21: "AIProxyLib", 22: "FastGPT", 23: "Tencent", 24: "Gemini",
		25: "Moonshot", 26: "ZhipuV4", 27: "Perplexity", 31: "LingYi",
		33: "AWS", 34: "Cohere", 35: "MiniMax", 36: "SunoAPI",
		37: "Dify", 38: "Jina", 39: "Cloudflare", 40: "SiliconFlow",
		41: "VertexAI", 42: "Mistral", 43: "DeepSeek", 44: "MokaAI",
		45: "VolcEngine", 46: "BaiduV2", 47: "Xinference", 48: "xAI",
		49: "Coze", 50: "Kling", 51: "Jimeng", 52: "Vidu",
		53: "Submodel", 54: "DoubaoVideo", 55: "Sora", 56: "Replicate",
		57: "Codex",
	}
	if name, ok := names[channelType]; ok {
		return name
	}
	return "OpenAI"
}

// safeKeyPreview 返回 key 的安全预览（前8个字符）。
func safeKeyPreview(key string) string {
	if len(key) > 8 {
		return key[:8]
	}
	return key
}

// ListUpstreamKeys 获取指定上游站点的 API Key 列表。
// 通过上游站点的 session 调用其 /api/v1/keys 接口，返回 key 列表供前端手动绑定时选择。
// ListUpstreamKeys 平台中性地获取上游站点的 Key/Token 列表。
// sub2api 列 API Key，new-api 列 Token（返回统一的 Sub2APIKeyItem 结构）。
func (s *Service) ListUpstreamKeys(ctx context.Context, userID string, siteID string) ([]upstream.Sub2APIKeyItem, error) {
	if strings.TrimSpace(siteID) == "" {
		return nil, requestError(ErrorRequest)
	}
	upstreamSite, err := s.upstreamLookup.GetSite(ctx, siteID)
	if err != nil || upstreamSite == nil || upstreamSite.Session == nil {
		return nil, requestError(ErrorRequest)
	}
	session := *upstreamSite.Session
	var keys []upstream.Sub2APIKeyItem
	switch session.Platform {
	case upstream.PlatformNewAPI:
		keys, err = s.platformService.ListNewAPITokens(session)
	default:
		keys, err = s.platformService.ListSub2APIKeys(session)
	}
	if err != nil {
		log.Printf("[list-upstream-keys] 获取上游 key 列表失败 site=%s platform=%s err=%v", upstreamSite.Name, session.Platform, err)
		return nil, err
	}
	return keys, nil
}

// RealBind 手动绑定已有的上游 Key/Token，仅创建绑定记录。
// new-api 场景下 token 列表返回的 key 是脱敏的，需要通过 /api/token/:id/key 获取完整 key。
func (s *Service) RealBind(ctx context.Context, userID string, req RealBindRequest) (RealConnectResponse, error) {
	if strings.TrimSpace(req.UpstreamSiteID) == "" || strings.TrimSpace(req.UpstreamGroupID) == "" ||
		strings.TrimSpace(req.UpstreamKeyID) == "" || len(req.OwnGroupIDs) == 0 {
		return RealConnectResponse{}, requestError(ErrorRequest)
	}

	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return RealConnectResponse{}, err
	}
	upstreamSite, err := s.upstreamLookup.GetSite(ctx, req.UpstreamSiteID)
	if err != nil || upstreamSite == nil || upstreamSite.UserID != userID || upstreamSite.AdminAccountID != adminAccountID {
		return RealConnectResponse{}, requestError(ErrorRequest)
	}

	groupType, _ := resolveGroupInfo(upstreamSite.Metrics.Groups, req.UpstreamGroupID)
	if groupType == "" && strings.TrimSpace(req.GroupType) != "" {
		groupType = strings.ToLower(strings.TrimSpace(req.GroupType))
	}
	isNewAPI := upstreamSite.Session != nil && upstreamSite.Session.Platform == upstream.PlatformNewAPI
	if groupType == "" && !isNewAPI {
		log.Printf("[real-bind] 无法识别分组平台类型 site=%s group_id=%s", upstreamSite.Name, req.UpstreamGroupID)
		return RealConnectResponse{}, requestError(ErrorRequest)
	}

	groupName := strings.TrimSpace(req.UpstreamGroupName)
	if groupName == "" {
		groupName = req.UpstreamGroupID
	}

	// new-api 场景：token 列表中的 key 是脱敏的，需要获取完整 key
	upstreamKey := strings.TrimSpace(req.UpstreamKey)
	if upstreamSite.Session != nil && upstreamSite.Session.Platform == upstream.PlatformNewAPI {
		fullKey, fErr := s.platformService.FetchNewAPITokenKey(*upstreamSite.Session, strings.TrimSpace(req.UpstreamKeyID))
		if fErr != nil {
			log.Printf("[real-bind] new-api 获取完整 token key 失败 token_id=%s err=%v", req.UpstreamKeyID, fErr)
			return RealConnectResponse{}, fErr
		}
		upstreamKey = fullKey
	}

	connID, err := randomConnID()
	if err != nil {
		return RealConnectResponse{}, err
	}
	conn := RealConnection{
		ID:                      connID,
		UserID:                  userID,
		WorkspaceAdminAccountID: adminAccountID,
		UpstreamSiteID:          req.UpstreamSiteID,
		UpstreamGroupID:         req.UpstreamGroupID,
		UpstreamGroupName:       groupName,
		UpstreamKeyID:           strings.TrimSpace(req.UpstreamKeyID),
		UpstreamKey:             upstreamKey,
		AdminAccountID:          "",
		AdminAccountName:        "",
		OwnGroupIDs:             req.OwnGroupIDs,
		GroupType:               groupType,
		CreatedAt:               time.Now().Format(time.RFC3339),
	}
	if s.connRepository != nil {
		if err := s.connRepository.SaveRealConnection(ctx, conn); err != nil {
			log.Printf("[real-bind] 保存绑定记录失败 conn_id=%s err=%v", connID, err)
			return RealConnectResponse{}, err
		}
	}

	s.addUpstreamMapping(ctx, userID, adminAccountID, req.OwnGroupIDs, req.UpstreamSiteID, groupName)

	log.Printf("[real-bind] 手动绑定完成 conn_id=%s site=%s group=%s type=%s", connID, upstreamSite.Name, groupName, groupType)
	return RealConnectResponse{Connection: conn}, nil
}

// ListRealConnections 获取指定用户的所有真实对接绑定记录。
func (s *Service) ListRealConnections(ctx context.Context, userID string) ([]RealConnection, error) {
	if s.connRepository == nil {
		return nil, nil
	}
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.connRepository.ListRealConnections(ctx, userID, adminAccountID)
}

// ListRealConnectionsForWorkspace 按显式传入的 userID + adminAccountID 查询真实对接绑定记录，
// 不解析"当前" workspace。供没有 HTTP 请求上下文的后台调度器使用：调度器持有的策略
// （connection_health_policies）本身就记录了 user_id/admin_account_id，必须按策略自带的
// workspace 读取对应连接，不能依赖 authctx/admin_accounts 的"当前工作区"语义（那是请求态概念）。
func (s *Service) ListRealConnectionsForWorkspace(ctx context.Context, userID string, adminAccountID string) ([]RealConnection, error) {
	if s.connRepository == nil {
		return nil, nil
	}
	return s.connRepository.ListRealConnections(ctx, userID, adminAccountID)
}

// RealDisconnect 取消真实对接：根据 mode 决定是仅删除记录还是同时清理远端资源。
// mode == "unlink"：仅删除 real_connections 记录（所有平台通用）。
// mode == "full"：按平台分支删除远端资源（sub2api 删 admin 账号+上游 key，new-api 删 channel+token），再删除记录。
func (s *Service) RealDisconnect(ctx context.Context, userID string, req RealDisconnectRequest) error {
	if strings.TrimSpace(req.ConnectionID) == "" || (req.Mode != "unlink" && req.Mode != "full") {
		return requestError(ErrorRequest)
	}
	if s.connRepository == nil {
		return requestError(ErrorRequest)
	}

	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return err
	}
	conn, err := s.connRepository.GetRealConnection(ctx, req.ConnectionID, userID, adminAccountID)
	if err != nil {
		return err
	}
	if conn == nil {
		return requestError(ErrorRequest)
	}

	if req.Mode == "full" {
		state, err := s.authenticatedState(ctx, userID, adminAccountID)
		if err != nil {
			return err
		}

		upstreamSite, err := s.upstreamLookup.GetSite(ctx, conn.UpstreamSiteID)
		if err != nil || upstreamSite == nil || upstreamSite.UserID != userID || upstreamSite.AdminAccountID != adminAccountID || upstreamSite.Session == nil {
			return requestError(ErrorRequest)
		}
		upstreamSession := *upstreamSite.Session

		switch upstreamSession.Platform {
		case upstream.PlatformNewAPI:
			// new-api：先删 admin channel，再删上游 token
			if conn.AdminAccountID != "" {
				log.Printf("[real-disconnect] new-api 开始删除 channel channel_id=%s", conn.AdminAccountID)
				if err := s.platformService.DeleteNewAPIChannel(state.Session, conn.AdminAccountID); err != nil {
					log.Printf("[real-disconnect] new-api 删除 channel 失败 channel_id=%s err=%v", conn.AdminAccountID, err)
					return err
				}
				log.Printf("[real-disconnect] new-api channel 已删除 channel_id=%s", conn.AdminAccountID)
			}
			if conn.UpstreamKeyID != "" {
				log.Printf("[real-disconnect] new-api 开始删除 token token_id=%s", conn.UpstreamKeyID)
				if err := s.platformService.DeleteNewAPIToken(upstreamSession, conn.UpstreamKeyID); err != nil {
					log.Printf("[real-disconnect] new-api 删除 token 失败 token_id=%s err=%v", conn.UpstreamKeyID, err)
					return err
				}
				log.Printf("[real-disconnect] new-api token 已删除 token_id=%s", conn.UpstreamKeyID)
			}

		default:
			// sub2api：先删 admin 账号，再删上游 key（保持原有逻辑）
			log.Printf("[real-disconnect] 开始删除 admin 账号 account_id=%s", conn.AdminAccountID)
			if err := s.platformService.DeleteSub2APIAdminAccount(state.Session, conn.AdminAccountID); err != nil {
				log.Printf("[real-disconnect] 删除 admin 账号失败 account_id=%s err=%v", conn.AdminAccountID, err)
				return err
			}
			log.Printf("[real-disconnect] admin 账号已删除 account_id=%s", conn.AdminAccountID)

			log.Printf("[real-disconnect] 开始删除上游 key key_id=%s", conn.UpstreamKeyID)
			if err := s.platformService.DeleteSub2APIKey(upstreamSession, conn.UpstreamKeyID); err != nil {
				log.Printf("[real-disconnect] 删除上游 key 失败 key_id=%s err=%v", conn.UpstreamKeyID, err)
				return err
			}
			log.Printf("[real-disconnect] 上游 key 已删除 key_id=%s", conn.UpstreamKeyID)
		}
	}

	if err := s.connRepository.DeleteRealConnection(ctx, req.ConnectionID, userID, adminAccountID); err != nil {
		return err
	}

	s.removeUpstreamMapping(ctx, userID, adminAccountID, conn.UpstreamSiteID, conn.UpstreamGroupName)

	log.Printf("[real-disconnect] 取消对接完成 conn_id=%s mode=%s", req.ConnectionID, req.Mode)
	return nil
}

// removeUpstreamMapping 从用户的 my_site_states.mappings 中移除引用了指定上游站点+分组的映射目标。
// 清理后如果某个自有分组的 upstreamTargets 为空，则整条映射也会被移除。
func (s *Service) removeUpstreamMapping(ctx context.Context, userID, adminAccountID, siteID, groupName string) {
	state, err := s.repository.Get(ctx, userID, adminAccountID)
	if err != nil || state == nil || len(state.Mappings) == 0 {
		return
	}
	cleaned := make([]GroupMapping, 0, len(state.Mappings))
	for _, m := range state.Mappings {
		targets := make([]UpstreamGroupRef, 0, len(m.UpstreamTargets))
		for _, t := range m.UpstreamTargets {
			if t.SiteID == siteID && t.GroupName == groupName {
				continue
			}
			targets = append(targets, t)
		}
		if len(targets) > 0 {
			m.UpstreamTargets = targets
			cleaned = append(cleaned, m)
		}
	}
	state.Mappings = cleaned
	_ = s.repository.Save(ctx, *state)
}

// backfillMappingsFromRealConnections uses real_connections as the source of truth for
// existing real-connect/manual-bind records and repairs my_site_states.mappings before the
// dashboard group modal is returned. This covers historical records created while mapping
// sync failed or before the mapping cache existed.
func (s *Service) backfillMappingsFromRealConnections(ctx context.Context, state *State, idToName map[string]string) error {
	if s.connRepository == nil || state == nil {
		return nil
	}
	connections, err := s.connRepository.ListRealConnections(ctx, state.UserID, state.AdminAccountID)
	if err != nil {
		return err
	}
	if len(connections) == 0 {
		return nil
	}

	existing := make(map[string]int, len(state.Mappings))
	for i := range state.Mappings {
		existing[state.Mappings[i].OwnGroup] = i
	}

	for _, conn := range connections {
		target := UpstreamGroupRef{SiteID: conn.UpstreamSiteID, GroupName: conn.UpstreamGroupName}
		for _, ownID := range conn.OwnGroupIDs {
			ownName, ok := idToName[ownID]
			if !ok {
				log.Printf("[mapping-backfill] 未找到分组 ID=%s 对应的名称，跳过 conn_id=%s", ownID, conn.ID)
				continue
			}
			mappingIndex, found := existing[ownName]
			if !found {
				state.Mappings = append(state.Mappings, GroupMapping{
					OwnGroup:        ownName,
					UpstreamTargets: []UpstreamGroupRef{target},
				})
				existing[ownName] = len(state.Mappings) - 1
				continue
			}
			if !hasUpstreamTarget(state.Mappings[mappingIndex].UpstreamTargets, target) {
				state.Mappings[mappingIndex].UpstreamTargets = append(state.Mappings[mappingIndex].UpstreamTargets, target)
			}
		}
	}
	return nil
}

func hasUpstreamTarget(targets []UpstreamGroupRef, target UpstreamGroupRef) bool {
	for _, existing := range targets {
		if existing.SiteID == target.SiteID && existing.GroupName == target.GroupName {
			return true
		}
	}
	return false
}

// addUpstreamMapping 将上游站点+分组添加到用户 my_site_states.mappings 中每个关联的自有分组里。
// 如果自有分组尚未有映射记录则创建，如果已有则在 upstreamTargets 中追加（去重）。
// 注意：mappings 中 OwnGroup 存储的是分组名称（非数字 ID），与仪表盘分组关联一致。
func (s *Service) addUpstreamMapping(ctx context.Context, userID string, adminAccountID string, ownGroupIDs []string, siteID, groupName string) {
	state, err := s.repository.Get(ctx, userID, adminAccountID)
	if err != nil || state == nil {
		return
	}

	// 获取 admin 分组列表，构建 ID → 分组名称 的映射
	// mappings 中 OwnGroup 使用分组名称（与 MappingOptions 清理逻辑和前端 GroupListModal 一致）
	adminGroups, err := s.platformService.FetchAdminAllGroups(state.Session)
	if err != nil {
		log.Printf("[add-upstream-mapping] 获取 admin 分组失败 err=%v", err)
		return
	}
	idToName := make(map[string]string, len(adminGroups))
	for _, g := range adminGroups {
		if name := strings.TrimSpace(g.Name); name != "" {
			idToName[g.ID] = name
		}
	}

	target := UpstreamGroupRef{SiteID: siteID, GroupName: groupName}

	existing := make(map[string]*GroupMapping, len(state.Mappings))
	for i := range state.Mappings {
		existing[state.Mappings[i].OwnGroup] = &state.Mappings[i]
	}

	for _, ownID := range ownGroupIDs {
		// 将数字 ID 解析为分组名称
		ownName, ok := idToName[ownID]
		if !ok {
			log.Printf("[add-upstream-mapping] 未找到分组 ID=%s 对应的名称，跳过", ownID)
			continue
		}

		if m, found := existing[ownName]; found {
			alreadyHas := false
			for _, t := range m.UpstreamTargets {
				if t.SiteID == siteID && t.GroupName == groupName {
					alreadyHas = true
					break
				}
			}
			if !alreadyHas {
				m.UpstreamTargets = append(m.UpstreamTargets, target)
			}
		} else {
			newMapping := GroupMapping{
				OwnGroup:        ownName,
				UpstreamTargets: []UpstreamGroupRef{target},
			}
			state.Mappings = append(state.Mappings, newMapping)
			existing[ownName] = &state.Mappings[len(state.Mappings)-1]
		}
	}
	_ = s.repository.Save(ctx, *state)
}

// keyPrefixes 创建 API Key 时随机选取的名称前缀池，契合 TransitHub（流量枢纽）项目主题。
var keyPrefixes = []string{
	"Relay",    // 中继站
	"Express",  // 快线
	"Conduit",  // 管道
	"Nexus",    // 枢纽
	"Voyage",   // 航程
	"Shuttle",  // 穿梭
	"Beacon",   // 信标
	"Meridian", // 子午线
	"Transit",  // 中转
	"Vector",   // 航向
	"Flux",     // 流
	"Pulse",    // 脉冲
	"Arc",      // 弧线
	"Drift",    // 漂流
	"Link",     // 链路
	"Orbit",    // 轨道
}

// randomKeyPrefix 从前缀池中随机选取一个，用于 API Key 命名。
func randomKeyPrefix() string {
	b := make([]byte, 1)
	_, _ = rand.Read(b)
	return keyPrefixes[int(b[0])%len(keyPrefixes)]
}

// groupTypePrefix 根据分组类型返回账号名称前缀（A=OpenAI, B=Anthropic, C=Gemini, D=Antigravity）。
func groupTypePrefix(groupType string) string {
	switch strings.ToLower(groupType) {
	case "openai":
		return "A"
	case "anthropic":
		return "B"
	case "gemini":
		return "C"
	case "antigravity":
		return "D"
	default:
		return "X"
	}
}

// resolveGroupInfo 从上游站点缓存的分组列表中查找指定分组的平台类型和倍率显示文本。
// 返回小写的平台名（如 "openai"、"anthropic"）和倍率显示文本（如 "1.5x"），未找到时返回空字符串。
func resolveGroupInfo(groups []upstream.GroupInfo, groupID string) (groupType string, multiplierDisplay string) {
	for _, g := range groups {
		if g.ID == groupID {
			if g.Platform != nil && strings.TrimSpace(*g.Platform) != "" {
				groupType = strings.ToLower(strings.TrimSpace(*g.Platform))
			}
			multiplierDisplay = g.MultiplierDisplay
			return
		}
	}
	return
}

// stringsToInts 将字符串切片转为整数切片（Sub2API 接口要求 group_ids 为整数数组）。
func stringsToInts(ss []string) ([]int, error) {
	result := make([]int, 0, len(ss))
	for _, s := range ss {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("invalid group id %q: %w", s, err)
		}
		result = append(result, n)
	}
	return result, nil
}

// buildAccountPayload 按分组类型组装 admin 站点创建转发账号的请求体。
// 不同类型有不同的 platform、extra、credentials 配置，详见计划文档中的类型表。
func buildAccountPayload(groupType, baseURL, apiKey string, ownGroupIDs []int, accountName string) map[string]any {
	credentials := map[string]any{
		"base_url": baseURL,
		"api_key":  apiKey,
	}

	payload := map[string]any{
		"name":        accountName,
		"type":        "apikey",
		"credentials": credentials,
		"priority":    1,
		"group_ids":   ownGroupIDs,
	}

	switch strings.ToLower(groupType) {
	case "openai":
		payload["platform"] = "openai"
		credentials["pool_mode"] = true
		payload["extra"] = map[string]any{"openai_passthrough": true}
		payload["concurrency"] = 1000
	case "anthropic":
		payload["platform"] = "anthropic"
		credentials["pool_mode"] = true
		payload["extra"] = map[string]any{"anthropic_passthrough": true}
		payload["concurrency"] = 1000
	case "gemini":
		payload["platform"] = "gemini"
		credentials["pool_mode"] = true
		credentials["tier_id"] = "aistudio_free"
		payload["concurrency"] = 1000
	case "antigravity":
		payload["platform"] = "antigravity"
		payload["concurrency"] = 10
	default:
		payload["platform"] = groupType
		payload["concurrency"] = 100
	}

	return payload
}

// randomConnID 生成真实对接绑定记录的唯一 ID。
func randomConnID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate connection id: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// authenticatedState 获取并校验用户的 admin 会话（平台感知），必要时刷新令牌。
func (s *Service) authenticatedState(ctx context.Context, userID string, adminAccountID string) (*State, error) {
	state, err := s.repository.Get(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	if state == nil || !state.Session.IsAuthenticated() {
		return nil, requestError(ErrorAuthRequired)
	}
	return s.validatedState(ctx, state)
}

// RequireSession 获取并校验用户的 admin 会话（必要时刷新令牌），供活动调价模块
// （group_rate_campaigns.AdminGroupOperator）在开启/恢复活动时复用同一套会话管理逻辑，
// 避免活动调价模块重复实现 token 刷新和 admin 角色校验。
func (s *Service) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	state, err := s.authenticatedState(ctx, userID, adminAccountID)
	if err != nil {
		return upstream.Session{}, err
	}
	return state.Session, nil
}

// FetchAdminGroups 透传 platformService 拉取 admin 自有分组列表。
func (s *Service) FetchAdminGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error) {
	return s.platformService.FetchAdminAllGroups(session)
}

// UpdateAdminGroupMultiplier 透传 platformService 修改 admin 自有分组倍率。
func (s *Service) UpdateAdminGroupMultiplier(session upstream.Session, group upstream.AdminGroupInfo, multiplier float64) error {
	return s.platformService.UpdateAdminGroupMultiplier(session, group, multiplier)
}

// validatedState 刷新临期令牌并校验 admin 角色（平台中性）。
func (s *Service) validatedState(ctx context.Context, state *State) (*State, error) {
	if !state.Session.IsAuthenticated() {
		return nil, requestError(ErrorAuthRequired)
	}
	refreshedSession, err := s.platformService.RefreshSession(state.Session)
	if err != nil {
		return nil, requestError(ErrorAdminOnly)
	}
	if refreshedSession.AccessToken != state.Session.AccessToken || refreshedSession.RefreshToken != state.Session.RefreshToken ||
		refreshedSession.Cookie != state.Session.Cookie {
		state.Session = refreshedSession
		if err := s.repository.Save(ctx, *state); err != nil {
			return nil, err
		}
	}
	if err := s.platformService.VerifyAdmin(state.Session); err != nil {
		return nil, requestError(ErrorAdminOnly)
	}
	return state, nil
}

// SyncAdminSession 实现 dashboard.MySiteStateSync 接口。
// dashboard 登录成功后调用此方法，将 admin session 同步到 my_site_states 表，
// 使 RealConnect 等依赖 my_site_states 的功能可以使用 admin 会话。
// 保留已有的 mappings 和 own_groups，仅更新 session 和身份信息。
func (s *Service) SyncAdminSession(ctx context.Context, userID string, adminAccountID string, session upstream.Session, identity string) {
	existing, err := s.repository.Get(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[my-sites] sync admin session: read failed user_id=%s err=%v", userID, err)
		return
	}
	if existing == nil {
		existing = &State{
			UserID:         userID,
			AdminAccountID: adminAccountID,
			Mappings:       []GroupMapping{},
		}
	}
	existing.AdminAccountID = adminAccountID
	existing.BaseURL = session.BaseURL
	existing.Email = identity
	existing.Session = session
	if err := s.repository.Save(ctx, *existing); err != nil {
		log.Printf("[my-sites] sync admin session: save failed user_id=%s err=%v", userID, err)
	}
}

func (s *Service) currentAdminAccountID(ctx context.Context, userID string) (string, error) {
	if s.accounts == nil {
		return "", requestError("admin.adminAccounts.errors.noCurrentAccount")
	}
	return s.accounts.RequireCurrentID(ctx, userID)
}

// floatOrDefault 解引用指针，nil 时返回默认值，非 nil 时返回实际值（含 0）。
func floatOrDefault(p *float64, defaultVal float64) float64 {
	if p == nil {
		return defaultVal
	}
	return *p
}

// changedGroup 表示一个上游分组在同步前后倍率发生了变化。
type changedGroup struct {
	GroupName     string
	OldMultiplier float64
	NewMultiplier float64
}

// groupMultiplierChange 记录单个分组在本次同步中的旧/新倍率。
// 用于构建同步站点的倍率变化快照，避免聚合来源从缓存读取到已被覆盖的新值。
type groupMultiplierChange struct {
	Old float64
	New float64
}

// changedUpstreamGroups 对比同步前后的 Metrics，返回倍率发生变化的上游分组列表。
// 使用 group.ID + "|" + group.Name 作为匹配 key，与通知逻辑保持一致。
func changedUpstreamGroups(oldMetrics, newMetrics upstream.Metrics) []changedGroup {
	if len(oldMetrics.Groups) == 0 || len(newMetrics.Groups) == 0 {
		return nil
	}
	oldMap := make(map[string]float64, len(oldMetrics.Groups))
	oldNameMap := make(map[string]string, len(oldMetrics.Groups))
	for _, g := range oldMetrics.Groups {
		if g.Multiplier != nil {
			key := g.ID + "|" + g.Name
			oldMap[key] = *g.Multiplier
			oldNameMap[key] = g.Name
		}
	}
	var result []changedGroup
	for _, g := range newMetrics.Groups {
		if g.Multiplier == nil {
			continue
		}
		key := g.ID + "|" + g.Name
		oldVal, existed := oldMap[key]
		if !existed || oldVal == *g.Multiplier {
			continue
		}
		result = append(result, changedGroup{
			GroupName:     g.Name,
			OldMultiplier: oldVal,
			NewMultiplier: *g.Multiplier,
		})
	}
	return result
}

// mappingUsesTarget 检查 mapping 的 UpstreamTargets 是否引用了指定的 siteID + groupName。
func mappingUsesTarget(mapping GroupMapping, siteID, groupName string) bool {
	for _, t := range mapping.UpstreamTargets {
		if t.SiteID == siteID && t.GroupName == groupName {
			return true
		}
	}
	return false
}

// autoPricingResult 记录单个分组自动调价的计算结果。
type autoPricingResult struct {
	OwnGroup         string
	OldReference     float64
	NewReference     float64
	TargetMultiplier float64
	Status           string // applied, threshold_exceeded, skipped, failed
	Reason           string
}

// percentEpsilon 阈值比较的浮点容差，避免 IEEE 754 精度问题把刚好等于阈值的变化误判为超限。
const percentEpsilon = 1e-9

// thresholdExceeded 判断参考倍率的变化百分比是否严格超过阈值。
// 等于阈值不算超限，使用 epsilon 容差消除浮点精度误差。
// 调用方须保证 oldRef > 0（除零保护在调用侧）。
func thresholdExceeded(oldRef, newRef, thresholdPercent float64) bool {
	changePercent := math.Abs(newRef-oldRef) / oldRef * 100
	return changePercent-thresholdPercent > percentEpsilon
}

// computeReferenceMultipliers 根据 mapping 的 AutoPricingSource 和本次同步站点的倍率变化快照
// 计算参考倍率（old 和 new），是可单元测试的纯函数。
//
// 参数：
//   - source: 调价来源（primary_upstream / lowest_upstream / highest_upstream / average_upstream）
//   - targets: mapping 关联的上游分组列表
//   - primarySiteID, primaryGroupName: 主上游配置
//   - syncSiteID: 本次同步的站点 ID
//   - changesByGroup: 本次同步站点所有变化分组的 old/new 快照（按 GroupName 索引）
//   - newMetricsGroups: 本次同步站点的最新分组列表（用于查找未变化分组的当前倍率）
//   - lookupMultiplier: 查询其他站点分组倍率的回调（从缓存读取）
func computeReferenceMultipliers(
	source string,
	targets []UpstreamGroupRef,
	primarySiteID, primaryGroupName string,
	syncSiteID string,
	changesByGroup map[string]groupMultiplierChange,
	newMetricsGroups []upstream.GroupInfo,
	lookupMultiplier func(siteID, groupName string) *float64,
) (oldRef, newRef float64, ok bool, reason string) {
	switch source {
	case "primary_upstream":
		// 主上游来源：仅当主上游在本次同步站点且发生了变化时才处理
		if primarySiteID != syncSiteID {
			return 0, 0, false, "primary_upstream_not_affected"
		}
		change, found := changesByGroup[primaryGroupName]
		if !found {
			return 0, 0, false, "primary_upstream_not_affected"
		}
		return change.Old, change.New, true, ""

	case "lowest_upstream", "highest_upstream", "average_upstream":
		// 聚合来源：收集所有关联上游的倍率，本次同步站点内的变化分组使用快照值
		var oldMultipliers, newMultipliers []float64
		for _, t := range targets {
			if t.SiteID == syncSiteID {
				// 同步站点内的分组：优先从变化快照取值
				if change, changed := changesByGroup[t.GroupName]; changed {
					oldMultipliers = append(oldMultipliers, change.Old)
					newMultipliers = append(newMultipliers, change.New)
				} else {
					// 同步站点但未变化的分组：old=new=当前值
					m := findGroupMultiplier(newMetricsGroups, t.GroupName)
					if m == nil {
						return 0, 0, false, "missing_reference_multiplier"
					}
					oldMultipliers = append(oldMultipliers, *m)
					newMultipliers = append(newMultipliers, *m)
				}
			} else {
				// 其他站点的分组：从缓存读取（不受本次同步影响）
				m := lookupMultiplier(t.SiteID, t.GroupName)
				if m == nil {
					return 0, 0, false, "missing_reference_multiplier"
				}
				oldMultipliers = append(oldMultipliers, *m)
				newMultipliers = append(newMultipliers, *m)
			}
		}
		if len(oldMultipliers) == 0 {
			return 0, 0, false, "missing_reference_multiplier"
		}
		return aggregateMultipliers(source, oldMultipliers),
			aggregateMultipliers(source, newMultipliers),
			true, ""

	default:
		return 0, 0, false, "unknown_pricing_source"
	}
}

// buildLookupMultiplier 构建从缓存查询其他站点分组倍率的回调函数。
func (s *Service) buildLookupMultiplier(ctx context.Context) func(siteID, groupName string) *float64 {
	return func(siteID, groupName string) *float64 {
		site, err := s.upstreamLookup.GetSite(ctx, siteID)
		if err != nil || site == nil {
			return nil
		}
		return findGroupMultiplier(site.Metrics.Groups, groupName)
	}
}

// findGroupMultiplier 在分组列表中按 Name 查找倍率。
func findGroupMultiplier(groups []upstream.GroupInfo, name string) *float64 {
	for _, g := range groups {
		if g.Name == name && g.Multiplier != nil {
			return g.Multiplier
		}
	}
	return nil
}

// aggregateMultipliers 按聚合策略计算多个倍率的聚合值。
func aggregateMultipliers(source string, multipliers []float64) float64 {
	switch source {
	case "lowest_upstream":
		min := multipliers[0]
		for _, m := range multipliers[1:] {
			if m < min {
				min = m
			}
		}
		return min
	case "highest_upstream":
		max := multipliers[0]
		for _, m := range multipliers[1:] {
			if m > max {
				max = m
			}
		}
		return max
	case "average_upstream":
		sum := 0.0
		for _, m := range multipliers {
			sum += m
		}
		return sum / float64(len(multipliers))
	default:
		return multipliers[0]
	}
}

// calculateAutoPricingTarget 根据自动调价策略和限制范围计算目标倍率。
// 返回目标倍率，四舍五入到 4 位小数。
func calculateAutoPricingTarget(mapping GroupMapping, newReference float64) float64 {
	var target float64
	if mapping.AutoPricingStrategy == "fixed" {
		target = newReference + mapping.FixedIncrease
	} else {
		target = newReference * (1 + mapping.PercentageIncrease/100)
	}
	// 套用最低/最高倍率限制
	if mapping.MinMultiplier != nil && target < *mapping.MinMultiplier {
		target = *mapping.MinMultiplier
	}
	if mapping.MaxMultiplier != nil && target > *mapping.MaxMultiplier {
		target = *mapping.MaxMultiplier
	}
	// 四舍五入到 4 位小数
	return math.Round(target*10000) / 10000
}

// ApplyAutoPricingAfterSync 在上游站点同步完成后，对所有启用自动调价的自有分组执行倍率调整。
// 只处理本次同步站点 siteID 相关的 mappings，每个 mapping 最多计算和更新一次。
// 使用 oldMetrics/newMetrics 构建变化快照，避免从缓存读取已被同步覆盖的旧值。
func (s *Service) ApplyAutoPricingAfterSync(ctx context.Context, userID, adminAccountID, siteID, siteName string, oldMetrics, newMetrics upstream.Metrics) {
	// 1. 构建本次同步站点的倍率变化快照（按 GroupName 索引）
	changesByGroup := buildChangesByGroup(oldMetrics, newMetrics)
	if len(changesByGroup) == 0 {
		return
	}

	// 2. 读取用户的 admin 状态和 mappings
	state, err := s.repository.Get(ctx, userID, adminAccountID)
	if err != nil || state == nil || !state.Session.IsAuthenticated() {
		log.Printf("[auto-pricing] 无法读取用户状态或未认证 user_id=%s err=%v", userID, err)
		return
	}

	// 刷新 session（如果需要），但不做完整的 admin 校验以避免额外请求
	refreshedSession, err := s.platformService.RefreshSession(state.Session)
	if err != nil {
		log.Printf("[auto-pricing] session 刷新失败 user_id=%s err=%v", userID, err)
		return
	}
	if refreshedSession.AccessToken != state.Session.AccessToken || refreshedSession.RefreshToken != state.Session.RefreshToken ||
		refreshedSession.Cookie != state.Session.Cookie {
		state.Session = refreshedSession
		_ = s.repository.Save(ctx, *state)
	}

	// 3. 筛选启用自动调价的 mappings
	var autoPricingMappings []GroupMapping
	for _, m := range state.Mappings {
		if !m.EnableAutoPricing {
			continue
		}
		autoPricingMappings = append(autoPricingMappings, m)
	}
	if len(autoPricingMappings) == 0 {
		return
	}

	// 4. 获取 admin 端全量分组（用于匹配 OwnGroup → 分组 ID 和当前倍率）
	adminGroups, err := s.platformService.FetchAdminAllGroups(state.Session)
	if err != nil {
		log.Printf("[auto-pricing] 获取 admin 分组失败 user_id=%s err=%v", userID, err)
		return
	}
	adminGroupMap := make(map[string]upstream.AdminGroupInfo, len(adminGroups))
	for _, g := range adminGroups {
		adminGroupMap[g.Name] = g
	}

	// 5. 遍历自动调价 mappings（非 changes×mappings），每个 mapping 最多处理一次
	lookupFn := s.buildLookupMultiplier(ctx)
	for _, mapping := range autoPricingMappings {
		// 检查该 mapping 是否引用了本次同步站点中发生变化的任意上游分组
		affected := false
		for _, t := range mapping.UpstreamTargets {
			if t.SiteID == siteID {
				if _, changed := changesByGroup[t.GroupName]; changed {
					affected = true
					break
				}
			}
		}
		if !affected {
			continue
		}

		result := s.processAutoPricing(ctx, userID, state, mapping, siteID, siteName, changesByGroup, newMetrics.Groups, adminGroupMap, lookupFn)
		logAutoPricingResult(siteName, result)
	}
}

// buildChangesByGroup 从同步前后的 Metrics 构建按 GroupName 索引的倍率变化快照。
// 只包含倍率确实发生变化的分组。
func buildChangesByGroup(oldMetrics, newMetrics upstream.Metrics) map[string]groupMultiplierChange {
	if len(oldMetrics.Groups) == 0 || len(newMetrics.Groups) == 0 {
		return nil
	}
	oldMap := make(map[string]float64, len(oldMetrics.Groups))
	for _, g := range oldMetrics.Groups {
		if g.Multiplier != nil {
			oldMap[g.ID+"|"+g.Name] = *g.Multiplier
		}
	}
	result := make(map[string]groupMultiplierChange)
	for _, g := range newMetrics.Groups {
		if g.Multiplier == nil {
			continue
		}
		key := g.ID + "|" + g.Name
		oldVal, existed := oldMap[key]
		if !existed || oldVal == *g.Multiplier {
			continue
		}
		result[g.Name] = groupMultiplierChange{Old: oldVal, New: *g.Multiplier}
	}
	return result
}

// processAutoPricing 处理单个 mapping 的自动调价逻辑。
// 使用 changesByGroup 快照和 newMetricsGroups 计算参考倍率，保证每个 mapping 只处理一次。
// siteName 为触发同步的上游站点名称，用于调价成功通知的模板变量。
func (s *Service) processAutoPricing(ctx context.Context, userID string, state *State, mapping GroupMapping, siteID, siteName string, changesByGroup map[string]groupMultiplierChange, newMetricsGroups []upstream.GroupInfo, adminGroupMap map[string]upstream.AdminGroupInfo, lookupFn func(string, string) *float64) autoPricingResult {
	result := autoPricingResult{OwnGroup: mapping.OwnGroup}

	// 计算参考倍率（纯函数，不依赖缓存读取本次同步站点的数据）
	oldRef, newRef, ok, reason := computeReferenceMultipliers(
		mapping.AutoPricingSource,
		mapping.UpstreamTargets,
		mapping.PrimaryUpstreamSiteID, mapping.PrimaryUpstreamGroupName,
		siteID,
		changesByGroup,
		newMetricsGroups,
		lookupFn,
	)
	if !ok {
		result.Status = "skipped"
		result.Reason = reason
		return result
	}
	result.OldReference = oldRef
	result.NewReference = newRef

	// 阈值判断：oldRef <= 0 防除零，thresholdExceeded 使用 epsilon 消除浮点误判
	if oldRef <= 0 {
		result.Status = "skipped"
		result.Reason = "invalid_old_reference_multiplier"
		return result
	}
	if thresholdExceeded(oldRef, newRef, mapping.AdjustThresholdPercent) {
		changePercent := math.Abs(newRef-oldRef) / oldRef * 100
		result.Status = "threshold_exceeded"
		result.Reason = fmt.Sprintf("change=%.2f%% threshold=%.2f%%", changePercent, mapping.AdjustThresholdPercent)
		return result
	}

	// 计算目标倍率
	target := calculateAutoPricingTarget(mapping, newRef)
	result.TargetMultiplier = target

	// 查找 admin 端对应的自有分组
	adminGroup, found := adminGroupMap[mapping.OwnGroup]
	if !found {
		result.Status = "skipped"
		result.Reason = "own_group_not_found_in_admin"
		return result
	}

	// 检查目标倍率是否与当前一致
	if adminGroup.Multiplier != nil && math.Round(*adminGroup.Multiplier*10000)/10000 == target {
		result.Status = "skipped"
		result.Reason = "target_unchanged"
		return result
	}

	// 记录调整前倍率，用于通知模板
	oldOwnMultiplier := adminGroup.Multiplier

	// 调用远端 API 更新倍率
	if err := s.platformService.UpdateAdminGroupMultiplier(state.Session, adminGroup, target); err != nil {
		result.Status = "failed"
		result.Reason = err.Error()
		return result
	}

	// 更新本地缓存的分组倍率
	for i, g := range state.OwnGroups {
		if g.Name == mapping.OwnGroup {
			state.OwnGroups[i].Multiplier = target
			break
		}
	}
	_ = s.repository.Save(ctx, *state)

	result.Status = "applied"

	// 自动调价成功后发送通知（仅在开启通知且配置了机器人时）
	if mapping.EnableAutoPricingNotify && len(mapping.AutoPricingNotifyBotIDs) > 0 && s.botNotifier != nil {
		msg := formatAutoPricingNotify(mapping, siteName, result, oldOwnMultiplier)
		s.botNotifier.SendToBots(ctx, userID, mapping.AutoPricingNotifyBotIDs, msg)
	}

	return result
}

// logAutoPricingResult 记录自动调价执行结果日志。
func logAutoPricingResult(siteName string, result autoPricingResult) {
	switch result.Status {
	case "applied":
		log.Printf("[auto-pricing] 已更新倍率 site=%s own_group=%s old_ref=%.4f new_ref=%.4f target=%.4f",
			siteName, result.OwnGroup, result.OldReference, result.NewReference, result.TargetMultiplier)
	case "threshold_exceeded":
		log.Printf("[auto-pricing] 阈值超限跳过 site=%s own_group=%s old_ref=%.4f new_ref=%.4f reason=%s",
			siteName, result.OwnGroup, result.OldReference, result.NewReference, result.Reason)
	case "skipped":
		log.Printf("[auto-pricing] 跳过 site=%s own_group=%s reason=%s",
			siteName, result.OwnGroup, result.Reason)
	case "failed":
		log.Printf("[auto-pricing] 执行失败 site=%s own_group=%s target=%.4f reason=%s",
			siteName, result.OwnGroup, result.TargetMultiplier, result.Reason)
	}
}

// filterEmptyStrings 过滤切片中的空字符串，保持输入顺序。
func filterEmptyStrings(ss []string) []string {
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// defaultAutoPricingNotifyTemplate 自动调价成功通知的默认模板。
const defaultAutoPricingNotifyTemplate = "【自动调价】{ownGroup} 已自动从 {oldOwnMultiplier}x 调整为 {newOwnMultiplier}x。参考来源：{upstreamSiteName} / {upstreamGroupName}，参考倍率 {oldReference}x -> {newReference}x。"

// autoPricingSourceLabel 返回 AutoPricingSource 的可读说明，用于通知模板中 {upstreamGroupName} 变量。
func autoPricingSourceLabel(source string) string {
	switch source {
	case "lowest_upstream":
		return "最低倍率上游"
	case "highest_upstream":
		return "最高倍率上游"
	case "average_upstream":
		return "平均倍率"
	default:
		return ""
	}
}

// formatAutoPricingNotify 格式化自动调价成功通知消息。
// mapping 提供模板和策略配置，siteName 为触发同步的上游站点名，
// result 提供参考倍率和目标倍率，oldOwnMultiplier 为调整前的自有分组倍率。
func formatAutoPricingNotify(mapping GroupMapping, siteName string, result autoPricingResult, oldOwnMultiplier *float64) string {
	tpl := mapping.AutoPricingNotifyTemplate
	if tpl == "" {
		tpl = defaultAutoPricingNotifyTemplate
	}

	oldOwnStr := "-"
	if oldOwnMultiplier != nil {
		oldOwnStr = fmt.Sprintf("%.4f", *oldOwnMultiplier)
	}

	// {upstreamGroupName}：主上游模式用主上游分组名，聚合模式用可读来源说明
	upstreamGroupName := mapping.PrimaryUpstreamGroupName
	if mapping.AutoPricingSource != "primary_upstream" {
		label := autoPricingSourceLabel(mapping.AutoPricingSource)
		if label != "" {
			upstreamGroupName = label
		}
	}

	// {strategy} 可读策略说明
	strategyStr := "percentage"
	if mapping.AutoPricingStrategy == "fixed" {
		strategyStr = "fixed"
	}

	r := strings.NewReplacer(
		"{ownGroup}", mapping.OwnGroup,
		"{upstreamSiteName}", siteName,
		"{upstreamGroupName}", upstreamGroupName,
		"{oldReference}", fmt.Sprintf("%.4f", result.OldReference),
		"{newReference}", fmt.Sprintf("%.4f", result.NewReference),
		"{oldOwnMultiplier}", oldOwnStr,
		"{newOwnMultiplier}", fmt.Sprintf("%.4f", result.TargetMultiplier),
		"{strategy}", strategyStr,
		"{fixedIncrease}", fmt.Sprintf("%.4f", mapping.FixedIncrease),
		"{percentageIncrease}", fmt.Sprintf("%.2f", mapping.PercentageIncrease),
		"{threshold}", fmt.Sprintf("%.2f", mapping.AdjustThresholdPercent),
	)
	return r.Replace(tpl)
}

type requestError string

func (e requestError) Error() string { return string(e) }
