package connection_health

import (
	"context"
	"log"
	"sync"
	"time"

	"transithub/backend/internal/modules/upstream"
)

const (
	probeSchedulerScanInterval = 30 * time.Second
	maxJobsPerTick             = 100
	globalProbeConcurrency     = 5
	perSiteProbeConcurrency    = 2
)

// adminProbeJob 是调度器一轮扫描出的、针对一个独立探活目标的到期任务集合。
// 一个目标下可能有多个到期模型，共用一次凭据解析（避免重复命中受保护的 key 接口）。
type adminProbeJob struct {
	userID             string
	adminAccountID     string
	session            upstream.Session
	target             AdminProbeTarget
	account            upstream.AdminGroupAccountInfo
	memberships        []adminTargetMembership
	snapshotBacked     bool
	snapshotFetchedAt  time.Time
	snapshotExpiresAt  time.Time
	snapshotGeneration uint64
	models             []probeModelSpec
	dueSpecs           []probeModelSpec
}

type probePolicyEventGroup struct {
	resolved       bool
	adminGroupID   string
	adminGroupName string
}

type adminInventoryGroup struct {
	group    upstream.AdminGroupInfo
	accounts []upstream.AdminGroupAccountInfo
	err      error
}

type adminWorkspaceInventory struct {
	session                upstream.Session
	groups                 []adminInventoryGroup
	complete               bool
	multiplierLookup       upstreamMultiplierLookup
	multiplierLookupLoaded bool
}

type adminInventoryCacheEntry struct {
	inventory          *adminWorkspaceInventory
	err                error
	snapshot           bool
	snapshotFetchedAt  time.Time
	snapshotExpiresAt  time.Time
	snapshotGeneration uint64
}

type adminInventoryCache map[string]adminInventoryCacheEntry

func (s *Service) fetchAdminAllGroups(ctx context.Context, session upstream.Session) ([]upstream.AdminGroupInfo, error) {
	if reader, ok := s.platformGroups.(PlatformGroupContextReader); ok {
		return reader.FetchAdminAllGroupsContext(ctx, session)
	}
	return s.platformGroups.FetchAdminAllGroups(session)
}

func (s *Service) listAdminGroupAccounts(ctx context.Context, session upstream.Session, group upstream.AdminGroupInfo) ([]upstream.AdminGroupAccountInfo, error) {
	if reader, ok := s.platformGroups.(PlatformGroupContextReader); ok {
		return reader.ListAdminGroupAccountsContext(ctx, session, group)
	}
	return s.platformGroups.ListAdminGroupAccounts(session, group)
}

func (s *Service) resolveProbeCredential(ctx context.Context, session upstream.Session, account upstream.AdminGroupAccountInfo) (upstream.ProbeCredential, error) {
	if reader, ok := s.platformGroups.(PlatformProbeCredentialContextReader); ok {
		return reader.ResolveProbeCredentialContext(ctx, session, account)
	}
	return s.platformGroups.ResolveProbeCredential(session, account)
}

func (s *Service) loadAdminInventory(ctx context.Context, userID string, adminAccountID string, cache adminInventoryCache) (*adminWorkspaceInventory, error) {
	key := userID + "|" + adminAccountID
	if cached, ok := cache[key]; ok {
		if cached.snapshot {
			current, valid := s.getAdminInventorySnapshot(userID, adminAccountID, s.schedulerNow())
			if !valid || current.generation != cached.snapshotGeneration ||
				!current.fetchedAt.Equal(cached.snapshotFetchedAt) || !s.schedulerNow().Before(cached.snapshotExpiresAt) {
				cache[key] = adminInventoryCacheEntry{err: errInventorySnapshotUnavailable}
				return nil, errInventorySnapshotUnavailable
			}
		}
		return cached.inventory, cached.err
	}
	session, err := s.mySites.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		cache[key] = adminInventoryCacheEntry{err: err}
		return nil, err
	}
	groups, err := s.fetchAdminAllGroups(ctx, session)
	if err != nil {
		cache[key] = adminInventoryCacheEntry{err: err}
		return nil, err
	}
	inventory := &adminWorkspaceInventory{session: session, groups: make([]adminInventoryGroup, 0, len(groups)), complete: true}
	for _, group := range groups {
		accounts, accountsErr := s.listAdminGroupAccounts(ctx, session, group)
		if accountsErr != nil {
			inventory.complete = false
		}
		inventory.groups = append(inventory.groups, adminInventoryGroup{group: group, accounts: accounts, err: accountsErr})
	}
	cache[key] = adminInventoryCacheEntry{inventory: inventory}
	return inventory, nil
}

