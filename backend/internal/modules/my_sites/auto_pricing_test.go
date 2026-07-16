package my_sites

import (
	"math"
	"strings"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

func floatPtr(v float64) *float64 { return &v }

func TestChangedUpstreamGroups(t *testing.T) {
	old := upstream.Metrics{
		Groups: []upstream.GroupInfo{
			{ID: "1", Name: "default", Multiplier: floatPtr(1.00)},
			{ID: "2", Name: "vip", Multiplier: floatPtr(2.00)},
			{ID: "3", Name: "stable", Multiplier: floatPtr(0.50)},
		},
	}

	t.Run("detects changes", func(t *testing.T) {
		newMetrics := upstream.Metrics{
			Groups: []upstream.GroupInfo{
				{ID: "1", Name: "default", Multiplier: floatPtr(1.08)},
				{ID: "2", Name: "vip", Multiplier: floatPtr(2.00)},
				{ID: "3", Name: "stable", Multiplier: floatPtr(0.60)},
			},
		}
		changes := changedUpstreamGroups(old, newMetrics)
		if len(changes) != 2 {
			t.Fatalf("expected 2 changes, got %d", len(changes))
		}
	})

	t.Run("no changes", func(t *testing.T) {
		changes := changedUpstreamGroups(old, old)
		if len(changes) != 0 {
			t.Fatalf("expected 0 changes, got %d", len(changes))
		}
	})

	t.Run("empty old metrics", func(t *testing.T) {
		changes := changedUpstreamGroups(upstream.Metrics{}, old)
		if len(changes) != 0 {
			t.Fatalf("expected 0 changes, got %d", len(changes))
		}
	})
}

func TestMappingUsesTarget(t *testing.T) {
	mapping := GroupMapping{
		UpstreamTargets: []UpstreamGroupRef{
			{SiteID: "site1", GroupName: "default"},
			{SiteID: "site2", GroupName: "vip"},
		},
	}

	if !mappingUsesTarget(mapping, "site1", "default") {
		t.Error("expected true for site1/default")
	}
	if mappingUsesTarget(mapping, "site1", "vip") {
		t.Error("expected false for site1/vip")
	}
	if mappingUsesTarget(mapping, "site3", "default") {
		t.Error("expected false for site3/default")
	}
}

func TestCalculateAutoPricingTarget(t *testing.T) {
	tests := []struct {
		name       string
		mapping    GroupMapping
		newRef     float64
		wantTarget float64
	}{
		{
			name: "fixed: 1.08 + 0.10 = 1.18",
			mapping: GroupMapping{
				AutoPricingStrategy: "fixed",
				FixedIncrease:       0.10,
			},
			newRef:     1.08,
			wantTarget: 1.18,
		},
		{
			name: "percentage: 1.20 * 1.10 = 1.32",
			mapping: GroupMapping{
				AutoPricingStrategy: "percentage",
				PercentageIncrease:  10,
			},
			newRef:     1.20,
			wantTarget: 1.32,
		},
		{
			name: "min clamp",
			mapping: GroupMapping{
				AutoPricingStrategy: "fixed",
				FixedIncrease:       0.01,
				MinMultiplier:       floatPtr(1.50),
			},
			newRef:     1.00,
			wantTarget: 1.50,
		},
		{
			name: "max clamp",
			mapping: GroupMapping{
				AutoPricingStrategy: "fixed",
				FixedIncrease:       1.00,
				MaxMultiplier:       floatPtr(1.30),
			},
			newRef:     1.00,
			wantTarget: 1.30,
		},
		{
			name: "min and max both nil, no clamp",
			mapping: GroupMapping{
				AutoPricingStrategy: "fixed",
				FixedIncrease:       0.50,
			},
			newRef:     1.00,
			wantTarget: 1.50,
		},
		{
			name: "zero threshold: 0% change is valid",
			mapping: GroupMapping{
				AutoPricingStrategy:    "fixed",
				FixedIncrease:          0.10,
				AdjustThresholdPercent: 0,
			},
			newRef:     1.00,
			wantTarget: 1.10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateAutoPricingTarget(tt.mapping, tt.newRef)
			if math.Abs(got-tt.wantTarget) > 1e-9 {
				t.Errorf("calculateAutoPricingTarget() = %.6f, want %.6f", got, tt.wantTarget)
			}
		})
	}
}

