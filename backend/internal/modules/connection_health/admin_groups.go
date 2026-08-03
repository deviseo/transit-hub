package connection_health

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/upstream"
)

// AdminGroupHealth 是「当前 admin workspace 下的一个 admin 分组」在分组健康主列表中的展示单元。
// 探活体系已改为独立目标：分组下的账号(sub2api)/渠道(new-api)本身就是探活目标，不再依赖
// real_connections 对接链路。探活字段（probeAvailable / modelHealth 等）来自独立 admin 探活
// 状态（connection_health_states 中以 targetId 为键的行），不再从 real_connections 叠加。
type AdminGroupHealth struct {
	ID                    string                  `json:"id"`
	Name                  string                  `json:"name"`
	Platform              string                  `json:"platform"`
	Status                string                  `json:"status"`
	Type                  string                  `json:"type"` // public / exclusive / subscription
	IsExclusive           bool                    `json:"isExclusive"`
	SubscriptionType      string                  `json:"subscriptionType"`
	Multiplier            *float64                `json:"multiplier"`
	MultiplierDisplay     string                  `json:"multiplierDisplay"`
	AccountCount          int                     `json:"accountCount"`
	MonitoredAccountCount int                     `json:"monitoredAccountCount"`
	ExcludedAccountCount  int                     `json:"excludedAccountCount"`
	AssignedPolicyIDs     []string                `json:"assignedPolicyIds"`
	AssignedPolicies      []AssignedPolicySummary `json:"assignedPolicies"`
	HasAssignedPolicy     bool                    `json:"hasAssignedPolicy"`
	HasEnabledPolicy      bool                    `json:"hasEnabledPolicy"`
	HasEnabledProbePolicy bool                    `json:"hasEnabledProbePolicy"`
	PriorityMode          string                  `json:"priorityMode"`
	PriorityConflictCount int                     `json:"priorityConflictCount"`
	PriorityConflicts     []AdminPriorityConflict `json:"priorityConflicts,omitempty"`
	ProbeModelsConfigured bool                    `json:"probeModelsConfigured"`
	HealthSummary         AdminGroupHealthSummary `json:"healthSummary"`
	// AccountsError 非空时表示该分组的账号/渠道列表拉取失败（i18n key）；此时 accountCount=0、
	// accounts 为空，但主列表其余分组不受影响，不会整页崩溃。
	AccountsError string              `json:"accountsError,omitempty"`
	Accounts      []AdminGroupAccount `json:"accounts"`
}

// AdminGroupHealthSummary 是单个 admin 分组的探活健康概览，用于主列表快速展示。
// 独立探活语义下：ProbeableAccounts = 可探活账号数，UnprobeableAccounts = 不可探活账号数
// （缺密钥/缺 base_url/缺模型等）。
type AdminGroupHealthSummary struct {
	TotalAccounts       int        `json:"totalAccounts"`
	ProbeableAccounts   int        `json:"probeableAccounts"`
	UnprobeableAccounts int        `json:"unprobeableAccounts"`
	HealthyModels       int        `json:"healthyModels"`
	DegradedModels      int        `json:"degradedModels"`
	ObservingModels     int        `json:"observingModels"`
	RecoveringModels    int        `json:"recoveringModels"`
	SuspendedModels     int        `json:"suspendedModels"`
	DisabledModels      int        `json:"disabledModels"`
	PendingModels       int        `json:"pendingModels"`
	UnconfiguredModels  int        `json:"unconfiguredModels"`
	LastProbeAt         *time.Time `json:"lastProbeAt"`
}