// StartScheduler starts the independent C-phase inventory reconcile, priority writeback and
// probe loops. A successful initial reconcile wakes the first probe scan.
func (s *Service) StartScheduler(ctx context.Context) {
	s.startFrequencySchedulers(ctx)
}

func (s *Service) runSchedulerTickSafely(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[connection-health] scheduler tick panic recovered: %v", r)
		}
	}()
	release, acquired, err := s.repo.TryAcquireSchedulerLease(ctx)
	if err != nil {
		log.Printf("[connection-health] acquire scheduler lease failed: %v", err)
		return
	}
	if !acquired {
		return
	}
	defer release()
	s.runSchedulerTick(ctx)
}

// runSchedulerTick 扫描全部已启用策略、旧版 target 分配和新版 admin 分组分配，按 workspace
// 生成独立探活目标。分组新增的账号/渠道会在下一轮扫描时自动继承，无需写入额外 target 行。
func (s *Service) runSchedulerTick(ctx context.Context) {
	if s.platformGroups == nil {
		return
	}
	policies, err := s.repo.ListEnabledPolicies(ctx)
	if err != nil {
		log.Printf("[connection-health] scheduler list policies failed: %v", err)
		return
	}
	assignments, err := s.repo.ListAllPolicyAssignments(ctx)
	if err != nil {
		log.Printf("[connection-health] scheduler list policy assignments failed: %v", err)
		return
	}
	groupAssignments, err := s.repo.ListAllGroupPolicyAssignments(ctx)
	if err != nil {
		log.Printf("[connection-health] scheduler list group policy assignments failed: %v", err)
		return
	}
	exclusions, err := s.repo.ListAllGroupTargetExclusions(ctx)
	if err != nil {
		log.Printf("[connection-health] scheduler list group target exclusions failed: %v", err)
		return
	}
	if len(assignments) == 0 && len(groupAssignments) == 0 {
		// 没有任何显式或分组分配：不解析凭据、不探活、不修改优先级。
		return
	}

	// C-phase probes consume only a valid reconcile snapshot. A missing or failed snapshot must
	// never fall through to a probe-triggered upstream inventory read.
	inventoryCache := s.inventoryCacheForSchedulerInputs(policies, assignments, groupAssignments)
	if len(policies) == 0 {
		return
	}
	jobs := s.collectAdminProbeJobsWithGroupsAndCache(ctx, policies, assignments, groupAssignments, exclusions, inventoryCache)
	if len(jobs) == 0 {
		return
	}

	var wg sync.WaitGroup
	type probedWorkspace struct {
		userID         string
		adminAccountID string
	}
	probedWorkspaces := make([]probedWorkspace, 0)
	seenWorkspaces := make(map[string]struct{})

	for _, j := range jobs {
		wsKey := j.userID + "|" + j.adminAccountID
		releaseSlot, acquired := s.sharedProbeLimiter().acquire(ctx, wsKey, true)
		if !acquired {
			break
		}
		if _, seen := seenWorkspaces[wsKey]; !seen {
			seenWorkspaces[wsKey] = struct{}{}
			probedWorkspaces = append(probedWorkspaces, probedWorkspace{userID: j.userID, adminAccountID: j.adminAccountID})
		}

		wg.Add(1)
		go s.runAdminProbeJob(ctx, j, releaseSlot, &wg)
	}
	wg.Wait()
	// 探活只更新本地排序签名和最新 pending。独立写回循环决定何时触碰主站。
	for _, workspace := range probedWorkspaces {
		s.evaluateCurrentWorkspacePriorities(ctx, workspace.userID, workspace.adminAccountID, priorityActionProbe)
	}
}

