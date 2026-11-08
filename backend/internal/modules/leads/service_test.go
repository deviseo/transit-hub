package leads

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/mass_email"
)

var testWorkspace = Workspace{UserID: "user-1", AdminAccountID: "admin-1"}

func TestParseImportFindsEmailsInMessyText(t *testing.T) {
	text := "email,name\n" +
		"alice@example.com,Alice\n" +
		"Bob <bob@example.org>; carol@example.net，dave@example.cn、ALICE@example.com\n" +
		"邮箱：erin@mail.example.com。 mailto:frank@example.io\n" +
		"broken@@example.com bad@localhost no-at-sign 联系我"
	emails, duplicates, invalid := parseImport(text)
	got := make([]string, 0, len(emails))
	for _, email := range emails {
		got = append(got, email.normalized)
	}
	want := "alice@example.com bob@example.org carol@example.net dave@example.cn erin@mail.example.com frank@example.io"
	if strings.Join(got, " ") != want {
		t.Fatalf("emails = %v", got)
	}
	if duplicates != 1 || invalid != 2 {
		t.Fatalf("duplicates=%d invalid=%d", duplicates, invalid)
	}
}

func TestNormalizeEmailRejectsNamesAndDomainsWithoutDots(t *testing.T) {
	for _, bad := range []string{"", "a@b", "Name <a@b.com>", "a b@c.com", "a@.com", "a@b..com", "a@b.com.", "@b.com"} {
		if _, _, ok := normalizeEmail(bad); ok {
			t.Errorf("%q should be rejected", bad)
		}
	}
	email, normalized, ok := normalizeEmail("  Mixed.Case@Example.COM ")
	if !ok || email != "Mixed.Case@Example.COM" || normalized != "mixed.case@example.com" {
		t.Fatalf("got %q %q %t", email, normalized, ok)
	}
}

func TestAddChecksFormatSuppressionAndDuplicates(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, &fakeBatches{})
	ctx := context.Background()
	if _, err := service.Add(ctx, testWorkspace, AddRequest{Email: "nope"}); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("invalid email: %v", err)
	}
	store.suppressed["gone@example.com"] = struct{}{}
	if _, err := service.Add(ctx, testWorkspace, AddRequest{Email: "Gone@example.com"}); !errors.Is(err, ErrSuppressed) {
		t.Fatalf("unsubscribed email: %v", err)
	}
	lead, err := service.Add(ctx, testWorkspace, AddRequest{Email: "new@example.com", Source: "  V2EX\n帖子  ", Note: strings.Repeat("注", 300)})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if lead.Source != "V2EX 帖子" || len([]rune(lead.Note)) != maxNoteRunes {
		t.Fatalf("source and note should be cleaned up: %+v", lead)
	}
	if _, err := service.Add(ctx, testWorkspace, AddRequest{Email: "NEW@example.com"}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
}

