package mass_email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/shared/authctx"
)

func TestHandlerRequiresWorkspaceAndDoesNotExposeHTML(t *testing.T) {
	mux := http.NewServeMux()
	repo := newFakeRepo()
	service := newTestService(repo, &fakeUsers{byID: map[string]upstream.Sub2APIAdminUser{"1": {ID: "1", Email: "a@example.com"}}}, &fakeSettings{template: settings.EmailTemplateSnapshot{ID: "tpl-1", Name: "Template", Subject: "Subject", HTMLBody: "<p>secret</p>"}})
	RegisterRoutes(mux, service, fakeAccounts{"user-1": "admin-1"})

	unauthReq := httptest.NewRequest(http.MethodGet, "/api/mass-email/batches", nil)
	unauthRec := httptest.NewRecorder()
	mux.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without auth context, got %d", unauthRec.Code)
	}

	body := `{"templateId":"tpl-1","selectionMode":"selected","userIds":["1"],"requestId":"req-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/mass-email/batches", strings.NewReader(body))
	req = req.WithContext(authctx.WithUserID(req.Context(), "user-1"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "templateHtml") || strings.Contains(rec.Body.String(), "templateHTML") {
		t.Fatalf("response leaked template HTML: %s", rec.Body.String())
	}
	var got BatchDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if got.TemplateSubject != "Subject" || got.RecipientCount != 1 {
		t.Fatalf("unexpected batch response: %#v", got)
	}
}

func TestHandlerListUsersForwardsQuery(t *testing.T) {
	mux := http.NewServeMux()
	users := &fakeUsers{page: upstream.Sub2APIAdminUsersPage{Items: []upstream.Sub2APIAdminUser{{ID: "1", Email: "a@example.com"}}, Total: 1, Page: 3, PageSize: 40, Pages: 1}}
	service := newTestService(newFakeRepo(), users, nil)
	RegisterRoutes(mux, service, fakeAccounts{"user-1": "admin-1"})

	req := authedRequest(http.MethodGet, "/api/mass-email/users?page=3&page_size=40&status=active&role=admin&search=++Alice%2Bnotes+&sort_by=email&sort_order=asc&timezone=Asia%2FShanghai", "", "user-1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	got := users.lastQuery
	if got.Page != 3 || got.PageSize != 40 || got.Status != "active" || got.Role != "admin" || got.Search != "Alice+notes" || got.SortBy != "email" || got.SortOrder != "asc" || got.Timezone != "Asia/Shanghai" {
		t.Fatalf("query not forwarded: %#v", got)
	}
}

func TestHandlerBatchDetailItemsAndCrossWorkspaceNotFound(t *testing.T) {
	mux := http.NewServeMux()
	repo := newFakeRepo()
	now := time.Now()
	repo.batches["batch-1"] = Batch{ID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", RequestID: "req-1", TemplateID: "tpl-1", TemplateName: "Template", TemplateSubject: "Subject", SelectionMode: SelectionModeSelected, Status: BatchStatusQueued, CreatedAt: now, UpdatedAt: now}
	repo.items["batch-1"] = []BatchItem{{ID: "item-1", BatchID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", UpstreamUserID: "up-1", RecipientEmail: "a@example.com", Status: ItemStatusPending, CreatedAt: now, UpdatedAt: now}}
	RegisterRoutes(mux, newTestService(repo, &fakeUsers{}, nil), fakeAccounts{"user-1": "admin-1", "user-2": "admin-2"})

	detailReq := authedRequest(http.MethodGet, "/api/mass-email/batches/batch-1", "", "user-1")
	detailRec := httptest.NewRecorder()
	mux.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	if strings.Contains(detailRec.Body.String(), "templateHTML") || strings.Contains(detailRec.Body.String(), "templateHtml") {
		t.Fatalf("detail leaked template HTML: %s", detailRec.Body.String())
	}

	itemsReq := authedRequest(http.MethodGet, "/api/mass-email/batches/batch-1/items?page=1&page_size=50", "", "user-1")
	itemsRec := httptest.NewRecorder()
	mux.ServeHTTP(itemsRec, itemsReq)
	if itemsRec.Code != http.StatusOK || !strings.Contains(itemsRec.Body.String(), "item-1") {
		t.Fatalf("expected items route 200 with item, got %d body=%s", itemsRec.Code, itemsRec.Body.String())
	}

	crossReq := authedRequest(http.MethodGet, "/api/mass-email/batches/batch-1", "", "user-2")
	crossRec := httptest.NewRecorder()
	mux.ServeHTTP(crossRec, crossReq)
	if crossRec.Code != http.StatusNotFound {
		t.Fatalf("expected cross-workspace 404, got %d body=%s", crossRec.Code, crossRec.Body.String())
	}
}

func TestHandlerCancelRouting(t *testing.T) {
	mux := http.NewServeMux()
	repo := newFakeRepo()
	now := time.Now()
	repo.batches["batch-1"] = Batch{ID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", RequestID: "req-1", Status: BatchStatusRunning, CreatedAt: now, UpdatedAt: now}
	RegisterRoutes(mux, newTestService(repo, &fakeUsers{}, nil), fakeAccounts{"user-1": "admin-1"})

	req := authedRequest(http.MethodPost, "/api/mass-email/batches/batch-1/cancel", "", "user-1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected cancel 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.batches["batch-1"].Status != BatchStatusCancelling {
		t.Fatalf("expected repository cancel path to run, got status %q", repo.batches["batch-1"].Status)
	}
}

func TestHandlerCreateBatchMapsActiveBatchExistsToConflict(t *testing.T) {
	mux := http.NewServeMux()
	repo := newFakeRepo()
	repo.enforceActiveLimit = true
	now := time.Now()
	repo.batches["active-1"] = Batch{ID: "active-1", UserID: "user-1", AdminAccountID: "admin-1", RequestID: "req-active", Status: BatchStatusQueued, CreatedAt: now, UpdatedAt: now}
	service := newTestService(repo, &fakeUsers{byID: map[string]upstream.Sub2APIAdminUser{"1": {ID: "1", Email: "a@example.com"}}}, nil)
	RegisterRoutes(mux, service, fakeAccounts{"user-1": "admin-1"})

	body := `{"templateId":"tpl-1","selectionMode":"selected","userIds":["1"],"requestId":"req-new"}`
	req := authedRequest(http.MethodPost, "/api/mass-email/batches", body, "user-1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), string(ErrActiveBatchExists)) {
		t.Fatalf("expected active batch error key, got %s", rec.Body.String())
	}
}

func TestHandlerCreateBatchRejectsOversizedBodyBeforeDecode(t *testing.T) {
	mux := http.NewServeMux()
	settingsProvider := &fakeSettings{template: settings.EmailTemplateSnapshot{ID: "tpl-1", Name: "Template", Subject: "Subject", HTMLBody: "<p>Body</p>"}}
	service := newTestService(newFakeRepo(), &fakeUsers{byID: map[string]upstream.Sub2APIAdminUser{"1": {ID: "1", Email: "a@example.com"}}}, settingsProvider)
	RegisterRoutes(mux, service, fakeAccounts{"user-1": "admin-1"})

	req := authedRequest(http.MethodPost, "/api/mass-email/batches", strings.Repeat("x", createBatchBodyLimitBytes+1), "user-1")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected oversized body 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), string(ErrInvalidRequest)) {
		t.Fatalf("expected stable invalid request key, got %s", rec.Body.String())
	}
	if settingsProvider.snapshotCalls != 0 {
		t.Fatalf("oversized request should fail before service work, snapshot calls=%d", settingsProvider.snapshotCalls)
	}
}

func authedRequest(method string, target string, body string, userID string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return req.WithContext(authctx.WithUserID(req.Context(), userID))
}

type fakeAccounts map[string]string

func (f fakeAccounts) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	return f[userID], nil
}

var _ = time.Now
