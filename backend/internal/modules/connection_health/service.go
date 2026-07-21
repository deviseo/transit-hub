package connection_health

import (
	"context"
	"log"
	"strings"
	"time"

	"transithub/backend/internal/modules/my_sites"
)

// healthRepository 是 Service 对存储层的全部依赖，由 *Repository 结构性满足。
// 定义为接口而不是直接依赖 *Repository 具体类型，使聚合、策略、手动动作等核心流程
// 可以在不连接真实数据库的情况下用内存假实现单测覆盖（同 group_rate_campaigns 的做法）。
type healthRepository interface {
	ListPolicies(ctx context.Context, userID string, adminAccountID string) ([]Policy, error)
	GetPolicy(ctx context.Context, id string, userID string, adminAccountID string) (*Policy, error)
	SavePolicyWithTargets(ctx context.Context, p Policy, targets []ModelTarget) error
	ListStatesByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]ConnectionHealthState, error)
	ListStatesByConnection(ctx context.Context, connectionID string) ([]ConnectionHealthState, error)
	GetState(ctx context.Context, connectionID string, modelName string) (*ConnectionHealthState, error)
	UpsertState(ctx context.Context, s ConnectionHealthState) error
	InsertEvent(ctx context.Context, e ConnectionHealthEvent) error
	ListEventsByConnection(ctx context.Context, connectionID string, userID string, adminAccountID string, limit int) ([]ConnectionHealthEvent, error)
	ListRecentEventsByWorkspace(ctx context.Context, userID string, adminAccountID string, limit int) ([]ConnectionHealthEvent, error)
	CountProbesToday(ctx context.Context, userID string, adminAccountID string, policyID string, dayStart time.Time) (int, error)
	TryConsumeProbeBudget(ctx context.Context, userID string, adminAccountID string, policyID string, dayStart time.Time, limit int) (bool, error)
	TryAcquireSchedulerLease(ctx context.Context) (release func(), acquired bool, err error)
	AcquireTargetLease(ctx context.Context, targetID string) (release func(), err error)
	ListEnabledPolicies(ctx context.Context) ([]Policy, error)
	ReplacePolicyAssignments(ctx context.Context, userID string, adminAccountID string, targetID string, policyIDs []string) error
	ListPolicyAssignmentsForTarget(ctx context.Context, userID string, adminAccountID string, targetID string) ([]PolicyAssignment, error)
	ListPolicyAssignmentsByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]PolicyAssignment, error)
	ListAllPolicyAssignments(ctx context.Context) ([]PolicyAssignment, error)
	ReplaceGroupPolicyConfiguration(ctx context.Context, userID string, adminAccountID string, adminGroupID string, adminGroupName string, policyIDs []string, excludedTargetIDs []string, groupTargetIDs []string) error
	CreatePolicyAndReplaceGroupConfiguration(ctx context.Context, policy Policy, targets []ModelTarget, adminGroupID string, adminGroupName string, policyIDs []string, excludedTargetIDs []string, groupTargetIDs []string) error
	ListGroupPolicyAssignmentsByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]GroupPolicyAssignment, error)
	ListAllGroupPolicyAssignments(ctx context.Context) ([]GroupPolicyAssignment, error)
	ListGroupTargetExclusionsByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]GroupTargetExclusion, error)
	ListAllGroupTargetExclusions(ctx context.Context) ([]GroupTargetExclusion, error)
	ListPrioritySyncStates(ctx context.Context, userID string, adminAccountID string) ([]PrioritySyncState, error)
	ListAllPrioritySyncStates(ctx context.Context) ([]PrioritySyncState, error)
	UpsertPrioritySyncState(ctx context.Context, state PrioritySyncState) error
	DeletePrioritySyncState(ctx context.Context, userID string, adminAccountID string, targetID string) error
	GetTargetActionState(ctx context.Context, userID string, adminAccountID string, targetID string) (*TargetActionState, error)
	ListAllTargetActionStates(ctx context.Context) ([]TargetActionState, error)
	UpsertTargetActionState(ctx context.Context, state TargetActionState) error
	DeleteTargetActionState(ctx context.Context, userID string, adminAccountID string, targetID string) error
	EnsureSchema(ctx context.Context) error
}

// Service 组装 connection_health 模块的全部业务逻辑：聚合查询、策略管理、手动动作、
// 真实探活执行。所有对外可见字段都不含 upstream_key，符合任务书的敏感信息约束。
type Service struct {
	repo            healthRepository
	mySites         MySitesReader
	sites           SiteLookup
	accounts        AdminAccountResolver
	dispatcher      RemoteActionRunner
	probeRunner     *RealProbeRunner
	modelDiscovery  *ModelDiscoveryRunner
	platformGroups  PlatformGroupReader
	priorityActions TargetPriorityActioner
}

func NewService(repo *Repository, mySites MySitesReader, sites SiteLookup, platform PlatformActioner) *Service {
	service := &Service{
		repo:           repo,
		mySites:        mySites,
		sites:          sites,
		dispatcher:     newRemoteActionDispatcher(sites, mySites, platform),
		probeRunner:    NewRealProbeRunner(),
		modelDiscovery: NewModelDiscoveryRunner(),
	}
	// 真实 PlatformService 同时实现优先级更新能力；测试或旧注入器如果尚未实现，倍率策略会
	// 安全跳过远端写入，不影响既有探活/降级流程。
	if actions, ok := platform.(TargetPriorityActioner); ok {
		service.priorityActions = actions
	}
	return service
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	return s.repo.EnsureSchema(ctx)
}

