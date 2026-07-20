package my_sites

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

func writeConnectionTestJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func platformTestSession(platform upstream.Platform, baseURL string) upstream.Session {
	if platform == upstream.PlatformNewAPI {
		return upstream.Session{Platform: platform, BaseURL: baseURL, Cookie: "session=test", UserID: "1"}
	}
	return upstream.Session{Platform: platform, BaseURL: baseURL, AccessToken: "access-token", TokenType: "Bearer"}
}

func TestManagedConnectionOperationsUseEachSidePlatform(t *testing.T) {
	var mu sync.Mutex
	var tokenName string
	var channelName string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/keys":
			writeConnectionTestJSON(w, map[string]any{"data": map[string]any{"id": 11, "key": "sk-sub2"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts":
			writeConnectionTestJSON(w, map[string]any{"data": map[string]any{"id": 22}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			tokenName, _ = body["name"].(string)
			mu.Unlock()
			writeConnectionTestJSON(w, map[string]any{"success": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/token/":
			mu.Lock()
			name := tokenName
			mu.Unlock()
			writeConnectionTestJSON(w, map[string]any{"data": []map[string]any{{"id": 33, "name": name}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/token/33/key":
			writeConnectionTestJSON(w, map[string]any{"data": map[string]any{"key": "sk-newapi"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/channel/":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			channel, _ := body["channel"].(map[string]any)
			mu.Lock()
			channelName, _ = channel["name"].(string)
			mu.Unlock()
			writeConnectionTestJSON(w, map[string]any{"success": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/channel/":
			mu.Lock()
			name := channelName
			mu.Unlock()
			writeConnectionTestJSON(w, map[string]any{"data": []map[string]any{{"id": 44, "name": name}}, "total": 1})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	service := &Service{platformService: upstream.NewPlatformService(upstream.NewHTTPClient(server.Client()))}
	platforms := []upstream.Platform{upstream.PlatformSub2API, upstream.PlatformNewAPI}
	for _, upstreamPlatform := range platforms {
		for _, adminPlatform := range platforms {
			name := string(upstreamPlatform) + "-to-" + string(adminPlatform)
			t.Run(name, func(t *testing.T) {
				upstreamSession := platformTestSession(upstreamPlatform, server.URL)
				keyID, key, err := service.createUpstreamCredential(upstreamSession, "credential-"+name, "7")
				if err != nil {
					t.Fatalf("create upstream credential: %v", err)
				}
				if keyID == "" || key == "" {
					t.Fatalf("missing upstream credential: id=%q key=%q", keyID, key)
				}

				adminSession := platformTestSession(adminPlatform, server.URL)
				connectionCtx := connectionContext{
					state:        &State{Session: adminSession},
					upstreamSite: &upstream.Site{Name: "source", BaseURL: "https://provider.example"},
					groupType:    "openai", groupName: "vip", multiplierLabel: "1.2x",
				}
				resourceID, _, err := service.createAdminResource(connectionCtx, 1, []string{"7"}, key)
				if err != nil {
					t.Fatalf("create admin resource: %v", err)
				}
				if resourceID == "" {
					t.Fatal("missing admin resource id")
				}
			})
		}
	}
}

func TestRealConnectCompensatesRemoteResourcesWhenPersistenceFails(t *testing.T) {
	var deletedAccount bool
	var deletedKey bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/auth/me":
			writeConnectionTestJSON(w, map[string]any{"data": map[string]any{"role": "admin"}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/groups":
			writeConnectionTestJSON(w, map[string]any{"data": []map[string]any{{"id": 7, "name": "vip", "platform": "openai", "status": "active"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/keys":
			writeConnectionTestJSON(w, map[string]any{"data": map[string]any{"id": 11, "key": "sk-created"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/accounts":
			writeConnectionTestJSON(w, map[string]any{"data": map[string]any{"id": 22}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/admin/accounts/22":
			deletedAccount = true
			writeConnectionTestJSON(w, map[string]any{"success": true})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/keys/11":
			deletedKey = true
			writeConnectionTestJSON(w, map[string]any{"success": true})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	session := platformTestSession(upstream.PlatformSub2API, server.URL)
	stateRepo := &testStateRepo{state: &State{UserID: "user-1", AdminAccountID: "admin-1", Session: session, Mappings: []GroupMapping{}}}
	connRepo := &testConnRepo{stateRepo: stateRepo, saveErr: errors.New("database unavailable")}
	lookup := testUpstreamLookup{sites: map[string]*upstream.Site{
		"site-1": {
			ID: "site-1", UserID: "user-1", AdminAccountID: "admin-1", Name: "source",
			BaseURL: server.URL, Platform: upstream.PlatformSub2API, Session: &session,
			Metrics: upstream.Metrics{Groups: []upstream.GroupInfo{{ID: "7", Name: "vip", Platform: stringPointer("openai")}}},
		},
	}}
	service := NewService(stateRepo, upstream.NewPlatformService(upstream.NewHTTPClient(server.Client())), lookup)
	service.SetAdminAccountResolver(testAdminResolver{currentID: "admin-1"})
	service.connRepository = connRepo
	addToPricing := false

	_, err := service.RealConnect(context.Background(), "user-1", RealConnectRequest{
		UpstreamSiteID: "site-1", UpstreamGroupID: "7", UpstreamGroupName: "vip",
		GroupType: "openai", OwnGroupIDs: []string{"7"}, AddToPricingMapping: &addToPricing,
	})
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("expected persistence error, got %v", err)
	}
	if !deletedAccount || !deletedKey {
		t.Fatalf("expected both compensating deletes, account=%t key=%t", deletedAccount, deletedKey)
	}
}

func stringPointer(value string) *string {
	return &value
}