func TestImportCountsEveryOutcome(t *testing.T) {
	store := newFakeStore()
	store.existing["old@example.com"] = struct{}{}
	store.suppressed["gone@example.com"] = struct{}{}
	service := NewService(store, &fakeBatches{})
	result, err := service.Import(context.Background(), testWorkspace, ImportRequest{
		Text:   "a@example.com b@example.com a@example.com old@example.com gone@example.com bad@x",
		Source: "QQ 群",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != (ImportResult{Added: 2, Duplicates: 2, Invalid: 1, Suppressed: 1}) {
		t.Fatalf("result = %+v", result)
	}
	if store.lastSource != "QQ 群" {
		t.Fatalf("source = %q", store.lastSource)
	}
	tooMany := strings.Repeat("x@example.com ", 1) + manyEmails(maxImportEmails+1)
	if _, err := service.Import(context.Background(), testWorkspace, ImportRequest{Text: tooMany}); !errors.Is(err, ErrTooMany) {
		t.Fatalf("too many: %v", err)
	}
}

func TestSendRequiresUnsubscribeLinkAndRespectsTheDailyLimit(t *testing.T) {
	store := newFakeStore()
	store.recipients = []mass_email.LeadRecipient{{LeadID: "l1", Email: "a@example.com"}, {LeadID: "l2", Email: "b@example.com"}, {LeadID: "l3", Email: "c@example.com"}}
	store.dailyLimit = 10
	store.sentSince = 8
	batches := &fakeBatches{}
	service := NewService(store, batches)
	ctx := context.Background()
	request := SendRequest{TemplateID: "tpl-1", RequestID: "req-1", PublicBaseURL: "https://hub.example.com/api", AdLabel: true, Selection: Selection{Mode: SelectionFilter, Source: "QQ 群"}}

	noLink := request
	noLink.PublicBaseURL = " "
	if _, err := service.Send(ctx, testWorkspace, noLink); !errors.Is(err, ErrUnsubscribeRequired) {
		t.Fatalf("unsubscribe link is required: %v", err)
	}
	if _, err := service.Send(ctx, testWorkspace, request); !errors.Is(err, ErrDailyLimit) {
		t.Fatalf("3 recipients with 2 left must be refused: %v", err)
	}
	if batches.created != 0 {
		t.Fatal("nothing should be queued")
	}
	if store.since.IsZero() || time.Since(store.since) < 23*time.Hour {
		t.Fatalf("the limit counts the last 24 hours, since=%v", store.since)
	}

	store.sentSince = 7
	batch, err := service.Send(ctx, testWorkspace, request)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if batches.created != 1 || len(batches.recipients) != 3 || !batches.request.AdLabel || batches.request.PublicBaseURL != request.PublicBaseURL || batch.ID != "batch-1" {
		t.Fatalf("unexpected handoff: %+v %+v", batches, batch)
	}

	// 重试同一个请求：直接拿回建好的批次，即使现在已经到上限了。
	store.sentSince = 10
	again, err := service.Send(ctx, testWorkspace, request)
	if err != nil || again.ID != batch.ID || batches.created != 1 {
		t.Fatalf("retry should return the same batch: %+v %v", again, err)
	}

	store.recipients = nil
	other := request
	other.RequestID = "req-2"
	store.sentSince = 0
	if _, err := service.Send(ctx, testWorkspace, other); !errors.Is(err, ErrNoRecipients) {
		t.Fatalf("no recipients: %v", err)
	}
}

func TestSelectionAndSettingsValidation(t *testing.T) {
	service := NewService(newFakeStore(), &fakeBatches{})
	ctx := context.Background()
	for _, selection := range []Selection{
		{Mode: ""},
		{Mode: SelectionSelected},
		{Mode: SelectionSelected, IDs: []string{"a", " "}},
		{Mode: SelectionFilter, Status: "whatever"},
	} {
		if _, err := service.Preview(ctx, testWorkspace, selection); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("selection %+v: %v", selection, err)
		}
	}
	for _, limit := range []int{0, -1, maxDailyLimit + 1} {
		if _, err := service.SaveSettings(ctx, testWorkspace, SettingsRequest{DailyLimit: limit}); !errors.Is(err, ErrInvalidDailyLimit) {
			t.Errorf("limit %d: %v", limit, err)
		}
	}
	settings, err := service.SaveSettings(ctx, testWorkspace, SettingsRequest{DailyLimit: 50})
	if err != nil || settings.DailyLimit != 50 || settings.Remaining != 50 {
		t.Fatalf("settings: %+v %v", settings, err)
	}
}

func manyEmails(count int) string {
	var builder strings.Builder
	for index := 0; index < count; index++ {
		builder.WriteString("user")
		builder.WriteString(strings.Repeat("x", index%7))
		builder.WriteString(string(rune('a' + index%26)))
		builder.WriteString(itoa(index))
		builder.WriteString("@example.com\n")
	}
	return builder.String()
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

type fakeStore struct {
	leads      map[string]Lead
	existing   map[string]struct{}
	suppressed map[string]struct{}
	recipients []mass_email.LeadRecipient
	dailyLimit int
	sentSince  int
	since      time.Time
	lastSource string
}

func newFakeStore() *fakeStore {
	return &fakeStore{leads: map[string]Lead{}, existing: map[string]struct{}{}, suppressed: map[string]struct{}{}, dailyLimit: defaultDailyLimit}
}

func (f *fakeStore) Counts(ctx context.Context, ws Workspace, filter leadFilter) (StatusCounts, error) {
	return StatusCounts{}, nil
}

func (f *fakeStore) Page(ctx context.Context, ws Workspace, filter leadFilter, status string, limit int, offset int) ([]Lead, error) {
	return nil, nil
}

func (f *fakeStore) Get(ctx context.Context, ws Workspace, id string) (Lead, error) {
	lead, ok := f.leads[id]
	if !ok {
		return Lead{}, ErrNotFound
	}
	return lead, nil
}

func (f *fakeStore) Selection(ctx context.Context, ws Workspace, filter leadFilter, status string) (int, int, error) {
	return len(f.recipients), 0, nil
}

func (f *fakeStore) SendableRecipients(ctx context.Context, ws Workspace, filter leadFilter, status string, limit int) ([]mass_email.LeadRecipient, error) {
	return f.recipients, nil
}

func (f *fakeStore) Insert(ctx context.Context, ws Workspace, lead newLead) (bool, error) {
	if _, exists := f.existing[lead.Normalized]; exists {
		return false, nil
	}
	f.existing[lead.Normalized] = struct{}{}
	f.leads[lead.ID] = Lead{ID: lead.ID, Email: lead.Email, Source: lead.Source, Note: lead.Note, Status: StatusNotSent}
	return true, nil
}

func (f *fakeStore) InsertMany(ctx context.Context, ws Workspace, source string, leads []newLead) (int, error) {
	f.lastSource = source
	added := 0
	for _, lead := range leads {
		lead.Source = source
		if created, _ := f.Insert(ctx, ws, lead); created {
			added++
		}
	}
	return added, nil
}

func (f *fakeStore) Suppressed(ctx context.Context, ws Workspace, normalized []string) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	for _, email := range normalized {
		if _, ok := f.suppressed[email]; ok {
			result[email] = struct{}{}
		}
	}
	return result, nil
}

func (f *fakeStore) Sources(ctx context.Context, ws Workspace) ([]SourceCount, error) {
	return nil, nil
}

func (f *fakeStore) Update(ctx context.Context, ws Workspace, id string, source string, note string) error {
	return nil
}

func (f *fakeStore) Delete(ctx context.Context, ws Workspace, ids []string) (int, error) {
	return len(ids), nil
}

func (f *fakeStore) SentSince(ctx context.Context, ws Workspace, since time.Time) (int, error) {
	f.since = since
	return f.sentSince, nil
}

func (f *fakeStore) DailyLimit(ctx context.Context, ws Workspace) (int, error) {
	return f.dailyLimit, nil
}

func (f *fakeStore) SetDailyLimit(ctx context.Context, ws Workspace, limit int) error {
	f.dailyLimit = limit
	return nil
}

type fakeBatches struct {
	created    int
	request    mass_email.CreateBatchRequest
	recipients []mass_email.LeadRecipient
	byRequest  map[string]mass_email.BatchDTO
}

func (f *fakeBatches) CreateLeadBatch(ctx context.Context, userID string, adminAccountID string, req mass_email.CreateBatchRequest, recipients []mass_email.LeadRecipient) (mass_email.BatchDTO, error) {
	f.created++
	f.request, f.recipients = req, recipients
	batch := mass_email.BatchDTO{ID: "batch-1", RequestID: req.RequestID, RecipientCount: len(recipients)}
	if f.byRequest == nil {
		f.byRequest = map[string]mass_email.BatchDTO{}
	}
	f.byRequest[req.RequestID] = batch
	return batch, nil
}

func (f *fakeBatches) BatchForRequest(ctx context.Context, userID string, adminAccountID string, requestID string) (mass_email.BatchDTO, bool) {
	batch, ok := f.byRequest[requestID]
	return batch, ok
}
