package connection_health

import (
	"context"
	"log"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// 本文件实现「独立 admin 账号/渠道探活」体系：分组健康不再依赖 real_connections，
// 而是把当前 admin workspace 下的 admin 分组、分组下账号/channel 本身作为探活目标。
// 后端在探活前 server-only 地临时解析 base_url + key + model，用现有 RealProbeRunner 发起探活。
//
// 存储复用：新目标的健康状态/事件复用 connection_health_states / connection_health_events 两张表，
// 其 connection_id 列存放稳定的 targetId（见 buildTargetID）。targetId 形如
// "newapi:<workspaceAdminAccountID>:<accountID>"，与 real_connections 的 UUID 不会碰撞，
// 旧连接维度的查询/路由完全不受影响。

// RemoteActionSkippedIndependentProbe 标记：策略未同时开启自动降级和自动远端动作时，即使
// 状态机判定需要远端动作也只记录这个标记，不真正调用上游。开启后，sub2api target 切换
// 账号状态，New API target 切换 channel status/weight，二者都通过 dispatcher 的平台中性接口执行。
const RemoteActionSkippedIndependentProbe = "skipped_independent_probe"

// 探活不可用原因 -> 前端可识别的 i18n 错误 key 映射。reason 取值来自 upstream.Reason* 常量。
const (
	ErrorCredentialUnavailable      = "admin.connectionHealth.errors.credentialUnavailable"
	ErrorSecureVerificationRequired = "admin.connectionHealth.errors.secureVerificationRequired"
	ErrorBaseURLUnavailable         = "admin.connectionHealth.errors.baseUrlUnavailable"
	ErrorModelUnavailable           = "admin.connectionHealth.errors.modelUnavailable"
	ErrorExportUnavailable          = "admin.connectionHealth.errors.exportUnavailable"
	ErrorCredentialsRedacted        = "admin.connectionHealth.errors.credentialsRedacted"
	ErrorProbeTargetNotFound        = "admin.connectionHealth.errors.targetNotFound"
)

// AdminProbeTarget 是平台中性的独立探活目标：一个 admin 分组下的账号(sub2api)/渠道(new-api)。
// 不再要求存在 real_connections。TargetID 稳定且可复算，是新状态/事件的核心键。
type AdminProbeTarget struct {
	TargetID               string   `json:"targetId"`
	Platform               string   `json:"platform"`
	AdminGroupID           string   `json:"adminGroupId"`
	AdminGroupName         string   `json:"adminGroupName"`
	AccountID              string   `json:"accountId"`
	AccountName            string   `json:"accountName"`
	AccountStatus          string   `json:"accountStatus"`
	AccountWeight          *int     `json:"accountWeight,omitempty"`
	ProviderFamily         string   `json:"providerFamily"`
	Models                 []string `json:"models"`
	ProbeAvailable         bool     `json:"probeAvailable"`
	ProbeUnavailableReason string   `json:"probeUnavailableReason,omitempty"`
}

// probeModelSpec 是一个「目标 + 具体探活模型」的组合，携带该模型来自哪条策略的探活参数。
type probeModelSpec struct {
	modelName      string
	providerFamily string
	maxProbeTokens int
	probePrompt    string
	policy         Policy
	// Event group metadata records where this policy was inherited for this target.
	// Explicit target assignments intentionally resolve to an empty group.
	eventGroupResolved  bool
	eventAdminGroupID   string
	eventAdminGroupName string
}

// targetProbeResult 暂存单模型探活结果。一个账号的全部到期模型完成后，再统一决定一次上游
// 动作并写事件，避免多模型按执行顺序互相启停同一个账号。
type targetProbeResult struct {
	state           *ConnectionHealthState
	previousState   State
	outcome         ProbeOutcome
	latencyMs       int
	spec            probeModelSpec
	triggeredRemote bool
}

// buildTargetID 生成稳定的探活目标 ID：platform:workspaceAdminAccountID:accountID。
// 不使用随机 ID；同一账号在同一 workspace 下每次都算出同一个 targetId，便于状态/事件持续累计。
func buildTargetID(platform string, adminAccountID string, accountID string) string {
	return platform + ":" + adminAccountID + ":" + accountID
}

// parsedTargetID 解析 targetId 的三段结构，用于手动探活时校验目标归属当前 workspace，
// 避免用户用别的 workspace 的 targetId 越权探活。
type parsedTargetID struct {
	platform       string
	adminAccountID string
	accountID      string
}

func parseTargetID(targetID string) (parsedTargetID, bool) {
	parts := strings.SplitN(targetID, ":", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return parsedTargetID{}, false
	}
	return parsedTargetID{platform: parts[0], adminAccountID: parts[1], accountID: parts[2]}, true
}

// candidateModelSpecs 计算一个目标当前可探活的候选模型：
//   - 策略池 = 当前 workspace 全部启用策略下的启用 modelTargets（按模型名去重，先出现的优先）。
//   - 目标自带模型列表（new-api channel.models 等）非空时，取「目标模型 ∩ 策略池」。
//   - 目标没有模型列表时，直接用策略池（策略里明确配置的模型）。
//
// 说明：独立探活维度下，admin 目标不再按 own group 精确匹配策略（own group 是「我的分组」，
// 与 admin 分组是不同概念），因此这里用 workspace 级策略池，保留现有策略 modelTargets 的配置语义。
func candidateModelSpecs(targetModels []string, policies []Policy) []probeModelSpec {
	pool := make([]probeModelSpec, 0)
	seen := make(map[string]int)
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		for _, t := range p.ModelTargets {
			if !t.Enabled {
				continue
			}
			name := strings.TrimSpace(t.ModelName)
			if name == "" {
				continue
			}
			if index, dup := seen[name]; dup {
				// 同一模型被多条策略覆盖时使用稳定且偏安全的策略：关闭远端动作优先，
				// 然后选择更低失败阈值、更长观察期和更短探活间隔，最后按 ID 决胜。
				if preferProbePolicy(p, pool[index].policy) {
					pool[index] = probeModelSpec{
						modelName: name, providerFamily: t.ProviderFamily, maxProbeTokens: t.MaxProbeTokens,
						probePrompt: t.ProbePrompt, policy: p,
					}
				}
				continue
			}
			seen[name] = len(pool)
			pool = append(pool, probeModelSpec{
				modelName:      name,
				providerFamily: t.ProviderFamily,
				maxProbeTokens: t.MaxProbeTokens,
				probePrompt:    t.ProbePrompt,
				policy:         p,
			})
		}
	}

	if len(targetModels) == 0 {
		return pool
	}
	allowed := make(map[string]struct{}, len(targetModels))
	for _, m := range targetModels {
		allowed[strings.TrimSpace(m)] = struct{}{}
	}
	filtered := make([]probeModelSpec, 0, len(pool))
	for _, spec := range pool {
		if _, ok := allowed[spec.modelName]; ok {
			filtered = append(filtered, spec)
		}
	}
	return filtered
}

