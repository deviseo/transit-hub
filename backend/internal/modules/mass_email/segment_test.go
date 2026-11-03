package mass_email

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/modules/upstream"
)

// 测试数据均为合成数据（example.test）。

// suppressingRepo 在 fakeRepo 之上加退订名单能力（对应 *Repository 实现的 suppressionStore）。
type suppressingRepo struct {
	*fakeRepo
	suppressed map[string]string // workspace|email -> reason
	itemsByID  map[string]BatchItem
}

func newSuppressingRepo() *suppressingRepo {
	return &suppressingRepo{fakeRepo: newFakeRepo(), suppressed: map[string]string{}, itemsByID: map[string]BatchItem{}}
}

func suppressionMapKey(userID, adminAccountID, email string) string {
	return userID + "/" + adminAccountID + "|" + email
}

func (r *suppressingRepo) GetItemByID(ctx context.Context, id string) (BatchItem, error) {
	item, ok := r.itemsByID[id]
	if !ok {
		return BatchItem{}, ErrNotFound
	}
	return item, nil
}

func (r *suppressingRepo) IsSuppressed(ctx context.Context, userID, adminAccountID, normalizedEmail string) (bool, error) {
	_, ok := r.suppressed[suppressionMapKey(userID, adminAccountID, normalizedEmail)]
	return ok, nil
}

func (r *suppressingRepo) SuppressedAmong(ctx context.Context, userID, adminAccountID string, normalizedEmails []string) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	for _, email := range normalizedEmails {
		if _, ok := r.suppressed[suppressionMapKey(userID, adminAccountID, email)]; ok {
			result[email] = struct{}{}
		}
	}
	return result, nil
}

func (r *suppressingRepo) AddSuppressions(ctx context.Context, userID, adminAccountID string, normalizedEmails []string, reason string) (int, error) {
	added := 0
	for _, email := range normalizedEmails {
		key := suppressionMapKey(userID, adminAccountID, email)
		if _, ok := r.suppressed[key]; !ok {
			r.suppressed[key] = reason
			added++
		}
	}
	return added, nil
}

func (r *suppressingRepo) RemoveSuppression(ctx context.Context, userID, adminAccountID, normalizedEmail string) (bool, error) {
	key := suppressionMapKey(userID, adminAccountID, normalizedEmail)
	_, ok := r.suppressed[key]
	delete(r.suppressed, key)
	return ok, nil
}

func (r *suppressingRepo) ListSuppressions(ctx context.Context, userID, adminAccountID, search string, page, pageSize int) ([]Suppression, int, error) {
	return []Suppression{}, len(r.suppressed), nil
}

type countingSessions struct{ calls int }

func (c *countingSessions) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	c.calls++
	return upstream.Session{}, errors.New("segment batches must not need the upstream session")
}

type fakeSegments struct {
	recipients []SegmentRecipient
	err        error
	filters    BatchFilters
	max        int
}

func (f *fakeSegments) ResolveSegmentRecipients(ctx context.Context, userID, adminAccountID string, filters BatchFilters, max int) ([]SegmentRecipient, error) {
	f.filters = filters
	f.max = max
	return f.recipients, f.err
}

type recordingSettings struct {
	fakeSettings
	bodies []string
}

func (r *recordingSettings) SendSavedSMTPEmailForWorkspace(ctx context.Context, userID, adminAccountID, recipientEmail, subject, htmlBody string) error {
	r.bodies = append(r.bodies, htmlBody)
	return nil
}

func testTemplateSettings() *fakeSettings {
	return &fakeSettings{template: settings.EmailTemplateSnapshot{ID: "tpl-1", Name: "T", Subject: "S", HTMLBody: "<p>Body</p>"}}
}