// AdminGroupAccount 是 admin 分组下的一个账号(sub2api) / 渠道(new-api)，同时是一个独立探活目标。
// 只要后端能安全解析 base_url + key + model 就可独立探活，不再需要 real_connections。
// 绝不包含 key / token / cookie / credentials / secret / authorization 明文。
type AdminGroupAccount struct {
	ID                            string     `json:"id"`
	Name                          string     `json:"name"`
	Platform                      string     `json:"platform"`
	Type                          string     `json:"type"`
	Status                        string     `json:"status"`
	Schedulable                   *bool      `json:"schedulable,omitempty"`
	SchedulableSource             string     `json:"schedulableSource"`
	SchedulableChangedAt          *time.Time `json:"schedulableChangedAt,omitempty"`
	LastSchedulableAction         string     `json:"lastSchedulableAction,omitempty"`
	LastSchedulableActionAt       *time.Time `json:"lastSchedulableActionAt,omitempty"`
	LastSchedulableActionResult   string     `json:"lastSchedulableActionResult,omitempty"`
	LastSchedulableActionErrorKey string     `json:"lastSchedulableActionErrorKey,omitempty"`
	UpstreamStatusSource          string     `json:"upstreamStatusSource"`
	HealthStatusSource            string     `json:"healthStatusSource"`
	Priority                      *int       `json:"priority,omitempty"`
	Concurrency                   *int       `json:"concurrency,omitempty"`
	RateMultiplier                *float64   `json:"rateMultiplier,omitempty"`
	LoadFactor                    *int       `json:"loadFactor,omitempty"`
	Weight                        *int       `json:"weight,omitempty"`
	Models                        string     `json:"models,omitempty"`
	GroupIDs                      []string   `json:"groupIds,omitempty"`
	// UpstreamKeyGroup* 来自 real_connections 中该 admin 转发账号实际绑定的上游 API Key
	// 分组，再以站点缓存的 Groups 解析其当前倍率。无法可靠关联时保持空值，绝不使用
	// admin 转发账号自身的 rate_multiplier 猜测。
	UpstreamKeyGroupName       string   `json:"upstreamKeyGroupName,omitempty"`
	UpstreamKeyGroupMultiplier *float64 `json:"upstreamKeyGroupMultiplier,omitempty"`
	// 独立探活字段。
	TargetID               string        `json:"targetId"`
	ProbeAvailable         bool          `json:"probeAvailable"`
	ProbeUnavailableReason string        `json:"probeUnavailableReason,omitempty"`
	ModelHealth            []ModelHealth `json:"modelHealth"`
	// UnprobedModels is separate from ModelHealth so older clients never interpret a
	// synthetic default state as a successful health check.
	UnprobedModels []AdminGroupUnprobedModel `json:"unprobedModels,omitempty"`
	// 策略分配字段：与 ProbeAvailable 完全解耦——未分配策略的账号/渠道仍可手动一次性探活，
	// 只是不会被调度器自动探活、不会进策略探活事件列表。
	AssignedPolicyIDs       []string                `json:"assignedPolicyIds"`
	AssignedPolicies        []AssignedPolicySummary `json:"assignedPolicies"`
	HasAssignedPolicy       bool                    `json:"hasAssignedPolicy"`
	HasEnabledPolicy        bool                    `json:"hasEnabledPolicy"`
	HasEnabledProbePolicy   bool                    `json:"hasEnabledProbePolicy"`
	PolicyAssignmentSource  string                  `json:"policyAssignmentSource"`
	ExcludedFromGroupPolicy bool                    `json:"excludedFromGroupPolicy"`
	PriorityManaged         bool                    `json:"priorityManaged"`
	PriorityConflict        bool                    `json:"priorityConflict"`
	PriorityOriginal        *int                    `json:"priorityOriginal,omitempty"`
	PriorityExpected        *int                    `json:"priorityExpected,omitempty"`
	PriorityConflictValue   *int                    `json:"priorityConflictValue,omitempty"`
	PriorityConflictAt      *time.Time              `json:"priorityConflictAt,omitempty"`
	ProbeModelsConfigured   bool                    `json:"probeModelsConfigured"`
	EffectiveMultiplier     *float64                `json:"effectiveMultiplier,omitempty"`
}

type AdminPriorityConflict struct {
	TargetID         string     `json:"targetId"`
	AccountName      string     `json:"accountName"`
	CurrentPriority  *int       `json:"currentPriority,omitempty"`
	ExpectedPriority *int       `json:"expectedPriority,omitempty"`
	ConflictAt       *time.Time `json:"conflictAt,omitempty"`
}

type AdminGroupUnprobedModel struct {
	ModelName                string                       `json:"modelName"`
	ProviderFamily           string                       `json:"providerFamily"`
	NextProbeAt              *time.Time                   `json:"nextProbeAt,omitempty"`
	BlockedReason            string                       `json:"blockedReason,omitempty"`
	EffectiveIntervalSeconds int                          `json:"effectiveIntervalSeconds,omitempty"`
	EffectivePolicySources   []EffectiveProbePolicySource `json:"effectivePolicySources,omitempty"`
	BudgetPolicyID           string                       `json:"budgetPolicyId,omitempty"`
}

// SetPlatformGroupReader 注入平台中性的分组/账号读取与凭据解析能力（由 upstream.PlatformService 满足）。
func (s *Service) SetPlatformGroupReader(reader PlatformGroupReader) {
	s.platformGroups = reader
}

