package connection_health

import (
	"context"
	"fmt"

	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/upstream"
)

// RemoteActionRunner 是自动降级/恢复对上游平台的远端动作接口，按平台类型分派到具体实现。
// 所有实现都必须是 panic-safe 的：远端调用失败绝不能让调度器崩溃。
//
// Degrade/Restore 服务旧 real_connections 对接链路路径；DegradeTarget/RestoreTarget 服务当前
// 分组健康的独立探活 targetId 路径（见 admin_targets.go 的 probeTargetOnce），不依赖
// real_connections，避免为了调用远端动作而伪造一条 RealConnection。
type RemoteActionRunner interface {
	Degrade(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (remoteAction string, err error)
	Restore(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (remoteAction string, err error)
	DegradeTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (remoteAction string, err error)
	RestoreTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (remoteAction string, err error)
}

// PlatformActioner 是 connection_health 对 upstream.PlatformService 远端降级能力的窄依赖，
// 避免直接依赖 PlatformService 的其余大量方法。
type PlatformActioner interface {
	UpdateNewAPIChannelWeightStatus(session upstream.Session, channelID string, weight int, status int) error
	// UpdateSub2APIAdminAccountStatus 切换 sub2api 转发账号的启用状态（active/inactive）。
	// 第一期远端动作只做状态开关，不做 priority 权重映射（见 dispatcher 顶部说明）。
	UpdateSub2APIAdminAccountStatus(session upstream.Session, accountID string, status string) error
}

// SessionProvider 复用 my_sites 已登录并自动刷新的 admin 会话，不重复实现登录逻辑。
type SessionProvider interface {
	RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error)
}

// RemoteActionUnsupported 是没有已验证安全接口时的统一标记，绝不发明未经证实的远端请求。
const RemoteActionUnsupported = "unsupported"

// Sub2API 账号状态切换的远端动作标记：第一期远端动作只做账号 active/inactive 开关，
// 不做 priority 权重映射——sub2api 的 priority 是调度优先级，不等同于 NewAPI 的 weight，
// 强行映射 CurrentWeight(0-100) -> priority 会改变调度语义，线上风险较高（详见任务书）。
// 后续如需要 priority 阶梯恢复，需要单独的产品规则，不在本次改造范围内。
//
// *Failed 常量专门用来和 RemoteActionUnsupported 区分开：unsupported 表示这个平台/维度本身
// 没有已验证的远端动作能力（不会尝试调用上游）；*Failed 表示 Sub2API 已支持该动作、也确实
// 发起了调用，但 UpdateSub2APIAdminAccountStatus 返回了 error（GET/PUT 失败、鉴权失败、
// 响应结构异常等）。把真实失败折叠成 unsupported 会让排查者误以为「这个平台不支持」，
// 从而错过真正的上游调用故障。
const (
	RemoteActionSub2APIStatusInactive       = "sub2api_account_status_inactive"
	RemoteActionSub2APIStatusActive         = "sub2api_account_status_active"
	RemoteActionSub2APIStatusInactiveFailed = "sub2api_account_status_inactive_failed"
	RemoteActionSub2APIStatusActiveFailed   = "sub2api_account_status_active_failed"
)

// remoteActionDispatcher 按连接所在上游站点的平台类型（new-api / sub2api）分派远端动作。
// new-api 通过 admin channel 的 weight/status 实现降级/恢复；sub2api 通过 admin account 的
// status（active/inactive）实现降级/恢复，不涉及 priority。
type remoteActionDispatcher struct {
	sites    SiteLookup
	sessions SessionProvider
	platform PlatformActioner
}

func newRemoteActionDispatcher(sites SiteLookup, sessions SessionProvider, platform PlatformActioner) *remoteActionDispatcher {
	return &remoteActionDispatcher{sites: sites, sessions: sessions, platform: platform}
}

func (d *remoteActionDispatcher) Degrade(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (remoteAction string, err error) {
	defer func() {
		if r := recover(); r != nil {
			remoteAction = RemoteActionUnsupported
			err = fmt.Errorf("remote degrade panic recovered: %v", r)
		}
	}()

	site, siteErr := d.sites.GetSite(ctx, conn.UpstreamSiteID)
	if siteErr != nil || site == nil {
		return RemoteActionUnsupported, siteErr
	}

	switch site.Platform {
	case upstream.PlatformNewAPI:
		return d.degradeNewAPI(ctx, conn)
	case upstream.PlatformSub2API:
		return d.degradeSub2API(ctx, conn)
	default:
		return RemoteActionUnsupported, nil
	}
}

func (d *remoteActionDispatcher) Restore(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (remoteAction string, err error) {
	defer func() {
		if r := recover(); r != nil {
			remoteAction = RemoteActionUnsupported
			err = fmt.Errorf("remote restore panic recovered: %v", r)
		}
	}()

	site, siteErr := d.sites.GetSite(ctx, conn.UpstreamSiteID)
	if siteErr != nil || site == nil {
		return RemoteActionUnsupported, siteErr
	}

	switch site.Platform {
	case upstream.PlatformNewAPI:
		return d.restoreNewAPI(ctx, conn, state)
	case upstream.PlatformSub2API:
		return d.restoreSub2API(ctx, conn)
	default:
		return RemoteActionUnsupported, nil
	}
}

// DegradeTarget / RestoreTarget 服务当前分组健康的独立探活 targetId 路径：不依赖
// real_connections，直接用调用方已经持有的 session + AdminProbeTarget 发起远端动作。
// NewAPI target 维度远端动作本任务不强制实现，明确返回 unsupported，不伪造 RealConnection。
func (d *remoteActionDispatcher) DegradeTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (remoteAction string, err error) {
	defer func() {
		if r := recover(); r != nil {
			remoteAction = RemoteActionUnsupported
			err = fmt.Errorf("remote degrade target panic recovered: %v", r)
		}
	}()
	if target.Platform != string(upstream.PlatformSub2API) {
		return RemoteActionUnsupported, nil
	}
	if target.AccountID == "" {
		return RemoteActionUnsupported, nil
	}
	if err := d.platform.UpdateSub2APIAdminAccountStatus(session, target.AccountID, "inactive"); err != nil {
		// 已经进入 sub2api 支持的动作分支，真的发起了调用但失败了：必须和「不支持」区分开，
		// 否则排查者会误以为 sub2api 从不支持这个动作。
		return RemoteActionSub2APIStatusInactiveFailed, err
	}
	return RemoteActionSub2APIStatusInactive, nil
}

func (d *remoteActionDispatcher) RestoreTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (remoteAction string, err error) {
	defer func() {
		if r := recover(); r != nil {
			remoteAction = RemoteActionUnsupported
			err = fmt.Errorf("remote restore target panic recovered: %v", r)
		}
	}()
	if target.Platform != string(upstream.PlatformSub2API) {
		return RemoteActionUnsupported, nil
	}
	if target.AccountID == "" {
		return RemoteActionUnsupported, nil
	}
	if err := d.platform.UpdateSub2APIAdminAccountStatus(session, target.AccountID, "active"); err != nil {
		return RemoteActionSub2APIStatusActiveFailed, err
	}
	return RemoteActionSub2APIStatusActive, nil
}

func (d *remoteActionDispatcher) degradeNewAPI(ctx context.Context, conn my_sites.RealConnection) (string, error) {
	// new-api 场景下 RealConnection.AdminAccountID 存的是创建真实对接时回查得到的 channel ID
	// （见 my_sites.Service.RealConnect），不是转发子账号 ID。
	channelID := conn.AdminAccountID
	if channelID == "" {
		return RemoteActionUnsupported, nil
	}
	session, err := d.sessions.RequireSession(ctx, conn.UserID, conn.WorkspaceAdminAccountID)
	if err != nil {
		return RemoteActionUnsupported, err
	}
	if err := d.platform.UpdateNewAPIChannelWeightStatus(session, channelID, 0, 2); err != nil {
		return RemoteActionUnsupported, err
	}
	return "newapi_channel_disabled", nil
}

func (d *remoteActionDispatcher) restoreNewAPI(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (string, error) {
	channelID := conn.AdminAccountID
	if channelID == "" {
		return RemoteActionUnsupported, nil
	}
	session, err := d.sessions.RequireSession(ctx, conn.UserID, conn.WorkspaceAdminAccountID)
	if err != nil {
		return RemoteActionUnsupported, err
	}
	weight := state.CurrentWeight
	status := 1
	if weight <= 0 {
		// 权重仍为 0 时不解除远端禁用，避免观察期误放流量。
		status = 2
	}
	if err := d.platform.UpdateNewAPIChannelWeightStatus(session, channelID, weight, status); err != nil {
		return RemoteActionUnsupported, err
	}
	return fmt.Sprintf("newapi_channel_weight_%d", weight), nil
}

// degradeSub2API / restoreSub2API 是旧 real_connections 对接链路路径下的 sub2api 远端动作：
// RealConnection.AdminAccountID 在 sub2api 场景下就是 sub2api admin account id（见
// my_sites.RealConnection 字段注释），只切换账号 active/inactive，不映射 priority。
func (d *remoteActionDispatcher) degradeSub2API(ctx context.Context, conn my_sites.RealConnection) (string, error) {
	accountID := conn.AdminAccountID
	if accountID == "" {
		return RemoteActionUnsupported, nil
	}
	session, err := d.sessions.RequireSession(ctx, conn.UserID, conn.WorkspaceAdminAccountID)
	if err != nil {
		return RemoteActionUnsupported, err
	}
	if err := d.platform.UpdateSub2APIAdminAccountStatus(session, accountID, "inactive"); err != nil {
		return RemoteActionSub2APIStatusInactiveFailed, err
	}
	return RemoteActionSub2APIStatusInactive, nil
}

func (d *remoteActionDispatcher) restoreSub2API(ctx context.Context, conn my_sites.RealConnection) (string, error) {
	accountID := conn.AdminAccountID
	if accountID == "" {
		return RemoteActionUnsupported, nil
	}
	session, err := d.sessions.RequireSession(ctx, conn.UserID, conn.WorkspaceAdminAccountID)
	if err != nil {
		return RemoteActionUnsupported, err
	}
	if err := d.platform.UpdateSub2APIAdminAccountStatus(session, accountID, "active"); err != nil {
		return RemoteActionSub2APIStatusActiveFailed, err
	}
	return RemoteActionSub2APIStatusActive, nil
}
