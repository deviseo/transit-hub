package mass_email

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/modules/upstream"
)

func TestCreateBatchSelectedResolvesServerSideDedupeAndSkips(t *testing.T) {
	repo := newFakeRepo()
	users := &fakeUsers{byID: map[string]upstream.Sub2APIAdminUser{
		"1": {ID: "1", Email: "Alice@Example.com", Username: "alice"},
		"2": {ID: "2", Email: "alice@example.com", Username: "dup"},
		"3": {ID: "3", Email: ""},
	}}
	service := newTestService(repo, users, nil)

	batch, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{
		TemplateID: "tpl-1", SelectionMode: SelectionModeSelected, UserIDs: []string{"1", "2", "3"}, RequestID: "req-1",
	})
	if err != nil {
		t.Fatalf("CreateBatch returned error: %v", err)
	}
	if batch.RecipientCount != 1 || batch.SkippedCount != 2 {
		t.Fatalf("expected 1 recipient and 2 skipped, got recipient=%d skipped=%d", batch.RecipientCount, batch.SkippedCount)
	}
	if len(repo.items[batch.ID]) != 1 || repo.items[batch.ID][0].RecipientEmail != "Alice@Example.com" {
		t.Fatalf("expected resolved server-side recipient, got %#v", repo.items[batch.ID])
	}
	if users.fetchOneCalls != 3 {
		t.Fatalf("expected selected IDs to be fetched from upstream, got %d calls", users.fetchOneCalls)
	}
}

func TestCreateBatchDuplicateRequestIDReturnsExistingWithoutSideEffects(t *testing.T) {
	repo := newFakeRepo()
	existing := Batch{ID: "existing", UserID: "user-1", AdminAccountID: "admin-1", RequestID: "req-1", Status: BatchStatusQueued, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	repo.request["user-1/admin-1/req-1"] = existing
	users := &fakeUsers{}
	settingsProvider := &fakeSettings{template: settings.EmailTemplateSnapshot{ID: "tpl-1", Name: "T", Subject: "S", HTMLBody: "<p>x</p>"}}
	service := NewService(repo, &fakeSessions{}, users, settingsProvider)

	batch, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "missing", SelectionMode: SelectionModeSelected, UserIDs: []string{"1"}, RequestID: "req-1"})
	if err != nil {
		t.Fatalf("CreateBatch duplicate returned error: %v", err)
	}
	if batch.ID != "existing" {
		t.Fatalf("expected existing batch, got %q", batch.ID)
	}
	if users.fetchOneCalls != 0 || settingsProvider.snapshotCalls != 0 {
		t.Fatalf("duplicate request should not resolve users or templates")
	}
	if repo.hasActiveCalls != 0 {
		t.Fatalf("duplicate request should return before active precheck, got %d calls", repo.hasActiveCalls)
	}
}

func TestCreateBatchRejectsSecondDistinctActiveBatchButKeepsIdempotency(t *testing.T) {
	repo := newFakeRepo()
	repo.enforceActiveLimit = true
	now := time.Now()
	repo.batches["active-1"] = Batch{ID: "active-1", UserID: "user-1", AdminAccountID: "admin-1", RequestID: "req-active", Status: BatchStatusQueued, CreatedAt: now, UpdatedAt: now}
	repo.request["user-1/admin-1/req-active"] = repo.batches["active-1"]
	users := &fakeUsers{byID: map[string]upstream.Sub2APIAdminUser{"1": {ID: "1", Email: "a@example.com"}}}
	service := newTestService(repo, users, nil)

	duplicate, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "missing", SelectionMode: SelectionModeSelected, UserIDs: []string{"1"}, RequestID: "req-active"})
	if err != nil {
		t.Fatalf("duplicate active request should return existing batch: %v", err)
	}
	if duplicate.ID != "active-1" || users.fetchOneCalls != 0 {
		t.Fatalf("duplicate should return existing before resolving users, got batch=%#v calls=%d", duplicate, users.fetchOneCalls)
	}

	_, err = service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeSelected, UserIDs: []string{"1"}, RequestID: "req-new"})
	if !errors.Is(err, ErrActiveBatchExists) {
		t.Fatalf("expected active batch error for second distinct active batch, got %v", err)
	}
	if users.fetchOneCalls != 0 || repo.hasActiveCalls == 0 {
		t.Fatalf("active precheck should reject before user resolution, user calls=%d precheck calls=%d", users.fetchOneCalls, repo.hasActiveCalls)
	}
}