// AdminGroups 按「当前 admin workspace 下的 admin 全量分组 -> 分组下账号/渠道（独立探活目标）
// -> 独立探活状态叠加」聚合分组健康主列表。探活状态来自以 targetId 为键的独立探活状态行，
// 不依赖 real_connections。
func (s *Service) AdminGroups(ctx context.Context, userID string) ([]AdminGroupHealth, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if s.platformGroups == nil {
		return nil, errors.New("connection_health: platform group reader not configured")
	}

	session, err := s.mySites.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	platform := string(session.Platform)

	groups, err := s.platformGroups.FetchAdminAllGroups(session)
	if err != nil {
		return nil, err
	}
	policies, err := s.repo.ListPolicies(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	states, err := s.repo.ListStatesByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	assignments, err := s.repo.ListPolicyAssignmentsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	groupAssignments, err := s.repo.ListGroupPolicyAssignmentsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	groupExclusions, err := s.repo.ListGroupTargetExclusionsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	priorityStates, err := s.repo.ListPrioritySyncStates(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	latestProbeFailureEvents, err := s.repo.ListLatestProbeFailureEventsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	latestSchedulableEvents, err := s.repo.ListLatestSchedulableActionEventsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	latestSuccessfulSchedulableEvents, err := s.repo.ListLatestSuccessfulSchedulableActionEventsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	// assignmentsByTarget: targetId -> 该 target 已分配的全部策略行（不限已启用/禁用，
	// 展示层需要如实反映分配关系，是否生效由调度器按启用状态另行判断）。
	assignmentsByTarget := make(map[string][]PolicyAssignment, len(assignments))
	for _, a := range assignments {
		assignmentsByTarget[a.TargetID] = append(assignmentsByTarget[a.TargetID], a)
	}
	policyByID := make(map[string]Policy, len(policies))
	for _, p := range policies {
		policyByID[p.ID] = p
	}
	groupPolicyIDs := make(map[string][]string)
	for _, assignment := range groupAssignments {
		groupPolicyIDs[assignment.AdminGroupID] = append(groupPolicyIDs[assignment.AdminGroupID], assignment.PolicyID)
	}
	excludedByGroup := make(map[string]map[string]struct{})
	for _, exclusion := range groupExclusions {
		if excludedByGroup[exclusion.AdminGroupID] == nil {
			excludedByGroup[exclusion.AdminGroupID] = make(map[string]struct{})
		}
		excludedByGroup[exclusion.AdminGroupID][exclusion.TargetID] = struct{}{}
	}
	priorityByTarget := make(map[string]PrioritySyncState, len(priorityStates))
	for _, state := range priorityStates {
		priorityByTarget[state.TargetID] = state
	}
	latestProbeFailureEvent := latestProbeFailureEventsByTargetModel(latestProbeFailureEvents)
	latestSchedulableEvent := latestSchedulableEventsByTarget(latestSchedulableEvents)
	latestSuccessfulSchedulableEvent := latestSchedulableEventsByTarget(latestSuccessfulSchedulableEvents)
	budgetCounts := make(map[string]int)
	budgetLoaded := make(map[string]bool)
	now := time.Now().UTC()
	budgetDayStart := probeBudgetDayStart(now)
	// 真实上游 API Key 分组倍率仅用于展示，不参与探活或优先级计算。读取失败时降级为空，
	// 保证既有分组健康功能不会因为可选的倍率信息不可用而中断。
	upstreamKeyGroups := s.upstreamKeyGroupsByAdminAccount(ctx, userID, adminAccountID, platform)

	type groupAccountInventory struct {
		accounts []upstream.AdminGroupAccountInfo
		err      error
	}
	accountsByGroup := make(map[string]groupAccountInventory, len(groups))
	inheritedPolicyIDsByTarget := make(map[string][]string)
	decisionAccountByTarget := make(map[string]upstream.AdminGroupAccountInfo)
	for _, group := range groups {
		accounts, accErr := s.platformGroups.ListAdminGroupAccounts(session, group)
		accountsByGroup[group.ID] = groupAccountInventory{accounts: accounts, err: accErr}
		if accErr != nil {
			continue
		}
		for _, acc := range accounts {
			targetID := buildTargetID(platform, adminAccountID, acc.ID)
			if _, exists := decisionAccountByTarget[targetID]; !exists {
				decisionAccountByTarget[targetID] = acc
			}
			if exclusions := excludedByGroup[group.ID]; exclusions != nil {
				if _, excluded := exclusions[targetID]; excluded {
					continue
				}
			}
			inheritedPolicyIDsByTarget[targetID] = mergePolicyIDs(inheritedPolicyIDsByTarget[targetID], groupPolicyIDs[group.ID])
		}
	}

	// stateIndex[targetId][modelName] = 独立探活当前健康状态。旧的 real_connection 状态行
	// 也会出现在这里（connection_id 为 UUID），但不会与 targetId 命名空间碰撞，互不影响。
	stateIndex := make(map[string]map[string]ConnectionHealthState, len(states))
	for _, st := range states {
		byModel, ok := stateIndex[st.ConnectionID]
		if !ok {
			byModel = make(map[string]ConnectionHealthState)
			stateIndex[st.ConnectionID] = byModel
		}
		byModel[st.ModelName] = st
	}

	result := make([]AdminGroupHealth, 0, len(groups))
	for _, group := range groups {
		health := AdminGroupHealth{
			ID:                group.ID,
			Name:              group.Name,
			Platform:          group.Platform,
			Status:            group.Status,
			Type:              adminGroupType(group),
			IsExclusive:       group.IsExclusive,
			SubscriptionType:  group.SubscriptionType,
			Multiplier:        group.Multiplier,
			MultiplierDisplay: group.MultiplierDisplay,
			Accounts:          []AdminGroupAccount{},
			AssignedPolicyIDs: append([]string(nil), groupPolicyIDs[group.ID]...),
		}
		health.AssignedPolicyIDs, health.AssignedPolicies = assignedPolicySummariesFromIDs(health.AssignedPolicyIDs, policyByID)
		health.HasAssignedPolicy = len(health.AssignedPolicyIDs) > 0
		health.HasEnabledPolicy = hasEnabledAssignedPolicy(health.AssignedPolicies)
		health.HasEnabledProbePolicy = hasEnabledProbePolicyByIDs(health.AssignedPolicyIDs, policyByID)
		for _, policyID := range health.AssignedPolicyIDs {
			if policy, ok := policyByID[policyID]; ok && policy.Enabled && normalizePriorityMode(policy.PriorityMode) == PriorityModeMultiplier {
				health.PriorityMode = PriorityModeMultiplier
				break
			}
		}

		inventory := accountsByGroup[group.ID]
		accounts, accErr := inventory.accounts, inventory.err
		if accErr != nil {
			log.Printf("[connection-health] admin group accounts fetch failed group_id=%s group_name=%s err=%v", group.ID, group.Name, accErr)
			health.AccountsError = ErrorAccountsFetch
			result = append(result, health)
			continue
		}

		summary := AdminGroupHealthSummary{TotalAccounts: len(accounts)}
		for _, acc := range accounts {
			targetID := buildTargetID(platform, adminAccountID, acc.ID)
			upstreamKeyGroup := upstreamKeyGroups[strings.TrimSpace(acc.ID)]
			available, reason := targetManualProbeAvailability(platform, acc.BaseURL)
			excluded := false
			if exclusions := excludedByGroup[group.ID]; exclusions != nil {
				_, excluded = exclusions[targetID]
			}
			explicitIDs, _ := assignedPolicySummaries(assignmentsByTarget[targetID], policyByID)
			inheritedIDs := inheritedPolicyIDsByTarget[targetID]
			assignedIDs := mergePolicyIDs(explicitIDs, inheritedIDs)
			assignedIDs, assignedSummaries := assignedPolicySummariesFromIDs(assignedIDs, policyByID)
			effectivePolicies := make([]Policy, 0, len(assignedIDs))
			for _, policyID := range assignedIDs {
				if policy, ok := policyByID[policyID]; ok {
					effectivePolicies = append(effectivePolicies, policy)
				}
			}
			decisionAccount := decisionAccountByTarget[targetID]
			activeSpecs := candidateModelSpecsForPlatform(splitModelList(decisionAccount.Models), effectivePolicies, platform)
			hasProbePolicy := hasEnabledProbePolicy(effectivePolicies)
			budgetUsage, budgetReady := s.loadProbeBudgetUsage(ctx, userID, adminAccountID, activeSpecs, decisionAccount.Schedulable, budgetCounts, budgetLoaded, budgetDayStart)
			if !budgetReady {
				budgetUsage = nil
			}
			modelHealth, unprobedModels := modelHealthForSpecs(stateIndex[targetID], activeSpecs, decisionAccount.Schedulable, now, budgetUsage, budgetReady)
			credentialReason := latestCredentialUnavailableReason(modelHealth)
			applyLatestProbeFailureDetails(modelHealth, latestProbeFailureEvent[targetID])
			if credentialReason != "" {
				available = false
				reason = credentialReason
			}
			assignmentSource := policyAssignmentSource(explicitIDs, inheritedIDs)
			priorityState, priorityManaged := priorityByTarget[targetID]
			var priorityOriginal, priorityExpected, priorityConflictValue *int
			var priorityConflictAt *time.Time
			if priorityManaged {
				original := priorityState.OriginalPriority
				expected := priorityState.LastAppliedPriority
				if priorityState.PendingPriority != nil {
					expected = *priorityState.PendingPriority
				}
				priorityOriginal, priorityExpected = &original, &expected
				if priorityState.LastConflictPriority != nil {
					priorityConflictValue = cloneIntPointer(priorityState.LastConflictPriority)
				}
				if priorityState.Conflict {
					priorityConflictAt = utcTimePointer(&priorityState.UpdatedAt)
				}
			}
			var schedulableSource string
			if decisionAccount.Schedulable == nil {
				schedulableSource = "unknown"
			} else {
				schedulableSource = "upstream_observed"
			}
			var lastSchedulableAction string
			var lastSchedulableActionAt *time.Time
			var lastSchedulableActionResult string
			var lastSchedulableActionErrorKey string
			var schedulableChangedAt *time.Time
			if decisionAccount.UpdatedAt != nil {
				schedulableChangedAt = utcTimePointer(decisionAccount.UpdatedAt)
			}
			if event, ok := latestSchedulableEvent[targetID]; ok {
				lastSchedulableAction = event.RemoteAction
				lastSchedulableActionAt = utcTimePointer(&event.CreatedAt)
				lastSchedulableActionResult = event.Result
				lastSchedulableActionErrorKey = event.ErrorKey
			}
			if event, ok := latestSuccessfulSchedulableEvent[targetID]; ok && schedulableUserActionMatchesObserved(event, decisionAccount.Schedulable, decisionAccount.UpdatedAt) {
				schedulableSource = ActionSourceUser
				schedulableChangedAt = utcTimePointer(&event.CreatedAt)
			}
			var effectiveMultiplier *float64
			if priorityManaged {
				value := priorityState.EffectiveMultiplier
				effectiveMultiplier = &value
			}

			item := AdminGroupAccount{
				ID:                            acc.ID,
				Name:                          acc.Name,
				Platform:                      acc.Platform,
				Type:                          acc.Type,
				Status:                        acc.Status,
				Schedulable:                   decisionAccount.Schedulable,
				SchedulableSource:             schedulableSource,
				SchedulableChangedAt:          schedulableChangedAt,
				LastSchedulableAction:         lastSchedulableAction,
				LastSchedulableActionAt:       lastSchedulableActionAt,
				LastSchedulableActionResult:   lastSchedulableActionResult,
				LastSchedulableActionErrorKey: lastSchedulableActionErrorKey,
				UpstreamStatusSource:          "upstream_observed",
				HealthStatusSource:            healthStatusSource(modelHealth, unprobedModels),
				Priority:                      acc.Priority,
				Concurrency:                   acc.Concurrency,
				RateMultiplier:                acc.RateMultiplier,
				LoadFactor:                    acc.LoadFactor,
				Weight:                        acc.Weight,
				Models:                        acc.Models,
				GroupIDs:                      acc.GroupIDs,
				UpstreamKeyGroupName:          upstreamKeyGroup.name,
				UpstreamKeyGroupMultiplier:    upstreamKeyGroup.multiplier,
				TargetID:                      targetID,
				ProbeAvailable:                available,
				ProbeUnavailableReason:        reason,
				ModelHealth:                   modelHealth,
				UnprobedModels:                unprobedModels,
				AssignedPolicyIDs:             assignedIDs,
				AssignedPolicies:              assignedSummaries,
				HasAssignedPolicy:             len(assignedIDs) > 0,
				HasEnabledPolicy:              hasEnabledAssignedPolicy(assignedSummaries),
				HasEnabledProbePolicy:         hasProbePolicy,
				PolicyAssignmentSource:        assignmentSource,
				ExcludedFromGroupPolicy:       excluded,
				PriorityManaged:               priorityManaged,
				PriorityConflict:              priorityManaged && priorityState.Conflict,
				PriorityOriginal:              priorityOriginal,
				PriorityExpected:              priorityExpected,
				PriorityConflictValue:         priorityConflictValue,
				PriorityConflictAt:            priorityConflictAt,
				ProbeModelsConfigured:         len(activeSpecs) > 0,
				EffectiveMultiplier:           effectiveMultiplier,
			}
			if item.HasEnabledProbePolicy {
				health.MonitoredAccountCount++
			}
			if excluded {
				health.ExcludedAccountCount++
			}
			if item.PriorityConflict {
				health.PriorityConflictCount++
				current := cloneIntPointer(acc.Priority)
				health.PriorityConflicts = append(health.PriorityConflicts, AdminPriorityConflict{
					TargetID: targetID, AccountName: acc.Name, CurrentPriority: current, ExpectedPriority: priorityExpected,
					ConflictAt: priorityConflictAt,
				})
			}
			if item.ProbeModelsConfigured {
				health.ProbeModelsConfigured = true
			}

			if hasProbePolicy && available {
				summary.ProbeableAccounts++
				if len(activeSpecs) == 0 {
					summary.UnconfiguredModels++
				} else {
					accumulateSummary(&summary, modelHealth)
					summary.PendingModels += len(unprobedModels)
				}
			} else if hasProbePolicy {
				summary.UnprobeableAccounts++
			}

			health.Accounts = append(health.Accounts, item)
		}
		health.AccountCount = summary.TotalAccounts
		health.HealthSummary = summary
		result = append(result, health)
	}
	return result, nil
}

func (s *Service) loadProbeBudgetUsage(ctx context.Context, userID string, adminAccountID string, specs []probeModelSpec, schedulable *bool, counts map[string]int, loaded map[string]bool, dayStart time.Time) (map[string]int, bool) {
	usage := make(map[string]int)
	for _, spec := range specs {
		for _, policy := range spec.policies {
			continueAutoProbe, _ := policyProbeCadence(policy, schedulable)
			if !continueAutoProbe {
				continue
			}
			if !loaded[policy.ID] {
				count, err := s.repo.CountProbesToday(ctx, userID, adminAccountID, policy.ID, dayStart)
				if err != nil {
					log.Printf("[connection-health] count policy budget for admin group failed policy_id=%s err=%v", policy.ID, err)
					return nil, false
				}
				counts[policy.ID] = count
				loaded[policy.ID] = true
			}
			usage[policy.ID] = counts[policy.ID]
		}
	}
	return usage, true
}

type upstreamKeyGroupInfo struct {
	siteID     string
	keyID      string
	groupID    string
	name       string
	multiplier *float64
}

type upstreamKeyMetadata struct {
	id        string
	groupID   string
	groupName string
}

// upstreamKeyGroupsByAdminAccount 构建 admin 转发账号 ID 到其当前真实上游 API Key 分组的映射。
// real_connections 只负责给出 admin 账号、站点和 Key ID 的可信绑定关系；Key 当前所在分组
// 必须再向上游 Key 列表回查，不能沿用连接创建时保存的历史 UpstreamGroupID/Name。
func (s *Service) upstreamKeyGroupsByAdminAccount(
	ctx context.Context,
	userID string,
	adminAccountID string,
	adminPlatform string,
) map[string]upstreamKeyGroupInfo {
	result := make(map[string]upstreamKeyGroupInfo)
	if s.mySites == nil || s.sites == nil {
		return result
	}
	keyReader, ok := s.mySites.(UpstreamKeyReader)
	if !ok {
		// 这是向后兼容的可选展示能力。旧注入实现没有 Key 查询能力时保持未知，
		// 不阻断分组健康、探活和优先级等原有功能。
		return result
	}

	connections, err := s.mySites.ListRealConnectionsForWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[connection-health] upstream key group lookup skipped workspace=%s err=%v", adminAccountID, err)
		return result
	}

	connectionsByAccount := make(map[string][]my_sites.RealConnection)
	for _, connection := range connections {
		accountID := strings.TrimSpace(connection.AdminAccountID)
		siteID := strings.TrimSpace(connection.UpstreamSiteID)
		if accountID == "" || siteID == "" {
			continue
		}
		if platform := strings.TrimSpace(connection.AdminPlatform); platform != "" && !strings.EqualFold(platform, adminPlatform) {
			continue
		}
		connectionsByAccount[accountID] = append(connectionsByAccount[accountID], connection)
	}

	// 站点、Key 元数据按站点缓存，避免同一页面为每个账号重复访问上游。Key 明文不会复制进
	// 缓存，也不会进入日志或响应；这里只保留识别当前分组所需的 ID 和分组字段。
	siteCache := make(map[string]*upstream.Site)
	missingSites := make(map[string]struct{})
	keysBySite := make(map[string][]upstreamKeyMetadata)
	failedKeySites := make(map[string]struct{})
	for accountID, accountConnections := range connectionsByAccount {
		var resolved upstreamKeyGroupInfo
		reliable := len(accountConnections) > 0
		for index, connection := range accountConnections {
			candidate, candidateOK := s.upstreamKeyGroupForConnection(
				ctx,
				userID,
				connection,
				keyReader,
				siteCache,
				missingSites,
				keysBySite,
				failedKeySites,
			)
			// 一个 admin 账号可能因脏数据存在多条连接。必须逐条成功解析并且精确指向同一
			// 站点、Key 和当前分组，才允许展示；不能跳过失败项后采用另一条看似有效的记录。
			if !candidateOK || (index > 0 && !sameUpstreamKeyGroup(resolved, candidate)) {
				reliable = false
				break
			}
			resolved = candidate
		}
		if reliable {
			result[accountID] = resolved
		}
	}
	return result
}

// upstreamKeyGroupForConnection 先用连接保存的 UpstreamKeyID 精确查找当前上游 Key，再用
// Key 当前返回的分组 ID/名称匹配站点分组倍率。连接里的历史分组字段不参与判断，避免 Key
// 被上游移动分组后继续展示旧倍率。任一阶段缺失、重复或冲突都返回未知。
func (s *Service) upstreamKeyGroupForConnection(
	ctx context.Context,
	userID string,
	connection my_sites.RealConnection,
	keyReader UpstreamKeyReader,
	siteCache map[string]*upstream.Site,
	missingSites map[string]struct{},
	keysBySite map[string][]upstreamKeyMetadata,
	failedKeySites map[string]struct{},
) (upstreamKeyGroupInfo, bool) {
	siteID := strings.TrimSpace(connection.UpstreamSiteID)
	keyID := strings.TrimSpace(connection.UpstreamKeyID)
	if siteID == "" || keyID == "" {
		return upstreamKeyGroupInfo{}, false
	}
	if _, failed := failedKeySites[siteID]; failed {
		return upstreamKeyGroupInfo{}, false
	}
	keys, cached := keysBySite[siteID]
	if !cached {
		items, err := keyReader.ListUpstreamKeys(ctx, userID, siteID)
		if err != nil {
			failedKeySites[siteID] = struct{}{}
			return upstreamKeyGroupInfo{}, false
		}
		keys = make([]upstreamKeyMetadata, 0, len(items))
		for _, item := range items {
			// item.Key 可能包含敏感明文，禁止复制、记录或向前端返回。
			keys = append(keys, upstreamKeyMetadata{
				id:        strings.TrimSpace(item.ID),
				groupID:   strings.TrimSpace(item.GroupID),
				groupName: strings.TrimSpace(item.GroupName),
			})
		}
		keysBySite[siteID] = keys
	}

	var currentKey *upstreamKeyMetadata
	for index := range keys {
		if keys[index].id != keyID {
			continue
		}
		if currentKey != nil {
			return upstreamKeyGroupInfo{}, false
		}
		currentKey = &keys[index]
	}
	if currentKey == nil || (currentKey.groupID == "" && currentKey.groupName == "") {
		return upstreamKeyGroupInfo{}, false
	}

	if _, missing := missingSites[siteID]; missing {
		return upstreamKeyGroupInfo{}, false
	}
	site, cached := siteCache[siteID]
	if !cached {
		var err error
		site, err = s.sites.GetSite(ctx, siteID)
		if err != nil || site == nil {
			missingSites[siteID] = struct{}{}
			return upstreamKeyGroupInfo{}, false
		}
		siteCache[siteID] = site
	}

	var matched *upstream.GroupInfo
	if currentKey.groupID != "" {
		for index := range site.Metrics.Groups {
			if strings.TrimSpace(site.Metrics.Groups[index].ID) != currentKey.groupID {
				continue
			}
			if matched != nil {
				return upstreamKeyGroupInfo{}, false
			}
			matched = &site.Metrics.Groups[index]
		}
	}
	// 有些兼容实现不返回分组 ID，或站点缓存的 ID 形态与 Key 接口不同；此时仅在完整
	// 分组名唯一命中时回退。模糊、部分匹配会把倍率关联到错误分组，因此明确禁止。
	if matched == nil && currentKey.groupName != "" {
		for index := range site.Metrics.Groups {
			if !strings.EqualFold(strings.TrimSpace(site.Metrics.Groups[index].Name), currentKey.groupName) {
				continue
			}
			if matched != nil {
				return upstreamKeyGroupInfo{}, false
			}
			matched = &site.Metrics.Groups[index]
		}
	}
	if matched == nil {
		return upstreamKeyGroupInfo{}, false
	}
	return newUpstreamKeyGroupInfo(siteID, keyID, *matched), true
}

func newUpstreamKeyGroupInfo(siteID string, keyID string, group upstream.GroupInfo) upstreamKeyGroupInfo {
	info := upstreamKeyGroupInfo{
		siteID:  strings.TrimSpace(siteID),
		keyID:   strings.TrimSpace(keyID),
		groupID: strings.TrimSpace(group.ID),
		name:    strings.TrimSpace(group.Name),
	}
	if group.Multiplier != nil {
		value := *group.Multiplier
		info.multiplier = &value
	}
	return info
}

func sameUpstreamKeyGroup(left upstreamKeyGroupInfo, right upstreamKeyGroupInfo) bool {
	if left.siteID != right.siteID || left.keyID != right.keyID || left.groupID != right.groupID {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(left.name), strings.TrimSpace(right.name)) {
		return false
	}
	if left.multiplier == nil || right.multiplier == nil {
		return left.multiplier == nil && right.multiplier == nil
	}
	return *left.multiplier == *right.multiplier
}

// modelHealthForConnection 把某个 targetId 的健康状态表（modelName -> state）展开为 ModelHealth 列表。
// 没有任何状态时返回空数组（可探活但尚未探活）。
func modelHealthForConnection(byModel map[string]ConnectionHealthState) []ModelHealth {
	models := make([]ModelHealth, 0, len(byModel))
	for modelName, st := range byModel {
		models = append(models, toModelHealth(modelName, st))
	}
	return models
}

func latestProbeFailureEventsByTargetModel(events []ConnectionHealthEvent) map[string]map[string]ConnectionHealthEvent {
	latest := make(map[string]map[string]ConnectionHealthEvent)
	for _, event := range events {
		byModel := latest[event.ConnectionID]
		if byModel == nil {
			byModel = make(map[string]ConnectionHealthEvent)
			latest[event.ConnectionID] = byModel
		}
		current, exists := byModel[event.ModelName]
		if !exists || event.CreatedAt.After(current.CreatedAt) {
			byModel[event.ModelName] = event
		}
	}
	return latest
}

func applyLatestProbeFailureDetails(models []ModelHealth, latestByModel map[string]ConnectionHealthEvent) {
	for i := range models {
		if models[i].LastFailureAt == nil {
			continue
		}
		event, exists := latestByModel[models[i].ModelName]
		if !exists || event.CreatedAt.Before(*models[i].LastFailureAt) {
			continue
		}
		models[i].LastErrorKey = event.ErrorKey
		if models[i].LastErrorKey == "" {
			models[i].LastErrorKey = event.Result
		}
		models[i].LastErrorDetail = event.ErrorDetail
	}
}

// modelHealthForSpecs 只展开当前有效策略仍启用的模型。历史状态继续留库用于审计，但模型被
// 删除、禁用或不再属于目标后，不得继续影响页面汇总、优先级和账号级动作。
func modelHealthForSpecs(byModel map[string]ConnectionHealthState, specs []probeModelSpec, schedulable *bool, now time.Time, budgetUsage map[string]int, budgetReady bool) ([]ModelHealth, []AdminGroupUnprobedModel) {
	models := make([]ModelHealth, 0, len(specs))
	unprobed := make([]AdminGroupUnprobedModel, 0)
	for _, spec := range specs {
		state, exists := byModel[spec.modelName]
		if !exists {
			decision := calculateEffectiveProbeDecisionWithBudgets(spec.policies, schedulable, nil, now, budgetUsage)
			if !budgetReady && decision.ContinueAutoProbe {
				decision.NextProbeAt = nil
				decision.BlockedReason = ProbeBlockedBudgetUnavailable
			}
			unprobed = append(unprobed, AdminGroupUnprobedModel{
				ModelName: spec.modelName, ProviderFamily: spec.providerFamily, NextProbeAt: decision.NextProbeAt,
				BlockedReason: decision.BlockedReason, EffectiveIntervalSeconds: decision.EffectiveIntervalSeconds,
				EffectivePolicySources: decision.SourcePolicies, BudgetPolicyID: decision.BudgetPolicyID,
			})
			continue
		}
		model := toModelHealth(spec.modelName, state)
		model.ProviderFamily = spec.providerFamily
		decision := calculateEffectiveProbeDecisionWithBudgets(spec.policies, schedulable, &state, now, budgetUsage)
		if !budgetReady && decision.ContinueAutoProbe {
			decision.NextProbeAt = nil
			decision.BlockedReason = ProbeBlockedBudgetUnavailable
		}
		model.NextProbeAt = decision.NextProbeAt
		model.BlockedReason = decision.BlockedReason
		model.EffectiveIntervalSeconds = decision.EffectiveIntervalSeconds
		model.EffectivePolicySources = decision.SourcePolicies
		model.BudgetPolicyID = decision.BudgetPolicyID
		model.ElapsedSeconds = elapsedSince(model.LastFailureAt, model.LastProbeAt, now)
		models = append(models, model)
	}
	return models, unprobed
}

func latestCredentialUnavailableReason(models []ModelHealth) string {
	var latest *time.Time
	latestErrorKey := ""
	for _, model := range models {
		timestamp := model.UpdatedAt
		if model.LastProbeAt != nil && (timestamp == nil || model.LastProbeAt.After(*timestamp)) {
			timestamp = model.LastProbeAt
		}
		if timestamp == nil {
			continue
		}
		if latest == nil || timestamp.After(*latest) {
			value := *timestamp
			latest = &value
			latestErrorKey = model.LastErrorKey
		}
	}
	if isCredentialUnavailableReason(latestErrorKey) {
		return latestErrorKey
	}
	return ""
}

func latestSchedulableEventsByTarget(events []ConnectionHealthEvent) map[string]ConnectionHealthEvent {
	latest := make(map[string]ConnectionHealthEvent)
	for _, event := range events {
		if event.ActionSource != ActionSourceUser {
			continue
		}
		previous, exists := latest[event.ConnectionID]
		if !exists || event.CreatedAt.After(previous.CreatedAt) {
			latest[event.ConnectionID] = event
		}
	}
	return latest
}

func schedulableUserActionMatchesObserved(event ConnectionHealthEvent, observed *bool, observedUpdatedAt *time.Time) bool {
	if event.Result != SchedulableActionSucceeded || observed == nil || observedUpdatedAt == nil || observedUpdatedAt.After(event.CreatedAt) {
		return false
	}
	switch event.RemoteAction {
	case RemoteActionSchedulableEnabled:
		return *observed
	case RemoteActionSchedulableDisabled:
		return !*observed
	default:
		return false
	}
}

func elapsedSince(lastFailure *time.Time, _ *time.Time, now time.Time) *int64 {
	if lastFailure == nil {
		return nil
	}
	seconds := int64(now.Sub(*lastFailure).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return &seconds
}

func healthStatusSource(models []ModelHealth, unprobed []AdminGroupUnprobedModel) string {
	if len(models) > 0 {
		for _, model := range models {
			if model.Configured {
				return "health_probe"
			}
		}
		return "unconfigured"
	}
	if len(unprobed) > 0 {
		return "unprobed"
	}
	return "none"
}

// accumulateSummary only counts persisted states. Configured models without a state are
// counted separately by the caller, so a partially-probed account cannot hide them.
func accumulateSummary(summary *AdminGroupHealthSummary, models []ModelHealth) {
	for _, m := range models {
		if isCredentialUnavailableReason(m.LastErrorKey) {
			summary.UnconfiguredModels++
			continue
		}
		switch m.State {
		case StateHealthy:
			summary.HealthyModels++
		case StateDegraded:
			summary.DegradedModels++
		case StateObserving:
			// DegradedModels 继续包含 observing/recovering，保持旧客户端依赖的聚合语义；
			// 新字段让新版页面可以拆开展示完整状态，并从聚合值中扣除得到严格降级数。
			summary.DegradedModels++
			summary.ObservingModels++
		case StateRecovering:
			summary.DegradedModels++
			summary.RecoveringModels++
		case StateSuspended:
			summary.SuspendedModels++
		case StateDisabled:
			summary.DisabledModels++
		}
		if m.LastProbeAt != nil {
			if summary.LastProbeAt == nil || m.LastProbeAt.After(*summary.LastProbeAt) {
				lastProbe := *m.LastProbeAt
				summary.LastProbeAt = &lastProbe
			}
		}
	}
}

// assignedPolicySummaries 把一个 target 的分配行 + workspace 全量策略索引拼装成展示用的
// policyIds/summaries。即使策略已被停用也要能展示名字，所以调用方传入的是全量 ListPolicies 索引。
func assignedPolicySummaries(assignments []PolicyAssignment, policyByID map[string]Policy) ([]string, []AssignedPolicySummary) {
	ids := make([]string, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.PolicyID)
	}
	return assignedPolicySummariesFromIDs(ids, policyByID)
}

func assignedPolicySummariesFromIDs(ids []string, policyByID map[string]Policy) ([]string, []AssignedPolicySummary) {
	ids = mergePolicyIDs(ids)
	summaries := make([]AssignedPolicySummary, 0, len(ids))
	for _, policyID := range ids {
		if p, ok := policyByID[policyID]; ok {
			summaries = append(summaries, AssignedPolicySummary{
				PolicyID: p.ID, PolicyName: p.Name, Enabled: p.Enabled,
				PriorityMode: normalizePriorityMode(p.PriorityMode), AutoRemoteActionEnabled: policyRemoteActionEnabled(p),
			})
		} else {
			summaries = append(summaries, AssignedPolicySummary{PolicyID: policyID})
		}
	}
	return ids, summaries
}

func hasEnabledAssignedPolicy(policies []AssignedPolicySummary) bool {
	for _, policy := range policies {
		if policy.Enabled {
			return true
		}
	}
	return false
}

func hasEnabledProbePolicy(policies []Policy) bool {
	// 仅倍率策略会改变排序，但不能让账号在界面上显示为“正在探活”。
	for _, policy := range policies {
		if policy.Enabled && policySupportsProbing(policy) {
			return true
		}
	}
	return false
}

func hasEnabledProbePolicyByIDs(policyIDs []string, policyByID map[string]Policy) bool {
	// 分组统计沿用与账号详情一致的探活口径。
	for _, policyID := range policyIDs {
		if policy, ok := policyByID[policyID]; ok && policy.Enabled && policySupportsProbing(policy) {
			return true
		}
	}
	return false
}

func mergePolicyIDs(groups ...[]string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, ids := range groups {
		for _, id := range ids {
			if _, exists := seen[id]; exists || id == "" {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result
}

func policyAssignmentSource(explicitIDs []string, inheritedIDs []string) string {
	switch {
	case len(explicitIDs) > 0 && len(inheritedIDs) > 0:
		return "mixed"
	case len(explicitIDs) > 0:
		return "target"
	case len(inheritedIDs) > 0:
		return "group"
	default:
		return "none"
	}
}

// adminGroupType 把 upstream 的分组标志归一化为主列表展示用的类型：
// 订阅分组优先于专属分组，其余为公开分组。
func adminGroupType(group upstream.AdminGroupInfo) string {
	if group.SubscriptionType == "subscription" {
		return "subscription"
	}
	if group.IsExclusive {
		return "exclusive"
	}
	return "public"
}