func (s *Service) SetAdminAccountResolver(accounts AdminAccountResolver) {
	s.accounts = accounts
}

func (s *Service) currentAdminAccountID(ctx context.Context, userID string) (string, error) {
	if s.accounts == nil {
		return "", requestError(ErrorNoCurrentAccount)
	}
	return s.accounts.RequireCurrentID(ctx, userID)
}

// ModelHealth 是单个模型在某条对接链路上的健康状态展示数据，绝不包含 upstream_key。
type ModelHealth struct {
	ModelName            string     `json:"modelName"`
	ProviderFamily       string     `json:"providerFamily"`
	Configured           bool       `json:"configured"`
	State                State      `json:"state"`
	CurrentWeight        int        `json:"currentWeight"`
	ConsecutiveFailures  int        `json:"consecutiveFailures"`
	ConsecutiveSuccesses int        `json:"consecutiveSuccesses"`
	LastProbeAt          *time.Time `json:"lastProbeAt"`
	LastSuccessAt        *time.Time `json:"lastSuccessAt"`
	LastFailureAt        *time.Time `json:"lastFailureAt"`
	LastLatencyMs        *int       `json:"lastLatencyMs"`
	LastErrorKey         string     `json:"lastErrorKey"`
	LastErrorDetail      string     `json:"lastErrorDetail"`
	LastRemoteAction     string     `json:"lastRemoteAction"`
	UpdatedAt            *time.Time `json:"updatedAt"`
}

// ConnectionHealth 是一条已对接上游分组链路的健康展示数据。UpstreamKeyID 只保留 ID 辅助排障，
// UpstreamKey 明文绝不出现在任何响应字段里。
type ConnectionHealth struct {
	ConnectionID      string        `json:"connectionId"`
	UpstreamSiteID    string        `json:"upstreamSiteId"`
	UpstreamGroupID   string        `json:"upstreamGroupId"`
	UpstreamGroupName string        `json:"upstreamGroupName"`
	UpstreamKeyID     string        `json:"upstreamKeyId"`
	GroupType         string        `json:"groupType"`
	Models            []ModelHealth `json:"models"`
}

// OwnGroupHealth 是「我的分组」维度的聚合：没有真实对接记录时 HasConnections=false，
// 前端应展示「尚未对接」，不进入探活大屏的健康统计。
type OwnGroupHealth struct {
	OwnGroupID     string             `json:"ownGroupId"`
	OwnGroupName   string             `json:"ownGroupName"`
	HasConnections bool               `json:"hasConnections"`
	Connections    []ConnectionHealth `json:"connections"`
}

// EventView 是事件的对外展示形态，字段命名与前端 camelCase 对齐。
type EventView struct {
	ID                string    `json:"id"`
	ConnectionID      string    `json:"connectionId"`
	ModelName         string    `json:"modelName"`
	OwnGroupName      string    `json:"ownGroupName"`
	UpstreamSiteID    string    `json:"upstreamSiteId"`
	UpstreamGroupName string    `json:"upstreamGroupName"`
	Result            string    `json:"result"`
	FromState         string    `json:"fromState"`
	ToState           string    `json:"toState"`
	LatencyMs         *int      `json:"latencyMs"`
	ErrorKey          string    `json:"errorKey"`
	RemoteAction      string    `json:"remoteAction"`
	CreatedAt         time.Time `json:"createdAt"`
}

// OverviewResponse 是大屏顶部汇总卡片的数据。
type OverviewResponse struct {
	TotalConnections int         `json:"totalConnections"`
	Healthy          int         `json:"healthy"`
	Degraded         int         `json:"degraded"`
	Suspended        int         `json:"suspended"`
	Observing        int         `json:"observing"`
	Recovering       int         `json:"recovering"`
	Disabled         int         `json:"disabled"`
	Unconfigured     int         `json:"unconfigured"`
	RecentEvents     []EventView `json:"recentEvents"`
}

