package connection_health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/shared/authctx"
)

type fakeTargetSchedulableActioner struct {
	calls       int
	wantAccount string
	err         error
	afterWrite  func(bool)
}

func (f *fakeTargetSchedulableActioner) SetSub2APIAdminAccountSchedulable(session upstream.Session, accountID string, schedulable bool) error {
	f.calls++
	if f.wantAccount != "" && accountID != f.wantAccount {
		return errors.New("unexpected account")
	}
	if f.err != nil {
		return f.err
	}
	if f.afterWrite != nil {
		f.afterWrite(schedulable)
	}
	return nil
}

func schedulableActionService(initial *bool, actioner *fakeTargetSchedulableActioner) (*Service, *fakeRepository) {
	repo := newFakeRepository()
	accounts := map[string][]upstream.AdminGroupAccountInfo{
		"g1": {{ID: "1515", Name: "account-1515", Status: "active", Schedulable: initial}},
	}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "default", Platform: string(upstream.PlatformSub2API)}},
		accountsByGrp: accounts,
	}
	if actioner.afterWrite == nil {
		actioner.afterWrite = func(value bool) {
			updated := accounts["g1"]
			updated[0].Schedulable = boolPointer(value)
			accounts["g1"] = updated
		}
	}
	service := newAdminGroupsService(reader, fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}}, repo)
	service.schedulableActions = actioner
	return service, repo
}

func TestSetTargetSchedulable_RejectsCrossWorkspaceBeforeWrite(t *testing.T) {
	actioner := &fakeTargetSchedulableActioner{}
	service, repo := schedulableActionService(boolPointer(true), actioner)

	_, err := service.SetTargetSchedulable(context.Background(), "user1", "sub2api:other:1515", false)
	if err == nil || err.Error() != ErrorProbeTargetNotFound {
		t.Fatalf("expected cross-workspace target rejection, got %v", err)
	}
	if actioner.calls != 0 || len(repo.events) != 0 {
		t.Fatalf("cross-workspace rejection must not write upstream or audit event: calls=%d events=%+v", actioner.calls, repo.events)
	}
}

func TestSetTargetSchedulable_UpstreamFailureIsNotSuccess(t *testing.T) {
	actioner := &fakeTargetSchedulableActioner{wantAccount: "1515", err: errors.New("upstream failed")}
	service, repo := schedulableActionService(boolPointer(true), actioner)

	_, err := service.SetTargetSchedulable(context.Background(), "user1", "sub2api:ws1:1515", false)
	if err == nil || err.Error() != ErrorSchedulableActionFailed {
		t.Fatalf("expected safe upstream failure, got %v", err)
	}
	if len(repo.events) != 1 || repo.events[0].Result != SchedulableActionFailed || repo.events[0].ActionSource != ActionSourceUser {
		t.Fatalf("failed user action must be audited, got %+v", repo.events)
	}
	if repo.events[0].RemoteAction != RemoteActionSchedulableDisableFailed {
		t.Fatalf("failed user action must not use a success-looking action code: %+v", repo.events[0])
	}
	groups, listErr := service.AdminGroups(context.Background(), "user1")
	if listErr != nil || len(groups) != 1 || len(groups[0].Accounts) != 1 {
		t.Fatalf("failed action must remain listable: groups=%+v err=%v", groups, listErr)
	}
	account := groups[0].Accounts[0]
	if account.LastSchedulableAction != RemoteActionSchedulableDisableFailed || account.LastSchedulableActionResult != SchedulableActionFailed || account.LastSchedulableActionErrorKey != ErrorSchedulableActionFailed {
		t.Fatalf("account row lost latest failed action result: %+v", account)
	}
	if account.SchedulableSource != "upstream_observed" {
		t.Fatalf("failed action must not own the observed scheduling state: %+v", account)
	}
}

func TestSetTargetSchedulable_ReadbackMissingOrMismatchIsNotSuccess(t *testing.T) {
	tests := []struct {
		name     string
		readback *bool
	}{
		{name: "missing", readback: nil},
		{name: "mismatch", readback: boolPointer(true)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actioner := &fakeTargetSchedulableActioner{}
			service, repo := schedulableActionService(boolPointer(true), actioner)
			actioner.afterWrite = func(bool) {
				reader := service.platformGroups.(fakePlatformGroupReader)
				updated := reader.accountsByGrp["g1"]
				updated[0].Schedulable = tt.readback
				reader.accountsByGrp["g1"] = updated
			}

			_, err := service.SetTargetSchedulable(context.Background(), "user1", "sub2api:ws1:1515", false)
			if err == nil || err.Error() != ErrorSchedulableReadbackFailed {
				t.Fatalf("expected readback failure, got %v", err)
			}
			if len(repo.events) != 1 || repo.events[0].Result != SchedulableActionFailed {
				t.Fatalf("readback failure must be audited as failure, got %+v", repo.events)
			}
		})
	}
}