func TestAggregateMultipliers(t *testing.T) {
	multipliers := []float64{1.0, 2.0, 3.0}

	if v := aggregateMultipliers("lowest_upstream", multipliers); v != 1.0 {
		t.Errorf("lowest: got %.2f, want 1.00", v)
	}
	if v := aggregateMultipliers("highest_upstream", multipliers); v != 3.0 {
		t.Errorf("highest: got %.2f, want 3.00", v)
	}
	if v := aggregateMultipliers("average_upstream", multipliers); v != 2.0 {
		t.Errorf("average: got %.2f, want 2.00", v)
	}
}

func TestThresholdExceeded(t *testing.T) {
	tests := []struct {
		name      string
		oldRef    float64
		newRef    float64
		threshold float64
		exceeded  bool
	}{
		{"8% within 10%", 1.00, 1.08, 10, false},
		{"50% exceeds 10%", 1.00, 1.50, 10, true},
		{"0% within 0%", 1.00, 1.00, 0, false},
		{"1% exceeds 0%", 1.00, 1.01, 0, true},
		{"exactly at threshold not exceeded", 1.00, 1.10, 10, false},
		{"slightly above threshold exceeded", 1.00, 1.1001, 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := thresholdExceeded(tt.oldRef, tt.newRef, tt.threshold)
			if got != tt.exceeded {
				t.Errorf("thresholdExceeded(%.4f, %.4f, %.2f) = %t, want %t", tt.oldRef, tt.newRef, tt.threshold, got, tt.exceeded)
			}
		})
	}
}

// staticLookup returns a stable-ID-aware multiplier lookup for tests.
func staticLookup(table map[string]float64) multiplierLookup {
	return func(target UpstreamGroupRef) *float64 {
		if target.GroupID != "" {
			if v, ok := table[target.SiteID+"|id:"+target.GroupID]; ok {
				return &v
			}
			return nil
		}
		if v, ok := table[target.SiteID+"|"+target.GroupName]; ok {
			return &v
		}
		return nil
	}
}