// Groups 按「我的分组 -> 对接链路 -> 模型」聚合当前 workspace 的健康状态。
// 数据源为 real_connections（通过 my_sites 只读接口）+ 本模块的健康状态表，
// 不新增任何手动配置的探活目标数据源。
func (s *Service) Groups(ctx context.Context, userID string) ([]OwnGroupHealth, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return nil, err
	}

	connections, err := s.mySites.ListRealConnections(ctx, userID)
	if err != nil {
		return nil, err
	}
	mappingOptions, err := s.mySites.MappingOptions(ctx, userID)
	if err != nil {
		return nil, err
	}
	states, err := s.repo.ListStatesByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}

	stateIndex := make(map[string]map[string]ConnectionHealthState, len(states))
	for _, st := range states {
		byModel, ok := stateIndex[st.ConnectionID]
		if !ok {
			byModel = make(map[string]ConnectionHealthState)
			stateIndex[st.ConnectionID] = byModel
		}
		byModel[st.ModelName] = st
	}

	idToName := make(map[string]string, len(mappingOptions.OwnGroups))
	order := make([]string, 0, len(mappingOptions.OwnGroups))
	for _, g := range mappingOptions.OwnGroups {
		idToName[g.ID] = g.GroupName
		order = append(order, g.ID)
	}

	groups := make(map[string]*OwnGroupHealth, len(order))
	for _, id := range order {
		groups[id] = &OwnGroupHealth{OwnGroupID: id, OwnGroupName: idToName[id], HasConnections: false, Connections: []ConnectionHealth{}}
	}

	for _, conn := range connections {
		modelsByModelName := stateIndex[conn.ID]
		models := make([]ModelHealth, 0, len(modelsByModelName))
		for modelName, st := range modelsByModelName {
			models = append(models, toModelHealth(modelName, st))
		}
		ch := ConnectionHealth{
			ConnectionID:      conn.ID,
			UpstreamSiteID:    conn.UpstreamSiteID,
			UpstreamGroupID:   conn.UpstreamGroupID,
			UpstreamGroupName: conn.UpstreamGroupName,
			UpstreamKeyID:     conn.UpstreamKeyID,
			GroupType:         conn.GroupType,
			Models:            models,
		}

		if len(conn.OwnGroupIDs) == 0 {
			continue
		}
		for _, ownGroupID := range conn.OwnGroupIDs {
			group, ok := groups[ownGroupID]
			if !ok {
				group = &OwnGroupHealth{OwnGroupID: ownGroupID, OwnGroupName: idToName[ownGroupID], Connections: []ConnectionHealth{}}
				groups[ownGroupID] = group
				order = append(order, ownGroupID)
			}
			group.HasConnections = true
			group.Connections = append(group.Connections, ch)
		}
	}

	result := make([]OwnGroupHealth, 0, len(order))
	seen := make(map[string]struct{}, len(order))
	for _, id := range order {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, *groups[id])
	}
	return result, nil
}

func toModelHealth(modelName string, st ConnectionHealthState) ModelHealth {
	updatedAt := st.UpdatedAt
	return ModelHealth{
		ModelName:            modelName,
		Configured:           !isCredentialUnavailableReason(st.LastErrorKey),
		State:                st.State,
		CurrentWeight:        st.CurrentWeight,
		ConsecutiveFailures:  st.ConsecutiveFailures,
		ConsecutiveSuccesses: st.ConsecutiveSuccesses,
		LastProbeAt:          st.LastProbeAt,
		LastSuccessAt:        st.LastSuccessAt,
		LastFailureAt:        st.LastFailureAt,
		LastLatencyMs:        st.LastLatencyMs,
		LastErrorKey:         st.LastErrorKey,
		LastErrorDetail:      st.LastErrorDetail,
		LastRemoteAction:     st.LastRemoteAction,
		UpdatedAt:            &updatedAt,
	}
}

// Overview 汇总当前 admin workspace 下已纳入策略的账号/渠道，而不是继续统计旧
// real_connections。响应字段保持不变，旧客户端仍可读取；旧链路接口 /groups 继续保留。
func (s *Service) Overview(ctx context.Context, userID string) (OverviewResponse, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return OverviewResponse{}, err
	}

	// 兼容只注入旧依赖的调用方/测试：新平台读取器未配置时仍按 real_connections 汇总。
	// 正常 HTTP 服务已注入 platformGroups，因此生产页面使用下面的 admin 目标主语义。
	if s.platformGroups == nil {
		legacyGroups, err := s.Groups(ctx, userID)
		if err != nil {
			return OverviewResponse{}, err
		}
		resp := OverviewResponse{}
		for _, group := range legacyGroups {
			for _, connection := range group.Connections {
				resp.TotalConnections++
				if len(connection.Models) == 0 {
					resp.Unconfigured++
					continue
				}
				for _, model := range connection.Models {
					accumulateOverviewState(&resp, model.State)
				}
			}
		}
		events, err := s.repo.ListRecentEventsByWorkspace(ctx, userID, adminAccountID, 50)
		if err != nil {
			return OverviewResponse{}, err
		}
		resp.RecentEvents = toEventViews(events)
		return resp, nil
	}

	groups, err := s.AdminGroups(ctx, userID)
	if err != nil {
		return OverviewResponse{}, err
	}

	resp := OverviewResponse{}
	type overviewTarget struct {
		probeAvailable bool
		models         map[string]ModelHealth
		unprobed       map[string]struct{}
	}
	targets := make(map[string]*overviewTarget)
	for _, group := range groups {
		for _, account := range group.Accounts {
			if !account.HasEnabledPolicy {
				continue
			}
			target := targets[account.TargetID]
			if target == nil {
				target = &overviewTarget{
					probeAvailable: true,
					models:         make(map[string]ModelHealth),
					unprobed:       make(map[string]struct{}),
				}
				targets[account.TargetID] = target
			}
			target.probeAvailable = target.probeAvailable && account.ProbeAvailable
			for _, model := range account.ModelHealth {
				target.models[model.ModelName] = model
				delete(target.unprobed, model.ModelName)
			}
			for _, model := range account.UnprobedModels {
				if _, hasState := target.models[model.ModelName]; !hasState {
					target.unprobed[model.ModelName] = struct{}{}
				}
			}
		}
	}
	for _, target := range targets {
		resp.TotalConnections++
		if !target.probeAvailable {
			resp.Unconfigured++
			continue
		}
		if len(target.models) == 0 && len(target.unprobed) == 0 {
			resp.Unconfigured++
			continue
		}
		resp.Unconfigured += len(target.unprobed)
		for _, model := range target.models {
			if isCredentialUnavailableReason(model.LastErrorKey) {
				resp.Unconfigured++
				continue
			}
			accumulateOverviewState(&resp, model.State)
		}
	}

	// 先拉取较宽窗口再过滤，避免已解绑目标的高频历史事件占满 LIMIT，导致当前有效事件为空。
	events, err := s.repo.ListRecentEventsByWorkspace(ctx, userID, adminAccountID, 500)
	if err != nil {
		return OverviewResponse{}, err
	}
	events, err = s.filterToAssignedTargetEvents(ctx, userID, adminAccountID, events)
	if err != nil {
		return OverviewResponse{}, err
	}
	if len(events) > 50 {
		events = events[:50]
	}
	resp.RecentEvents = toEventViews(events)
	return resp, nil
}