// runAdminProbeJob 处理单个目标的到期任务：先解析一次凭据；凭据不可用时对每个到期模型记录
// 一次「不可探活」事件并回填 last_probe_at 退避（不驱动状态机、不计入探活预算），
// 凭据可用时逐个模型执行独立探活。
func (s *Service) runAdminProbeJob(ctx context.Context, j adminProbeJob, releaseSlot func(), wg *sync.WaitGroup) {
	defer wg.Done()
	defer releaseSlot()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[connection-health] admin probe goroutine panic recovered target_id=%s: %v", j.target.TargetID, r)
		}
	}()
	release, err := s.repo.AcquireTargetLease(ctx, j.target.TargetID)
	if err != nil {
		log.Printf("[connection-health] acquire target lease failed target_id=%s err=%v", j.target.TargetID, err)
		return
	}
	defer release()
	jobCtx := ctx
	if j.snapshotBacked {
		now := s.schedulerNow()
		current, valid := s.getAdminInventorySnapshot(j.userID, j.adminAccountID, now)
		if !valid || current.generation != j.snapshotGeneration || !current.fetchedAt.Equal(j.snapshotFetchedAt) ||
			!current.expiresAt.Equal(j.snapshotExpiresAt) || !now.Before(j.snapshotExpiresAt) {
			return
		}
		jobCtx = current.ctx
		if err := jobCtx.Err(); err != nil {
			return
		}
	}
	currentSpecs, queuedSpecs, policyOK := s.currentScheduledProbeSpecs(jobCtx, j.userID, j.adminAccountID, j.target, j.memberships, j.dueSpecs)
	if !policyOK {
		log.Printf("[connection-health] refresh scheduled policy decision failed target_id=%s", j.target.TargetID)
		return
	}
	j.models = currentSpecs
	j.dueSpecs = s.recheckAdminProbeSpecs(jobCtx, j.userID, j.adminAccountID, j.target, queuedSpecs, time.Now().UTC())
	if len(j.dueSpecs) == 0 {
		return
	}
	if err := jobCtx.Err(); err != nil {
		return
	}

	cred, err := s.resolveProbeCredential(jobCtx, j.session, j.account)
	if err != nil {
		reason := upstream.ProbeCredentialReason(err)
		if jobCtx.Err() == nil {
			s.recordTargetCredentialUnavailable(jobCtx, j.userID, j.adminAccountID, j.target, j.dueSpecs, reason)
		}
		return
	}
	results := make([]targetProbeResult, 0, len(j.dueSpecs))
	for _, spec := range j.dueSpecs {
		if err := jobCtx.Err(); err != nil {
			return
		}
		result, err := s.probeTargetOnce(jobCtx, j.userID, j.adminAccountID, j.target, cred, spec, true)
		if err != nil {
			log.Printf("[connection-health] scheduled target probe failed target_id=%s model=%s err=%v", j.target.TargetID, spec.modelName, err)
			continue
		}
		if result != nil {
			results = append(results, *result)
		}
	}
	if err := jobCtx.Err(); err != nil {
		return
	}
	if err := s.finishTargetProbeBatch(jobCtx, j.userID, j.adminAccountID, j.session, j.target, j.models, results, EventSourceScheduled); err != nil {
		log.Printf("[connection-health] finish scheduled target probe failed target_id=%s err=%v", j.target.TargetID, err)
	}
}