func TestCreateSegmentBatchUsesLocalRecipientsAndSkipsSuppressed(t *testing.T) {
	repo := newSuppressingRepo()
	repo.suppressed[suppressionMapKey("user-1", "admin-1", "blocked@example.test")] = SuppressionReasonUnsubscribe
	sessions := &countingSessions{}
	service := NewService(repo, sessions, &fakeUsers{}, testTemplateSettings())
	segments := &fakeSegments{recipients: []SegmentRecipient{
		{UpstreamUserID: "1", Email: "Alice@Example.test", Username: "alice"},
		{UpstreamUserID: "2", Email: "alice@example.test", Username: "dup"},
		{UpstreamUserID: "3", Email: "not-an-email", Username: "bad"},
		{UpstreamUserID: "4", Email: "blocked@example.test", Username: "blocked"},
		{UpstreamUserID: "5", Email: "carol@example.test", Username: "carol"},
	}}
	service.SetSegmentResolver(segments)

	batch, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{
		TemplateID: "tpl-1", SelectionMode: SelectionModeSegment, RequestID: "req-1",
		Filters:       BatchFilters{Segment: " inactive ", InactiveDays: 10, Status: "active"},
		PublicBaseURL: "https://th.example.test/api/",
	})
	if err != nil {
		t.Fatalf("CreateBatch returned error: %v", err)
	}
	if sessions.calls != 0 {
		t.Fatal("segment batches resolve recipients locally and must not require the upstream session")
	}
	if batch.SelectionMode != SelectionModeSegment || batch.RecipientCount != 2 || batch.SkippedCount != 3 || !batch.UnsubscribeLink {
		t.Fatalf("expected 2 recipients and 3 skipped (duplicate, invalid, suppressed), got %+v", batch)
	}
	if segments.filters.Segment != "inactive" || segments.filters.InactiveDays != 10 || segments.max != maxBatchRecipients {
		t.Fatalf("unexpected resolver call: %+v max=%d", segments.filters, segments.max)
	}
	stored := repo.batches[batch.ID]
	if stored.PublicBaseURL != "https://th.example.test/api" {
		t.Fatalf("public base url must be normalized, got %q", stored.PublicBaseURL)
	}
	items := repo.items[batch.ID]
	if len(items) != 2 || items[0].RecipientEmail != "Alice@Example.test" || items[1].UpstreamUserID != "5" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestCreateBatchAllModeAlsoSkipsSuppressed(t *testing.T) {
	repo := newSuppressingRepo()
	repo.suppressed[suppressionMapKey("user-1", "admin-1", "bob@example.test")] = SuppressionReasonManual
	users := &fakeUsers{page: upstream.Sub2APIAdminUsersPage{Items: []upstream.Sub2APIAdminUser{
		{ID: "1", Email: "alice@example.test"}, {ID: "2", Email: "Bob@example.test"},
	}, Total: 2, Pages: 1, TotalKnown: true, PagesKnown: true}}
	service := NewService(repo, &fakeSessions{}, users, testTemplateSettings())

	batch, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeAll, RequestID: "req-all"})
	if err != nil {
		t.Fatalf("CreateBatch returned error: %v", err)
	}
	if batch.RecipientCount != 1 || batch.SkippedCount != 1 || batch.UnsubscribeLink {
		t.Fatalf("unsubscribed addresses must be skipped in every mode: %+v", batch)
	}
}

func TestCreateSegmentBatchValidation(t *testing.T) {
	service := NewService(newSuppressingRepo(), &fakeSessions{}, &fakeUsers{}, testTemplateSettings())
	_, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeSegment, RequestID: "r1", Filters: BatchFilters{Segment: "all"}})
	if !errors.Is(err, ErrSegmentUnavailable) {
		t.Fatalf("expected segment unavailable without resolver, got %v", err)
	}

	service.SetSegmentResolver(&fakeSegments{})
	if _, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeSegment, RequestID: "r2"}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("segment mode requires a segment, got %v", err)
	}
	if _, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeSegment, RequestID: "r3", Filters: BatchFilters{Segment: "all"}, PublicBaseURL: "javascript:alert(1)"}); !errors.Is(err, ErrInvalidPublicURL) {
		t.Fatalf("expected invalid public url, got %v", err)
	}

	service.SetSegmentResolver(&fakeSegments{err: ErrRecipientLimitReached})
	if _, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeSegment, RequestID: "r4", Filters: BatchFilters{Segment: "all"}}); !errors.Is(err, ErrRecipientLimitReached) {
		t.Fatalf("resolver limit errors must pass through, got %v", err)
	}
	service.SetSegmentResolver(&fakeSegments{err: errors.New("db down")})
	if _, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeSegment, RequestID: "r5", Filters: BatchFilters{Segment: "all"}}); !errors.Is(err, ErrPersistence) {
		t.Fatalf("unexpected resolver errors must become persistence errors, got %v", err)
	}
}

