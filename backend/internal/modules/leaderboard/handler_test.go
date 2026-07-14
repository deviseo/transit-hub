package leaderboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transithub/backend/internal/shared/authctx"
)

func TestWriteAdminErrorKeepsSub2APIAdminSessionSeparateFromTransitHubAuth(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeAdminError(recorder, requestError(ErrorAdminOnly))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected missing Sub2API admin session to return 403, got %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), ErrorAdminOnly) {
		t.Fatalf("expected response to preserve business error key, got %q", recorder.Body.String())
	}
}

func TestUpdateEmbedConfigIgnoresEmptyLegacyBody(t *testing.T) {
	repo := newFakeRepo()
	service := newTestService(repo, &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, &fakeSub2API{})
	handler := &Handler{service: service}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/leaderboard/embed-config", nil)
	request = request.WithContext(authctx.WithUserID(request.Context(), "user-1"))

	handler.updateEmbedConfig(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected empty body to succeed, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var resp EmbedConfigResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Sub2apiSourceOrigin != "https://src.example.com" {
		t.Fatalf("expected server-derived origin, got %+v", resp)
	}
}

func TestUpdateEmbedConfigIgnoresLegacyAndExtraFields(t *testing.T) {
	repo := newFakeRepo()
	service := newTestService(repo, &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, &fakeSub2API{})
	handler := &Handler{service: service}
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"sub2apiSourceOrigin":"https://evil.example.com","extra":"ignored"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/leaderboard/embed-config", body)
	request = request.WithContext(authctx.WithUserID(request.Context(), "user-1"))

	handler.updateEmbedConfig(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected legacy body with extra fields to succeed, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var resp EmbedConfigResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Sub2apiSourceOrigin != "https://src.example.com" {
		t.Fatalf("expected client origin to be ignored, got %+v", resp)
	}
}
