package group_rate_campaigns

import (
	"math"
	"testing"
	"time"
)

func TestApplyAdjustment(t *testing.T) {
	tests := []struct {
		name     string
		mode     AdjustmentMode
		value    float64
		original float64
		want     float64
		wantErr  bool
	}{
		{name: "set to positive value", mode: AdjustmentSet, value: 1.5, original: 1.0, want: 1.5},
		{name: "set to zero is allowed", mode: AdjustmentSet, value: 0, original: 1.0, want: 0},
		{name: "set negative rejected", mode: AdjustmentSet, value: -1, original: 1.0, wantErr: true},
		{name: "multiply doubles original", mode: AdjustmentMultiply, value: 2, original: 1.5, want: 3.0},
		{name: "multiply by zero rejected", mode: AdjustmentMultiply, value: 0, original: 1.0, wantErr: true},
		{name: "multiply by negative rejected", mode: AdjustmentMultiply, value: -1, original: 1.0, wantErr: true},
		{name: "add positive delta", mode: AdjustmentAdd, value: 0.2, original: 1.0, want: 1.2},
		{name: "add resulting negative rejected", mode: AdjustmentAdd, value: -2, original: 1.0, wantErr: true},
		{name: "add resulting exactly zero allowed", mode: AdjustmentAdd, value: -1, original: 1.0, want: 0},
		{name: "NaN value rejected", mode: AdjustmentSet, value: math.NaN(), original: 1.0, wantErr: true},
		{name: "Inf value rejected", mode: AdjustmentMultiply, value: math.Inf(1), original: 1.0, wantErr: true},
		{name: "unknown mode rejected", mode: AdjustmentMode("bogus"), value: 1, original: 1.0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyAdjustment(tt.mode, tt.value, tt.original)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got result %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

func TestMultiplierChanged(t *testing.T) {
	if multiplierChanged(1.0, 1.0) {
		t.Error("expected equal values to report unchanged")
	}
	if !multiplierChanged(1.0, 1.01) {
		t.Error("expected different values to report changed")
	}
	if multiplierChanged(1.0, 1.0+1e-12) {
		t.Error("expected values within epsilon to report unchanged")
	}
}

func TestValidateCreateRequest(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	baseReq := func() CreateCampaignRequest {
		return CreateCampaignRequest{
			Name: "activity",
			Selection: Selection{
				Mode:   SelectionManual,
				Groups: []SelectionGroupRef{{GroupName: "vip", CampaignMultiplier: floatPtr(0.6)}},
			},
			Adjustment: Adjustment{Mode: AdjustmentSet, Value: 0},
			Schedule:   Schedule{StartMode: StartNow, EndMode: EndManual},
		}
	}

	t.Run("valid now/manual passes", func(t *testing.T) {
		if err := validateCreateRequest(baseReq(), now); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		req := baseReq()
		req.Name = "   "
		if err := validateCreateRequest(req, now); err != ErrInvalidName {
			t.Fatalf("expected ErrInvalidName, got %v", err)
		}
	})

	t.Run("name too long rejected", func(t *testing.T) {
		req := baseReq()
		runes := make([]rune, 81)
		for i := range runes {
			runes[i] = 'a'
		}
		req.Name = string(runes)
		if err := validateCreateRequest(req, now); err != ErrInvalidName {
			t.Fatalf("expected ErrInvalidName, got %v", err)
		}
	})

	t.Run("selection mode all rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Mode = SelectionAll
		if err := validateCreateRequest(req, now); err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})

	t.Run("selection mode type rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Mode = SelectionType
		if err := validateCreateRequest(req, now); err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})

	t.Run("selection mode currentFilter rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Mode = SelectionCurrentFilter
		if err := validateCreateRequest(req, now); err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})

	t.Run("empty groups rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Groups = nil
		if err := validateCreateRequest(req, now); err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})

	t.Run("empty group name rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Groups = []SelectionGroupRef{{GroupName: "   ", CampaignMultiplier: floatPtr(0.6)}}
		if err := validateCreateRequest(req, now); err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})

	t.Run("duplicate group name rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Groups = []SelectionGroupRef{
			{GroupName: "vip", CampaignMultiplier: floatPtr(0.6)},
			{GroupName: " vip ", CampaignMultiplier: floatPtr(0.8)},
		}
		if err := validateCreateRequest(req, now); err != ErrDuplicateGroup {
			t.Fatalf("expected ErrDuplicateGroup, got %v", err)
		}
	})

	t.Run("missing campaignMultiplier rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Groups = []SelectionGroupRef{{GroupName: "vip"}}
		if err := validateCreateRequest(req, now); err != ErrInvalidAdjustment {
			t.Fatalf("expected ErrInvalidAdjustment, got %v", err)
		}
	})

	t.Run("negative campaignMultiplier rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Groups = []SelectionGroupRef{{GroupName: "vip", CampaignMultiplier: floatPtr(-1)}}
		if err := validateCreateRequest(req, now); err != ErrInvalidAdjustment {
			t.Fatalf("expected ErrInvalidAdjustment, got %v", err)
		}
	})

	t.Run("NaN campaignMultiplier rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Groups = []SelectionGroupRef{{GroupName: "vip", CampaignMultiplier: floatPtr(math.NaN())}}
		if err := validateCreateRequest(req, now); err != ErrInvalidAdjustment {
			t.Fatalf("expected ErrInvalidAdjustment, got %v", err)
		}
	})

	t.Run("Inf campaignMultiplier rejected", func(t *testing.T) {
		req := baseReq()
		req.Selection.Groups = []SelectionGroupRef{{GroupName: "vip", CampaignMultiplier: floatPtr(math.Inf(1))}}
		if err := validateCreateRequest(req, now); err != ErrInvalidAdjustment {
			t.Fatalf("expected ErrInvalidAdjustment, got %v", err)
		}
	})

	t.Run("zero campaignMultiplier allowed", func(t *testing.T) {
		req := baseReq()
		req.Selection.Groups = []SelectionGroupRef{{GroupName: "vip", CampaignMultiplier: floatPtr(0)}}
		if err := validateCreateRequest(req, now); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("multiple valid groups with distinct rates pass", func(t *testing.T) {
		req := baseReq()
		req.Selection.Groups = []SelectionGroupRef{
			{GroupName: "vip", CampaignMultiplier: floatPtr(0.6)},
			{GroupName: "svip", CampaignMultiplier: floatPtr(0.8)},
		}
		if err := validateCreateRequest(req, now); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("adjustment mode multiply rejected", func(t *testing.T) {
		req := baseReq()
		req.Adjustment.Mode = AdjustmentMultiply
		if err := validateCreateRequest(req, now); err != ErrInvalidAdjustment {
			t.Fatalf("expected ErrInvalidAdjustment, got %v", err)
		}
	})

	t.Run("adjustment mode add rejected", func(t *testing.T) {
		req := baseReq()
		req.Adjustment.Mode = AdjustmentAdd
		if err := validateCreateRequest(req, now); err != ErrInvalidAdjustment {
			t.Fatalf("expected ErrInvalidAdjustment, got %v", err)
		}
	})

	t.Run("unknown adjustment mode rejected", func(t *testing.T) {
		req := baseReq()
		req.Adjustment.Mode = AdjustmentMode("bogus")
		if err := validateCreateRequest(req, now); err != ErrInvalidAdjustment {
			t.Fatalf("expected ErrInvalidAdjustment, got %v", err)
		}
	})

	t.Run("scheduled start without startAt rejected", func(t *testing.T) {
		req := baseReq()
		req.Schedule.StartMode = StartScheduled
		if err := validateCreateRequest(req, now); err != ErrInvalidSchedule {
			t.Fatalf("expected ErrInvalidSchedule, got %v", err)
		}
	})

	t.Run("scheduled start in the past rejected", func(t *testing.T) {
		req := baseReq()
		req.Schedule.StartMode = StartScheduled
		req.Schedule.StartAt = &past
		if err := validateCreateRequest(req, now); err != ErrInvalidSchedule {
			t.Fatalf("expected ErrInvalidSchedule, got %v", err)
		}
	})

	t.Run("scheduled start in the future passes", func(t *testing.T) {
		req := baseReq()
		req.Schedule.StartMode = StartScheduled
		req.Schedule.StartAt = &future
		if err := validateCreateRequest(req, now); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unknown start mode rejected", func(t *testing.T) {
		req := baseReq()
		req.Schedule.StartMode = StartMode("bogus")
		if err := validateCreateRequest(req, now); err != ErrInvalidSchedule {
			t.Fatalf("expected ErrInvalidSchedule, got %v", err)
		}
	})

	t.Run("scheduled end before effective start rejected", func(t *testing.T) {
		req := baseReq()
		req.Schedule.StartMode = StartScheduled
		req.Schedule.StartAt = &future
		req.Schedule.EndMode = EndScheduled
		endAt := future.Add(-time.Minute)
		req.Schedule.EndAt = &endAt
		if err := validateCreateRequest(req, now); err != ErrInvalidSchedule {
			t.Fatalf("expected ErrInvalidSchedule, got %v", err)
		}
	})

	t.Run("scheduled end after effective start passes", func(t *testing.T) {
		req := baseReq()
		req.Schedule.StartMode = StartScheduled
		req.Schedule.StartAt = &future
		req.Schedule.EndMode = EndScheduled
		endAt := future.Add(time.Hour)
		req.Schedule.EndAt = &endAt
		if err := validateCreateRequest(req, now); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unknown end mode rejected", func(t *testing.T) {
		req := baseReq()
		req.Schedule.EndMode = EndMode("bogus")
		if err := validateCreateRequest(req, now); err != ErrInvalidSchedule {
			t.Fatalf("expected ErrInvalidSchedule, got %v", err)
		}
	})

	t.Run("notify enabled without bots rejected", func(t *testing.T) {
		req := baseReq()
		req.Notify = Notify{Enabled: true}
		if err := validateCreateRequest(req, now); err != ErrNoNotifyBots {
			t.Fatalf("expected ErrNoNotifyBots, got %v", err)
		}
	})

	t.Run("notify enabled with bots passes", func(t *testing.T) {
		req := baseReq()
		req.Notify = Notify{Enabled: true, BotIDs: []string{"bot1"}}
		if err := validateCreateRequest(req, now); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestRenderTemplate(t *testing.T) {
	t.Run("replaces known placeholders", func(t *testing.T) {
		got := renderTemplate("活动 {activityName} 已开始，共 {totalCount} 个分组", map[string]string{
			"activityName": "双十一",
			"totalCount":   "5",
		})
		want := "活动 双十一 已开始，共 5 个分组"
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("empty template returns empty string", func(t *testing.T) {
		if got := renderTemplate("", map[string]string{"a": "b"}); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("unknown placeholder left untouched", func(t *testing.T) {
		got := renderTemplate("value={unknown}", map[string]string{"other": "1"})
		if got != "value={unknown}" {
			t.Fatalf("expected placeholder to survive untouched, got %q", got)
		}
	})
}

func TestBuildSummary(t *testing.T) {
	items := []CampaignItem{
		{ApplyStatus: ItemApplied, RestoreStatus: ItemRestored},
		{ApplyStatus: ItemApplied, RestoreStatus: ItemUnchanged},
		{ApplyStatus: ItemApplied, RestoreStatus: ItemFailed},
		{ApplyStatus: ItemFailed, RestoreStatus: ItemPending},
		{ApplyStatus: ItemPending, RestoreStatus: ItemPending},
	}
	summary := buildSummary(items)
	if summary.Total != 5 {
		t.Errorf("expected total 5, got %d", summary.Total)
	}
	if summary.Applied != 3 {
		t.Errorf("expected applied 3, got %d", summary.Applied)
	}
	if summary.ApplyFailed != 1 {
		t.Errorf("expected applyFailed 1, got %d", summary.ApplyFailed)
	}
	if summary.Restored != 2 {
		t.Errorf("expected restored 2, got %d", summary.Restored)
	}
	if summary.RestoreFailed != 1 {
		t.Errorf("expected restoreFailed 1, got %d", summary.RestoreFailed)
	}
}