// recheckAdminProbeSpecs closes the gap between scheduler collection and job execution:
// the target lease protects in-process user actions, while this decision pass consumes the
// fresh schedulable value, current state and budget owner immediately before any probe request.
func (s *Service) recheckAdminProbeSpecs(ctx context.Context, userID string, adminAccountID string, target AdminProbeTarget, specs []probeModelSpec, now time.Time) []probeModelSpec {
	if len(specs) == 0 {
		return nil
	}
	budgetUsage := make(map[string]int)
	budgetLoaded := make(map[string]bool)
	due := make([]probeModelSpec, 0, len(specs))
	for _, spec := range specs {
		policies := spec.policies
		if len(policies) == 0 {
			policies = []Policy{spec.policy}
		}
		decision, stateOK := s.effectiveProbeDecisionForSpec(ctx, target, spec, policies, now, nil)
		if !stateOK || !decision.ContinueAutoProbe || decision.NextProbeAt == nil || now.Before(*decision.NextProbeAt) {
			continue
		}
		specBudgetUsage := make(map[string]int)
		budgetReady := true
		for _, sourcePolicy := range policies {
			continueAutoProbe, _ := policyProbeCadence(sourcePolicy, target.Schedulable)
			if !continueAutoProbe {
				continue
			}
			if !budgetLoaded[sourcePolicy.ID] {
				count, err := s.repo.CountProbesToday(ctx, userID, adminAccountID, sourcePolicy.ID, probeBudgetDayStart(now))
				if err != nil {
					budgetReady = false
					break
				}
				budgetUsage[sourcePolicy.ID] = count
				budgetLoaded[sourcePolicy.ID] = true
			}
			specBudgetUsage[sourcePolicy.ID] = budgetUsage[sourcePolicy.ID]
		}
		if !budgetReady {
			continue
		}
		decision, stateOK = s.effectiveProbeDecisionForSpec(ctx, target, spec, policies, now, specBudgetUsage)
		if !stateOK || !decision.ContinueAutoProbe || decision.NextProbeAt == nil || now.Before(*decision.NextProbeAt) {
			continue
		}
		budgetPolicy, found := policyWithID(policies, decision.BudgetPolicyID)
		if !found {
			continue
		}
		spec.budgetPolicy = budgetPolicy
		due = append(due, spec)
		budgetUsage[budgetPolicy.ID]++
	}
	return due
}

func (s *Service) currentScheduledProbeSpecs(ctx context.Context, userID string, adminAccountID string, target AdminProbeTarget, memberships []adminTargetMembership, queued []probeModelSpec) ([]probeModelSpec, []probeModelSpec, bool) {
	policies, err := s.repo.ListPolicies(ctx, userID, adminAccountID)
	if err != nil {
		return nil, nil, false
	}
	directAssignments, err := s.repo.ListPolicyAssignmentsForTarget(ctx, userID, adminAccountID, target.TargetID)
	if err != nil {
		return nil, nil, false
	}
	groupAssignments, err := s.repo.ListGroupPolicyAssignmentsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, nil, false
	}
	exclusions, err := s.repo.ListGroupTargetExclusionsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return nil, nil, false
	}

	policyByID := make(map[string]Policy, len(policies))
	for _, policy := range policies {
		policyByID[policy.ID] = policy
	}
	policySources := make(map[string]probePolicyEventGroup)
	effectivePolicies := make([]Policy, 0)
	for _, assignment := range directAssignments {
		policy, exists := policyByID[assignment.PolicyID]
		if !exists {
			continue
		}
		effectivePolicies = mergePoliciesByID(effectivePolicies, []Policy{policy})
		policySources[policy.ID] = probePolicyEventGroup{resolved: true}
	}
	excluded := groupTargetExclusionIndex(exclusions)[userID+"|"+adminAccountID]
	for _, membership := range memberships {
		if excluded[membership.groupID][target.TargetID] {
			continue
		}
		for _, assignment := range groupAssignments {
			if assignment.AdminGroupID != membership.groupID {
				continue
			}
			policy, exists := policyByID[assignment.PolicyID]
			if !exists {
				continue
			}
			effectivePolicies = mergePoliciesByID(effectivePolicies, []Policy{policy})
			if _, resolved := policySources[policy.ID]; !resolved {
				policySources[policy.ID] = probePolicyEventGroup{
					resolved: true, adminGroupID: membership.groupID, adminGroupName: membership.groupName,
				}
			}
		}
	}

	allSpecs := candidateModelSpecsForPlatform(target.Models, effectivePolicies, target.Platform)
	for index := range allSpecs {
		allSpecs[index].policySources = policySources
		if source, exists := policySources[allSpecs[index].policy.ID]; exists {
			allSpecs[index].eventGroupResolved = source.resolved
			allSpecs[index].eventAdminGroupID = source.adminGroupID
			allSpecs[index].eventAdminGroupName = source.adminGroupName
		}
	}
	queuedModels := make(map[string]struct{}, len(queued))
	for _, spec := range queued {
		queuedModels[spec.modelName] = struct{}{}
	}
	queuedNow := make([]probeModelSpec, 0, len(allSpecs))
	for _, spec := range allSpecs {
		if _, exists := queuedModels[spec.modelName]; exists {
			queuedNow = append(queuedNow, spec)
		}
	}
	return allSpecs, queuedNow, true
}

