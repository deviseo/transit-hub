package dashboard

import (
	"errors"
	"testing"

	"transithub/backend/internal/modules/my_sites"
)

func TestAllocateRealConnectionProfit_UsesStableIDsAndExcludesUnboundKeys(t *testing.T) {
	connections := []my_sites.RealConnection{
		{
			ID: "conn-1", Status: "active", UpstreamSiteID: "site-a", UpstreamKeyID: "key-1",
			AdminAccountID: "account-1", OwnGroupIDs: []string{"group-1"},
		},
	}

	result := allocateRealConnectionProfit(
		connections,
		map[string]float64{"group-1": 100},
		map[string]float64{"site-a\x00key-1": 40, "site-a\x00unbound": 60},
	)

	group := result.Groups["group-1"]
	if group.Status != ProfitAllocationExact {
		t.Fatalf("group status = %q, want exact", group.Status)
	}
	if group.Cost == nil || *group.Cost != 40 || group.Profit == nil || *group.Profit != 60 {
		t.Fatalf("group allocation = %+v, want cost=40 profit=60", group)
	}
	if result.UnboundCost != 60 {
		t.Fatalf("unbound cost = %.2f, want 60.00", result.UnboundCost)
	}
}

func TestSafeProfitErrorDoesNotExposeRawErrorText(t *testing.T) {
	if got := safeProfitError(errors.New("Authorization: Bearer secret-token")); got != "request_failed" {
		t.Fatalf("safe error = %q, want request_failed", got)
	}
}

func TestAllocateRealConnectionProfit_RejectsMultiGroupWithoutSplitting(t *testing.T) {
	connections := []my_sites.RealConnection{
		{
			ID: "conn-1", Status: "active", UpstreamSiteID: "site-a", UpstreamKeyID: "key-1",
			AdminAccountID: "account-1", OwnGroupIDs: []string{"group-1", "group-2"},
		},
	}

	result := allocateRealConnectionProfit(
		connections,
		map[string]float64{"group-1": 60, "group-2": 40},
		map[string]float64{"site-a\x00key-1": 40},
	)

	for _, groupID := range []string{"group-1", "group-2"} {
		group := result.Groups[groupID]
		if group.Status != ProfitAllocationUnallocatable {
			t.Fatalf("group %s status = %q, want unallocatable", groupID, group.Status)
		}
		if group.Profit != nil || group.Cost != nil {
			t.Fatalf("group %s contains guessed amounts: %+v", groupID, group)
		}
	}
}

func TestAllocateRealConnectionProfit_RejectsDuplicateKeyBinding(t *testing.T) {
	connections := []my_sites.RealConnection{
		{ID: "conn-1", Status: "active", UpstreamSiteID: "site-a", UpstreamKeyID: "key-1", AdminAccountID: "account-1", OwnGroupIDs: []string{"group-1"}},
		{ID: "conn-2", Status: "active", UpstreamSiteID: "site-a", UpstreamKeyID: "key-1", AdminAccountID: "account-2", OwnGroupIDs: []string{"group-2"}},
	}

	result := allocateRealConnectionProfit(
		connections,
		map[string]float64{"group-1": 60, "group-2": 40},
		map[string]float64{"site-a\x00key-1": 40},
	)

	if result.Groups["group-1"].Profit != nil || result.Groups["group-2"].Profit != nil {
		t.Fatalf("duplicate key produced formal profit: %+v", result.Groups)
	}
	if !hasProfitIssue(result.Issues, ProfitIssueDuplicateBinding) {
		t.Fatalf("issues = %+v, want %q", result.Issues, ProfitIssueDuplicateBinding)
	}
}

func TestAllocateRealConnectionProfit_RejectsDuplicateAccountBinding(t *testing.T) {
	connections := []my_sites.RealConnection{
		{ID: "conn-1", Status: "active", UpstreamSiteID: "site-a", UpstreamKeyID: "key-1", AdminAccountID: "account-1", OwnGroupIDs: []string{"group-1"}},
		{ID: "conn-2", Status: "active", UpstreamSiteID: "site-a", UpstreamKeyID: "key-2", AdminAccountID: "account-1", OwnGroupIDs: []string{"group-2"}},
	}

	result := allocateRealConnectionProfit(
		connections,
		map[string]float64{"group-1": 60, "group-2": 40},
		map[string]float64{"site-a\x00key-1": 20, "site-a\x00key-2": 10},
	)
	if result.Groups["group-1"].Profit != nil || result.Groups["group-2"].Profit != nil {
		t.Fatalf("duplicate account produced formal profit: %+v", result.Groups)
	}
	if !hasProfitIssue(result.Issues, ProfitIssueDuplicateBinding) {
		t.Fatalf("issues = %+v, want %q", result.Issues, ProfitIssueDuplicateBinding)
	}
}