func TestSetTargetSchedulable_ReturnsReadbackAndAuditsUserSource(t *testing.T) {
	actioner := &fakeTargetSchedulableActioner{wantAccount: "1515"}
	service, repo := schedulableActionService(boolPointer(true), actioner)

	result, err := service.SetTargetSchedulable(context.Background(), "user1", "sub2api:ws1:1515", false)
	if err != nil {
		t.Fatalf("unexpected action error: %v", err)
	}
	if result.TargetID != "sub2api:ws1:1515" || result.Schedulable || result.ActionSource != ActionSourceUser || result.ActionAt.IsZero() {
		t.Fatalf("unexpected readback result: %+v", result)
	}
	if actioner.calls != 1 || len(repo.events) != 1 {
		t.Fatalf("expected one narrow write and one audit event: calls=%d events=%+v", actioner.calls, repo.events)
	}
	event := repo.events[0]
	if event.Result != SchedulableActionSucceeded || event.ActionSource != ActionSourceUser || event.RemoteAction != RemoteActionSchedulableDisabled {
		t.Fatalf("unexpected success audit event: %+v", event)
	}
}

func TestSetTargetSchedulable_AuditFailureIsNotReportedAsSuccess(t *testing.T) {
	actioner := &fakeTargetSchedulableActioner{wantAccount: "1515"}
	service, repo := schedulableActionService(boolPointer(true), actioner)
	repo.insertEventErr = errors.New("audit unavailable")

	_, err := service.SetTargetSchedulable(context.Background(), "user1", "sub2api:ws1:1515", false)
	if err == nil || err.Error() != ErrorSchedulableAuditFailed {
		t.Fatalf("audit failure must not be reported as success, got %v", err)
	}
	if actioner.calls != 1 {
		t.Fatalf("upstream write should already have completed once, calls=%d", actioner.calls)
	}
}

func TestSetTargetSchedulableHandler_RequiresBooleanField(t *testing.T) {
	actioner := &fakeTargetSchedulableActioner{}
	service, _ := schedulableActionService(boolPointer(true), actioner)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	request := httptest.NewRequest(http.MethodPost, "/api/connection-health/targets/sub2api:ws1:1515/schedulable", strings.NewReader(`{}`))
	request = request.WithContext(authctx.WithUserID(request.Context(), "user1"))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
	}
	if actioner.calls != 0 {
		t.Fatalf("invalid request must not write upstream, calls=%d", actioner.calls)
	}
}

func TestSetTargetSchedulableHandler_ReturnsVerifiedReadback(t *testing.T) {
	actioner := &fakeTargetSchedulableActioner{wantAccount: "1515"}
	service, _ := schedulableActionService(boolPointer(true), actioner)
	mux := http.NewServeMux()
	RegisterRoutes(mux, service)

	request := httptest.NewRequest(http.MethodPost, "/api/connection-health/targets/sub2api:ws1:1515/schedulable", strings.NewReader(`{"schedulable":false}`))
	request = request.WithContext(authctx.WithUserID(request.Context(), "user1"))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var result TargetSchedulableActionResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.TargetID != "sub2api:ws1:1515" || result.Schedulable || result.ActionSource != ActionSourceUser || result.ActionAt.IsZero() {
		t.Fatalf("unexpected handler response: %+v", result)
	}
	if actioner.calls != 1 {
		t.Fatalf("handler must perform exactly one narrow upstream write, calls=%d", actioner.calls)
	}
}

func TestSchedulableUserActionMatchesObserved_RejectsStaleDirection(t *testing.T) {
	actionAt := time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
	disabledEvent := ConnectionHealthEvent{Result: SchedulableActionSucceeded, RemoteAction: RemoteActionSchedulableDisabled, CreatedAt: actionAt}
	observedAtAction := actionAt.Add(-time.Second)
	laterUpstreamChange := actionAt.Add(time.Minute)
	if schedulableUserActionMatchesObserved(disabledEvent, boolPointer(true), &observedAtAction) {
		t.Fatal("a later observed enabled value must not be attributed to the older disable action")
	}
	if !schedulableUserActionMatchesObserved(disabledEvent, boolPointer(false), &observedAtAction) {
		t.Fatal("matching observed value should retain the explicit user-action source")
	}
	if schedulableUserActionMatchesObserved(disabledEvent, boolPointer(false), &laterUpstreamChange) {
		t.Fatal("a later upstream update must not be attributed to the historical user action")
	}
	if schedulableUserActionMatchesObserved(disabledEvent, nil, &observedAtAction) {
		t.Fatal("an unknown observed value cannot be attributed to a historical user action")
	}
	if schedulableUserActionMatchesObserved(disabledEvent, boolPointer(false), nil) {
		t.Fatal("a value without authoritative upstream update time cannot prove user-action ownership")
	}
}
