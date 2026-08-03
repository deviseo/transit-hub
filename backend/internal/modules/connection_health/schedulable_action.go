package connection_health

import (
	"context"
	"log"
	"time"

	"transithub/backend/internal/modules/upstream"
)

const (
	ErrorSchedulableActionFailed   = "admin.connectionHealth.errors.schedulableActionFailed"
	ErrorSchedulableReadbackFailed = "admin.connectionHealth.errors.schedulableReadbackFailed"
	ErrorSchedulableAuditFailed    = "admin.connectionHealth.errors.schedulableAuditFailed"
	ErrorSchedulableUnsupported    = "admin.connectionHealth.errors.schedulableUnsupported"

	SchedulableActionSucceeded = "schedulable_user_action_succeeded"
	SchedulableActionFailed    = "schedulable_user_action_failed"
	ActionSourceUser           = "user_action"
	ActionSourceHealthProbe    = "health_probe"

	RemoteActionSchedulableEnabled       = "sub2api_schedulable_enabled"
	RemoteActionSchedulableDisabled      = "sub2api_schedulable_disabled"
	RemoteActionSchedulableEnableFailed  = "sub2api_schedulable_enable_failed"
	RemoteActionSchedulableDisableFailed = "sub2api_schedulable_disable_failed"
)

// TargetSchedulableActioner is the only write capability for the Sub2API business
// scheduling switch. The scheduler and state machine do not depend on this interface.
type TargetSchedulableActioner interface {
	SetSub2APIAdminAccountSchedulable(session upstream.Session, accountID string, schedulable bool) error
}

type TargetSchedulableActionResult struct {
	TargetID     string    `json:"targetId"`
	Schedulable  bool      `json:"schedulable"`
	ActionSource string    `json:"actionSource"`
	ActionAt     time.Time `json:"actionAt"`
}

// SetTargetSchedulable executes an explicit user command, then re-reads the upstream
// account. A successful write without a matching, parseable readback is still a failure.
func (s *Service) SetTargetSchedulable(ctx context.Context, userID string, targetID string, schedulable bool) (TargetSchedulableActionResult, error) {
	session, target, _, adminAccountID, err := s.resolveManualTarget(ctx, userID, targetID)
	if err != nil {
		return TargetSchedulableActionResult{}, err
	}
	if session.Platform != upstream.PlatformSub2API || s.schedulableActions == nil {
		return TargetSchedulableActionResult{}, requestError(ErrorSchedulableUnsupported)
	}
	release, err := s.repo.AcquireTargetLease(ctx, targetID)
	if err != nil {
		return TargetSchedulableActionResult{}, err
	}
	defer release()
	// The scheduler uses the same target lease. Resolve again after acquiring it
	// so a queued user command cannot write from a pre-probe account snapshot.
	session, target, _, adminAccountID, err = s.resolveManualTarget(ctx, userID, targetID)
	if err != nil {
		return TargetSchedulableActionResult{}, err
	}
	remoteAction := schedulableRemoteAction(schedulable)
	if err := s.schedulableActions.SetSub2APIAdminAccountSchedulable(session, target.AccountID, schedulable); err != nil {
		if auditErr := s.recordSchedulableActionEvent(ctx, userID, adminAccountID, target, SchedulableActionFailed, ErrorSchedulableActionFailed, schedulableFailedRemoteAction(schedulable)); auditErr != nil {
			log.Printf("[connection-health] audit failed schedulable action target_id=%s err=%v", target.TargetID, auditErr)
		}
		return TargetSchedulableActionResult{}, requestError(ErrorSchedulableActionFailed)
	}

	readbackTarget, readbackAccount, found, _, readbackErr := s.findAdminTarget(ctx, session, adminAccountID, target.AccountID)
	if readbackErr != nil || !found || readbackTarget.TargetID != targetID || readbackAccount.Schedulable == nil || *readbackAccount.Schedulable != schedulable {
		if auditErr := s.recordSchedulableActionEvent(ctx, userID, adminAccountID, target, SchedulableActionFailed, ErrorSchedulableReadbackFailed, schedulableFailedRemoteAction(schedulable)); auditErr != nil {
			log.Printf("[connection-health] audit failed schedulable readback target_id=%s err=%v", target.TargetID, auditErr)
		}
		return TargetSchedulableActionResult{}, requestError(ErrorSchedulableReadbackFailed)
	}

	actionAt := time.Now().UTC()
	if err := s.recordSchedulableActionEvent(ctx, userID, adminAccountID, readbackTarget, SchedulableActionSucceeded, "", remoteAction); err != nil {
		log.Printf("[connection-health] audit successful schedulable action failed target_id=%s err=%v", target.TargetID, err)
		return TargetSchedulableActionResult{}, requestError(ErrorSchedulableAuditFailed)
	}
	return TargetSchedulableActionResult{
		TargetID: targetID, Schedulable: *readbackAccount.Schedulable, ActionSource: ActionSourceUser, ActionAt: actionAt,
	}, nil
}

func schedulableRemoteAction(schedulable bool) string {
	if schedulable {
		return RemoteActionSchedulableEnabled
	}
	return RemoteActionSchedulableDisabled
}

func schedulableFailedRemoteAction(schedulable bool) string {
	if schedulable {
		return RemoteActionSchedulableEnableFailed
	}
	return RemoteActionSchedulableDisableFailed
}

func (s *Service) recordSchedulableActionEvent(ctx context.Context, userID string, adminAccountID string, target AdminProbeTarget, result string, errorKey string, remoteAction string) error {
	id, err := newID()
	if err != nil {
		return err
	}
	event := ConnectionHealthEvent{
		ID: id, ConnectionID: target.TargetID, ModelName: "*", UserID: userID, AdminAccountID: adminAccountID,
		AdminGroupID: target.AdminGroupID, OwnGroupName: target.AdminGroupName, UpstreamGroupName: target.AdminGroupName,
		Result: result, ErrorKey: errorKey, RemoteAction: remoteAction, ActionSource: ActionSourceUser, CreatedAt: time.Now().UTC(),
	}
	return s.repo.InsertEvent(ctx, event)
}
