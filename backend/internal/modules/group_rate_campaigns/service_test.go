package group_rate_campaigns

import (
	"context"
	"errors"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// fakeOperator 是 AdminGroupOperator 的测试替身，记录调用次数以断言 Preview 等只读路径
// 从不触碰远端倍率写接口。
type fakeOperator struct {
	session     upstream.Session
	sessionErr  error
	groups      []upstream.AdminGroupInfo
	groupsErr   error
	updateCalls int
	updateErr   error
	// updates 记录每次远端调用实际使用的 (分组名, 倍率)，用于断言每个分组是否按自己的固定倍率更新。
	updates []multiplierUpdate
}

type multiplierUpdate struct {
	groupName  string
	multiplier float64
}

func (f *fakeOperator) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	return f.session, f.sessionErr
}

func (f *fakeOperator) FetchAdminGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error) {
	return f.groups, f.groupsErr
}

func (f *fakeOperator) UpdateAdminGroupMultiplier(session upstream.Session, group upstream.AdminGroupInfo, multiplier float64) error {
	f.updateCalls++
	f.updates = append(f.updates, multiplierUpdate{groupName: group.Name, multiplier: multiplier})
	return f.updateErr
}

// fakeNotifier 记录发送次数，用于断言 Preview 从不发送通知。
type fakeNotifier struct {
	calls int
}

func (f *fakeNotifier) SendToBots(ctx context.Context, userID string, botIDs []string, message string) {
	f.calls++
}

// fakeTypeLookup 是 GroupTypeLookup 的测试替身，按预设的 type/search/platform 返回固定分组名。
type fakeTypeLookup struct {
	byType map[string][]string
	err    error
}

func (f *fakeTypeLookup) ListGroupNames(ctx context.Context, userID string, adminAccountID string, search string, groupType string, platform string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	if groupType != "" {
		return f.byType[groupType], nil
	}
	// currentFilter 模式：无 type 时按 search 返回预设集合。
	return f.byType[search], nil
}

// fakeRepository is an in-memory campaignRepository test double. It exists so that
// startCampaign/endCampaign/End/Cancel — the status-transition-heavy paths that a real
// *Repository can only exercise against a live Postgres connection — can be unit tested
// without a database, per this backend's "no test touches a real database" convention.
type fakeRepository struct {
	campaigns map[string]*Campaign
	items     map[string][]*CampaignItem
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{campaigns: map[string]*Campaign{}, items: map[string][]*CampaignItem{}}
}

func (f *fakeRepository) EnsureSchema(ctx context.Context) error { return nil }

func (f *fakeRepository) Insert(ctx context.Context, c Campaign) error {
	cp := c
	f.campaigns[c.ID] = &cp
	return nil
}

func (f *fakeRepository) SaveItems(ctx context.Context, items []CampaignItem) error {
	for _, item := range items {
		cp := item
		cp.ApplyStatus = ItemPending
		cp.RestoreStatus = ItemPending
		f.items[item.CampaignID] = append(f.items[item.CampaignID], &cp)
	}
	return nil
}

func (f *fakeRepository) findItem(id string) *CampaignItem {
	for _, items := range f.items {
		for _, item := range items {
			if item.ID == id {
				return item
			}
		}
	}
	return nil
}

func (f *fakeRepository) UpdateItemApply(ctx context.Context, id string, originalMultiplier *float64, status string, reason string, appliedAt *time.Time) error {
	item := f.findItem(id)
	if item == nil {
		return nil
	}
	item.OriginalMultiplier = originalMultiplier
	item.ApplyStatus = status
	item.ApplyReason = reason
	item.AppliedAt = appliedAt
	return nil
}

func (f *fakeRepository) UpdateItemRestore(ctx context.Context, id string, restoredMultiplier *float64, status string, reason string, restoredAt *time.Time) error {
	item := f.findItem(id)
	if item == nil {
		return nil
	}
	item.RestoredMultiplier = restoredMultiplier
	item.RestoreStatus = status
	item.RestoreReason = reason
	item.RestoredAt = restoredAt
	return nil
}