func TestComputeReferenceMultipliers(t *testing.T) {
	t.Run("average: two groups in same site both changed", func(t *testing.T) {
		// A: 1.00→1.10, B: 2.00→2.20
		// oldRef = avg(1.00, 2.00) = 1.50
		// newRef = avg(1.10, 2.20) = 1.65
		targets := []UpstreamGroupRef{
			{SiteID: "sync1", GroupName: "A"},
			{SiteID: "sync1", GroupName: "B"},
		}
		changesByGroup := map[string]groupMultiplierChange{
			groupIdentity("", "A"): {Old: 1.00, New: 1.10},
			groupIdentity("", "B"): {Old: 2.00, New: 2.20},
		}
		newMetrics := []upstream.GroupInfo{
			{Name: "A", Multiplier: floatPtr(1.10)},
			{Name: "B", Multiplier: floatPtr(2.20)},
		}

		oldRef, newRef, ok, reason := computeReferenceMultipliers(
			"average_upstream", targets, "", "", "", "sync1",
			changesByGroup, newMetrics, staticLookup(nil),
		)
		if !ok {
			t.Fatalf("expected ok=true, got reason=%s", reason)
		}
		if math.Abs(oldRef-1.50) > 1e-9 {
			t.Errorf("oldRef = %.4f, want 1.5000", oldRef)
		}
		if math.Abs(newRef-1.65) > 1e-9 {
			t.Errorf("newRef = %.4f, want 1.6500", newRef)
		}
	})

	t.Run("lowest: two groups in same site both changed", func(t *testing.T) {
		// A: 1.00→1.10, B: 2.00→1.80
		// oldRef = min(1.00, 2.00) = 1.00
		// newRef = min(1.10, 1.80) = 1.10
		targets := []UpstreamGroupRef{
			{SiteID: "sync1", GroupName: "A"},
			{SiteID: "sync1", GroupName: "B"},
		}
		changesByGroup := map[string]groupMultiplierChange{
			groupIdentity("", "A"): {Old: 1.00, New: 1.10},
			groupIdentity("", "B"): {Old: 2.00, New: 1.80},
		}
		newMetrics := []upstream.GroupInfo{
			{Name: "A", Multiplier: floatPtr(1.10)},
			{Name: "B", Multiplier: floatPtr(1.80)},
		}

		oldRef, newRef, ok, _ := computeReferenceMultipliers(
			"lowest_upstream", targets, "", "", "", "sync1",
			changesByGroup, newMetrics, staticLookup(nil),
		)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if oldRef != 1.00 {
			t.Errorf("oldRef = %.4f, want 1.0000", oldRef)
		}
		if newRef != 1.10 {
			t.Errorf("newRef = %.4f, want 1.1000", newRef)
		}
	})

	t.Run("highest: two groups in same site both changed", func(t *testing.T) {
		// A: 1.00→1.50, B: 2.00→2.20
		// oldRef = max(1.00, 2.00) = 2.00
		// newRef = max(1.50, 2.20) = 2.20
		targets := []UpstreamGroupRef{
			{SiteID: "sync1", GroupName: "A"},
			{SiteID: "sync1", GroupName: "B"},
		}
		changesByGroup := map[string]groupMultiplierChange{
			groupIdentity("", "A"): {Old: 1.00, New: 1.50},
			groupIdentity("", "B"): {Old: 2.00, New: 2.20},
		}
		newMetrics := []upstream.GroupInfo{
			{Name: "A", Multiplier: floatPtr(1.50)},
			{Name: "B", Multiplier: floatPtr(2.20)},
		}

		oldRef, newRef, ok, _ := computeReferenceMultipliers(
			"highest_upstream", targets, "", "", "", "sync1",
			changesByGroup, newMetrics, staticLookup(nil),
		)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if oldRef != 2.00 {
			t.Errorf("oldRef = %.4f, want 2.0000", oldRef)
		}
		if newRef != 2.20 {
			t.Errorf("newRef = %.4f, want 2.2000", newRef)
		}
	})

	t.Run("primary: primary group changed", func(t *testing.T) {
		changesByGroup := map[string]groupMultiplierChange{
			groupIdentity("", "default"): {Old: 1.00, New: 1.08},
		}

		oldRef, newRef, ok, _ := computeReferenceMultipliers(
			"primary_upstream", nil, "sync1", "", "default", "sync1",
			changesByGroup, nil, staticLookup(nil),
		)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if oldRef != 1.00 {
			t.Errorf("oldRef = %.4f, want 1.0000", oldRef)
		}
		if newRef != 1.08 {
			t.Errorf("newRef = %.4f, want 1.0800", newRef)
		}
	})

	t.Run("primary: primary group not changed", func(t *testing.T) {
		changesByGroup := map[string]groupMultiplierChange{
			groupIdentity("", "vip"): {Old: 2.00, New: 2.10},
		}

		_, _, ok, reason := computeReferenceMultipliers(
			"primary_upstream", nil, "sync1", "", "default", "sync1",
			changesByGroup, nil, staticLookup(nil),
		)
		if ok {
			t.Fatal("expected ok=false for unchanged primary")
		}
		if reason != "primary_upstream_not_affected" {
			t.Errorf("reason = %s, want primary_upstream_not_affected", reason)
		}
	})

	t.Run("primary: primary not in sync site", func(t *testing.T) {
		changesByGroup := map[string]groupMultiplierChange{
			groupIdentity("", "default"): {Old: 1.00, New: 1.08},
		}

		_, _, ok, reason := computeReferenceMultipliers(
			"primary_upstream", nil, "other_site", "", "default", "sync1",
			changesByGroup, nil, staticLookup(nil),
		)
		if ok {
			t.Fatal("expected ok=false for primary on different site")
		}
		if reason != "primary_upstream_not_affected" {
			t.Errorf("reason = %s, want primary_upstream_not_affected", reason)
		}
	})

	t.Run("average: mixed sync and other site", func(t *testing.T) {
		// sync1/A: 1.00→1.20, other/B: cached 3.00
		// oldRef = avg(1.00, 3.00) = 2.00
		// newRef = avg(1.20, 3.00) = 2.10
		targets := []UpstreamGroupRef{
			{SiteID: "sync1", GroupName: "A"},
			{SiteID: "other", GroupName: "B"},
		}
		changesByGroup := map[string]groupMultiplierChange{
			groupIdentity("", "A"): {Old: 1.00, New: 1.20},
		}
		newMetrics := []upstream.GroupInfo{
			{Name: "A", Multiplier: floatPtr(1.20)},
		}
		lookup := staticLookup(map[string]float64{"other|B": 3.00})

		oldRef, newRef, ok, _ := computeReferenceMultipliers(
			"average_upstream", targets, "", "", "", "sync1",
			changesByGroup, newMetrics, lookup,
		)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if math.Abs(oldRef-2.00) > 1e-9 {
			t.Errorf("oldRef = %.4f, want 2.0000", oldRef)
		}
		if math.Abs(newRef-2.10) > 1e-9 {
			t.Errorf("newRef = %.4f, want 2.1000", newRef)
		}
	})

	t.Run("average: sync site group unchanged uses current value", func(t *testing.T) {
		// sync1/A: changed 1.00→1.20, sync1/B: unchanged at 2.00
		// oldRef = avg(1.00, 2.00) = 1.50
		// newRef = avg(1.20, 2.00) = 1.60
		targets := []UpstreamGroupRef{
			{SiteID: "sync1", GroupName: "A"},
			{SiteID: "sync1", GroupName: "B"},
		}
		changesByGroup := map[string]groupMultiplierChange{
			groupIdentity("", "A"): {Old: 1.00, New: 1.20},
		}
		newMetrics := []upstream.GroupInfo{
			{Name: "A", Multiplier: floatPtr(1.20)},
			{Name: "B", Multiplier: floatPtr(2.00)},
		}

		oldRef, newRef, ok, _ := computeReferenceMultipliers(
			"average_upstream", targets, "", "", "", "sync1",
			changesByGroup, newMetrics, staticLookup(nil),
		)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if math.Abs(oldRef-1.50) > 1e-9 {
			t.Errorf("oldRef = %.4f, want 1.5000", oldRef)
		}
		if math.Abs(newRef-1.60) > 1e-9 {
			t.Errorf("newRef = %.4f, want 1.6000", newRef)
		}
	})
}

