package connection_health

import (
	"context"
	"log"
	"sync"
	"time"

	"transithub/backend/internal/modules/upstream"
)

const (
	schedulerTickInterval   = 30 * time.Second
	maxJobsPerTick          = 100
	globalProbeConcurrency  = 5
	perSiteProbeConcurrency = 2
)

// adminProbeJob 是调度器一轮扫描出的、针对一个独立探活目标的到期任务集合。
// 一个目标下可能有多个到期模型，共用一次凭据解析（避免重复命中受保护的 key 接口）。
type adminProbeJob struct {
	userID         string
	adminAccountID string
	session        upstream.Session
	target         AdminProbeTarget
	account        upstream.AdminGroupAccountInfo
	dueSpecs       []probeModelSpec
}

// StartScheduler 启动后台探活调度：立即跑一次，之后每 30s 一次。tick 和每个探活 goroutine
// 都有独立的 panic recover，任意一次探活失败或 panic 都不能影响调度器持续运行。
func (s *Service) StartScheduler(ctx context.Context) {
	go func() {
		s.runSchedulerTickSafely(ctx)
		ticker := time.NewTicker(schedulerTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runSchedulerTickSafely(ctx)
			}
		}
	}()
}

func (s *Service) runSchedulerTickSafely(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[connection-health] scheduler tick panic recovered: %v", r)
		}
	}()
	s.runSchedulerTick(ctx)
}

// runSchedulerTick 扫描全部已启用策略 + 全部策略分配关系，按 workspace 生成独立 admin 探活
// 目标，挑出到期的 (target, model) 任务并发探活。未显式分配策略的 target 直接跳过（见
// collectAdminProbeJobs）。单轮最多处理 maxJobsPerTick 个模型任务，全局并发 5，单 workspace 并发 2。
func (s *Service) runSchedulerTick(ctx context.Context) {
	if s.platformGroups == nil {
		return
	}
	policies, err := s.repo.ListEnabledPolicies(ctx)
	if err != nil {
		log.Printf("[connection-health] scheduler list policies failed: %v", err)
		return
	}
	if len(policies) == 0 {
		return
	}
	assignments, err := s.repo.ListAllPolicyAssignments(ctx)
	if err != nil {
		log.Printf("[connection-health] scheduler list policy assignments failed: %v", err)
		return
	}
	if len(assignments) == 0 {
		// 没有任何账号/channel 被显式分配过策略：不自动探活任何目标。
		return
	}

	jobs := s.collectAdminProbeJobs(ctx, policies, assignments)
	if len(jobs) == 0 {
		return
	}

	globalSem := make(chan struct{}, globalProbeConcurrency)
	workspaceSemaphores := make(map[string]chan struct{})
	var wg sync.WaitGroup

	for _, j := range jobs {
		wsKey := j.userID + "|" + j.adminAccountID
		wsSem, ok := workspaceSemaphores[wsKey]
		if !ok {
			wsSem = make(chan struct{}, perSiteProbeConcurrency)
			workspaceSemaphores[wsKey] = wsSem
		}

		wg.Add(1)
		globalSem <- struct{}{}
		wsSem <- struct{}{}
		go s.runAdminProbeJob(ctx, j, globalSem, wsSem, &wg)
	}
	wg.Wait()
}

// runAdminProbeJob 处理单个目标的到期任务：先解析一次凭据；凭据不可用时对每个到期模型记录
// 一次「不可探活」事件并回填 last_probe_at 退避（不驱动状态机、不计入探活预算），
// 凭据可用时逐个模型执行独立探活。
func (s *Service) runAdminProbeJob(ctx context.Context, j adminProbeJob, globalSem chan struct{}, wsSem chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() { <-wsSem; <-globalSem }()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[connection-health] admin probe goroutine panic recovered target_id=%s: %v", j.target.TargetID, r)
		}
	}()

	cred, err := s.platformGroups.ResolveProbeCredential(j.session, j.account)
	if err != nil {
		reason := upstream.ProbeCredentialReason(err)
		s.recordTargetCredentialUnavailable(ctx, j.userID, j.adminAccountID, j.target, j.dueSpecs, reason)
		return
	}
	for _, spec := range j.dueSpecs {
		if _, err := s.probeTargetOnce(ctx, j.userID, j.adminAccountID, j.session, j.target, cred, spec); err != nil {
			log.Printf("[connection-health] scheduled target probe failed target_id=%s model=%s err=%v", j.target.TargetID, spec.modelName, err)
		}
	}
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
		next.LastProbeAt = &now
		next.LastErrorKey = reason
		next.LastErrorDetail = ""
		next.LastRemoteAction = ""
		if err := s.repo.UpsertState(ctx, next); err != nil {
			log.Printf("[connection-health] upsert unavailable target state failed target_id=%s model=%s err=%v", target.TargetID, spec.modelName, err)
			continue
		}
		s.recordTargetEvent(ctx, userID, adminAccountID, target, spec.modelName, string(ResultUnsupported), string(next.State), string(next.State), nil, reason, "", "")
	}
}

