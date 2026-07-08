package upstream

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// availableGroupsFixture 是各测试共用的 /api/v1/groups/available 响应：
// default(id=1) 与 vip(id=2) 会在 /groups/rates 中命中专属倍率，stable(id=3) 不会。
func availableGroupsFixture(w http.ResponseWriter) {
	writeJSON(w, map[string]any{
		"data": []map[string]any{
			{"id": 1, "name": "default", "platform": "openai", "rate_multiplier": 1.0},
			{"id": 2, "name": "vip", "platform": "openai", "rate_multiplier": 2.0},
			{"id": 3, "name": "stable", "platform": "claude", "rate_multiplier": 3.0},
		},
	})
}

// TestFetchSub2APIAdminGroups_DedicatedMultiplier 验证 FetchSub2APIAdminGroups 按分组 ID
// 合并 /groups/rates 专属倍率：命中的分组使用专属倍率覆盖，缺失 ID 的分组保留默认倍率，
// rates 中出现的未知 ID 不会被新增到结果里。
func TestFetchSub2APIAdminGroups_DedicatedMultiplier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/groups/available":
			availableGroupsFixture(w)
		case "/api/v1/groups/rates":
			writeJSON(w, map[string]any{"data": map[string]any{
				"1":   0.8,
				"2":   1.5,
				"999": 9.9,
			}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}

	groups, err := service.FetchSub2APIAdminGroups(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups (unknown id 999 must not be added), got %d", len(groups))
	}

	byName := map[string]GroupInfo{}
	for _, g := range groups {
		byName[g.Name] = g
	}

	def := byName["default"]
	if def.Multiplier == nil || *def.Multiplier != 0.8 {
		t.Errorf("default final multiplier = %v, want 0.8", def.Multiplier)
	}
	if !def.HasDedicatedMultiplier {
		t.Errorf("default should have dedicated multiplier flag set")
	}
	if def.DefaultMultiplier == nil || *def.DefaultMultiplier != 1.0 {
		t.Errorf("default DefaultMultiplier = %v, want 1.0", def.DefaultMultiplier)
	}
	if def.DedicatedMultiplier == nil || *def.DedicatedMultiplier != 0.8 {
		t.Errorf("default DedicatedMultiplier = %v, want 0.8", def.DedicatedMultiplier)
	}

	vip := byName["vip"]
	if vip.Multiplier == nil || *vip.Multiplier != 1.5 {
		t.Errorf("vip final multiplier = %v, want 1.5", vip.Multiplier)
	}
	if !vip.HasDedicatedMultiplier {
		t.Errorf("vip should have dedicated multiplier flag set")
	}

	stable := byName["stable"]
	if stable.Multiplier == nil || *stable.Multiplier != 3.0 {
		t.Errorf("stable final multiplier = %v, want 3.0 (no rates entry, keep default)", stable.Multiplier)
	}
	if stable.HasDedicatedMultiplier {
		t.Errorf("stable should not have dedicated multiplier flag set")
	}
	if stable.DedicatedMultiplier != nil {
		t.Errorf("stable DedicatedMultiplier should be nil, got %v", *stable.DedicatedMultiplier)
	}
}

// TestFetchSub2APIAdminGroups_RatesMissingID 单独验证：/groups/rates 缺失某个分组 ID 时，
// 该分组必须保留 /groups/available 的默认倍率，不受其它分组覆盖影响。
func TestFetchSub2APIAdminGroups_RatesMissingID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/groups/available":
			availableGroupsFixture(w)
		case "/api/v1/groups/rates":
			writeJSON(w, map[string]any{"data": map[string]any{"1": 0.5}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}

	groups, err := service.FetchSub2APIAdminGroups(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byName := map[string]GroupInfo{}
	for _, g := range groups {
		byName[g.Name] = g
	}
	if vip := byName["vip"]; vip.Multiplier == nil || *vip.Multiplier != 2.0 {
		t.Errorf("vip final multiplier = %v, want 2.0 (kept default, no rates entry)", vip.Multiplier)
	}
	if stable := byName["stable"]; stable.Multiplier == nil || *stable.Multiplier != 3.0 {
		t.Errorf("stable final multiplier = %v, want 3.0 (kept default, no rates entry)", stable.Multiplier)
	}
}

// TestFetchSub2APIAdminGroups_RatesUnknownIDNotAdded 验证 /groups/rates 中出现
// /groups/available 不包含的分组 ID 时不会新增分组条目（缺少 name/platform 等展示字段）。
func TestFetchSub2APIAdminGroups_RatesUnknownIDNotAdded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/groups/available":
			availableGroupsFixture(w)
		case "/api/v1/groups/rates":
			writeJSON(w, map[string]any{"data": map[string]any{"42": 5.0}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}

	groups, err := service.FetchSub2APIAdminGroups(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	for _, g := range groups {
		if g.ID == "42" {
			t.Fatalf("unknown rates-only id 42 must not be added as a group")
		}
	}
}

// TestFetchSub2APIAdminGroups_RatesUnavailable 验证 /groups/rates 返回 404/非 2xx 时
// （模拟旧版 sub2api 尚未支持该接口），FetchSub2APIAdminGroups 仍应成功返回 available
// 默认倍率的分组列表，不应整体失败。
func TestFetchSub2APIAdminGroups_RatesUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/groups/available":
			availableGroupsFixture(w)
		case "/api/v1/groups/rates":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}

	groups, err := service.FetchSub2APIAdminGroups(session)
	if err != nil {
		t.Fatalf("expected FetchSub2APIAdminGroups to succeed when /groups/rates is unavailable, got err: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups from available fallback, got %d", len(groups))
	}
	for _, g := range groups {
		if g.HasDedicatedMultiplier {
			t.Errorf("group %s should not have dedicated multiplier when rates endpoint is unavailable", g.Name)
		}
		if g.Multiplier == nil || g.DefaultMultiplier == nil || *g.Multiplier != *g.DefaultMultiplier {
			t.Errorf("group %s multiplier should equal default multiplier when rates endpoint is unavailable", g.Name)
		}
	}
}