func TestCreateBatchActivePrecheckRepositoryErrorFailsClosed(t *testing.T) {
	repo := newFakeRepo()
	repo.hasActiveErr = errors.New("database unavailable")
	users := &fakeUsers{byID: map[string]upstream.Sub2APIAdminUser{"1": {ID: "1", Email: "a@example.com"}}}
	settingsProvider := &fakeSettings{template: settings.EmailTemplateSnapshot{ID: "tpl-1", Name: "T", Subject: "S", HTMLBody: "<p>x</p>"}}
	service := NewService(repo, &fakeSessions{}, users, settingsProvider)

	_, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeSelected, UserIDs: []string{"1"}, RequestID: "req-new"})
	if !errors.Is(err, ErrPersistence) {
		t.Fatalf("expected persistence error from precheck failure, got %v", err)
	}
	if users.fetchOneCalls != 0 || settingsProvider.snapshotCalls != 0 {
		t.Fatalf("failed precheck should reject before external work, user calls=%d snapshot calls=%d", users.fetchOneCalls, settingsProvider.snapshotCalls)
	}
}

func TestCreateBatchActiveLimitIsWorkspaceScopedAndTerminalPermitsNext(t *testing.T) {
	repo := newFakeRepo()
	repo.enforceActiveLimit = true
	now := time.Now()
	repo.batches["other-workspace"] = Batch{ID: "other-workspace", UserID: "user-2", AdminAccountID: "admin-2", RequestID: "req-other", Status: BatchStatusRunning, CreatedAt: now, UpdatedAt: now}
	repo.batches["terminal"] = Batch{ID: "terminal", UserID: "user-1", AdminAccountID: "admin-1", RequestID: "req-old", Status: BatchStatusCompleted, CreatedAt: now, UpdatedAt: now}
	users := &fakeUsers{byID: map[string]upstream.Sub2APIAdminUser{"1": {ID: "1", Email: "a@example.com"}}}
	service := newTestService(repo, users, nil)

	created, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeSelected, UserIDs: []string{"1"}, RequestID: "req-new"})
	if err != nil {
		t.Fatalf("terminal same-workspace and active other-workspace batches should permit creation: %v", err)
	}
	if created.ID == "" || created.RequestID != "req-new" {
		t.Fatalf("unexpected created batch: %#v", created)
	}
}

func TestCreateBatchAllModeRecipientLimitFailsWithoutPersistence(t *testing.T) {
	repo := newFakeRepo()
	users := &fakeUsers{pages: map[int]upstream.Sub2APIAdminUsersPage{
		1: {Items: []upstream.Sub2APIAdminUser{{ID: "1", Email: "a@example.com"}}, Total: maxBatchRecipients + 1, Page: 1, PageSize: 100, Pages: 101, TotalKnown: true, PagesKnown: true},
	}}
	service := newTestService(repo, users, nil)

	_, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeAll, RequestID: "req-limit"})
	if !errors.Is(err, ErrRecipientLimitReached) {
		t.Fatalf("expected recipient limit error, got %v", err)
	}
	if len(repo.batches) != 0 || len(repo.items) != 0 {
		t.Fatalf("over-limit all-mode should not persist partial data, batches=%#v items=%#v", repo.batches, repo.items)
	}
}

func TestCreateBatchAllModeUnknownPaginationFailsPastLimitBoundary(t *testing.T) {
	repo := newFakeRepo()
	pageMap := map[int]upstream.Sub2APIAdminUsersPage{}
	for page := 1; page <= pages(maxBatchRecipients, maxUsersPageSize)+1; page++ {
		items := make([]upstream.Sub2APIAdminUser, 0, maxUsersPageSize)
		for i := 0; i < maxUsersPageSize; i++ {
			items = append(items, upstream.Sub2APIAdminUser{ID: fmt.Sprintf("%d-%d", page, i), Email: fmt.Sprintf("user-%d-%d@example.com", page, i)})
		}
		pageMap[page] = upstream.Sub2APIAdminUsersPage{Items: items, Page: page, PageSize: maxUsersPageSize, Total: len(items)}
	}
	service := newTestService(repo, &fakeUsers{pages: pageMap}, nil)

	_, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeAll, RequestID: "req-unknown-limit"})
	if !errors.Is(err, ErrRecipientLimitReached) {
		t.Fatalf("expected unknown-pagination over-limit error, got %v", err)
	}
	if len(repo.batches) != 0 || len(repo.items) != 0 {
		t.Fatalf("unknown over-limit all-mode should not persist partial data, batches=%#v items=%#v", repo.batches, repo.items)
	}
}