// collectAdminProbeJobs 按 workspace 生成独立探活目标，并挑出到期的 (target, model) 任务。
// 调度器用 context.Background() 启动，没有请求态「当前 workspace」，必须用策略自带的
// userID + adminAccountID 复合键读取会话与分组，缓存也用复合键，避免多 workspace 串台。
//
// assignments 是全部 workspace 的「target 显式分配策略」关系：只有分配了至少一条已启用策略的
// target 才会被本函数处理；未分配的 target 不解析凭据、不计入 dueSpecs、不生成任何 job。
func (s *Service) collectAdminProbeJobs(ctx context.Context, policies []Policy, assignments []PolicyAssignment) []adminProbeJob {
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

	jobs := make([]adminProbeJob, 0, maxJobsPerTick)
	now := time.Now()
	modelBudget := maxJobsPerTick

	for _, key := range order {
		if modelBudget <= 0 {
			break
		}
		ws := byWorkspace[key]
		// 该 workspace 下没有任何 target 被分配过策略：直接跳过，不建 session、不拉分组/账号，
		// 避免为完全没有分配关系的 workspace 发起任何上游调用。
		assignedTargets := assignedByWorkspace[key]
		if len(assignedTargets) == 0 {
			continue
		}
		// 若该 workspace 的策略没有任何启用的模型目标，直接跳过，避免无谓地拉取分组/账号。
		if !hasEnabledModelTarget(ws.policies) {
			continue
		}
		session, err := s.mySites.RequireSession(ctx, ws.userID, ws.adminAccountID)
		if err != nil {
			log.Printf("[connection-health] scheduler require session failed user_id=%s admin_account_id=%s err=%v", ws.userID, ws.adminAccountID, err)
			continue
		}
		platform := string(session.Platform)
		groups, err := s.platformGroups.FetchAdminAllGroups(session)
		if err != nil {
			log.Printf("[connection-health] scheduler fetch admin groups failed user_id=%s admin_account_id=%s err=%v", ws.userID, ws.adminAccountID, err)
			continue
		}

		for _, group := range groups {
			if modelBudget <= 0 {
				break
			}
			accounts, accErr := s.platformGroups.ListAdminGroupAccounts(session, group)
			if accErr != nil {
				log.Printf("[connection-health] scheduler list accounts failed group_id=%s err=%v", group.ID, accErr)
				continue
			}
			for _, acc := range accounts {
				if modelBudget <= 0 {
					break
				}
				target := AdminProbeTarget{
					TargetID:       buildTargetID(platform, ws.adminAccountID, acc.ID),
					Platform:       platform,
					AdminGroupID:   group.ID,
					AdminGroupName: group.Name,
					AccountID:      acc.ID,
					AccountName:    acc.Name,
					AccountStatus:  acc.Status,
					ProviderFamily: acc.Platform,
					Models:         splitModelList(acc.Models),
				}
				// target 没有被分配任何已启用策略：跳过，不解析凭据、不调用 /v1/models、不探活、不写事件。
				assignedPolicies := assignedTargets[target.TargetID]
				if len(assignedPolicies) == 0 {
					continue
				}
				specs := candidateModelSpecs(target.Models, assignedPolicies)
				available, _ := targetProbeAvailability(platform, acc.BaseURL, len(specs))
				if !available {
					continue
				}
				dueSpecs := make([]probeModelSpec, 0, len(specs))
				for _, spec := range specs {
					if modelBudget <= 0 {
						break
					}
					if !s.isDue(ctx, target.TargetID, spec.modelName, spec.policy, now) {
						continue
					}
					dueSpecs = append(dueSpecs, spec)
					modelBudget--
				}
				if len(dueSpecs) > 0 {
					jobs = append(jobs, adminProbeJob{
						userID: ws.userID, adminAccountID: ws.adminAccountID, session: session,
						target: target, account: acc, dueSpecs: dueSpecs,
					})
				}
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
	state, err := s.repo.GetState(ctx, targetID, modelName)
	if err != nil {
		log.Printf("[connection-health] get state failed target_id=%s model=%s err=%v", targetID, modelName, err)
		return false
	}
	if state == nil {
		return true
	}
	if state.State == StateDisabled {
		return false
	}
	if state.CooldownUntil != nil && now.Before(*state.CooldownUntil) {
		return false
	}
	if state.LastProbeAt == nil {
		return true
	}

	interval := time.Duration(policy.ProbeIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if backoff := ProbeBackoff(state.ConsecutiveFailures); backoff > interval {
		interval = backoff
	}
	return now.Sub(*state.LastProbeAt) >= interval
}