func TestNormalizePublicBaseURL(t *testing.T) {
	cases := map[string]string{
		"":                               "",
		"  https://th.example.test/api ": "https://th.example.test/api",
		"http://127.0.0.1:5555/api/":     "http://127.0.0.1:5555/api",
		"https://th.example.test":        "https://th.example.test",
	}
	for input, want := range cases {
		got, err := normalizePublicBaseURL(input)
		if err != nil || got != want {
			t.Fatalf("normalizePublicBaseURL(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"ftp://th.example.test", "https://user:pw@th.example.test", "https://th.example.test/api?x=1", "https://th.example.test/#x", "/api", "https://", "https://th.example.test/a\"b", "https://th.example.test/" + strings.Repeat("a", maxPublicBaseURLLength)} {
		if _, err := normalizePublicBaseURL(input); !errors.Is(err, ErrInvalidPublicURL) {
			t.Fatalf("normalizePublicBaseURL(%q) must fail, got %v", input, err)
		}
	}
}

func TestRenderEmailHTML(t *testing.T) {
	link := unsubscribeURL("https://th.example.test/api", "0123456789abcdef0123456789abcdef")
	if link != "https://th.example.test/api/email-unsubscribe?id=0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected link %q", link)
	}
	withPlaceholder := renderEmailHTML(`<a href="{{unsubscribe_url}}">x</a>`, link+"&a=b")
	if withPlaceholder != `<a href="`+link+`&amp;a=b">x</a>` {
		t.Fatalf("placeholder must be replaced with the escaped link, got %q", withPlaceholder)
	}
	withBody := renderEmailHTML("<html><BODY><p>Hi</p></BODY></html>", link)
	if !strings.Contains(withBody, "点此退订") || !strings.HasSuffix(withBody, "</BODY></html>") || strings.Index(withBody, link) > strings.Index(withBody, "</BODY>") {
		t.Fatalf("footer must be inserted before </body>, got %q", withBody)
	}
	if got := renderEmailHTML("<p>Hi</p>", ""); got != "<p>Hi</p>" {
		t.Fatalf("batches without unsubscribe links must keep the template unchanged, got %q", got)
	}
	if got := renderEmailHTML(`<a href="{{unsubscribe_url}}">x</a>`, ""); got != `<a href="">x</a>` {
		t.Fatalf("placeholder must be removed when the batch has no link, got %q", got)
	}
}

