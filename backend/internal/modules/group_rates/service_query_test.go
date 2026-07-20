package group_rates

import "testing"

func TestNormalizeListQueryPreservesLegacyAndAcceptsExplicitControls(t *testing.T) {
	legacy := normalizeListQuery(ListQuery{})
	if legacy.Status != "legacy" || legacy.Sort != "default" {
		t.Fatalf("legacy controls changed: status=%q sort=%q", legacy.Status, legacy.Sort)
	}

	explicit := normalizeListQuery(ListQuery{Status: "mapped", Sort: "multiplierDesc", PageSize: 500})
	if explicit.Status != "mapped" || explicit.Sort != "multiplierDesc" || explicit.PageSize != 100 {
		t.Fatalf("explicit controls not normalized: %#v", explicit)
	}

	invalid := normalizeListQuery(ListQuery{Status: "unexpected", Sort: "unsafe"})
	if invalid.Status != "legacy" || invalid.Sort != "default" {
		t.Fatalf("invalid controls should use compatibility defaults: %#v", invalid)
	}
}