// recordTargetCredentialUnavailable 在凭据解析失败时，对每个到期模型回填 last_probe_at（按探活
// 间隔退避，避免每 30s 反复命中受保护的 key/导出接口）并记录一条 unsupported 事件，
// 事件 error_key 为脱敏 reason。不驱动状态机、不计入探活预算。
func (s *Service) recordTargetCredentialUnavailable(ctx context.Context, userID string, adminAccountID string, target AdminProbeTarget, specs []probeModelSpec, reason string) {
	now := time.Now()
	for _, spec := range specs {
		current, err := s.repo.GetState(ctx, target.TargetID, spec.modelName)
		if err != nil {
			log.Printf("[connection-health] get target state failed target_id=%s model=%s err=%v", target.TargetID, spec.modelName, err)
			continue
		}
		var next ConnectionHealthState
		if current == nil {
			next = defaultTargetState(userID, adminAccountID, target, spec.modelName)
		} else {
			next = *current
		}
		next.UpdatedAt = now
		next.LastErrorKey = reason
		next.LastErrorDetail = ""
		// LastRemoteAction 也是旧版本判断「该上游状态是否由健康模块接管」的兼容证据。
		// 凭据暂时不可用只更新探活错误，不能抹掉此前成功执行的远端动作。
		if err := s.repo.UpsertState(ctx, next); err != nil {
			log.Printf("[connection-health] upsert unavailable target state failed target_id=%s model=%s err=%v", target.TargetID, spec.modelName, err)
			continue
		}
		eventTarget := targetForProbeSpec(target, spec)
		if err := s.recordTargetEvent(ctx, userID, adminAccountID, eventTarget, spec.effectiveBudgetPolicy().ID, spec.modelName, string(ResultUnsupported), string(next.State), string(next.State), nil, reason, "", "", EventSourceScheduled); err != nil {
			log.Printf("[connection-health] insert unavailable target event failed target_id=%s model=%s err=%v", target.TargetID, spec.modelName, err)
		}
	}
}

// collectAdminProbeJobs 按 workspace 生成独立探活目标，并挑出到期的 (target, model) 任务。
// 调度器用 context.Background() 启动，没有请求态「当前 workspace」，必须用策略自带的
// userID + adminAccountID 复合键读取会话与分组，缓存也用复合键，避免多 workspace 串台。
//
// assignments 是全部 workspace 的「target 显式分配策略」关系：只有分配了至少一条已启用策略的
// target 才会被本函数处理；未分配的 target 不解析凭据、不计入 dueSpecs、不生成任何 job。
func (s *Service) collectAdminProbeJobs(ctx context.Context, policies []Policy, assignments []PolicyAssignment) []adminProbeJob {
	return s.collectAdminProbeJobsWithGroups(ctx, policies, assignments, nil, nil)
}

func (s *Service) collectAdminProbeJobsWithGroups(ctx context.Context, policies []Policy, assignments []PolicyAssignment, groupAssignments []GroupPolicyAssignment, exclusions []GroupTargetExclusion) []adminProbeJob {
	return s.collectAdminProbeJobsWithGroupsAndCache(ctx, policies, assignments, groupAssignments, exclusions, make(adminInventoryCache))
}

