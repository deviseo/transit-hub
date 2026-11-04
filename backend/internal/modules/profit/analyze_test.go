package profit

import (
	"math"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

func TestUpstreamKey(t *testing.T) {
	cases := map[string]string{
		"":                                 "",
		"   ":                              "",
		"https://api.relay.example.com/v1": "example.com",
		"relay.example.com":                "example.com",
		"HTTPS://WWW.Example.COM.":         "example.com",
		"https://foo.vercel.app":           "foo.vercel.app",
		"https://bar.vercel.app/api":       "bar.vercel.app",
		"https://up.example.co.uk":         "example.co.uk",
		"http://10.0.0.8:3000/v1":          "10.0.0.8:3000",
		"http://10.0.0.8":                  "10.0.0.8",
		"http://localhost:8080":            "localhost:8080",
		"https://[2001:db8::1]:443/x":      "[2001:db8::1]:443",
	}
	for input, want := range cases {
		if got := upstreamKey(input); got != want {
			t.Fatalf("upstreamKey(%q) = %q, want %q", input, got, want)
		}
	}
	if unattributedCategory("") != CategoryOfficial || unattributedCategory("anthropic.com") != CategoryOfficial || unattributedCategory("someone.example") != CategoryUnregistered {
		t.Fatal("unexpected unattributed categories")
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestAnalyzeAttributesRevenueAndAllocatesCost(t *testing.T) {
	routes := []Route{
		{ID: "1", Name: "alpha-main", BaseURL: "https://api.alpha.example/v1", Groups: []string{"claude"}},
		{ID: "2", Name: "alpha-backup", BaseURL: "https://www.alpha.example", Groups: []string{"claude", "gpt"}},
		{ID: "3", Name: "beta", BaseURL: "https://beta.example", Groups: []string{"gpt"}},
		{ID: "4", Name: "official", BaseURL: "", Groups: []string{"claude"}},
		{ID: "5", Name: "unregistered", BaseURL: "https://gamma.example", Groups: []string{"gpt"}},
		{ID: "6", Name: "idle-alpha", BaseURL: "https://alpha.example", Groups: []string{"gpt"}},
		{ID: "7", Name: "gone", BaseURL: "https://alpha.example", Deleted: true},
	}
	revenue := []RouteRevenue{
		{RouteID: "1", Group: "claude", Revenue: 60},
		{RouteID: "2", Group: "claude", Revenue: 20},
		{RouteID: "2", Group: "gpt", Revenue: 20},
		{RouteID: "3", Group: "gpt", Revenue: 50},
		{RouteID: "4", Group: "claude", Revenue: 30},
		{RouteID: "5", Group: "gpt", Revenue: 10},
		{RouteID: "", Group: "", Revenue: 5},
	}
	costs := []SiteCost{
		{SiteID: "s-alpha-1", SiteName: "Alpha A", BaseURL: "https://alpha.example", Cost: 40},
		{SiteID: "s-alpha-2", SiteName: "Alpha B (old)", BaseURL: "https://panel.alpha.example", Cost: 20, FailedDays: 1},
		{SiteID: "s-beta", SiteName: "Beta", BaseURL: "https://beta.example", Cost: 70},
		{SiteID: "s-delta", SiteName: "Delta", BaseURL: "https://delta.example", Cost: 15},
	}
	live := []upstream.Response{
		{ID: "s-alpha-1", Name: "Alpha A", BaseURL: "https://alpha.example", Platform: upstream.PlatformSub2API, RechargeRate: 2},
		{ID: "s-beta", Name: "Beta", BaseURL: "https://beta.example", Platform: upstream.PlatformNewAPI, RechargeRate: 1},
		{ID: "s-delta", Name: "Delta", BaseURL: "https://delta.example", RechargeRate: 1},
		{ID: "s-new", Name: "Epsilon", BaseURL: "https://epsilon.example", RechargeRate: 0},
	}

	// 售价 2：营收（站点额度）× 2 = 人民币。
	result := analyze(routes, revenue, costs, live, 2)
	totals := result.totals
	if !near(totals.Revenue, 390) || !near(totals.Cost, 145) || !near(totals.Profit, 245) || !near(totals.AttributedRevenue, 300) || !near(totals.UnattributedRevenue, 90) {
		t.Fatalf("unexpected totals: %+v", totals)
	}
	if !near(*totals.AttributedShare, 300.0/390) || !near(totals.UnmatchedCost, 15) || totals.FailedSiteDays != 1 {
		t.Fatalf("unexpected coverage figures: %+v", totals)
	}

	byKey := map[string]UpstreamRow{}
	for _, row := range result.upstreams {
		byKey[row.Key] = row
	}
	alpha := byKey["alpha.example"]
	if !near(alpha.Revenue, 200) || !near(alpha.Cost, 60) || !near(alpha.Profit, 140) || len(alpha.Sites) != 2 {
		t.Fatalf("api.alpha / www.alpha / panel.alpha must merge into one upstream: %+v", alpha)
	}
	if len(alpha.Routes) != 3 || alpha.Routes[0].ID != "1" || alpha.Routes[2].ID != "6" {
		t.Fatalf("all current routes of an upstream are listed (deleted ones only with revenue): %+v", alpha.Routes)
	}
	beta := byKey["beta.example"]
	if !near(beta.Revenue, 100) || !near(beta.Cost, 70) || !near(*beta.Margin, 0.3) {
		t.Fatalf("unexpected beta row: %+v", beta)
	}
	if delta := byKey["delta.example"]; delta.Revenue != 0 || delta.Margin != nil || !near(delta.Cost, 15) {
		t.Fatalf("an upstream without routes keeps its cost as unmatched: %+v", delta)
	}
	if epsilon := byKey["epsilon.example"]; len(epsilon.Sites) != 1 || !epsilon.Sites[0].NoRate || epsilon.Cost != 0 {
		t.Fatalf("live sites without cost rows still appear: %+v", epsilon)
	}
	if !alpha.Sites[1].Deleted || alpha.Sites[1].Name != "Alpha B (old)" {
		t.Fatalf("sites missing from the live list are marked deleted: %+v", alpha.Sites)
	}

	groups := map[string]GroupRow{}
	for _, row := range result.groups {
		groups[row.Group] = row
	}
	// claude：alpha 上的营收 160（占 alpha 的 0.8）分摊 alpha 成本 48；另有官方账号营收 60（成本未知）。
	claude := groups["claude"]
	if !near(claude.Revenue, 220) || !near(claude.AttributedRevenue, 160) || !near(claude.UnattributedRevenue, 60) || !near(claude.Cost, 48) || !near(claude.Profit, 112) || !near(*claude.Margin, 0.7) {
		t.Fatalf("unexpected claude group: %+v", claude)
	}
	// gpt：alpha 40（分摊 12）+ beta 100（分摊 70），未登记上游 20。
	gpt := groups["gpt"]
	if !near(gpt.Revenue, 160) || !near(gpt.AttributedRevenue, 140) || !near(gpt.Cost, 82) || !near(gpt.UnattributedRevenue, 20) {
		t.Fatalf("unexpected gpt group: %+v", gpt)
	}

	categories := map[string]UnattributedRow{}
	for _, row := range result.unattributed {
		categories[row.Category+"|"+row.Key] = row
	}
	if row := categories["official|"]; !near(row.Revenue, 60) || len(row.Routes) != 1 || row.Routes[0].ID != "4" {
		t.Fatalf("routes without an upstream address are official: %+v", row)
	}
	if row := categories["unregistered|gamma.example"]; !near(row.Revenue, 20) || row.Routes[0].Name != "unregistered" {
		t.Fatalf("unregistered upstreams are listed by domain: %+v", row)
	}
	if row := categories["unknown|"]; !near(row.Revenue, 10) {
		t.Fatalf("revenue without a route is unknown: %+v", row)
	}
	if result.siteKeys["s-alpha-2"] != "alpha.example" {
		t.Fatalf("site keys must be recorded: %v", result.siteKeys)
	}
}
