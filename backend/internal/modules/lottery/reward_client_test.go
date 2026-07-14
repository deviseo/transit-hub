package lottery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type markerRoundTripper struct{}

func (markerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return jsonResponse(http.StatusOK, `{}`), nil
}

func TestRewardClientBalancePayloadAndIdempotency(t *testing.T) {
	var gotHeader string
	var gotBody map[string]any
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotHeader = r.Header.Get("Idempotency-Key")
		if r.URL.Path != "/api/v1/admin/redeem-codes/create-and-redeem" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.URL.Scheme != "https" || r.URL.Host != "reward.example.com" {
			t.Fatalf("unexpected reward target: %s", r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer admin-token" {
			t.Fatalf("authorization header mismatch")
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		return jsonResponse(http.StatusOK, `{"data":{"id":"remote-1"}}`), nil
	})

	job := RewardJob{ID: "job-1", CampaignID: "campaign-1", WinnerID: "winner-1", IdempotencyKey: "th-lottery-job-1", Winner: Winner{Sub2apiUserID: "123"}, Prize: Prize{Type: PrizeTypeBalance, BalanceAmount: "12.50"}}
	result := NewRewardClient(&http.Client{Transport: transport}).Redeem(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://reward.example.com/path?q=1", AccessToken: "admin-token", TokenType: "Bearer"}, job)
	if result.Status != RewardFulfilled || result.RemoteRef != "remote-1" {
		t.Fatalf("result = %#v", result)
	}
	expectedCode := sub2APIRewardCode(job.IdempotencyKey)
	if gotHeader != expectedCode || gotBody["code"] != expectedCode || gotBody["type"] != PrizeTypeBalance || gotBody["value"] != float64(12.5) {
		t.Fatalf("unexpected request header=%q body=%#v", gotHeader, gotBody)
	}
	if len(expectedCode) > 32 {
		t.Fatalf("Sub2API reward code is too long: %q", expectedCode)
	}
	if gotBody["user_id"] != float64(123) {
		t.Fatalf("user_id must be encoded as a JSON number, body=%#v", gotBody)
	}
}

func TestRewardClientSubscriptionConflictIsManualAttention(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["group_id"] != float64(456) {
			t.Fatalf("group_id must be encoded as a JSON number, body=%#v", body)
		}
		return jsonResponse(http.StatusConflict, `{"message":"already has subscription"}`), nil
	})
	job := RewardJob{ID: "job-1", IdempotencyKey: "th-lottery-job-1", Winner: Winner{Sub2apiUserID: "123"}, Prize: Prize{Type: PrizeTypeSubscription, GroupID: "456", ValidityDays: intPtr(30)}}
	result := NewRewardClient(&http.Client{Transport: transport}).Redeem(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://reward.example.com", AccessToken: "admin-token", TokenType: "Bearer"}, job)
	if result.Status != RewardManualAttention {
		t.Fatalf("status = %s, want manual_attention", result.Status)
	}
}

func TestRewardClientSubscriptionAppliesDedicatedRate(t *testing.T) {
	var gotBody map[string]any
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/admin/users/123" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		return jsonResponse(http.StatusOK, `{"data":{"id":123}}`), nil
	})
	job := RewardJob{
		ID:     "job-rate",
		Winner: Winner{Sub2apiUserID: "123"},
		Prize:  Prize{Type: PrizeTypeSubscription, GroupID: "456", Multiplier: "0.9", ValidityDays: intPtr(30)},
	}
	result := NewRewardClient(&http.Client{Transport: transport}).Redeem(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://reward.example.com", AccessToken: "admin-token", TokenType: "Bearer"}, job)
	if result.Status != RewardFulfilled || result.SkipRateCleanup {
		t.Fatalf("result = %#v", result)
	}
	rates, ok := gotBody["group_rates"].(map[string]any)
	if !ok || rates["456"] != float64(0.9) || len(rates) != 1 {
		t.Fatalf("group_rates = %#v", gotBody["group_rates"])
	}
}