func TestListUsersForwardsPaginationFiltersAndTimezone(t *testing.T) {
	users := &fakeUsers{page: upstream.Sub2APIAdminUsersPage{Items: []upstream.Sub2APIAdminUser{{ID: "1", Email: "a@example.com"}}, Total: 1, Page: 2, PageSize: 50, Pages: 1}}
	service := newTestService(newFakeRepo(), users, nil)

	result, err := service.ListUsers(context.Background(), "user-1", "admin-1", UserQuery{Page: 2, PageSize: 50, Status: "active", Role: "user", Search: "  alice+notes  ", SortBy: "email", SortOrder: "asc", Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatalf("ListUsers returned error: %v", err)
	}
	if result.Page != 2 || result.PageSize != 50 || len(result.Items) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	got := users.lastQuery
	if got.Page != 2 || got.PageSize != 50 || got.Status != "active" || got.Role != "user" || got.Search != "alice+notes" || got.SortBy != "email" || got.SortOrder != "asc" || got.Timezone != "Asia/Shanghai" {
		t.Fatalf("query not forwarded correctly: %#v", got)
	}
}

func TestListUsersSearchNormalizesUnicodeRuneCapAndOmitsEmpty(t *testing.T) {
	t.Run("caps unicode runes", func(t *testing.T) {
		users := &fakeUsers{page: upstream.Sub2APIAdminUsersPage{Page: 1, PageSize: 20}}
		service := newTestService(newFakeRepo(), users, nil)
		input := "  " + strings.Repeat("界", maxUserSearchRunes+5) + "  "

		_, err := service.ListUsers(context.Background(), "user-1", "admin-1", UserQuery{Search: input})
		if err != nil {
			t.Fatalf("ListUsers returned error: %v", err)
		}
		if got := users.lastQuery.Search; got != strings.Repeat("界", maxUserSearchRunes) {
			t.Fatalf("search = %q (%d runes), want %d capped runes", got, len([]rune(got)), maxUserSearchRunes)
		}
	})

	t.Run("omits empty after trim", func(t *testing.T) {
		users := &fakeUsers{page: upstream.Sub2APIAdminUsersPage{Page: 1, PageSize: 20}}
		service := newTestService(newFakeRepo(), users, nil)

		_, err := service.ListUsers(context.Background(), "user-1", "admin-1", UserQuery{Search: " \t\n "})
		if err != nil {
			t.Fatalf("ListUsers returned error: %v", err)
		}
		if users.lastQuery.Search != "" {
			t.Fatalf("expected empty normalized search, got %q", users.lastQuery.Search)
		}
	})
}

func TestResolveAllRecipientsReadsEveryPageAndIncludesAdminUnlessRoleFiltered(t *testing.T) {
	users := &fakeUsers{pages: map[int]upstream.Sub2APIAdminUsersPage{
		1: {Items: []upstream.Sub2APIAdminUser{{ID: "1", Email: "admin@example.com", Role: "admin"}}, Page: 1, PageSize: 100, Pages: 2, PagesKnown: true},
		2: {Items: []upstream.Sub2APIAdminUser{{ID: "2", Email: "user@example.com", Role: "user"}}, Page: 2, PageSize: 100, Pages: 2, PagesKnown: true},
	}}
	service := newTestService(newFakeRepo(), users, nil)

	batch, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeAll, Filters: BatchFilters{Status: "active"}, RequestID: "req-all"})
	if err != nil {
		t.Fatalf("CreateBatch returned error: %v", err)
	}
	if batch.RecipientCount != 2 {
		t.Fatalf("expected all pages including admin role, got %d", batch.RecipientCount)
	}
	if users.lastQuery.Role != "" || users.lastQuery.Status != "active" {
		t.Fatalf("unexpected all-mode filters: %#v", users.lastQuery)
	}
}

