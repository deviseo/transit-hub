package upstream

import "testing"

func TestNewAPIGroupsPreservesAutoGroupWithoutNumericRatio(t *testing.T) {
	groups := newAPIGroups(
		map[string]any{},
		map[string]any{"usable_group": map[string]any{"auto": "automatic routing"}},
	)
	if len(groups) != 1 {
		t.Fatalf("expected auto group to remain visible, got %#v", groups)
	}
	if groups[0].Name != "auto" || groups[0].Multiplier != nil || groups[0].MultiplierMode != "auto" {
		t.Fatalf("unexpected auto group normalization: %#v", groups[0])
	}
}