func (s *Service) collectAdminProbeJobsWithGroupsAndCache(ctx context.Context, policies []Policy, assignments []PolicyAssignment, groupAssignments []GroupPolicyAssignment, exclusions []GroupTargetExclusion, inventoryCache adminInventoryCache) []adminProbeJob {
	// 按 workspace 归拢策略。
	type workspace struct {
		userID         string
		adminAccountID string
		policies       []Policy
	}
	order := make([]string, 0)
	byWorkspace := make(map[string]*workspace)
	for _, p := range policies {
		key := p.UserID + "|" + p.AdminAccountID
		ws, ok := byWorkspace[key]
		if !ok {
			ws = &workspace{userID: p.UserID, adminAccountID: p.AdminAccountID}
			byWorkspace[key] = ws
			order = append(order, key)
		}
		ws.policies = append(ws.policies, p)
	}

	// assignedByWorkspace: wsKey -> targetId -> 该 target 已分配且已启用的策略列表。
	// 分配指向的策略如果已被禁用/删除（不在 policies/policyByID 中），对应分配行会被忽略，
	// 相当于该 target 暂时没有生效的分配。
	assignedByWorkspace := assignedEnabledPoliciesByTarget(policies, assignments)
	assignedGroupsByWorkspace := assignedEnabledPoliciesByGroup(policies, groupAssignments)
	excludedByWorkspace := groupTargetExclusionIndex(exclusions)

	jobs := make([]adminProbeJob, 0, maxJobsPerTick)
	now := time.Now()
	modelBudget := maxJobsPerTick
	budgetUsage := make(map[string]int)
	budgetLoaded := make(map[string]bool)
	dayStart := probeBudgetDayStart(time.Now())

	for _, key := range order {
		if modelBudget <= 0 {
			break
		}
		ws := byWorkspace[key]
		// 该 workspace 下没有任何 target 被分配过策略：直接跳过，不建 session、不拉分组/账号，
		// 避免为完全没有分配关系的 workspace 发起任何上游调用。
		assignedTargets := assignedByWorkspace[key]
		assignedGroups := assignedGroupsByWorkspace[key]
		if len(assignedTargets) == 0 && len(assignedGroups) == 0 {
			continue
		}
		// 若该 workspace 的策略没有任何启用的模型目标，直接跳过，避免无谓地拉取分组/账号。
		if !hasEnabledModelTarget(ws.policies) {
			continue
		}
		inventory, err := s.loadAdminInventory(ctx, ws.userID, ws.adminAccountID, inventoryCache)
		if err != nil {
			log.Printf("[connection-health] scheduler load admin inventory failed user_id=%s admin_account_id=%s err=%v", ws.userID, ws.adminAccountID, err)
			continue
		}
		session := inventory.session
		platform := string(session.Platform)

		// 账号/渠道可能同时属于多个 admin 分组。先按稳定 targetId 合并所有来源策略，再生成
		// 一次任务，避免同一目标在一轮中被重复探活。
		type targetCandidate struct {
			target        AdminProbeTarget
			account       upstream.AdminGroupAccountInfo
			memberships   []adminTargetMembership
			membershipIDs map[string]struct{}
			policies      []Policy
			policySources map[string]probePolicyEventGroup
		}
		candidates := make(map[string]*targetCandidate)
		targetOrder := make([]string, 0)
		for _, groupInventory := range inventory.groups {
			if modelBudget <= 0 {
				break
			}
			group := groupInventory.group
			if groupInventory.err != nil {
				log.Printf("[connection-health] scheduler list accounts failed group_id=%s err=%v", group.ID, groupInventory.err)
				continue
			}
			for _, acc := range groupInventory.accounts {
				target := AdminProbeTarget{
					TargetID:       buildTargetID(platform, ws.adminAccountID, acc.ID),
					Platform:       platform,
					AdminGroupID:   group.ID,
					AdminGroupName: group.Name,
					AccountID:      acc.ID,
					AccountName:    acc.Name,
					AccountStatus:  acc.Status,
					Schedulable:    cloneBoolPointer(acc.Schedulable),
					AccountWeight:  cloneIntPointer(acc.Weight),
					ProviderFamily: acc.Platform,
					Models:         splitModelList(acc.Models),
				}
				inheritedPolicies := assignedGroups[group.ID]
				if excludedByWorkspace[key][group.ID][target.TargetID] {
					inheritedPolicies = nil
				}
				effectivePolicies := mergePoliciesByID(assignedTargets[target.TargetID], inheritedPolicies)
				if len(effectivePolicies) == 0 {
					continue
				}
				candidate, exists := candidates[target.TargetID]
				if !exists {
					candidate = &targetCandidate{
						target: target, account: acc, membershipIDs: make(map[string]struct{}), policySources: make(map[string]probePolicyEventGroup),
					}
					candidates[target.TargetID] = candidate
					targetOrder = append(targetOrder, target.TargetID)
				}
				if _, seenMembership := candidate.membershipIDs[group.ID]; !seenMembership {
					candidate.membershipIDs[group.ID] = struct{}{}
					candidate.memberships = append(candidate.memberships, adminTargetMembership{groupID: group.ID, groupName: group.Name})
				}
				for _, policy := range assignedTargets[target.TargetID] {
					// An explicit target assignment has no single group owner, even when the
					// target is currently being enumerated through a group membership.
					candidate.policySources[policy.ID] = probePolicyEventGroup{resolved: true}
				}
				for _, policy := range inheritedPolicies {
					if _, alreadyResolved := candidate.policySources[policy.ID]; alreadyResolved {
						continue
					}
					candidate.policySources[policy.ID] = probePolicyEventGroup{
						resolved: true, adminGroupID: group.ID, adminGroupName: group.Name,
					}
				}
				candidate.policies = mergePoliciesByID(candidate.policies, effectivePolicies)
			}
		}

		for _, targetID := range targetOrder {
			if modelBudget <= 0 {
				break
			}
			candidate := candidates[targetID]
			specs := candidateModelSpecsForPlatform(candidate.target.Models, candidate.policies, candidate.target.Platform)
			for index := range specs {
				specs[index].policySources = candidate.policySources
				if source, exists := candidate.policySources[specs[index].policy.ID]; exists {
					specs[index].eventGroupResolved = source.resolved
					specs[index].eventAdminGroupID = source.adminGroupID
					specs[index].eventAdminGroupName = source.adminGroupName
				}
			}
			available, _ := targetProbeAvailability(platform, candidate.account.BaseURL, len(specs))
			if !available {
				continue
			}
			dueSpecs := make([]probeModelSpec, 0, len(specs))
			for _, spec := range specs {
				if modelBudget <= 0 {
					break
				}
				decision, stateOK := s.effectiveProbeDecisionForSpec(ctx, candidate.target, spec, spec.policies, now, nil)
				if !stateOK || !decision.ContinueAutoProbe || decision.NextProbeAt == nil || now.Before(*decision.NextProbeAt) {
					continue
				}
				specBudgetUsage := make(map[string]int)
				budgetReady := true
				for _, sourcePolicy := range spec.policies {
					continueAutoProbe, _ := policyProbeCadence(sourcePolicy, candidate.target.Schedulable)
					if !continueAutoProbe {
						continue
					}
					budgetKey := ws.userID + "|" + ws.adminAccountID + "|" + sourcePolicy.ID
					if !budgetLoaded[budgetKey] {
						count, countErr := s.repo.CountProbesToday(ctx, ws.userID, ws.adminAccountID, sourcePolicy.ID, dayStart)
						if countErr != nil {
							log.Printf("[connection-health] count policy probe budget failed policy_id=%s err=%v", sourcePolicy.ID, countErr)
							budgetReady = false
							break
						}
						budgetUsage[budgetKey] = count
						budgetLoaded[budgetKey] = true
					}
					specBudgetUsage[sourcePolicy.ID] = budgetUsage[budgetKey]
				}
				if !budgetReady {
					continue
				}
				decision, stateOK = s.effectiveProbeDecisionForSpec(ctx, candidate.target, spec, spec.policies, now, specBudgetUsage)
				if !stateOK || decision.NextProbeAt == nil || now.Before(*decision.NextProbeAt) {
					continue
				}
				budgetPolicy, found := policyWithID(spec.policies, decision.BudgetPolicyID)
				if !found {
					continue
				}
				spec.budgetPolicy = budgetPolicy
				budgetKey := ws.userID + "|" + ws.adminAccountID + "|" + budgetPolicy.ID
				dueSpecs = append(dueSpecs, spec)
				budgetUsage[budgetKey]++
				modelBudget--
			}
			if len(dueSpecs) > 0 {
				cacheEntry := inventoryCache[ws.userID+"|"+ws.adminAccountID]
				jobs = append(jobs, adminProbeJob{
					userID: ws.userID, adminAccountID: ws.adminAccountID, session: session,
					target: candidate.target, account: candidate.account, memberships: candidate.memberships,
					snapshotBacked: cacheEntry.snapshot, snapshotFetchedAt: cacheEntry.snapshotFetchedAt,
					snapshotExpiresAt: cacheEntry.snapshotExpiresAt, snapshotGeneration: cacheEntry.snapshotGeneration,
					models: specs, dueSpecs: dueSpecs,
				})
			}
		}
	}
	return jobs
}

