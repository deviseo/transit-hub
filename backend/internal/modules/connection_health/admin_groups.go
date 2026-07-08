package connection_health

import (
	"context"
	"errors"
	"log"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// AdminGroupHealth 是「当前 admin workspace 下的一个 admin 分组」在分组健康主列表中的展示单元。
// 探活体系已改为独立目标：分组下的账号(sub2api)/渠道(new-api)本身就是探活目标，不再依赖
// real_connections 对接链路。探活字段（probeAvailable / modelHealth 等）来自独立 admin 探活
// 状态（connection_health_states 中以 targetId 为键的行），不再从 real_connections 叠加。
type AdminGroupHealth struct {
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	Platform          string                  `json:"platform"`
	Status            string                  `json:"status"`
	Type              string                  `json:"type"` // public / exclusive / subscription
	IsExclusive       bool                    `json:"isExclusive"`
	SubscriptionType  string                  `json:"subscriptionType"`
	Multiplier        *float64                `json:"multiplier"`
	MultiplierDisplay string                  `json:"multiplierDisplay"`
	AccountCount      int                     `json:"accountCount"`
	HealthSummary     AdminGroupHealthSummary `json:"healthSummary"`
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
	SuspendedModels     int        `json:"suspendedModels"`
	DisabledModels      int        `json:"disabledModels"`
	UnconfiguredModels  int        `json:"unconfiguredModels"`
	LastProbeAt         *time.Time `json:"lastProbeAt"`
}

// AdminGroupAccount 是 admin 分组下的一个账号(sub2api) / 渠道(new-api)，同时是一个独立探活目标。
// 只要后端能安全解析 base_url + key + model 就可独立探活，不再需要 real_connections。
// 绝不包含 key / token / cookie / credentials / secret / authorization 明文。
type AdminGroupAccount struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Platform       string   `json:"platform"`
	Type           string   `json:"type"`
	Status         string   `json:"status"`
	Schedulable    *bool    `json:"schedulable,omitempty"`
	Priority       *int     `json:"priority,omitempty"`
	Concurrency    *int     `json:"concurrency,omitempty"`
	RateMultiplier *float64 `json:"rateMultiplier,omitempty"`
	LoadFactor     *int     `json:"loadFactor,omitempty"`
	Weight         *int     `json:"weight,omitempty"`
	Models         string   `json:"models,omitempty"`
	GroupIDs       []string `json:"groupIds,omitempty"`
	// 独立探活字段。
	TargetID               string        `json:"targetId"`
	ProbeAvailable         bool          `json:"probeAvailable"`
	ProbeUnavailableReason string        `json:"probeUnavailableReason,omitempty"`
	ModelHealth            []ModelHealth `json:"modelHealth"`
	// 策略分配字段：与 ProbeAvailable 完全解耦——未分配策略的账号/渠道仍可手动一次性探活，
	// 只是不会被调度器自动探活、不会进策略探活事件列表。
	AssignedPolicyIDs []string                `json:"assignedPolicyIds"`
	AssignedPolicies  []AssignedPolicySummary `json:"assignedPolicies"`
	HasAssignedPolicy bool                    `json:"hasAssignedPolicy"`
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
		}

		accounts, accErr := s.platformGroups.ListAdminGroupAccounts(session, group)
		if accErr != nil {
			log.Printf("[connection-health] admin group accounts fetch failed group_id=%s group_name=%s err=%v", group.ID, group.Name, accErr)
			health.AccountsError = ErrorAccountsFetch
			result = append(result, health)
			continue
		}

		summary := AdminGroupHealthSummary{TotalAccounts: len(accounts)}
		for _, acc := range accounts {
			targetID := buildTargetID(platform, adminAccountID, acc.ID)
			specs := candidateModelSpecs(splitModelList(acc.Models), policies)
			available, reason := targetProbeAvailability(platform, acc.BaseURL, len(specs))
			modelHealth := modelHealthForConnection(stateIndex[targetID])
			assignedIDs, assignedSummaries := assignedPolicySummaries(assignmentsByTarget[targetID], policyByID)

			item := AdminGroupAccount{
				ID:                     acc.ID,
				Name:                   acc.Name,
				Platform:               acc.Platform,
				Type:                   acc.Type,
				Status:                 acc.Status,
				Schedulable:            acc.Schedulable,
				Priority:               acc.Priority,
				Concurrency:            acc.Concurrency,
				RateMultiplier:         acc.RateMultiplier,
				LoadFactor:             acc.LoadFactor,
				Weight:                 acc.Weight,
				Models:                 acc.Models,
				GroupIDs:               acc.GroupIDs,
				TargetID:               targetID,
				ProbeAvailable:         available,
				ProbeUnavailableReason: reason,
				ModelHealth:            modelHealth,
				AssignedPolicyIDs:      assignedIDs,
				AssignedPolicies:       assignedSummaries,
				HasAssignedPolicy:      len(assignedIDs) > 0,
			}

			if available {
				summary.ProbeableAccounts++
				accumulateSummary(&summary, modelHealth)
			} else {
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

// modelHealthForConnection 把某个 targetId 的健康状态表（modelName -> state）展开为 ModelHealth 列表。
// 没有任何状态时返回空数组（可探活但尚未探活）。
func modelHealthForConnection(byModel map[string]ConnectionHealthState) []ModelHealth {
	models := make([]ModelHealth, 0, len(byModel))
	for modelName, st := range byModel {
		models = append(models, toModelHealth(modelName, st))
	}
	return models
}

// accumulateSummary 把一个可探活目标的模型健康结果累加进分组概览。
// 可探活但没有任何模型探活记录的目标计入 unconfiguredModels（可探活但还没探活过）。
func accumulateSummary(summary *AdminGroupHealthSummary, models []ModelHealth) {
	if len(models) == 0 {
		summary.UnconfiguredModels++
		return
	}
	for _, m := range models {
		switch m.State {
		case StateHealthy:
			summary.HealthyModels++
		case StateDegraded, StateObserving, StateRecovering:
			summary.DegradedModels++
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
	summaries := make([]AssignedPolicySummary, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.PolicyID)
		if p, ok := policyByID[a.PolicyID]; ok {
			summaries = append(summaries, AssignedPolicySummary{PolicyID: p.ID, PolicyName: p.Name, Enabled: p.Enabled})
		} else {
			summaries = append(summaries, AssignedPolicySummary{PolicyID: a.PolicyID})
		}
	}
	return ids, summaries
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
