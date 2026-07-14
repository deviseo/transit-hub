package admin_accounts

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transithub/backend/internal/shared/authctx"
)

func TestDeleteHandlerRequiresAuthenticatedUser(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewService(&deleteFakeAccountRepository{}))

	req := httptest.NewRequest(http.MethodDelete, "/api/admin-accounts/account-1", strings.NewReader(`{"confirmation":"DELETE WORKSPACE"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteHandlerDecodesConfirmationAndAccountID(t *testing.T) {
	repo := &deleteFakeAccountRepository{deleteResult: &DeleteResult{DeletedID: "account-1"}}
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewService(repo))

	req := httptest.NewRequest(http.MethodDelete, "/api/admin-accounts/account-1", bytes.NewBufferString(`{"confirmation":"DELETE WORKSPACE"}`))
	req = req.WithContext(authctx.WithUserID(context.Background(), "user-1"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if repo.userID != "user-1" || repo.accountID != "account-1" {
		t.Fatalf("unexpected scoped delete: user=%q account=%q", repo.userID, repo.accountID)
	}
}

func TestDeleteHandlerMapsUnownedAccountToNotFound(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewService(&deleteFakeAccountRepository{}))

	req := httptest.NewRequest(http.MethodDelete, "/api/admin-accounts/account-2", bytes.NewBufferString(`{"confirmation":"DELETE WORKSPACE"}`))
	req = req.WithContext(authctx.WithUserID(context.Background(), "user-1"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing or unowned workspace, got %d body=%s", rec.Code, rec.Body.String())
	}
}
