package group_rates

import "testing"

func TestNormalizeListQueryPreservesLegacyAndAcceptsExplicitControls(t *testing.T) {
	legacy := normalizeListQuery(ListQuery{})
	if legacy.Status != "legacy" || legacy.Sort != "default" {
		t.Fatalf("legacy controls changed: status=%q sort=%q", legacy.Status, legacy.Sort)
	}

	for _, status := range []string{"all", "deleted"} {
		explicit := normalizeListQuery(ListQuery{Status: status, Sort: "multiplierDesc", PageSize: 500})
		if explicit.Status != status || explicit.Sort != "multiplierDesc" || explicit.PageSize != 100 {
			t.Fatalf("explicit controls not normalized: %#v", explicit)
		}
	}

	// 已下线的「已对接 / 未对接」筛选和非法值一样回退到兼容口径（返回全部行）。
	for _, status := range []string{"mapped", "unmapped", "unexpected"} {
		invalid := normalizeListQuery(ListQuery{Status: status, Sort: "unsafe"})
		if invalid.Status != "legacy" || invalid.Sort != "default" {
			t.Fatalf("status %q should use compatibility defaults: %#v", status, invalid)
		}
	}
}