// hasEnabledModelTarget 判断一组策略里是否存在至少一个启用策略下的启用模型目标。
func hasEnabledModelTarget(policies []Policy) bool {
	for _, p := range policies {
		if !p.Enabled {
			continue
		}
		for _, t := range p.ModelTargets {
			if t.Enabled {
				return true
			}
		}
	}
	return false
}

// isDue 判断某个 (targetId, model) 组合当前是否到期需要探活。
// 从未探活过立即探活；disabled 状态永不自动探活；cooldown_until 未到不探活；
// 探活间隔在策略配置的基础上，按连续失败次数叠加 2/5/10 分钟退避。
func (s *Service) isDue(ctx context.Context, targetID string, modelName string, policy Policy, now time.Time) bool {
	decision, ok := s.effectiveProbeDecision(ctx, targetID, modelName, []Policy{policy}, nil, now)
	return ok && decision.ContinueAutoProbe && decision.NextProbeAt != nil && !now.Before(*decision.NextProbeAt)
}

func (s *Service) effectiveProbeDecision(ctx context.Context, targetID string, modelName string, policies []Policy, schedulable *bool, now time.Time) (EffectiveProbeDecision, bool) {
	return s.effectiveProbeDecisionWithBudgets(ctx, targetID, modelName, policies, schedulable, now, nil)
}

