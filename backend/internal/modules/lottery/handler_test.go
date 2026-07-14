package lottery

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorKeepsWorkspaceAndAdminSessionErrorsOutOfTransitHubAuth(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{name: "no current workspace", err: requestError(ErrorNoCurrentAccount), status: http.StatusConflict},
		{name: "sub2api admin session", err: requestError(ErrorAdminOnly), status: http.StatusForbidden},
		{name: "embed admin session", err: requestError(ErrorEmbedAdminSession), status: http.StatusForbidden},
		{name: "source binding", err: requestError(ErrorEmbedSourceBinding), status: http.StatusForbidden},
		{name: "viewer session", err: requestError(ErrorEmbedSessionInvalid), status: http.StatusUnauthorized},
		{name: "viewer token", err: requestError(ErrorEmbedSub2apiAuth), status: http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeError(recorder, tc.err)
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, tc.status, recorder.Body.String())
			}
			if decodeLotteryErrorMessage(t, recorder) != tc.err.Error() {
				t.Fatalf("unexpected body: %s", recorder.Body.String())
			}
		})
	}
}

func TestRequireDrawnCampaignMapsNilToNotFound(t *testing.T) {
	_, err := requireDrawnCampaign(nil)
	if !errors.Is(err, requestError(ErrorNotFound)) {
		t.Fatalf("expected not found, got %v", err)
	}
	campaign := &Campaign{ID: "campaign-1"}
	got, err := requireDrawnCampaign(campaign)
	if err != nil || got != campaign {
		t.Fatalf("expected campaign passthrough, got campaign=%+v err=%v", got, err)
	}
}

func decodeLotteryErrorMessage(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v body=%s", err, recorder.Body.String())
	}
	return payload.Message
}