func preferProbePolicy(candidate Policy, current Policy) bool {
	if policyRemoteActionEnabled(candidate) != policyRemoteActionEnabled(current) {
		return !policyRemoteActionEnabled(candidate)
	}
	if failureThreshold(candidate) != failureThreshold(current) {
		return failureThreshold(candidate) < failureThreshold(current)
	}
	if observationWindow(candidate) != observationWindow(current) {
		return observationWindow(candidate) > observationWindow(current)
	}
	if candidate.ProbeIntervalSeconds != current.ProbeIntervalSeconds {
		return defaultInt(candidate.ProbeIntervalSeconds, 60) < defaultInt(current.ProbeIntervalSeconds, 60)
	}
	return candidate.ID < current.ID
}

// targetProbeAvailability 在「不获取密钥」的前提下静态判断目标是否可探活，用于主列表展示：
//   - 没有任何候选模型 -> model_unavailable。
//   - new-api channel 缺少 base_url -> base_url_unavailable（凭据要点之一，list 阶段即可知）。
//   - 其余情况乐观标记可探活；密钥/安全验证等只有真正探活时才知道，失败会在 modelHealth/手动
//     探活错误里体现，不在这里预取密钥（避免高频命中受保护的 key 接口触发限流/安全验证）。
func targetProbeAvailability(platform string, baseURL string, specCount int) (bool, string) {
	if specCount == 0 {
		return false, upstream.ReasonModelUnavailable
	}
	if platform == string(upstream.PlatformNewAPI) && strings.TrimSpace(baseURL) == "" {
		return false, upstream.ReasonBaseURLUnavailable
	}
	return true, ""
}