func TestProcessOneSkipsSuppressedAndPersonalizesLink(t *testing.T) {
	repo := newSuppressingRepo()
	repo.batches["batch-1"] = Batch{ID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", Status: BatchStatusRunning,
		TemplateSubject: "S", TemplateHTML: "<p>Hi {{unsubscribe_url}}</p>", PublicBaseURL: "https://th.example.test/api"}
	repo.suppressed[suppressionMapKey("user-1", "admin-1", "gone@example.test")] = SuppressionReasonUnsubscribe
	settingsProvider := &recordingSettings{}
	service := NewService(repo, &fakeSessions{}, &fakeUsers{}, settingsProvider)

	service.processOne(context.Background(), BatchItem{ID: "item-gone", BatchID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", RecipientEmail: "Gone@example.test"})
	if got := repo.completed["item-gone"]; got.status != ItemStatusCancelled || got.errorKey != string(ErrSuppressed) {
		t.Fatalf("suppressed recipients must be cancelled before sending, got %+v", got)
	}
	if len(settingsProvider.bodies) != 0 {
		t.Fatal("no email may be sent to a suppressed address")
	}

	service.processOne(context.Background(), BatchItem{ID: "0123456789abcdef0123456789abcdef", BatchID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", RecipientEmail: "ok@example.test"})
	if got := repo.completed["0123456789abcdef0123456789abcdef"]; got.status != ItemStatusSent {
		t.Fatalf("expected sent item, got %+v", got)
	}
	if len(settingsProvider.bodies) != 1 || settingsProvider.bodies[0] != "<p>Hi https://th.example.test/api/email-unsubscribe?id=0123456789abcdef0123456789abcdef</p>" {
		t.Fatalf("each email must carry the recipient's own unsubscribe link, got %v", settingsProvider.bodies)
	}
}

func TestUnsubscribePublicPages(t *testing.T) {
	repo := newSuppressingRepo()
	itemID := "0123456789abcdef0123456789abcdef"
	repo.itemsByID[itemID] = BatchItem{ID: itemID, UserID: "user-1", AdminAccountID: "admin-1", RecipientEmail: "Alice@Example.test", NormalizedEmail: "alice@example.test"}
	service := NewService(repo, &fakeSessions{}, &fakeUsers{}, testTemplateSettings())
	mux := http.NewServeMux()
	RegisterPublicRoutes(mux, service)

	get := httptest.NewRecorder()
	mux.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/email-unsubscribe?id="+itemID, nil))
	body := get.Body.String()
	if get.Code != http.StatusOK || !strings.Contains(body, "A***@Example.test") || !strings.Contains(body, `<form method="post">`) || strings.Contains(body, "Alice@Example.test") {
		t.Fatalf("GET must show a confirmation form with a masked address, got %d %s", get.Code, body)
	}
	if csp := get.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") || get.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected security headers: %v", get.Header())
	}
	if len(repo.suppressed) != 0 {
		t.Fatal("GET must not unsubscribe (mail scanners prefetch links)")
	}

	post := httptest.NewRecorder()
	mux.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/email-unsubscribe?id="+itemID, strings.NewReader("List-Unsubscribe=One-Click")))
	if post.Code != http.StatusOK || repo.suppressed[suppressionMapKey("user-1", "admin-1", "alice@example.test")] != SuppressionReasonUnsubscribe {
		t.Fatalf("POST must add the address to the workspace suppression list, got %d %v", post.Code, repo.suppressed)
	}

	for _, id := range []string{"", "nothex", strings.Repeat("f", 32)} {
		invalid := httptest.NewRecorder()
		mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/email-unsubscribe?id="+id, nil))
		if invalid.Code != http.StatusNotFound {
			t.Fatalf("id %q must be rejected, got %d", id, invalid.Code)
		}
	}
}

func TestManageSuppressions(t *testing.T) {
	repo := newSuppressingRepo()
	service := NewService(repo, &fakeSessions{}, &fakeUsers{}, testTemplateSettings())

	result, err := service.AddSuppressions(context.Background(), "user-1", "admin-1", AddSuppressionsRequest{Emails: []string{" Alice@Example.test ", "alice@example.test", "bad", "Name <x@example.test>"}})
	if err != nil || result.Added != 1 || result.Invalid != 2 {
		t.Fatalf("unexpected add result: %+v %v", result, err)
	}
	if _, err := service.AddSuppressions(context.Background(), "user-1", "admin-1", AddSuppressionsRequest{Emails: []string{"bad"}}); !errors.Is(err, ErrInvalidSuppression) {
		t.Fatalf("all-invalid input must be rejected, got %v", err)
	}
	if err := service.RemoveSuppression(context.Background(), "user-1", "admin-1", "ALICE@example.test"); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if err := service.RemoveSuppression(context.Background(), "user-1", "admin-1", "alice@example.test"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removing a missing address must return not found, got %v", err)
	}
	// 未实现退订存储的仓储：退订功能关闭，原有群发行为不变。
	plain := NewService(newFakeRepo(), &fakeSessions{}, &fakeUsers{}, testTemplateSettings())
	if page, err := plain.ListSuppressions(context.Background(), "user-1", "admin-1", "", 1, 20); err != nil || len(page.Items) != 0 {
		t.Fatalf("unexpected page without suppression store: %+v %v", page, err)
	}
}

func TestMaskEmail(t *testing.T) {
	for input, want := range map[string]string{"alice@example.test": "a***@example.test", "张三@example.test": "张***@example.test", "@x": "***", "nope": "***"} {
		if got := maskEmail(input); got != want {
			t.Fatalf("maskEmail(%q) = %q, want %q", input, got, want)
		}
	}
}