func TestRewardClientDedicatedRateMissingUserIsFulfilledWithoutCleanup(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusNotFound, `{"message":"user not found"}`), nil
	})
	job := RewardJob{
		Winner: Winner{Sub2apiUserID: "123"},
		Prize:  Prize{Type: PrizeTypeSubscription, GroupID: "456", Multiplier: "0.9", ValidityDays: intPtr(30)},
	}
	result := NewRewardClient(&http.Client{Transport: transport}).Redeem(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://reward.example.com", AccessToken: "admin-token", TokenType: "Bearer"}, job)
	if result.Status != RewardFulfilled || !result.SkipRateCleanup {
		t.Fatalf("result = %#v, want fulfilled without cleanup", result)
	}
}

func TestRewardClientCleanupDedicatedRateIsIdempotentAndRestoresReplacement(t *testing.T) {
	var bodies []map[string]any
	responses := []int{http.StatusOK, http.StatusNotFound}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
		status := responses[len(bodies)-1]
		return jsonResponse(status, `{}`), nil
	})
	client := NewRewardClient(&http.Client{Transport: transport})
	session := upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://reward.example.com", AccessToken: "admin-token", TokenType: "Bearer"}
	job := RateCleanupJob{RewardJob: RewardJob{Winner: Winner{Sub2apiUserID: "123"}, Prize: Prize{Type: PrizeTypeSubscription, GroupID: "456", Multiplier: "0.9"}}}

	replacement := &RateCleanupReplacement{RewardJobID: "newer", Multiplier: "0.8"}
	if result := client.CleanupDedicatedRate(context.Background(), session, job, replacement); result.Status != RewardFulfilled {
		t.Fatalf("replacement cleanup result = %#v", result)
	}
	if result := client.CleanupDedicatedRate(context.Background(), session, job, nil); result.Status != RewardFulfilled {
		t.Fatalf("missing cleanup result = %#v", result)
	}

	firstRates := bodies[0]["group_rates"].(map[string]any)
	secondRates := bodies[1]["group_rates"].(map[string]any)
	if firstRates["456"] != float64(0.8) {
		t.Fatalf("replacement group_rates = %#v", firstRates)
	}
	if value, exists := secondRates["456"]; !exists || value != nil {
		t.Fatalf("clear group_rates = %#v", secondRates)
	}
}