func accumulateOverviewState(resp *OverviewResponse, state State) {
	switch state {
	case StateHealthy:
		resp.Healthy++
	case StateDegraded:
		resp.Degraded++
	case StateSuspended:
		resp.Suspended++
	case StateObserving:
		resp.Observing++
	case StateRecovering:
		resp.Recovering++
	case StateDisabled:
		resp.Disabled++
	}
}

// filterToAssignedTargetEvents 过滤掉「admin target 维度但该 target 当前没有被分配任何策略」的
// 事件行：全局/大屏「探活事件」只展示已分配策略的 target 的策略探活事件。
// 只对 ConnectionID 能解析成 targetId 形态（parseTargetID 成功）的行生效——旧
// real_connections 事件的 connection_id 是 UUID，解析不出 targetId 结构，原样保留，不受影响。
// 历史上曾经存在过、后来取消分配的 target 事件会被这里过滤掉，但不会删库数据。
func (s *Service) filterToAssignedTargetEvents(ctx context.Context, userID string, adminAccountID string, events []ConnectionHealthEvent) ([]ConnectionHealthEvent, error) {
	assignments, err := s.repo.ListPolicyAssignmentsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	assignedTargets := make(map[string]struct{}, len(assignments))
	for _, a := range assignments {
		assignedTargets[a.TargetID] = struct{}{}
	}
	groupAssignments, err := s.repo.ListGroupPolicyAssignmentsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	groupExclusions, err := s.repo.ListGroupTargetExclusionsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	groupIDByName := make(map[string][]string)
	groupIDsByPolicy := make(map[string][]string)
	assignedGroupIDs := make(map[string]struct{})
	for _, assignment := range groupAssignments {
		groupIDByName[assignment.AdminGroupName] = append(groupIDByName[assignment.AdminGroupName], assignment.AdminGroupID)
		groupIDsByPolicy[assignment.PolicyID] = append(groupIDsByPolicy[assignment.PolicyID], assignment.AdminGroupID)
		assignedGroupIDs[assignment.AdminGroupID] = struct{}{}
	}
	excluded := make(map[string]map[string]bool)
	for _, exclusion := range groupExclusions {
		if excluded[exclusion.AdminGroupID] == nil {
			excluded[exclusion.AdminGroupID] = make(map[string]bool)
		}
		excluded[exclusion.AdminGroupID][exclusion.TargetID] = true
	}

	out := make([]ConnectionHealthEvent, 0, len(events))
	for _, e := range events {
		if _, ok := parseTargetID(e.ConnectionID); ok {
			if e.Result == "policy_unmanaged_restore" {
				// This event is emitted after the final assignment is removed. Filtering it by
				// current assignments would make the automatic upstream restore impossible to audit.
				out = append(out, e)
				continue
			}
			_, explicitlyAssigned := assignedTargets[e.ConnectionID]
			groupAssigned := false
			candidateGroupIDs := groupIDByName[e.OwnGroupName]
			if e.AdminGroupID != "" {
				if _, stillAssigned := assignedGroupIDs[e.AdminGroupID]; stillAssigned {
					candidateGroupIDs = []string{e.AdminGroupID}
				} else if e.PolicyID != "" {
					// Older scheduler versions could stamp the target's first group even when
					// this event's policy came from another group. Fall back to the persisted
					// policy ID so those historical events remain visible after an unrelated
					// group is unbound.
					candidateGroupIDs = groupIDsByPolicy[e.PolicyID]
				} else {
					candidateGroupIDs = nil
				}
			}
			for _, groupID := range candidateGroupIDs {
				// 同名分组可能同时包含一个目标；只要其中一个已绑定分组没有排除该目标，
				// 事件就仍属于有效监控范围，不能被另一个同名分组的排除项误过滤。
				if !excluded[groupID][e.ConnectionID] {
					groupAssigned = true
					break
				}
			}
			if !explicitlyAssigned && !groupAssigned {
				continue
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// Events 返回指定连接（或 workspace 全量）最近的探活/远端动作事件。
// Events 查询探活/远端动作事件。传入 connectionId 时必须先确认该连接属于当前用户当前
// workspace，避免同一登录用户猜测其他 workspace 的 connection_id 越权读取事件（IDOR）。
func (s *Service) Events(ctx context.Context, userID string, connectionID string, limit int) ([]EventView, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return nil, err
	}
	var events []ConnectionHealthEvent
	if strings.TrimSpace(connectionID) != "" {
		conn, findErr := s.findConnection(ctx, userID, connectionID)
		if findErr != nil {
			return nil, findErr
		}
		if conn == nil {
			// 不是当前 workspace 的 real_connection：再判断是否是当前 workspace 的独立探活 targetId。
			// targetId 内嵌 workspaceAdminAccountID，归属当前 workspace 时按 targetId 查询独立探活事件；
			// 否则一律返回空列表，不泄露归属信息（防 IDOR）。
			if parsed, ok := parseTargetID(connectionID); ok && parsed.adminAccountID == adminAccountID {
				events, err = s.repo.ListEventsByConnection(ctx, connectionID, userID, adminAccountID, limit)
				if err != nil {
					return nil, err
				}
				// 聚焦查看某个 target 的事件：该 target 没有分配任何策略时，filterToAssignedTargetEvents
				// 会把这些行全部过滤掉，天然返回空数组，前端展示「暂无策略探活事件」。
				events, err = s.filterToAssignedTargetEvents(ctx, userID, adminAccountID, events)
				if err != nil {
					return nil, err
				}
				return toEventViews(events), nil
			}
			return []EventView{}, nil
		}
		events, err = s.repo.ListEventsByConnection(ctx, connectionID, userID, adminAccountID, limit)
	} else {
		events, err = s.repo.ListRecentEventsByWorkspace(ctx, userID, adminAccountID, limit)
	}
	if err != nil {
		return nil, err
	}
	// real_connections 分支（connectionID 是真实连接的 UUID）和全局分支都过滤一遍：
	// filterToAssignedTargetEvents 只对能解析成 targetId 结构的行生效，UUID 形态的
	// real_connection 事件不受影响，保持旧行为。
	events, err = s.filterToAssignedTargetEvents(ctx, userID, adminAccountID, events)
	if err != nil {
		return nil, err
	}
	return toEventViews(events), nil
}

func toEventViews(events []ConnectionHealthEvent) []EventView {
	views := make([]EventView, 0, len(events))
	for _, e := range events {
		views = append(views, EventView{
			ID: e.ID, ConnectionID: e.ConnectionID, ModelName: e.ModelName, OwnGroupName: e.OwnGroupName,
			UpstreamSiteID: e.UpstreamSiteID, UpstreamGroupName: e.UpstreamGroupName, Result: e.Result,
			FromState: e.FromState, ToState: e.ToState, LatencyMs: e.LatencyMs, ErrorKey: e.ErrorKey,
			RemoteAction: e.RemoteAction, CreatedAt: e.CreatedAt,
		})
	}
	return views
}

// ModelTargetInput / PolicyInput 是保存策略接口的请求体，米字段与 connection_health_policies /
// connection_health_model_targets 表一一对应。
type ModelTargetInput struct {
	ID             string `json:"id"`
	ModelName      string `json:"modelName"`
	ProviderFamily string `json:"providerFamily"`
	Enabled        bool   `json:"enabled"`
	ProbePrompt    string `json:"probePrompt"`
	MaxProbeTokens int    `json:"maxProbeTokens"`
}

type PolicyInput struct {
	ID                      string             `json:"id"`
	Name                    string             `json:"name"`
	Enabled                 bool               `json:"enabled"`
	OwnGroupID              string             `json:"ownGroupId"`
	OwnGroupName            string             `json:"ownGroupName"`
	ModelPattern            string             `json:"modelPattern"`
	ProbeIntervalSeconds    int                `json:"probeIntervalSeconds"`
	FailureThreshold        int                `json:"failureThreshold"`
	SuccessThreshold        int                `json:"successThreshold"`
	CooldownSeconds         int                `json:"cooldownSeconds"`
	ObservationSeconds      int                `json:"observationSeconds"`
	RecoveryStepPercent     int                `json:"recoveryStepPercent"`
	AutoDegradeEnabled      bool               `json:"autoDegradeEnabled"`
	AutoRemoteActionEnabled bool               `json:"autoRemoteActionEnabled"`
	PriorityMode            string             `json:"priorityMode"`
	DailyProbeBudget        int                `json:"dailyProbeBudget"`
	ModelTargets            []ModelTargetInput `json:"modelTargets"`
}

func (s *Service) ListPolicies(ctx context.Context, userID string) ([]Policy, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListPolicies(ctx, userID, adminAccountID)
}

// SavePolicy 创建或更新一条策略（含 model targets 整体替换）。id 为空时创建新策略。
func (s *Service) SavePolicy(ctx context.Context, userID string, in PolicyInput) (Policy, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return Policy{}, err
	}

	id := strings.TrimSpace(in.ID)
	if id == "" {
		generated, genErr := newID()
		if genErr != nil {
			return Policy{}, genErr
		}
		id = generated
	} else {
		existing, getErr := s.repo.GetPolicy(ctx, id, userID, adminAccountID)
		if getErr != nil {
			return Policy{}, getErr
		}
		if existing == nil {
			return Policy{}, requestError(ErrorNotFound)
		}
	}

	policy, targets, err := buildPolicyAndTargets(userID, adminAccountID, id, in)
	if err != nil {
		return Policy{}, err
	}
	if err := s.repo.SavePolicyWithTargets(ctx, policy, targets); err != nil {
		return Policy{}, err
	}

	saved, err := s.repo.GetPolicy(ctx, id, userID, adminAccountID)
	if err != nil {
		return Policy{}, err
	}
	if saved == nil {
		return Policy{}, requestError(ErrorNotFound)
	}
	return *saved, nil
}

func buildPolicyAndTargets(userID string, adminAccountID string, id string, in PolicyInput) (Policy, []ModelTarget, error) {
	policy := Policy{
		ID: id, UserID: userID, AdminAccountID: adminAccountID, Name: strings.TrimSpace(in.Name), Enabled: in.Enabled,
		OwnGroupID: in.OwnGroupID, OwnGroupName: in.OwnGroupName, ModelPattern: defaultString(in.ModelPattern, "*"),
		ProbeMode: "real_model", ProbeIntervalSeconds: defaultInt(in.ProbeIntervalSeconds, 60),
		FailureThreshold: defaultInt(in.FailureThreshold, 3), SuccessThreshold: defaultInt(in.SuccessThreshold, 2),
		CooldownSeconds: defaultInt(in.CooldownSeconds, 300), ObservationSeconds: defaultInt(in.ObservationSeconds, 300),
		RecoveryStepPercent: defaultInt(in.RecoveryStepPercent, 25), AutoDegradeEnabled: in.AutoDegradeEnabled,
		AutoRemoteActionEnabled: in.AutoDegradeEnabled && in.AutoRemoteActionEnabled, PriorityMode: normalizePriorityMode(in.PriorityMode),
		DailyProbeBudget: defaultInt(in.DailyProbeBudget, 1000),
	}
	targets := make([]ModelTarget, 0, len(in.ModelTargets))
	for _, t := range in.ModelTargets {
		targetID := strings.TrimSpace(t.ID)
		if targetID == "" {
			generated, genErr := newID()
			if genErr != nil {
				return Policy{}, nil, genErr
			}
			targetID = generated
		}
		targets = append(targets, ModelTarget{
			ID: targetID, PolicyID: id, UserID: userID, AdminAccountID: adminAccountID,
			ModelName: strings.TrimSpace(t.ModelName), ProviderFamily: t.ProviderFamily, Enabled: t.Enabled,
			ProbePrompt: t.ProbePrompt, MaxProbeTokens: defaultInt(t.MaxProbeTokens, 1),
		})
	}
	policy.ModelTargets = targets
	return policy, targets, nil
}

func normalizePriorityMode(mode string) string {
	if strings.TrimSpace(mode) == PriorityModeMultiplier {
		return PriorityModeMultiplier
	}
	return PriorityModeNone
}

// policyRemoteActionEnabled 是自动接管上游状态的统一有效判定。自动降级关闭时状态机不会
// 产生可靠的降级/恢复决策，因此单独开启远端动作属于无效组合，保存和运行时都按关闭处理。
func policyRemoteActionEnabled(policy Policy) bool {
	return policy.AutoDegradeEnabled && policy.AutoRemoteActionEnabled
}

// ProbeConnectionInput 是手动探活接口的可选请求体。Models 为空（或请求体整体缺省）时
// 保持旧行为：探活该连接匹配到的全部启用模型目标。Models 非空时只探活其中命中匹配目标
// 的模型名，不允许探活策略之外的模型（绕过策略配置）。
type ProbeConnectionInput struct {
	Models []string `json:"models"`
}

// ProbeConnection 手动触发一次真实探活：对该连接匹配到的全部（或 input.Models 指定的）
// 启用策略/模型目标逐一探活，立即执行，不受 60s 调度间隔和失败退避限制，但仍计入每日探活预算。
func (s *Service) ProbeConnection(ctx context.Context, userID string, connectionID string, input ProbeConnectionInput) ([]ModelHealth, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return nil, err
	}
	conn, err := s.findConnection(ctx, userID, connectionID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, requestError(ErrorNotFound)
	}

	policies, err := s.repo.ListPolicies(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	targets := matchingModelTargets(policies, *conn)

	requestedModels := make([]string, 0, len(input.Models))
	for _, m := range input.Models {
		if trimmed := strings.TrimSpace(m); trimmed != "" {
			requestedModels = append(requestedModels, trimmed)
		}
	}
	if len(requestedModels) > 0 {
		wanted := make(map[string]struct{}, len(requestedModels))
		for _, m := range requestedModels {
			wanted[m] = struct{}{}
		}
		filtered := make([]policyModelTarget, 0, len(targets))
		for _, mt := range targets {
			if _, ok := wanted[mt.target.ModelName]; ok {
				filtered = append(filtered, mt)
			}
		}
		if len(filtered) == 0 {
			// 指定的模型全部未命中当前连接匹配到的启用策略/模型目标：明确拒绝，不静默退化
			// 成"探活全部"或返回可能被误读为"探活完成但为空"的 200 空数组。
			return nil, requestError(ErrorNoMatchingModels)
		}
		targets = filtered
	}

	if len(targets) == 0 {
		return []ModelHealth{}, nil
	}

	results := make([]ModelHealth, 0, len(targets))
	for _, mt := range targets {
		st, probeErr := s.probeOnce(ctx, *conn, mt.policy, mt.target)
		if probeErr != nil {
			log.Printf("[connection-health] manual probe failed connection_id=%s model=%s err=%v", connectionID, mt.target.ModelName, probeErr)
			continue
		}
		results = append(results, toModelHealth(mt.target.ModelName, *st))
	}
	return results, nil
}

// DisableConnection 人工禁用一条对接链路（所有已探活模型），写入事件，可选触发远端降级。
func (s *Service) DisableConnection(ctx context.Context, userID string, connectionID string) error {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return err
	}
	conn, err := s.findConnection(ctx, userID, connectionID)
	if err != nil {
		return err
	}
	if conn == nil {
		return requestError(ErrorNotFound)
	}

	states, err := s.repo.ListStatesByConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		states = []ConnectionHealthState{s.defaultState(*conn, "*")}
	}

	remoteAction := ""
	for i, st := range states {
		fromState := st.State
		st.State = StateDisabled
		st.CurrentWeight = 0
		st.UserID = userID
		st.AdminAccountID = adminAccountID
		if i == 0 {
			action, actionErr := s.dispatcher.Degrade(ctx, *conn, st)
			remoteAction = action
			if actionErr != nil {
				log.Printf("[connection-health] manual disable remote degrade failed connection_id=%s err=%v", connectionID, actionErr)
			}
		}
		st.LastRemoteAction = remoteAction
		if err := s.repo.UpsertState(ctx, st); err != nil {
			return err
		}
		s.recordEvent(ctx, *conn, "", st.ModelName, "manual_disable", string(fromState), string(StateDisabled), nil, "", "", remoteAction)
	}
	return nil
}