func TestFormatAutoPricingNotify_Default(t *testing.T) {
	mapping := GroupMapping{
		OwnGroup:                 "my-group",
		AutoPricingSource:        "primary_upstream",
		PrimaryUpstreamGroupName: "default",
		AutoPricingStrategy:      "fixed",
		FixedIncrease:            0.10,
		PercentageIncrease:       10,
		AdjustThresholdPercent:   10,
	}
	result := autoPricingResult{
		OwnGroup:         "my-group",
		OldReference:     1.0000,
		NewReference:     1.0800,
		TargetMultiplier: 1.1800,
		Status:           "applied",
	}
	oldOwn := 1.1000
	msg := formatAutoPricingNotify(mapping, "upstream-site-1", result, &oldOwn)

	// 默认模板应包含关键变量
	for _, want := range []string{"my-group", "1.1000", "1.1800", "upstream-site-1", "default", "1.0000", "1.0800"} {
		if !strings.Contains(msg, want) {
			t.Errorf("default template missing %q in: %s", want, msg)
		}
	}
}

func TestFormatAutoPricingNotify_Custom(t *testing.T) {
	mapping := GroupMapping{
		OwnGroup:                  "vip",
		AutoPricingSource:         "primary_upstream",
		PrimaryUpstreamGroupName:  "pro",
		AutoPricingStrategy:       "percentage",
		PercentageIncrease:        10,
		FixedIncrease:             0.10,
		AdjustThresholdPercent:    15,
		AutoPricingNotifyTemplate: "Group {ownGroup} changed from {oldOwnMultiplier}x to {newOwnMultiplier}x. Strategy: {strategy}, threshold: {threshold}%.",
	}
	result := autoPricingResult{
		OwnGroup:         "vip",
		OldReference:     2.0000,
		NewReference:     2.1000,
		TargetMultiplier: 2.3100,
		Status:           "applied",
	}
	oldOwn := 2.2000
	msg := formatAutoPricingNotify(mapping, "site-A", result, &oldOwn)

	for _, want := range []string{"vip", "2.2000", "2.3100", "percentage", "15.00"} {
		if !strings.Contains(msg, want) {
			t.Errorf("custom template missing %q in: %s", want, msg)
		}
	}
}

func TestFormatAutoPricingNotify_NilOldOwn(t *testing.T) {
	mapping := GroupMapping{
		OwnGroup:                 "test",
		AutoPricingSource:        "primary_upstream",
		PrimaryUpstreamGroupName: "g1",
	}
	result := autoPricingResult{
		OwnGroup:         "test",
		OldReference:     1.00,
		NewReference:     1.10,
		TargetMultiplier: 1.20,
		Status:           "applied",
	}
	msg := formatAutoPricingNotify(mapping, "s1", result, nil)
	if !strings.Contains(msg, "-") {
		t.Errorf("expected '-' for nil oldOwnMultiplier in: %s", msg)
	}
}