func TestCreateBatchAllModePersistsNormalizedSearchAndReusesEveryPage(t *testing.T) {
	repo := newFakeRepo()
	users := &fakeUsers{pages: map[int]upstream.Sub2APIAdminUsersPage{
		1: {Items: []upstream.Sub2APIAdminUser{{ID: "1", Email: "first@example.com"}}, Page: 1, PageSize: 100, Pages: 2, PagesKnown: true},
		2: {Items: []upstream.Sub2APIAdminUser{{ID: "2", Email: "second@example.com"}}, Page: 2, PageSize: 100, Pages: 2, PagesKnown: true},
	}}
	service := newTestService(repo, users, nil)
	longSearch := strings.Repeat("界", maxUserSearchRunes+2)
	wantSearch := strings.Repeat("界", maxUserSearchRunes)

	batch, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", SelectionMode: SelectionModeAll, Filters: BatchFilters{Status: " active ", Role: " user ", Search: "  " + longSearch + "  "}, RequestID: "req-all-search"})
	if err != nil {
		t.Fatalf("CreateBatch returned error: %v", err)
	}
	if batch.Filters.Status != "active" || batch.Filters.Role != "user" || batch.Filters.Search != wantSearch {
		t.Fatalf("expected normalized persisted filters, got %#v", batch.Filters)
	}
	if len(users.pageQueries) != 2 {
		t.Fatalf("expected two all-mode page requests, got %#v", users.pageQueries)
	}
	for _, got := range users.pageQueries {
		if got.Search != wantSearch || got.Status != "active" || got.Role != "user" {
			t.Fatalf("expected normalized filters on every page, got %#v", users.pageQueries)
		}
	}
}