// RestoreConnection 人工恢复一条被禁用/暂停的对接链路，进入观察期，可选触发远端恢复。
func (s *Service) RestoreConnection(ctx context.Context, userID string, connectionID string) error {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return err
	}
	conn, err := s.findConnection(ctx, userID, connectionID)
	if err != nil {
		return err
	}
	if conn == nil {
		return requestError(ErrorNotFound)
	}

	states, err := s.repo.ListStatesByConnection(ctx, connectionID)
	if err != nil {
		return err
	}
	if len(states) == 0 {
		states = []ConnectionHealthState{s.defaultState(*conn, "*")}
	}

	remoteAction := ""
	for i, st := range states {
		fromState := st.State
		st.State = StateObserving
		observingUntil := time.Now().Add(5 * time.Minute)
		st.ObservingUntil = &observingUntil
		st.ConsecutiveFailures = 0
		st.ConsecutiveSuccesses = 0
		st.UserID = userID
		st.AdminAccountID = adminAccountID
		if i == 0 {
			action, actionErr := s.dispatcher.Restore(ctx, *conn, st)
			remoteAction = action
			if actionErr != nil {
				log.Printf("[connection-health] manual restore remote action failed connection_id=%s err=%v", connectionID, actionErr)
			}
		}
		st.LastRemoteAction = remoteAction
		if err := s.repo.UpsertState(ctx, st); err != nil {
			return err
		}
		s.recordEvent(ctx, *conn, "", st.ModelName, "manual_restore", string(fromState), string(StateObserving), nil, "", "", remoteAction)
	}
	return nil
}