func TestRewardClientRejectsNonNumericSub2APIGroupID(t *testing.T) {
	called := false
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	job := RewardJob{ID: "job-1", IdempotencyKey: "th-lottery-job-1", Winner: Winner{Sub2apiUserID: "123"}, Prize: Prize{Type: PrizeTypeSubscription, GroupID: "not-numeric", ValidityDays: intPtr(30)}}
	result := NewRewardClient(&http.Client{Transport: transport}).Redeem(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://reward.example.com", AccessToken: "admin-token", TokenType: "Bearer"}, job)
	if result.Status != RewardFailed || result.ErrorKey != ErrorValidation {
		t.Fatalf("result=%#v, want failed validation", result)
	}
	if called {
		t.Fatal("invalid group id must be rejected before the upstream request")
	}
}

func TestSub2APIRewardCodeIsStableAndBounded(t *testing.T) {
	first := sub2APIRewardCode("th-lottery-lrwd_1234567890abcdef1234567890abcdef")
	if first != sub2APIRewardCode("th-lottery-lrwd_1234567890abcdef1234567890abcdef") {
		t.Fatal("reward code must be deterministic")
	}
	if first == sub2APIRewardCode("th-lottery-lrwd_1234567890abcdef1234567890abcdee") {
		t.Fatal("different jobs must not share the same reward code")
	}
	if len(first) > 32 {
		t.Fatalf("reward code length = %d, want <= 32", len(first))
	}
}

func TestRewardClientRejectsNonNumericSub2APIUserID(t *testing.T) {
	called := false
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	job := RewardJob{ID: "job-1", IdempotencyKey: "th-lottery-job-1", Winner: Winner{Sub2apiUserID: "not-numeric"}, Prize: Prize{Type: PrizeTypeBalance, BalanceAmount: "12.50"}}
	result := NewRewardClient(&http.Client{Transport: transport}).Redeem(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://reward.example.com", AccessToken: "admin-token", TokenType: "Bearer"}, job)
	if result.Status != RewardFailed || result.ErrorKey != ErrorValidation {
		t.Fatalf("result=%#v, want failed validation", result)
	}
	if called {
		t.Fatal("invalid user id must be rejected before the upstream request")
	}
}

func TestNewRewardClientInstallsSafeTransportForNilPlainAndDefaultClients(t *testing.T) {
	for _, client := range []*http.Client{nil, {}, {Transport: http.DefaultTransport}} {
		rewardClient := NewRewardClient(client)
		transport, ok := rewardClient.client.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("expected safe http transport, got %T", rewardClient.client.Transport)
		}
		if transport.Proxy != nil {
			t.Fatal("safe reward transport must disable environment proxies")
		}
		if transport == http.DefaultTransport {
			t.Fatal("default transport must be cloned before use")
		}
	}
}

func TestNewRewardClientDoesNotMutateCallerOwnedClient(t *testing.T) {
	client := &http.Client{Timeout: 5}
	rewardClient := NewRewardClient(client)
	if client.Transport != nil {
		t.Fatalf("caller client transport was mutated: %T", client.Transport)
	}
	if rewardClient.client == client {
		t.Fatal("reward client must clone caller client before installing safe transport")
	}
}

func TestNewRewardClientPreservesCustomTransport(t *testing.T) {
	transport := markerRoundTripper{}
	client := NewRewardClient(&http.Client{Transport: transport})
	if client.client.Transport != transport {
		t.Fatal("custom reward transport was not preserved")
	}
}

func TestRewardClientRejectsUnsafeBaseURLBeforeRoundTrip(t *testing.T) {
	called := false
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, `{}`), nil
	})
	job := RewardJob{ID: "job-1", IdempotencyKey: "th-lottery-job-1", Winner: Winner{Sub2apiUserID: "user-1"}, Prize: Prize{Type: PrizeTypeBalance, BalanceAmount: "12.50"}}
	for _, baseURL := range []string{"", "://bad", "ftp://reward.example.com", "http://127.0.0.1:8080", "http://169.254.169.254"} {
		called = false
		result := NewRewardClient(&http.Client{Transport: transport}).Redeem(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: baseURL, AccessToken: "admin-token", TokenType: "Bearer"}, job)
		if result.Status != RewardManualAttention || result.ErrorKey != ErrorInvalidSourceOrigin {
			t.Fatalf("baseURL=%q result=%#v, want manual_attention invalid source", baseURL, result)
		}
		if called {
			t.Fatalf("baseURL=%q reached custom transport", baseURL)
		}
	}
}

func TestRewardClientAllowsLocalBaseURLWhenDebuggingEnabled(t *testing.T) {
	called := false
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		if r.URL.Host != "127.0.0.1:8080" {
			t.Fatalf("unexpected local reward target: %s", r.URL.String())
		}
		return jsonResponse(http.StatusOK, `{"data":{"id":"local-reward"}}`), nil
	})
	job := RewardJob{ID: "job-local", IdempotencyKey: "th-lottery-local", Winner: Winner{Sub2apiUserID: "123"}, Prize: Prize{Type: PrizeTypeBalance, BalanceAmount: "12.50"}}

	result := NewRewardClientWithPrivateTargets(&http.Client{Transport: transport}, true).Redeem(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "http://127.0.0.1:8080/path", AccessToken: "admin-token", TokenType: "Bearer"}, job)

	if !called || result.Status != RewardFulfilled || result.RemoteRef != "local-reward" {
		t.Fatalf("result=%#v called=%v", result, called)
	}
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func intPtr(value int) *int { return &value }