// targetManualProbeAvailability 只判断一次性手动探活是否具备静态前置条件。手动探活会在打开
// 弹窗后实时发现模型，因此不能再依赖策略 modelTargets；否则一个尚未创建策略的新 workspace
// 会把所有目标误标为不可探活，用户也就无法通过简化流程开始配置。
func targetManualProbeAvailability(platform string, baseURL string) (bool, string) {
	if platform == string(upstream.PlatformNewAPI) && strings.TrimSpace(baseURL) == "" {
		return false, upstream.ReasonBaseURLUnavailable
	}
	return true, ""
}

// reasonToErrorKey 把探活不可用 reason 映射为前端 i18n 错误 key。
func reasonToErrorKey(reason string) string {
	switch reason {
	case upstream.ReasonSecureVerificationRequired:
		return ErrorSecureVerificationRequired
	case upstream.ReasonBaseURLUnavailable:
		return ErrorBaseURLUnavailable
	case upstream.ReasonModelUnavailable:
		return ErrorModelUnavailable
	case upstream.ReasonExportUnavailable:
		return ErrorExportUnavailable
	case upstream.ReasonCredentialsRedacted:
		return ErrorCredentialsRedacted
	default:
		return ErrorCredentialUnavailable
	}
}

func isCredentialUnavailableReason(reason string) bool {
	switch reason {
	case upstream.ReasonCredentialUnavailable, upstream.ReasonSecureVerificationRequired,
		upstream.ReasonBaseURLUnavailable, upstream.ReasonExportUnavailable, upstream.ReasonCredentialsRedacted:
		return true
	default:
		return false
	}
}