// probeOnce 对一个 (connection, model) 组合执行一次真实探活、状态机决策、必要的远端动作，
// 并把结果落库 + 写事件。调度器和手动探活接口共用这一核心逻辑，保证行为一致。
// 每日探活预算耗尽时跳过真实请求，只保留当前状态（不写探活事件，不驱动状态机)。
func (s *Service) probeOnce(ctx context.Context, conn my_sites.RealConnection, policy Policy, target ModelTarget) (*ConnectionHealthState, error) {
	site, err := s.sites.GetSite(ctx, conn.UpstreamSiteID)
	if err != nil || site == nil {
		return nil, err
	}

	current, err := s.repo.GetState(ctx, conn.ID, target.ModelName)
	if err != nil {
		return nil, err
	}
	if current == nil {
		defaultState := s.defaultState(conn, target.ModelName)
		current = &defaultState
	}

	dayStart := probeBudgetDayStart(time.Now())
	allowed, err := s.repo.TryConsumeProbeBudget(ctx, policy.UserID, policy.AdminAccountID, policy.ID, dayStart, probeBudgetLimit(policy))
	if err != nil {
		return nil, err
	}
	if !allowed {
		return current, nil
	}

	outcome := s.probeRunner.Probe(ctx, ProbeRequest{
		BaseURL: site.BaseURL, UpstreamKey: conn.UpstreamKey, ProviderFamily: target.ProviderFamily,
		ModelName: target.ModelName, MaxTokens: target.MaxProbeTokens, ProbePrompt: target.ProbePrompt,
	})

	now := time.Now()
	transitionOut := Transition(TransitionInput{
		Current: current.State, CurrentWeight: current.CurrentWeight, ConsecutiveFailures: current.ConsecutiveFailures,
		ConsecutiveSuccesses: current.ConsecutiveSuccesses, ObservingUntil: current.ObservingUntil, Now: now,
		Result: outcome.Result, Policy: policy,
	})
	if !policy.AutoDegradeEnabled {
		// 自动降级关闭：只记录探活结果，状态机不推进，也不触发远端动作。
		transitionOut = TransitionOutput{
			NextState: current.State, Weight: current.CurrentWeight,
			ConsecutiveFailures: transitionOut.ConsecutiveFailures, ConsecutiveSuccesses: transitionOut.ConsecutiveSuccesses,
		}
	}

	next := *current
	next.State = transitionOut.NextState
	next.CurrentWeight = transitionOut.Weight
	next.ConsecutiveFailures = transitionOut.ConsecutiveFailures
	next.ConsecutiveSuccesses = transitionOut.ConsecutiveSuccesses
	next.CooldownUntil = transitionOut.CooldownUntil
	next.ObservingUntil = transitionOut.ObservingUntil
	next.LastProbeAt = &now
	latencyMs := outcome.LatencyMs
	next.LastLatencyMs = &latencyMs
	next.UserID = policy.UserID
	next.AdminAccountID = policy.AdminAccountID
	next.OwnGroupID = policy.OwnGroupID
	next.OwnGroupName = policy.OwnGroupName

	if outcome.Result == ResultOK {
		next.LastSuccessAt = &now
		next.LastErrorKey = ""
		next.LastErrorDetail = ""
	} else {
		next.LastFailureAt = &now
		next.LastErrorKey = string(outcome.Result)
		next.LastErrorDetail = outcome.Detail
	}

	remoteAction := ""
	if policy.AutoRemoteActionEnabled {
		if transitionOut.TriggerRemoteDegrade {
			action, actionErr := s.dispatcher.Degrade(ctx, conn, next)
			remoteAction = action
			if actionErr != nil {
				log.Printf("[connection-health] auto degrade failed connection_id=%s model=%s err=%v", conn.ID, target.ModelName, actionErr)
			}
		} else if transitionOut.TriggerRemoteRestore {
			action, actionErr := s.dispatcher.Restore(ctx, conn, next)
			remoteAction = action
			if actionErr != nil {
				log.Printf("[connection-health] auto restore failed connection_id=%s model=%s err=%v", conn.ID, target.ModelName, actionErr)
			}
		}
	}
	next.LastRemoteAction = remoteAction

	if err := s.repo.UpsertState(ctx, next); err != nil {
		return nil, err
	}
	s.recordEvent(ctx, conn, policy.ID, target.ModelName, string(outcome.Result), string(current.State), string(next.State), &latencyMs, next.LastErrorKey, next.LastErrorDetail, remoteAction)

	return &next, nil
}