func TestFormatAutoPricingNotify_AggregateSource(t *testing.T) {
	mapping := GroupMapping{
		OwnGroup:          "group-x",
		AutoPricingSource: "lowest_upstream",
	}
	result := autoPricingResult{
		OwnGroup:         "group-x",
		OldReference:     1.00,
		NewReference:     1.05,
		TargetMultiplier: 1.15,
		Status:           "applied",
	}
	oldOwn := 1.10
	msg := formatAutoPricingNotify(mapping, "site-B", result, &oldOwn)
	if !strings.Contains(msg, "最低倍率上游") {
		t.Errorf("expected aggregate source label in: %s", msg)
	}
}

func TestSaveMappings_NotifyValidation(t *testing.T) {
	// EnableAutoPricingNotify=true 但 botIDs 为空应该报错
	mapping := MappingRequest{
		OwnGroup:                "test-group",
		EnableAutoPricingNotify: true,
		AutoPricingNotifyBotIDs: []string{},
	}

	// 归一化后应为空
	filtered := filterEmptyStrings(mapping.AutoPricingNotifyBotIDs)
	if mapping.EnableAutoPricingNotify && len(filtered) == 0 {
		// 这是期望的校验失败路径
	} else {
		t.Error("expected validation to fail for notify enabled with empty bot IDs")
	}
}

func TestFilterEmptyStrings(t *testing.T) {
	result := filterEmptyStrings([]string{"a", "", "  ", "b", " c "})
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("got %v", result)
	}
}

func TestBuildChangesByGroup(t *testing.T) {
	old := upstream.Metrics{
		Groups: []upstream.GroupInfo{
			{ID: "1", Name: "default", Multiplier: floatPtr(1.00)},
			{ID: "2", Name: "vip", Multiplier: floatPtr(2.00)},
		},
	}
	newM := upstream.Metrics{
		Groups: []upstream.GroupInfo{
			{ID: "1", Name: "default", Multiplier: floatPtr(1.10)},
			{ID: "2", Name: "vip", Multiplier: floatPtr(2.00)},
		},
	}

	result := buildChangesByGroup(old, newM)
	if len(result) != 1 {
		t.Fatalf("expected 1 change, got %d", len(result))
	}
	ch, ok := result[groupIdentity("1", "default")]
	if !ok {
		t.Fatal("expected change for 'default'")
	}
	if ch.Old != 1.00 || ch.New != 1.10 {
		t.Errorf("default change = {%.2f, %.2f}, want {1.00, 1.10}", ch.Old, ch.New)
	}
}

func TestBuildChangesByGroupUsesStableIDAcrossRename(t *testing.T) {
	old := upstream.Metrics{Groups: []upstream.GroupInfo{{
		ID: "22", Name: "pool-0.03", Multiplier: floatPtr(0.03),
	}}}
	current := upstream.Metrics{Groups: []upstream.GroupInfo{{
		ID: "22", Name: "pool-0.02", Multiplier: floatPtr(0.02),
	}}}

	change, ok := buildChangesByGroup(old, current)[groupIdentity("22", "pool-0.02")]
	if !ok || change.Old != 0.03 || change.New != 0.02 {
		t.Fatalf("stable-ID rename change = %#v, %t", change, ok)
	}
}

func TestFindUpstreamGroupDoesNotRebindMissingStableIDByName(t *testing.T) {
	groups := []upstream.GroupInfo{{ID: "99", Name: "same-name", Multiplier: floatPtr(0.2)}}
	if group := findUpstreamGroup(groups, UpstreamGroupRef{GroupID: "54", GroupName: "same-name"}); group != nil {
		t.Fatalf("missing stable ID rebound to %#v", group)
	}
	if group := findUpstreamGroup(groups, UpstreamGroupRef{GroupName: "same-name"}); group == nil || group.ID != "99" {
		t.Fatalf("legacy name lookup failed: %#v", group)
	}
}

func TestComputeReferenceMultipliersUsesPrimaryStableIDAcrossRename(t *testing.T) {
	changes := map[string]groupMultiplierChange{
		groupIdentity("22", "pool-0.02"): {Old: 0.03, New: 0.02},
	}
	oldRef, newRef, ok, reason := computeReferenceMultipliers(
		"primary_upstream", nil, "site-a", "22", "pool-0.03", "site-a",
		changes, nil, staticLookup(nil),
	)
	if !ok || reason != "" || oldRef != 0.03 || newRef != 0.02 {
		t.Fatalf("stable primary result = %.4f/%.4f ok=%t reason=%s", oldRef, newRef, ok, reason)
	}
}