func TestProcessOneMarksSendErrorUncertainWithoutRetry(t *testing.T) {
	repo := newFakeRepo()
	batch := Batch{ID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", TemplateSubject: "S", TemplateHTML: "<p>x</p>", Status: BatchStatusRunning, RecipientCount: 1}
	repo.batches[batch.ID] = batch
	settingsProvider := &fakeSettings{sendErr: errors.New("smtp exploded")}
	service := NewService(repo, &fakeSessions{}, &fakeUsers{}, settingsProvider)

	service.processOne(context.Background(), BatchItem{ID: "item-1", BatchID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", RecipientEmail: "a@example.com"})

	completed := repo.completed["item-1"]
	if completed.status != ItemStatusUncertain || completed.errorKey != string(ErrSendFailed) {
		t.Fatalf("expected uncertain send failure, got %#v", completed)
	}
	if settingsProvider.sendCalls != 1 {
		t.Fatalf("expected one SMTP attempt, got %d", settingsProvider.sendCalls)
	}
}

func TestProcessOneCancellingBatchCancelsBeforeSend(t *testing.T) {
	repo := newFakeRepo()
	repo.batches["batch-1"] = Batch{ID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", Status: BatchStatusCancelling, RecipientCount: 1}
	settingsProvider := &fakeSettings{}
	service := NewService(repo, &fakeSessions{}, &fakeUsers{}, settingsProvider)

	service.processOne(context.Background(), BatchItem{ID: "item-1", BatchID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", RecipientEmail: "a@example.com"})

	completed := repo.completed["item-1"]
	if completed.status != ItemStatusCancelled || completed.errorKey != "" {
		t.Fatalf("expected cancellation before send, got %#v", completed)
	}
	if settingsProvider.sendCalls != 0 {
		t.Fatalf("SMTP should not be called for cancelling batch, got %d calls", settingsProvider.sendCalls)
	}
}

func TestRecoverStaleSendingFinalizesAffectedBatches(t *testing.T) {
	repo := newFakeRepo()
	repo.staleBatchIDs = []string{"batch-1", "batch-2"}
	service := NewService(repo, &fakeSessions{}, &fakeUsers{}, &fakeSettings{})

	service.recoverStaleSending(context.Background(), time.Minute)

	if got, want := repo.finalized, []string{"batch-1", "batch-2"}; !sameStrings(got, want) {
		t.Fatalf("finalized batches = %#v, want %#v", got, want)
	}
}

func TestWorkerTickClaimsProcessesAndDoesNotRetry(t *testing.T) {
	repo := newFakeRepo()
	repo.batches["batch-1"] = Batch{ID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", TemplateSubject: "S", TemplateHTML: "<p>x</p>", Status: BatchStatusRunning, RecipientCount: 1}
	repo.claimItems = []BatchItem{{ID: "item-1", BatchID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", RecipientEmail: "a@example.com"}}
	settingsProvider := &fakeSettings{sendErr: errors.New("smtp failed")}
	service := NewService(repo, &fakeSessions{}, &fakeUsers{}, settingsProvider)
	worker := NewWorker(service)

	worker.tick(context.Background())

	if repo.claimLimit != workerConcurrency {
		t.Fatalf("claim limit = %d, want %d", repo.claimLimit, workerConcurrency)
	}
	if settingsProvider.sendCalls != 1 {
		t.Fatalf("expected one SMTP attempt and no retry, got %d", settingsProvider.sendCalls)
	}
	completed := repo.completed["item-1"]
	if completed.status != ItemStatusUncertain {
		t.Fatalf("expected uncertain after send error, got %#v", completed)
	}
}

func TestWorkerTickNoClaimedItemsDoesNothing(t *testing.T) {
	repo := newFakeRepo()
	settingsProvider := &fakeSettings{}
	service := NewService(repo, &fakeSessions{}, &fakeUsers{}, settingsProvider)
	worker := NewWorker(service)

	worker.tick(context.Background())

	if repo.claimLimit != workerConcurrency {
		t.Fatalf("claim limit = %d, want %d", repo.claimLimit, workerConcurrency)
	}
	if settingsProvider.sendCalls != 0 || len(repo.completed) != 0 {
		t.Fatalf("expected no send or completion without claimed items, sends=%d completed=%#v", settingsProvider.sendCalls, repo.completed)
	}
}

func newTestService(repo *fakeRepo, users *fakeUsers, settingsProvider *fakeSettings) *Service {
	if settingsProvider == nil {
		settingsProvider = &fakeSettings{template: settings.EmailTemplateSnapshot{ID: "tpl-1", Name: "Template", Subject: "Subject", HTMLBody: "<p>Body</p>"}}
	}
	return NewService(repo, &fakeSessions{}, users, settingsProvider)
}

type fakeSessions struct{}

func (f *fakeSessions) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	return upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://sub2api.test", AccessToken: "token", TokenType: "Bearer"}, nil
}

type fakeUsers struct {
	byID          map[string]upstream.Sub2APIAdminUser
	page          upstream.Sub2APIAdminUsersPage
	pages         map[int]upstream.Sub2APIAdminUsersPage
	lastQuery     upstream.Sub2APIAdminUsersQuery
	pageQueries   []upstream.Sub2APIAdminUsersQuery
	fetchOneCalls int
}

func (f *fakeUsers) FetchSub2APIAdminUsersPage(session upstream.Session, query upstream.Sub2APIAdminUsersQuery) (upstream.Sub2APIAdminUsersPage, error) {
	f.lastQuery = query
	f.pageQueries = append(f.pageQueries, query)
	if f.pages != nil {
		return f.pages[query.Page], nil
	}
	return f.page, nil
}

func (f *fakeUsers) FetchSub2APIAdminUser(session upstream.Session, userID string) (upstream.Sub2APIAdminUser, error) {
	f.fetchOneCalls++
	return f.byID[userID], nil
}

type fakeSettings struct {
	template      settings.EmailTemplateSnapshot
	sendErr       error
	snapshotCalls int
	sendCalls     int
}

func (f *fakeSettings) SnapshotEmailTemplateForWorkspace(ctx context.Context, userID string, adminAccountID string, id string) (settings.EmailTemplateSnapshot, error) {
	f.snapshotCalls++
	if f.template.ID == "" {
		return settings.EmailTemplateSnapshot{}, settings.ErrEmailTemplateNotFound
	}
	return f.template, nil
}

func (f *fakeSettings) ValidateSMTPReadyForWorkspace(ctx context.Context, userID string, adminAccountID string) error {
	return nil
}

func (f *fakeSettings) SendSavedSMTPEmailForWorkspace(ctx context.Context, userID string, adminAccountID string, recipientEmail string, subject string, htmlBody string) error {
	f.sendCalls++
	return f.sendErr
}

type fakeRepo struct {
	batches            map[string]Batch
	request            map[string]Batch
	items              map[string][]BatchItem
	completed          map[string]struct{ status, errorKey string }
	staleBatchIDs      []string
	finalized          []string
	claimItems         []BatchItem
	claimLimit         int
	enforceActiveLimit bool
	hasActiveCalls     int
	hasActiveErr       error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{batches: map[string]Batch{}, request: map[string]Batch{}, items: map[string][]BatchItem{}, completed: map[string]struct{ status, errorKey string }{}}
}

func (f *fakeRepo) EnsureSchema(ctx context.Context) error { return nil }

func (f *fakeRepo) GetByRequestID(ctx context.Context, userID string, adminAccountID string, requestID string) (Batch, error) {
	batch, ok := f.request[userID+"/"+adminAccountID+"/"+requestID]
	if !ok {
		return Batch{}, errors.New("no rows")
	}
	return batch, nil
}

func (f *fakeRepo) HasActiveBatch(ctx context.Context, userID string, adminAccountID string) (bool, error) {
	f.hasActiveCalls++
	if f.hasActiveErr != nil {
		return false, f.hasActiveErr
	}
	for _, batch := range f.batches {
		if batch.UserID == userID && batch.AdminAccountID == adminAccountID && isActiveStatus(batch.Status) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRepo) CreateBatch(ctx context.Context, batch Batch, items []BatchItem) (Batch, bool, error) {
	if f.enforceActiveLimit && isActiveStatus(batch.Status) {
		for _, existing := range f.batches {
			if existing.UserID == batch.UserID && existing.AdminAccountID == batch.AdminAccountID && isActiveStatus(existing.Status) {
				return Batch{}, false, ErrActiveBatchExists
			}
		}
	}
	now := time.Now()
	batch.CreatedAt = now
	batch.UpdatedAt = now
	f.batches[batch.ID] = batch
	f.request[batch.UserID+"/"+batch.AdminAccountID+"/"+batch.RequestID] = batch
	f.items[batch.ID] = items
	return batch, true, nil
}

func isActiveStatus(status string) bool {
	return status == BatchStatusQueued || status == BatchStatusRunning || status == BatchStatusCancelling
}

func (f *fakeRepo) Get(ctx context.Context, userID string, adminAccountID string, id string) (Batch, error) {
	batch, ok := f.batches[id]
	if !ok || batch.UserID != userID || batch.AdminAccountID != adminAccountID {
		return Batch{}, errors.New("no rows")
	}
	return batch, nil
}

func (f *fakeRepo) List(ctx context.Context, userID string, adminAccountID string, page int, pageSize int) ([]Batch, int, error) {
	var batches []Batch
	for _, batch := range f.batches {
		if batch.UserID == userID && batch.AdminAccountID == adminAccountID {
			batches = append(batches, batch)
		}
	}
	return batches, len(batches), nil
}

func (f *fakeRepo) ListItems(ctx context.Context, userID string, adminAccountID string, batchID string, page int, pageSize int) ([]BatchItem, int, error) {
	items := f.items[batchID]
	return items, len(items), nil
}

func (f *fakeRepo) CancelBatch(ctx context.Context, userID string, adminAccountID string, batchID string) (Batch, error) {
	batch, err := f.Get(ctx, userID, adminAccountID, batchID)
	if err != nil {
		return Batch{}, err
	}
	batch.Status = BatchStatusCancelling
	f.batches[batchID] = batch
	return batch, nil
}

func (f *fakeRepo) RecoverStaleSending(ctx context.Context, staleBefore time.Time) ([]string, error) {
	return f.staleBatchIDs, nil
}

func (f *fakeRepo) ClaimPendingItems(ctx context.Context, limit int) ([]BatchItem, error) {
	f.claimLimit = limit
	return f.claimItems, nil
}

func (f *fakeRepo) GetBatchByID(ctx context.Context, id string) (Batch, error) {
	batch, ok := f.batches[id]
	if !ok {
		return Batch{}, errors.New("no rows")
	}
	return batch, nil
}

func (f *fakeRepo) CompleteItem(ctx context.Context, itemID string, status string, errorKey string) error {
	f.completed[itemID] = struct{ status, errorKey string }{status: status, errorKey: errorKey}
	return nil
}

func (f *fakeRepo) FinalizeBatch(ctx context.Context, batchID string) error {
	f.finalized = append(f.finalized, batchID)
	return nil
}

func sameStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