func (s *Service) findConnection(ctx context.Context, userID string, connectionID string) (*my_sites.RealConnection, error) {
	connections, err := s.mySites.ListRealConnections(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, c := range connections {
		if c.ID == connectionID {
			return &c, nil
		}
	}
	return nil, nil
}

func (s *Service) defaultState(conn my_sites.RealConnection, modelName string) ConnectionHealthState {
	return ConnectionHealthState{
		ConnectionID: conn.ID, ModelName: modelName, UpstreamSiteID: conn.UpstreamSiteID,
		UpstreamGroupID: conn.UpstreamGroupID, UpstreamGroupName: conn.UpstreamGroupName,
		State: StateHealthy, CurrentWeight: 100,
	}
}

func (s *Service) recordEvent(ctx context.Context, conn my_sites.RealConnection, policyID string, modelName string, result string, fromState string, toState string, latencyMs *int, errorKey string, errorDetail string, remoteAction string) {
	id, err := newID()
	if err != nil {
		log.Printf("[connection-health] generate event id failed: %v", err)
		return
	}
	event := ConnectionHealthEvent{
		ID: id, ConnectionID: conn.ID, ModelName: modelName, UserID: conn.UserID, AdminAccountID: conn.WorkspaceAdminAccountID, PolicyID: policyID,
		UpstreamSiteID: conn.UpstreamSiteID, UpstreamGroupName: conn.UpstreamGroupName, Result: result,
		FromState: fromState, ToState: toState, LatencyMs: latencyMs, ErrorKey: errorKey, ErrorDetail: errorDetail, RemoteAction: remoteAction,
	}
	if err := s.repo.InsertEvent(ctx, event); err != nil {
		log.Printf("[connection-health] insert event failed connection_id=%s err=%v", conn.ID, err)
	}
}

type policyModelTarget struct {
	policy Policy
	target ModelTarget
}

// matchingModelTargets 返回一条连接匹配到的全部（已启用策略, 已启用模型目标）组合。
// own_group_id 为空的策略视为通配，匹配该 workspace 下全部已对接分组。
func matchingModelTargets(policies []Policy, conn my_sites.RealConnection) []policyModelTarget {
	ownGroupSet := make(map[string]struct{}, len(conn.OwnGroupIDs))
	for _, id := range conn.OwnGroupIDs {
		ownGroupSet[id] = struct{}{}
	}

	matches := make([]policyModelTarget, 0)
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		if p.OwnGroupID != "" {
			if _, ok := ownGroupSet[p.OwnGroupID]; !ok {
				continue
			}
		}
		for _, t := range p.ModelTargets {
			if !t.Enabled {
				continue
			}
			matches = append(matches, policyModelTarget{policy: p, target: t})
		}
	}
	return matches
}

func defaultString(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func defaultInt(v int, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}