// TestFetchSub2APIMetrics_UsesOverriddenMultiplier 验证 fetchSub2APIMetrics 的
// Metrics.Groups 复用同一套合并逻辑，使用覆盖后的最终生效倍率。
func TestFetchSub2APIMetrics_UsesOverriddenMultiplier(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/me":
			writeJSON(w, map[string]any{"data": map[string]any{"balance": 10.0, "total_recharged": 20.0}})
		case "/api/v1/usage/dashboard/stats":
			writeJSON(w, map[string]any{"data": map[string]any{"today_actual_cost": 1.0}})
		case "/api/v1/groups/available":
			availableGroupsFixture(w)
		case "/api/v1/groups/rates":
			writeJSON(w, map[string]any{"data": map[string]any{"1": 0.8}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}

	metrics, err := service.fetchSub2APIMetrics(session)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byName := map[string]GroupInfo{}
	for _, g := range metrics.Groups {
		byName[g.Name] = g
	}
	def := byName["default"]
	if def.Multiplier == nil || *def.Multiplier != 0.8 {
		t.Errorf("Metrics.Groups default multiplier = %v, want 0.8 (overridden)", def.Multiplier)
	}
	if !def.HasDedicatedMultiplier {
		t.Errorf("Metrics.Groups default should have dedicated multiplier flag set")
	}
	stable := byName["stable"]
	if stable.Multiplier == nil || *stable.Multiplier != 3.0 {
		t.Errorf("Metrics.Groups stable multiplier = %v, want 3.0 (kept default)", stable.Multiplier)
	}
}

// TestSub2APIGroupRateOverrides 覆盖 sub2APIGroupRateOverrides helper 对上游几种常见
// payload 形态的解析，以及无效条目的容错行为。
func TestSub2APIGroupRateOverrides(t *testing.T) {
	t.Run("object map wrapped in data", func(t *testing.T) {
		got := sub2APIGroupRateOverrides(map[string]any{"data": map[string]any{"1": 0.8, "2": 1.2}})
		if got["1"] != 0.8 || got["2"] != 1.2 {
			t.Errorf("unexpected overrides: %v", got)
		}
	})

	t.Run("array of objects wrapped in data with snake_case fields", func(t *testing.T) {
		got := sub2APIGroupRateOverrides(map[string]any{"data": []any{
			map[string]any{"group_id": 1.0, "rate_multiplier": 0.8},
		}})
		if got["1"] != 0.8 {
			t.Errorf("unexpected overrides: %v", got)
		}
	})

	t.Run("unwrapped array with camelCase string fields", func(t *testing.T) {
		got := sub2APIGroupRateOverrides([]any{
			map[string]any{"groupId": "1", "rateMultiplier": "0.8"},
		})
		if got["1"] != 0.8 {
			t.Errorf("unexpected overrides: %v", got)
		}
	})

	t.Run("invalid entries are ignored without affecting others", func(t *testing.T) {
		got := sub2APIGroupRateOverrides(map[string]any{"data": []any{
			map[string]any{"group_id": 1.0, "rate_multiplier": 0.8},
			map[string]any{"rate_multiplier": 1.0},                             // 缺 ID，忽略
			map[string]any{"group_id": 2.0, "rate_multiplier": "not-a-number"}, // 无效倍率，忽略
		}})
		if len(got) != 1 || got["1"] != 0.8 {
			t.Errorf("unexpected overrides: %v", got)
		}
	})
}