func (f *fakeRepository) ListItems(ctx context.Context, campaignID string) ([]CampaignItem, error) {
	items := f.items[campaignID]
	out := make([]CampaignItem, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	return out, nil
}

func (f *fakeRepository) Get(ctx context.Context, userID string, adminAccountID string, id string) (*Campaign, error) {
	c, ok := f.campaigns[id]
	if !ok || c.UserID != userID || c.AdminAccountID != adminAccountID {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (f *fakeRepository) GetByID(ctx context.Context, id string) (*Campaign, error) {
	c, ok := f.campaigns[id]
	if !ok {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (f *fakeRepository) List(ctx context.Context, userID string, adminAccountID string, query ListQuery) ([]Campaign, int, error) {
	return nil, 0, nil
}

func (f *fakeRepository) ListDueScheduled(ctx context.Context, now time.Time) ([]string, error) {
	ids := make([]string, 0)
	for id, c := range f.campaigns {
		if c.Status == StatusScheduled && c.StartAt != nil && !c.StartAt.After(now) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ListDueRunning mirrors the fixed repository.go query: it must scan both running and
// partial campaigns, since a partial campaign can still hold applied-but-not-restored items.
func (f *fakeRepository) ListDueRunning(ctx context.Context, now time.Time) ([]string, error) {
	ids := make([]string, 0)
	for id, c := range f.campaigns {
		if c.Status != StatusRunning && c.Status != StatusPartial {
			continue
		}
		if c.EndMode == EndScheduled && c.EndAt != nil && !c.EndAt.After(now) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (f *fakeRepository) ClaimForRunning(ctx context.Context, id string) (bool, error) {
	c, ok := f.campaigns[id]
	if !ok || (c.Status != StatusDraft && c.Status != StatusScheduled) {
		return false, nil
	}
	c.Status = StatusRunning
	now := time.Now()
	c.StartedAt = &now
	return true, nil
}

// ClaimForEnding mirrors the fixed repository.go query: running and partial campaigns
// can both be claimed for the ending/restore path.
func (f *fakeRepository) ClaimForEnding(ctx context.Context, id string) (bool, error) {
	c, ok := f.campaigns[id]
	if !ok || (c.Status != StatusRunning && c.Status != StatusPartial) {
		return false, nil
	}
	c.Status = StatusEnding
	return true, nil
}

func (f *fakeRepository) FinishStart(ctx context.Context, id string, status string) error {
	if c, ok := f.campaigns[id]; ok {
		c.Status = status
	}
	return nil
}

func (f *fakeRepository) FinishEnd(ctx context.Context, id string, status string) error {
	if c, ok := f.campaigns[id]; ok {
		c.Status = status
		now := time.Now()
		c.EndedAt = &now
	}
	return nil
}

func (f *fakeRepository) ClaimForCancel(ctx context.Context, id string) (bool, error) {
	c, ok := f.campaigns[id]
	if !ok || (c.Status != StatusDraft && c.Status != StatusScheduled) {
		return false, nil
	}
	c.Status = StatusCancelled
	return true, nil
}

func floatPtr(v float64) *float64 { return &v }

func adminGroups() []upstream.AdminGroupInfo {
	return []upstream.AdminGroupInfo{
		{ID: "g1", Name: "default", Multiplier: floatPtr(1.0)},
		{ID: "g2", Name: "vip", Multiplier: floatPtr(2.0)},
		{ID: "g3", Name: "svip", Multiplier: floatPtr(3.0)},
	}
}

func TestServicePreview(t *testing.T) {
	t.Run("computes per-group fixed campaign multiplier without touching repository or remote writes", func(t *testing.T) {
		operator := &fakeOperator{groups: adminGroups()}
		notifier := &fakeNotifier{}
		svc := NewService(nil, operator, notifier, nil, Config{})

		req := CreateCampaignRequest{
			Name: "activity",
			Selection: Selection{
				Mode: SelectionManual,
				Groups: []SelectionGroupRef{
					{GroupName: "vip", CampaignMultiplier: floatPtr(0.6)},
					{GroupName: "svip", CampaignMultiplier: floatPtr(0.8)},
				},
			},
			Adjustment: Adjustment{Mode: AdjustmentSet, Value: 0},
			Schedule:   Schedule{StartMode: StartNow, EndMode: EndManual},
		}
		resp, err := svc.Preview(context.Background(), "user1", "account1", req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Total != 2 {
			t.Fatalf("expected 2 items, got %d", resp.Total)
		}
		byName := map[string]PreviewItem{}
		for _, item := range resp.Items {
			byName[item.GroupName] = item
		}
		if byName["vip"].CampaignMultiplier != 0.6 || byName["vip"].OriginalMultiplier != 2.0 || byName["vip"].RestoredMultiplier != 2.0 {
			t.Errorf("unexpected vip item: %+v", byName["vip"])
		}
		if byName["svip"].CampaignMultiplier != 0.8 || byName["svip"].OriginalMultiplier != 3.0 || byName["svip"].RestoredMultiplier != 3.0 {
			t.Errorf("unexpected svip item: %+v", byName["svip"])
		}
		if operator.updateCalls != 0 {
			t.Errorf("expected no remote update calls during preview, got %d", operator.updateCalls)
		}
		if notifier.calls != 0 {
			t.Errorf("expected no notifications during preview, got %d", notifier.calls)
		}
	})

	t.Run("invalid request rejected before touching operator", func(t *testing.T) {
		operator := &fakeOperator{groups: adminGroups()}
		svc := NewService(nil, operator, nil, nil, Config{})

		req := CreateCampaignRequest{
			Name:       "",
			Adjustment: Adjustment{Mode: AdjustmentSet, Value: 1},
			Schedule:   Schedule{StartMode: StartNow, EndMode: EndManual},
		}
		_, err := svc.Preview(context.Background(), "user1", "account1", req)
		if err != ErrInvalidName {
			t.Fatalf("expected ErrInvalidName, got %v", err)
		}
	})

	t.Run("session error propagates", func(t *testing.T) {
		wantErr := errors.New("session expired")
		operator := &fakeOperator{sessionErr: wantErr}
		svc := NewService(nil, operator, nil, nil, Config{})

		req := CreateCampaignRequest{
			Name: "activity",
			Selection: Selection{
				Mode:   SelectionManual,
				Groups: []SelectionGroupRef{{GroupName: "vip", CampaignMultiplier: floatPtr(0.6)}},
			},
			Adjustment: Adjustment{Mode: AdjustmentSet, Value: 0},
			Schedule:   Schedule{StartMode: StartNow, EndMode: EndManual},
		}
		_, err := svc.Preview(context.Background(), "user1", "account1", req)
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected session error, got %v", err)
		}
	})

	t.Run("group not found among remote groups surfaces domain error", func(t *testing.T) {
		operator := &fakeOperator{groups: adminGroups()}
		svc := NewService(nil, operator, nil, nil, Config{})

		req := CreateCampaignRequest{
			Name: "activity",
			Selection: Selection{
				Mode:   SelectionManual,
				Groups: []SelectionGroupRef{{GroupName: "not-exists", CampaignMultiplier: floatPtr(0.6)}},
			},
			Adjustment: Adjustment{Mode: AdjustmentSet, Value: 0},
			Schedule:   Schedule{StartMode: StartNow, EndMode: EndManual},
		}
		_, err := svc.Preview(context.Background(), "user1", "account1", req)
		if err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})
}

// TestCreateStartNowAppliesEachGroupsOwnFixedMultiplier is the end-to-end check for the
// per-group fixed multiplier model: two manually selected groups with different rates must
// each be updated remotely with their own value, not a single shared/computed value.
func TestCreateStartNowAppliesEachGroupsOwnFixedMultiplier(t *testing.T) {
	repo := newFakeRepository()
	operator := &fakeOperator{groups: adminGroups()}
	svc := &Service{repository: repo, operator: operator, config: Config{}}

	req := CreateCampaignRequest{
		Name: "flash sale",
		Selection: Selection{
			Mode: SelectionManual,
			Groups: []SelectionGroupRef{
				{GroupName: "vip", CampaignMultiplier: floatPtr(0.6)},
				{GroupName: "svip", CampaignMultiplier: floatPtr(0.9)},
			},
		},
		Adjustment: Adjustment{Mode: AdjustmentSet, Value: 0},
		Schedule:   Schedule{StartMode: StartNow, EndMode: EndManual},
	}

	detail, err := svc.Create(context.Background(), "user1", "account1", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Status != StatusRunning {
		t.Fatalf("expected campaign to be running, got %q", detail.Status)
	}
	if operator.updateCalls != 2 {
		t.Fatalf("expected 2 remote update calls, got %d", operator.updateCalls)
	}
	byGroup := map[string]float64{}
	for _, u := range operator.updates {
		byGroup[u.groupName] = u.multiplier
	}
	if byGroup["vip"] != 0.6 {
		t.Errorf("expected vip updated to 0.6, got %v", byGroup["vip"])
	}
	if byGroup["svip"] != 0.9 {
		t.Errorf("expected svip updated to 0.9, got %v", byGroup["svip"])
	}

	items, err := repo.ListItems(context.Background(), detail.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byName := map[string]CampaignItem{}
	for _, item := range items {
		byName[item.GroupName] = item
	}
	if byName["vip"].CampaignMultiplier != 0.6 || floatOrZero(byName["vip"].OriginalMultiplier) != 2.0 {
		t.Errorf("unexpected vip item: %+v", byName["vip"])
	}
	if byName["svip"].CampaignMultiplier != 0.9 || floatOrZero(byName["svip"].OriginalMultiplier) != 3.0 {
		t.Errorf("unexpected svip item: %+v", byName["svip"])
	}

	// Simulate the remote state after a successful start: vip/svip now sit at their campaign
	// multipliers. fakeOperator.groups is static and doesn't auto-mutate from UpdateAdminGroupMultiplier
	// calls, so this must be set explicitly for End to see a real difference from OriginalMultiplier.
	operator.groups = []upstream.AdminGroupInfo{
		{ID: "g1", Name: "default", Multiplier: floatPtr(1.0)},
		{ID: "g2", Name: "vip", Multiplier: floatPtr(0.6)},
		{ID: "g3", Name: "svip", Multiplier: floatPtr(0.9)},
	}

	// End must restore each item back to its own original_multiplier, not a shared value.
	detail, err = svc.End(context.Background(), "user1", "account1", detail.ID)
	if err != nil {
		t.Fatalf("unexpected error ending campaign: %v", err)
	}
	if detail.Status != StatusEnded {
		t.Fatalf("expected campaign to end as %q, got %q", StatusEnded, detail.Status)
	}
	if operator.updateCalls != 4 {
		t.Fatalf("expected 2 more remote restore calls (4 total), got %d", operator.updateCalls)
	}
	restoredByGroup := map[string]float64{}
	for _, u := range operator.updates[2:] {
		restoredByGroup[u.groupName] = u.multiplier
	}
	if restoredByGroup["vip"] != 2.0 {
		t.Errorf("expected vip restored to 2.0, got %v", restoredByGroup["vip"])
	}
	if restoredByGroup["svip"] != 3.0 {
		t.Errorf("expected svip restored to 3.0, got %v", restoredByGroup["svip"])
	}
}

func TestServiceResolveSelection(t *testing.T) {
	svcWithLookup := func(lookup GroupTypeLookup) *Service {
		return NewService(nil, &fakeOperator{}, nil, lookup, Config{})
	}

	t.Run("all mode returns every group", func(t *testing.T) {
		svc := svcWithLookup(nil)
		targets, err := svc.resolveSelection(context.Background(), "u", "a", Selection{Mode: SelectionAll}, adminGroups())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 3 {
			t.Fatalf("expected 3 targets, got %d", len(targets))
		}
	})

	t.Run("manual mode filters by exact name", func(t *testing.T) {
		svc := svcWithLookup(nil)
		selection := Selection{Mode: SelectionManual, Groups: []SelectionGroupRef{{GroupName: "vip"}, {GroupName: " svip "}}}
		targets, err := svc.resolveSelection(context.Background(), "u", "a", selection, adminGroups())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 2 {
			t.Fatalf("expected 2 targets, got %d", len(targets))
		}
	})

	t.Run("manual mode with no matches returns empty selection error", func(t *testing.T) {
		svc := svcWithLookup(nil)
		selection := Selection{Mode: SelectionManual, Groups: []SelectionGroupRef{{GroupName: "ghost"}}}
		_, err := svc.resolveSelection(context.Background(), "u", "a", selection, adminGroups())
		if err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})

	t.Run("type mode unions across types and intersects with admin groups", func(t *testing.T) {
		lookup := &fakeTypeLookup{byType: map[string][]string{
			"promo":   {"vip", "ghost"},
			"premium": {"svip"},
		}}
		svc := svcWithLookup(lookup)
		selection := Selection{Mode: SelectionType, Types: []string{"promo", "premium"}}
		targets, err := svc.resolveSelection(context.Background(), "u", "a", selection, adminGroups())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 2 {
			t.Fatalf("expected 2 targets (vip, svip), got %d", len(targets))
		}
	})

	t.Run("type mode with nil lookup returns nil names and thus empty selection", func(t *testing.T) {
		svc := svcWithLookup(nil)
		selection := Selection{Mode: SelectionType, Types: []string{"promo"}}
		_, err := svc.resolveSelection(context.Background(), "u", "a", selection, adminGroups())
		if err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})

	t.Run("currentFilter mode delegates to lookup with filter fields", func(t *testing.T) {
		lookup := &fakeTypeLookup{byType: map[string][]string{"vip-search": {"vip"}}}
		svc := svcWithLookup(lookup)
		selection := Selection{Mode: SelectionCurrentFilter, Filter: SelectionFilter{Search: "vip-search"}}
		targets, err := svc.resolveSelection(context.Background(), "u", "a", selection, adminGroups())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(targets) != 1 || targets[0].Name != "vip" {
			t.Fatalf("expected only vip, got %+v", targets)
		}
	})

	t.Run("currentFilter mode with nil lookup returns empty selection error", func(t *testing.T) {
		svc := svcWithLookup(nil)
		selection := Selection{Mode: SelectionCurrentFilter}
		_, err := svc.resolveSelection(context.Background(), "u", "a", selection, adminGroups())
		if err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})

	t.Run("unknown mode returns empty selection error", func(t *testing.T) {
		svc := svcWithLookup(nil)
		selection := Selection{Mode: SelectionMode("bogus")}
		_, err := svc.resolveSelection(context.Background(), "u", "a", selection, adminGroups())
		if err != ErrEmptySelection {
			t.Fatalf("expected ErrEmptySelection, got %v", err)
		}
	})

	t.Run("lookup error propagates", func(t *testing.T) {
		wantErr := errors.New("lookup failed")
		lookup := &fakeTypeLookup{err: wantErr}
		svc := svcWithLookup(lookup)
		selection := Selection{Mode: SelectionType, Types: []string{"promo"}}
		_, err := svc.resolveSelection(context.Background(), "u", "a", selection, adminGroups())
		if !errors.Is(err, wantErr) {
			t.Fatalf("expected lookup error, got %v", err)
		}
	})
}

func TestNormalizeNotify(t *testing.T) {
	cfg := Config{
		DefaultNotifyBotIDs:  []string{"default-bot"},
		StartTemplateDefault: "start default",
		EndTemplateDefault:   "end default",
	}

	t.Run("disabled notify collapses to bare disabled struct", func(t *testing.T) {
		got := normalizeNotify(Notify{Enabled: false, BotIDs: []string{"explicit"}, StartTemplate: "x"}, cfg)
		if got.Enabled || len(got.BotIDs) != 0 || got.StartTemplate != "" || got.EndTemplate != "" {
			t.Fatalf("expected bare disabled struct, got %+v", got)
		}
	})

	t.Run("enabled notify fills empty fields from config defaults", func(t *testing.T) {
		got := normalizeNotify(Notify{Enabled: true}, cfg)
		if len(got.BotIDs) != 1 || got.BotIDs[0] != "default-bot" {
			t.Errorf("expected default bot ids, got %v", got.BotIDs)
		}
		if got.StartTemplate != "start default" {
			t.Errorf("expected default start template, got %q", got.StartTemplate)
		}
		if got.EndTemplate != "end default" {
			t.Errorf("expected default end template, got %q", got.EndTemplate)
		}
	})

	t.Run("enabled notify never overrides explicit non-empty values", func(t *testing.T) {
		explicit := Notify{Enabled: true, BotIDs: []string{"explicit-bot"}, StartTemplate: "custom start", EndTemplate: "custom end"}
		got := normalizeNotify(explicit, cfg)
		if got.BotIDs[0] != "explicit-bot" {
			t.Errorf("expected explicit bot ids preserved, got %v", got.BotIDs)
		}
		if got.StartTemplate != "custom start" {
			t.Errorf("expected explicit start template preserved, got %q", got.StartTemplate)
		}
		if got.EndTemplate != "custom end" {
			t.Errorf("expected explicit end template preserved, got %q", got.EndTemplate)
		}
	})
}

func TestNormalizeListQuery(t *testing.T) {
	tests := []struct {
		name         string
		in           ListQuery
		wantPage     int
		wantPageSize int
	}{
		{name: "defaults applied for zero values", in: ListQuery{}, wantPage: 1, wantPageSize: 20},
		{name: "negative page clamped to 1", in: ListQuery{Page: -5, PageSize: 10}, wantPage: 1, wantPageSize: 10},
		{name: "oversized page size clamped to 100", in: ListQuery{Page: 2, PageSize: 500}, wantPage: 2, wantPageSize: 100},
		{name: "status trimmed", in: ListQuery{Page: 1, PageSize: 10, Status: "  running  "}, wantPage: 1, wantPageSize: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeListQuery(tt.in)
			if got.Page != tt.wantPage {
				t.Errorf("expected page %d, got %d", tt.wantPage, got.Page)
			}
			if got.PageSize != tt.wantPageSize {
				t.Errorf("expected pageSize %d, got %d", tt.wantPageSize, got.PageSize)
			}
		})
	}
}

func TestTotalPages(t *testing.T) {
	tests := []struct {
		total    int
		pageSize int
		want     int
	}{
		{total: 0, pageSize: 10, want: 0},
		{total: 10, pageSize: 0, want: 0},
		{total: 10, pageSize: 10, want: 1},
		{total: 11, pageSize: 10, want: 2},
		{total: 25, pageSize: 10, want: 3},
	}
	for _, tt := range tests {
		if got := totalPages(tt.total, tt.pageSize); got != tt.want {
			t.Errorf("totalPages(%d, %d) = %d, want %d", tt.total, tt.pageSize, got, tt.want)
		}
	}
}

func TestToListItemAndToDetail(t *testing.T) {
	startedAt := time.Now().Add(-time.Hour)
	endedAt := time.Now()
	campaign := Campaign{
		ID:        "c1",
		UserID:    "user1",
		Name:      "activity",
		Status:    StatusEnded,
		StartMode: StartNow,
		EndMode:   EndManual,
		StartedAt: &startedAt,
		EndedAt:   &endedAt,
		Notify:    Notify{Enabled: true},
	}
	items := []CampaignItem{
		{GroupName: "vip", ApplyStatus: ItemApplied, RestoreStatus: ItemRestored},
		{GroupName: "svip", ApplyStatus: ItemFailed, RestoreStatus: ItemPending},
	}

	listItem := toListItem(campaign, items)
	if listItem.LastExecutedAt != campaign.EndedAt {
		t.Errorf("expected lastExecutedAt to prefer endedAt when set")
	}
	if listItem.Summary.Total != 2 || listItem.Summary.Applied != 1 || listItem.Summary.ApplyFailed != 1 {
		t.Errorf("unexpected summary: %+v", listItem.Summary)
	}
	if !listItem.NotifyEnabled {
		t.Errorf("expected notifyEnabled true")
	}

	detail := toDetail(campaign, items)
	if len(detail.Items) != 2 {
		t.Fatalf("expected 2 item views, got %d", len(detail.Items))
	}
	if detail.Items[0].GroupName != "vip" || detail.Items[1].GroupName != "svip" {
		t.Errorf("expected item order preserved, got %+v", detail.Items)
	}
}

func TestToListItemLastExecutedFallsBackToStartedAt(t *testing.T) {
	startedAt := time.Now()
	campaign := Campaign{StartedAt: &startedAt}
	listItem := toListItem(campaign, nil)
	if listItem.LastExecutedAt != campaign.StartedAt {
		t.Errorf("expected lastExecutedAt to fall back to startedAt when endedAt is nil")
	}
}

func TestFindGroup(t *testing.T) {
	groups := adminGroups()
	if g := findGroup(groups, " vip "); g == nil || g.Name != "vip" {
		t.Errorf("expected to find vip with trimmed match, got %+v", g)
	}
	if g := findGroup(groups, "missing"); g != nil {
		t.Errorf("expected nil for missing group, got %+v", g)
	}
}

func TestFloatOrZero(t *testing.T) {
	if floatOrZero(nil) != 0 {
		t.Errorf("expected 0 for nil pointer")
	}
	if floatOrZero(floatPtr(2.5)) != 2.5 {
		t.Errorf("expected 2.5")
	}
}

func TestToSet(t *testing.T) {
	set := toSet([]string{" a ", "b", "a"})
	if len(set) != 2 {
		t.Fatalf("expected 2 unique entries, got %d", len(set))
	}
	if _, ok := set["a"]; !ok {
		t.Errorf("expected trimmed key 'a' to be present")
	}
}

func TestFormatTime(t *testing.T) {
	if formatTime(nil) != "" {
		t.Errorf("expected empty string for nil time")
	}
	now := time.Now()
	if formatTime(&now) == "" {
		t.Errorf("expected non-empty formatted time")
	}
}

// partialCampaignFixture builds a repository pre-loaded with one partial campaign that has
// three items: one applied-but-not-restored (must be restored), one failed-to-apply (must
// never be restored), and one already-restored (must not be restored again).
func partialCampaignFixture(endMode EndMode, endAt *time.Time) (*fakeRepository, string) {
	repo := newFakeRepository()
	campaignID := "campaign-partial"
	repo.campaigns[campaignID] = &Campaign{
		ID:             campaignID,
		UserID:         "user1",
		AdminAccountID: "account1",
		Name:           "partial activity",
		Status:         StatusPartial,
		EndMode:        endMode,
		EndAt:          endAt,
	}
	repo.items[campaignID] = []*CampaignItem{
		{ID: "item-applied", CampaignID: campaignID, GroupName: "vip", OriginalMultiplier: floatPtr(1.0), ApplyStatus: ItemApplied, RestoreStatus: ItemPending},
		{ID: "item-failed", CampaignID: campaignID, GroupName: "svip", OriginalMultiplier: floatPtr(3.0), ApplyStatus: ItemFailed, RestoreStatus: ItemPending},
		{ID: "item-already-restored", CampaignID: campaignID, GroupName: "default", OriginalMultiplier: floatPtr(1.0), ApplyStatus: ItemApplied, RestoreStatus: ItemRestored},
	}
	return repo, campaignID
}

// remoteGroupsAfterPartialStart returns the admin's remote groups as they would look right
// after a partial start: "vip" was actually changed to 5.0 by the campaign, "svip" was never
// touched because it failed to apply, "default" already sits back at its original value.
func remoteGroupsAfterPartialStart() []upstream.AdminGroupInfo {
	return []upstream.AdminGroupInfo{
		{ID: "g1", Name: "default", Multiplier: floatPtr(1.0)},
		{ID: "g2", Name: "vip", Multiplier: floatPtr(5.0)},
		{ID: "g3", Name: "svip", Multiplier: floatPtr(3.0)},
	}
}

func TestEndManuallyRestoresPartialCampaign(t *testing.T) {
	repo, campaignID := partialCampaignFixture(EndManual, nil)
	operator := &fakeOperator{groups: remoteGroupsAfterPartialStart()}
	svc := &Service{repository: repo, operator: operator, config: Config{}}

	detail, err := svc.End(context.Background(), "user1", "account1", campaignID)
	if err != nil {
		t.Fatalf("expected End to accept a partial campaign, got error: %v", err)
	}
	if detail.Status != StatusEnded {
		t.Fatalf("expected campaign to end as %q once the only outstanding item is restored, got %q", StatusEnded, detail.Status)
	}

	// Only the applied-but-unrestored item should trigger a remote write.
	if operator.updateCalls != 1 {
		t.Fatalf("expected exactly 1 remote restore call, got %d", operator.updateCalls)
	}

	items := repo.items[campaignID]
	byID := map[string]*CampaignItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	if byID["item-applied"].RestoreStatus != ItemRestored {
		t.Errorf("expected applied item to be restored, got restoreStatus=%q", byID["item-applied"].RestoreStatus)
	}
	if byID["item-failed"].RestoreStatus != ItemPending {
		t.Errorf("expected failed-to-apply item to be left untouched, got restoreStatus=%q", byID["item-failed"].RestoreStatus)
	}
	if byID["item-already-restored"].RestoreStatus != ItemRestored {
		t.Errorf("expected already-restored item to remain restored")
	}
}

func TestEndRejectsCampaignsThatAreNotRunningOrPartial(t *testing.T) {
	repo := newFakeRepository()
	repo.campaigns["draft-campaign"] = &Campaign{ID: "draft-campaign", UserID: "user1", AdminAccountID: "account1", Status: StatusDraft}
	svc := &Service{repository: repo, operator: &fakeOperator{}, config: Config{}}

	_, err := svc.End(context.Background(), "user1", "account1", "draft-campaign")
	if err != ErrInvalidState {
		t.Fatalf("expected ErrInvalidState for a draft campaign, got %v", err)
	}
}

func TestSchedulerRestoresDuePartialCampaign(t *testing.T) {
	due := time.Now().Add(-time.Minute)
	repo, campaignID := partialCampaignFixture(EndScheduled, &due)
	operator := &fakeOperator{groups: remoteGroupsAfterPartialStart()}
	svc := &Service{repository: repo, operator: operator, config: Config{}}

	svc.runSchedulerTick(context.Background())

	campaign := repo.campaigns[campaignID]
	if campaign.Status != StatusEnded {
		t.Fatalf("expected scheduler to restore the due partial campaign to %q, got %q", StatusEnded, campaign.Status)
	}
	if operator.updateCalls != 1 {
		t.Fatalf("expected exactly 1 remote restore call from the scheduler tick, got %d", operator.updateCalls)
	}
}

func TestSchedulerIgnoresPartialCampaignNotYetDue(t *testing.T) {
	notDue := time.Now().Add(time.Hour)
	repo, campaignID := partialCampaignFixture(EndScheduled, &notDue)
	operator := &fakeOperator{groups: remoteGroupsAfterPartialStart()}
	svc := &Service{repository: repo, operator: operator, config: Config{}}

	svc.runSchedulerTick(context.Background())

	campaign := repo.campaigns[campaignID]
	if campaign.Status != StatusPartial {
		t.Fatalf("expected campaign not yet due to remain %q, got %q", StatusPartial, campaign.Status)
	}
	if operator.updateCalls != 0 {
		t.Fatalf("expected no remote calls before the end time is reached, got %d", operator.updateCalls)
	}
}

// TestEndingClaimIsIdempotentForPartialCampaigns proves that repeatedly triggering the
// ending flow for the same partial campaign (e.g. two overlapping scheduler ticks, or a
// scheduler tick racing a manual "end" click) restores each item at most once.
func TestEndingClaimIsIdempotentForPartialCampaigns(t *testing.T) {
	repo, campaignID := partialCampaignFixture(EndManual, nil)
	operator := &fakeOperator{groups: remoteGroupsAfterPartialStart()}
	svc := &Service{repository: repo, operator: operator, config: Config{}}

	svc.endCampaign(context.Background(), campaignID)
	firstCallCount := operator.updateCalls
	if firstCallCount != 1 {
		t.Fatalf("expected the first endCampaign call to restore exactly 1 item, got %d", firstCallCount)
	}

	// The campaign is now "ended", so a second invocation must not re-claim it or
	// perform any further remote writes, even though nothing prevents it from being called again.
	svc.endCampaign(context.Background(), campaignID)
	if operator.updateCalls != firstCallCount {
		t.Fatalf("expected no additional remote calls on a repeated endCampaign call, got %d (was %d)", operator.updateCalls, firstCallCount)
	}

	claimed, err := repo.ClaimForEnding(context.Background(), campaignID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claimed {
		t.Fatalf("expected ClaimForEnding to fail once the campaign has already ended")
	}
}