func (s *Service) effectiveProbeDecisionWithBudgets(ctx context.Context, targetID string, modelName string, policies []Policy, schedulable *bool, now time.Time, budgetUsage map[string]int) (EffectiveProbeDecision, bool) {
	state, err := s.repo.GetState(ctx, targetID, modelName)
	if err != nil {
		log.Printf("[connection-health] get state failed target_id=%s model=%s err=%v", targetID, modelName, err)
		return EffectiveProbeDecision{}, false
	}
	return calculateEffectiveProbeDecisionWithBudgets(policies, schedulable, state, now, budgetUsage), true
}

func (s *Service) effectiveProbeDecisionForSpec(ctx context.Context, target AdminProbeTarget, spec probeModelSpec, policies []Policy, now time.Time, budgetUsage map[string]int) (EffectiveProbeDecision, bool) {
	state, err := s.repo.GetState(ctx, target.TargetID, spec.modelName)
	if err != nil {
		log.Printf("[connection-health] get state failed target_id=%s model=%s err=%v", target.TargetID, spec.modelName, err)
		return EffectiveProbeDecision{}, false
	}
	reuseProbeInterval := probeDecisionCanReuseInterval(state, probeDecisionKey(target, spec))
	return calculateEffectiveProbeDecisionWithBudgetAndReuse(policies, target.Schedulable, state, now, budgetUsage, reuseProbeInterval), true
}

func policyWithID(policies []Policy, id string) (Policy, bool) {
	for _, policy := range policies {
		if policy.ID == id {
			return policy, true
		}
	}
	return Policy{}, false
}