// resolveManualTarget 是手动一次性动作（旧策略候选手动探活 / 新模型发现 / 新一次性探活）共用的
// target 解析入口：校验 targetId 归属当前 workspace + platform、重新解析目标账号（不信任前端传入
// 的任何细节），返回 canonical 后的 session/target/account/adminAccountID。
// 不解析凭据（凭据解析由调用方按需调用 ResolveProbeCredential），因为策略分配管理等轻量场景
// 不需要真的打上游拿明文 key。
func (s *Service) resolveManualTarget(ctx context.Context, userID string, targetID string) (upstream.Session, AdminProbeTarget, upstream.AdminGroupAccountInfo, string, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return upstream.Session{}, AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, "", err
	}
	if s.platformGroups == nil {
		return upstream.Session{}, AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, "", requestError(ErrorUnknown)
	}

	// 校验目标归属当前 workspace：targetId 内嵌的 adminAccountID 必须等于当前 workspace，
	// 防止用别的 workspace 的 targetId 越权操作。
	parsed, ok := parseTargetID(targetID)
	if !ok || parsed.adminAccountID != adminAccountID {
		return upstream.Session{}, AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, "", requestError(ErrorProbeTargetNotFound)
	}

	session, err := s.mySites.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return upstream.Session{}, AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, "", err
	}

	// 校验 targetId 的 platform 段与当前 workspace 的 session 平台一致。否则 platform 段被伪造
	// （如 session 是 new-api 却传 sub2api:ws1:100）时，findAdminTarget 会用 session 平台重建
	// 出 canonical targetId（newapi:ws1:100），导致请求 targetId 与状态/事件 key 不一致。
	if parsed.platform != string(session.Platform) {
		return upstream.Session{}, AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, "", requestError(ErrorProbeTargetNotFound)
	}

	// 重新解析目标账号（不信任前端），拿到 base_url/models 等探活必需信息。
	target, account, found, accountsReadError, err := s.findAdminTarget(ctx, session, adminAccountID, parsed.accountID)
	if err != nil {
		return upstream.Session{}, AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, "", err
	}
	if !found {
		// 目标未找到时，若过程中发生过账号列表读取错误，说明可能是上游临时故障而非目标不存在，
		// 返回账号列表读取错误（安全 i18n key，不含上游明文），避免掩盖真实上游故障。
		if accountsReadError {
			return upstream.Session{}, AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, "", requestError(ErrorAccountsFetch)
		}
		return upstream.Session{}, AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, "", requestError(ErrorProbeTargetNotFound)
	}
	// canonical 校验：重建出的 targetId 必须与请求完全一致，杜绝任何 targetId 段不一致的写入。
	if target.TargetID != targetID {
		return upstream.Session{}, AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, "", requestError(ErrorProbeTargetNotFound)
	}

	return session, target, account, adminAccountID, nil
}

// ProbeTarget 手动探活一个独立目标：前端传 targetId + models（不再传 connectionId/base_url/key）。
// 后端按当前 user/admin workspace 重新解析目标与凭据，不信任前端传入的任何上游地址或密钥。
// 不可探活时返回结构化 requestError（credential_unavailable / secure_verification_required /
// base_url_unavailable / model_unavailable 等对应的 i18n key）。
//
// 注意：这是旧的「策略候选池」手动探活路径，会写入 connection_health_states/events。
// 新账号弹窗的一次性手动探活已改用 ManualProbeTarget（见 manual_probe.go），不写状态/事件。
// 本接口继续保留只为兼容可能存在的旧调用方。
func (s *Service) ProbeTarget(ctx context.Context, userID string, targetID string, models []string) ([]ModelHealth, error) {
	session, target, account, adminAccountID, err := s.resolveManualTarget(ctx, userID, targetID)
	if err != nil {
		return nil, err
	}
	release, err := s.repo.AcquireTargetLease(ctx, targetID)
	if err != nil {
		return nil, err
	}
	defer release()

	policies, err := s.repo.ListPolicies(ctx, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	allSpecs := candidateModelSpecs(target.Models, policies)
	specs := allSpecs

	// 按请求的 models 过滤（语义与 ProbeConnection 一致）：显式指定但一个都没命中 -> 明确拒绝。
	requested := make([]string, 0, len(models))
	for _, m := range models {
		if trimmed := strings.TrimSpace(m); trimmed != "" {
			requested = append(requested, trimmed)
		}
	}
	if len(requested) > 0 {
		wanted := make(map[string]struct{}, len(requested))
		for _, m := range requested {
			wanted[m] = struct{}{}
		}
		filtered := make([]probeModelSpec, 0, len(specs))
		for _, spec := range specs {
			if _, ok := wanted[spec.modelName]; ok {
				filtered = append(filtered, spec)
			}
		}
		if len(filtered) == 0 {
			return nil, requestError(ErrorNoMatchingModels)
		}
		specs = filtered
	}

	if len(specs) == 0 {
		return nil, requestError(ErrorModelUnavailable)
	}

	// 解析凭据（server-only，明文只在内存短暂存在）。失败 -> 结构化不可探活错误。
	cred, err := s.platformGroups.ResolveProbeCredential(session, account)
	if err != nil {
		return nil, requestError(reasonToErrorKey(upstream.ProbeCredentialReason(err)))
	}

	results := make([]ModelHealth, 0, len(specs))
	probeResults := make([]targetProbeResult, 0, len(specs))
	for _, spec := range specs {
		result, probeErr := s.probeTargetOnce(ctx, userID, adminAccountID, target, cred, spec)
		if probeErr != nil {
			log.Printf("[connection-health] manual target probe failed target_id=%s model=%s err=%v", target.TargetID, spec.modelName, probeErr)
			continue
		}
		if result != nil {
			probeResults = append(probeResults, *result)
		}
	}
	s.finishTargetProbeBatch(ctx, userID, adminAccountID, session, target, allSpecs, probeResults)
	for _, result := range probeResults {
		results = append(results, toModelHealth(result.spec.modelName, *result.state))
	}
	return results, nil
}

// findAdminTarget 在当前 workspace 的 admin 分组/账号里按 accountID 找到目标，并构造 AdminProbeTarget。
// 用于手动探活时按 targetId 重新解析目标（不信任前端传入的目标细节）。
// 返回的 accountsReadError 表示遍历过程中是否有分组的账号列表读取失败：单分组失败仍会继续查
// 其它分组，但若最终没找到目标，调用方可据此区分「目标不存在」与「上游读取失败」。
func (s *Service) findAdminTarget(ctx context.Context, session upstream.Session, adminAccountID string, accountID string) (target AdminProbeTarget, account upstream.AdminGroupAccountInfo, found bool, accountsReadError bool, err error) {
	groups, err := s.platformGroups.FetchAdminAllGroups(session)
	if err != nil {
		return AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, false, false, err
	}
	platform := string(session.Platform)
	for _, group := range groups {
		accounts, accErr := s.platformGroups.ListAdminGroupAccounts(session, group)
		if accErr != nil {
			// 单分组失败不影响在其它分组里继续找，但记录下发生过读取错误。
			accountsReadError = true
			continue
		}
		for _, acc := range accounts {
			if acc.ID != accountID {
				continue
			}
			resolved := AdminProbeTarget{
				TargetID:       buildTargetID(platform, adminAccountID, acc.ID),
				Platform:       platform,
				AdminGroupID:   group.ID,
				AdminGroupName: group.Name,
				AccountID:      acc.ID,
				AccountName:    acc.Name,
				AccountStatus:  acc.Status,
				AccountWeight:  cloneIntPointer(acc.Weight),
				ProviderFamily: acc.Platform,
				Models:         splitModelList(acc.Models),
			}
			return resolved, acc, true, accountsReadError, nil
		}
	}
	return AdminProbeTarget{}, upstream.AdminGroupAccountInfo{}, false, accountsReadError, nil
}

// probeTargetOnce 对一个 (target, model) 组合执行一次独立探活并落库状态。事件和账号级上游
// 动作由 finishTargetProbeBatch 在同一账号全部模型完成后统一处理。
// 与 probeOnce 的关键差异：状态/事件以 targetId 为键（存在 connection_id 列）。
// 远端动作规则：
//   - 自动降级或自动远端动作任一关闭时，即使状态机判定需要远端动作，也只记
//     RemoteActionSkippedIndependentProbe，绝不调用上游（与旧行为一致）。
//   - 两个开关都开启且 target.Platform 是 sub2api 时，真实调用
//     dispatcher.DegradeTarget/RestoreTarget 切换 sub2api 账号 active/inactive。
//   - New API target 按 currentWeight 更新 channel weight/status，实现逐步恢复。
//
// session 来自调用方（ProbeTarget 的 resolveManualTarget / 调度器 job 的 RequireSession），
// 不信任前端传入的任何 platform/account 信息。
// 每日探活预算耗尽时跳过真实请求，只保留当前状态。
func (s *Service) probeTargetOnce(ctx context.Context, userID string, adminAccountID string, target AdminProbeTarget, cred upstream.ProbeCredential, spec probeModelSpec) (*targetProbeResult, error) {
	current, err := s.repo.GetState(ctx, target.TargetID, spec.modelName)
	if err != nil {
		return nil, err
	}
	if current == nil {
		defaultState := defaultTargetState(userID, adminAccountID, target, spec.modelName)
		current = &defaultState
	}

	dayStart := probeBudgetDayStart(time.Now())
	allowed, err := s.repo.TryConsumeProbeBudget(ctx, userID, adminAccountID, spec.policy.ID, dayStart, probeBudgetLimit(spec.policy))
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, nil
	}

	providerFamily := spec.providerFamily
	if providerFamily == "" {
		providerFamily = target.ProviderFamily
	}
	outcome := s.probeRunner.Probe(ctx, ProbeRequest{
		BaseURL: cred.BaseURL, UpstreamKey: cred.Key, ProviderFamily: providerFamily,
		ModelName: spec.modelName, MaxTokens: spec.maxProbeTokens, ProbePrompt: spec.probePrompt,
	})

	now := time.Now()
	transitionOut := Transition(TransitionInput{
		Current: current.State, CurrentWeight: current.CurrentWeight, ConsecutiveFailures: current.ConsecutiveFailures,
		ConsecutiveSuccesses: current.ConsecutiveSuccesses, ObservingUntil: current.ObservingUntil, Now: now,
		Result: outcome.Result, Policy: spec.policy,
	})
	if !spec.policy.AutoDegradeEnabled {
		// 自动降级关闭：只记录探活结果，状态机不推进。
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

	if outcome.Result == ResultOK {
		next.LastSuccessAt = &now
		next.LastErrorKey = ""
		next.LastErrorDetail = ""
	} else {
		next.LastFailureAt = &now
		next.LastErrorKey = string(outcome.Result)
		next.LastErrorDetail = outcome.Detail
	}

	if err := s.repo.UpsertState(ctx, next); err != nil {
		return nil, err
	}
	return &targetProbeResult{
		state: &next, previousState: current.State, outcome: outcome, latencyMs: latencyMs, spec: spec,
		triggeredRemote: transitionOut.TriggerRemoteDegrade || transitionOut.TriggerRemoteRestore,
	}, nil
}

func probeBudgetDayStart(now time.Time) time.Time {
	// 产品当前按中国自然日展示“每日预算”，使用固定 UTC+8 避免容器运行在 UTC 时于早上 8 点重置。
	location := time.FixedZone("UTC+8", 8*60*60)
	local := now.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
}

func probeBudgetLimit(policy Policy) int {
	return defaultInt(policy.DailyProbeBudget, 1000)
}

func (s *Service) finishTargetProbeBatch(ctx context.Context, userID string, adminAccountID string, session upstream.Session, target AdminProbeTarget, specs []probeModelSpec, results []targetProbeResult) {
	if len(results) == 0 {
		return
	}
	remoteAction, actionErr := s.reconcileTargetRemoteAction(ctx, userID, adminAccountID, session, target, specs)
	if actionErr != nil {
		log.Printf("[connection-health] reconcile target action failed target_id=%s action=%s err=%v", target.TargetID, remoteAction, actionErr)
	}
	if remoteAction == "" {
		for _, result := range results {
			if result.triggeredRemote && !policyRemoteActionEnabled(result.spec.policy) {
				remoteAction = RemoteActionSkippedIndependentProbe
				break
			}
		}
	}
	if remoteAction != "" {
		actionIndex := len(results) - 1
		for index := len(results) - 1; index >= 0; index-- {
			if policyRemoteActionEnabled(results[index].spec.policy) {
				actionIndex = index
				break
			}
		}
		results[actionIndex].state.LastRemoteAction = remoteAction
		if err := s.repo.UpsertState(ctx, *results[actionIndex].state); err != nil {
			log.Printf("[connection-health] store target remote action failed target_id=%s err=%v", target.TargetID, err)
		}
		for index := range results {
			result := &results[index]
			eventTarget := targetForProbeSpec(target, result.spec)
			action := ""
			if index == actionIndex {
				action = remoteAction
			}
			s.recordTargetEvent(ctx, userID, adminAccountID, eventTarget, result.spec.policy.ID, result.spec.modelName,
				string(result.outcome.Result), string(result.previousState), string(result.state.State), &result.latencyMs,
				result.state.LastErrorKey, result.state.LastErrorDetail, action)
		}
		return
	}
	for index := range results {
		result := &results[index]
		eventTarget := targetForProbeSpec(target, result.spec)
		s.recordTargetEvent(ctx, userID, adminAccountID, eventTarget, result.spec.policy.ID, result.spec.modelName,
			string(result.outcome.Result), string(result.previousState), string(result.state.State), &result.latencyMs,
			result.state.LastErrorKey, result.state.LastErrorDetail, "")
	}
}

func targetForProbeSpec(target AdminProbeTarget, spec probeModelSpec) AdminProbeTarget {
	if !spec.eventGroupResolved {
		return target
	}
	target.AdminGroupID = spec.eventAdminGroupID
	target.AdminGroupName = spec.eventAdminGroupName
	return target
}

// defaultTargetState 构造一个目标模型的初始健康状态。connection_id 列存 targetId，
// upstream_site_id 允许为空字符串（NOT NULL 但可为空串），admin 分组信息落在 own_group_* /
// upstream_group_name 字段里，复用现有列语义。
func defaultTargetState(userID string, adminAccountID string, target AdminProbeTarget, modelName string) ConnectionHealthState {
	return ConnectionHealthState{
		ConnectionID:      target.TargetID,
		ModelName:         modelName,
		UserID:            userID,
		AdminAccountID:    adminAccountID,
		OwnGroupID:        target.AdminGroupID,
		OwnGroupName:      target.AdminGroupName,
		UpstreamSiteID:    "",
		UpstreamGroupID:   target.AdminGroupID,
		UpstreamGroupName: target.AdminGroupName,
		State:             StateHealthy,
		CurrentWeight:     100,
	}
}

// recordTargetEvent 写入一条独立探活事件（connection_id 列存 targetId）。error_detail 已在
// probe_runner 里脱敏，绝不含明文 key。
func (s *Service) recordTargetEvent(ctx context.Context, userID string, adminAccountID string, target AdminProbeTarget, policyID string, modelName string, result string, fromState string, toState string, latencyMs *int, errorKey string, errorDetail string, remoteAction string) {
	id, err := newID()
	if err != nil {
		log.Printf("[connection-health] generate target event id failed: %v", err)
		return
	}
	event := ConnectionHealthEvent{
		ID: id, ConnectionID: target.TargetID, ModelName: modelName, UserID: userID, AdminAccountID: adminAccountID,
		PolicyID: policyID, AdminGroupID: target.AdminGroupID,
		OwnGroupName: target.AdminGroupName, UpstreamSiteID: "", UpstreamGroupName: target.AdminGroupName, Result: result,
		FromState: fromState, ToState: toState, LatencyMs: latencyMs, ErrorKey: errorKey, ErrorDetail: errorDetail, RemoteAction: remoteAction,
	}
	if err := s.repo.InsertEvent(ctx, event); err != nil {
		log.Printf("[connection-health] insert target event failed target_id=%s err=%v", target.TargetID, err)
	}
}

// splitModelList 把逗号分隔的模型字符串拆成去空列表（连接层已有 splitModels 在 upstream 包，
// 此处提供 connection_health 包内等价实现，避免跨包耦合一个纯字符串工具）。
func splitModelList(models string) []string {
	if strings.TrimSpace(models) == "" {
		return nil
	}
	parts := strings.Split(models, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
