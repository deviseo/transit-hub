package settings

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeEmailTemplateRepository struct {
	mu   sync.Mutex
	rows map[string]EmailTemplate
	err  error
	now  time.Time
}

func newFakeEmailTemplateRepository() *fakeEmailTemplateRepository {
	return &fakeEmailTemplateRepository{rows: map[string]EmailTemplate{}, now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)}
}

func emailTemplateRepoKey(userID, adminAccountID, id string) string {
	return userID + "|" + adminAccountID + "|" + id
}

func (f *fakeEmailTemplateRepository) EnsureBuiltInEmailTemplate(_ context.Context, userID string, adminAccountID string, template EmailTemplate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	key := emailTemplateRepoKey(userID, adminAccountID, template.ID)
	if _, ok := f.rows[key]; ok {
		return nil
	}
	createdAt := f.now
	template.IsBuiltIn = true
	template.CreatedAt = &createdAt
	template.UpdatedAt = &createdAt
	f.rows[key] = template
	return nil
}

func (f *fakeEmailTemplateRepository) ListEmailTemplates(_ context.Context, userID string, adminAccountID string) ([]EmailTemplate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	templates := []EmailTemplate{}
	for key, template := range f.rows {
		if strings.HasPrefix(key, userID+"|"+adminAccountID+"|") {
			templates = append(templates, template)
		}
	}
	sort.SliceStable(templates, func(i, j int) bool {
		if templates[i].IsBuiltIn != templates[j].IsBuiltIn {
			return templates[i].IsBuiltIn
		}
		return templates[i].UpdatedAt.After(*templates[j].UpdatedAt)
	})
	return templates, nil
}

func (f *fakeEmailTemplateRepository) GetEmailTemplate(_ context.Context, userID string, adminAccountID string, id string) (EmailTemplate, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return EmailTemplate{}, false, f.err
	}
	template, ok := f.rows[emailTemplateRepoKey(userID, adminAccountID, id)]
	return template, ok, nil
}

func (f *fakeEmailTemplateRepository) CreateEmailTemplate(_ context.Context, userID string, adminAccountID string, template EmailTemplate, limit int) (EmailTemplate, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return EmailTemplate{}, false, f.err
	}
	customCount := 0
	for key, existing := range f.rows {
		if strings.HasPrefix(key, userID+"|"+adminAccountID+"|") && !existing.IsBuiltIn {
			customCount++
		}
	}
	if customCount >= limit {
		return EmailTemplate{}, false, nil
	}
	createdAt := f.now.Add(time.Duration(len(f.rows)+1) * time.Minute)
	template.IsBuiltIn = false
	template.CreatedAt = &createdAt
	template.UpdatedAt = &createdAt
	f.rows[emailTemplateRepoKey(userID, adminAccountID, template.ID)] = template
	return template, true, nil
}

func (f *fakeEmailTemplateRepository) UpdateEmailTemplate(_ context.Context, userID string, adminAccountID string, id string, input SaveEmailTemplateInput) (EmailTemplate, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return EmailTemplate{}, false, f.err
	}
	key := emailTemplateRepoKey(userID, adminAccountID, id)
	template, ok := f.rows[key]
	if !ok {
		return EmailTemplate{}, false, nil
	}
	updatedAt := f.now.Add(24 * time.Hour)
	template.Name = input.Name
	template.Subject = input.Subject
	template.HTMLBody = input.HTMLBody
	template.UpdatedAt = &updatedAt
	f.rows[key] = template
	return template, true, nil
}

func (f *fakeEmailTemplateRepository) DeleteEmailTemplate(_ context.Context, userID string, adminAccountID string, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return false, f.err
	}
	key := emailTemplateRepoKey(userID, adminAccountID, id)
	if template, ok := f.rows[key]; ok && !template.IsBuiltIn {
		delete(f.rows, key)
		return true, nil
	}
	return false, nil
}

func newTestEmailTemplateService(repo *fakeEmailTemplateRepository, smtpRepo *fakeSMTPRepository, resolver AdminAccountResolver) *Service {
	return &Service{accounts: resolver, emailTemplateRepo: repo, smtpRepo: smtpRepo}
}

func validEmailTemplateInput() SaveEmailTemplateInput {
	return SaveEmailTemplateInput{Name: "活动", Subject: "TransitHub 活动", HTMLBody: "<h1>活动正文</h1>"}
}

func TestEmailTemplateListSeedsBuiltInOnceAndDoesNotOverwriteEdits(t *testing.T) {
	repo := newFakeEmailTemplateRepository()
	svc := newTestEmailTemplateService(repo, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"})

	first, err := svc.ListEmailTemplates(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(first) != 1 || first[0].ID != builtInMarketingTemplateID || !first[0].IsBuiltIn {
		t.Fatalf("expected seeded built-in template, got %+v", first)
	}

	updated, err := svc.UpdateEmailTemplate(context.Background(), "user-1", builtInMarketingTemplateID, SaveEmailTemplateInput{Name: "自定义活动", Subject: "自定义标题", HTMLBody: "<p>edited</p>"})
	if err != nil {
		t.Fatalf("update built-in: %v", err)
	}
	if !updated.IsBuiltIn {
		t.Fatalf("built-in flag must be preserved: %+v", updated)
	}
	second, err := svc.ListEmailTemplates(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("list again: %v", err)
	}
	if len(second) != 1 || second[0].Name != "自定义活动" || second[0].Subject != "自定义标题" || second[0].HTMLBody != "<p>edited</p>" {
		t.Fatalf("built-in seed must not overwrite edits: %+v", second)
	}
}

func TestEmailTemplateWorkspaceIsolation(t *testing.T) {
	repo := newFakeEmailTemplateRepository()
	svc := newTestEmailTemplateService(repo, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"})
	created, err := svc.CreateEmailTemplate(context.Background(), "user-1", validEmailTemplateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	otherSvc := newTestEmailTemplateService(repo, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-2"})
	if _, err := otherSvc.GetEmailTemplate(context.Background(), "user-1", created.ID); !errors.Is(err, ErrEmailTemplateNotFound) {
		t.Fatalf("expected workspace-scoped not found, got %v", err)
	}
}

func TestEmailTemplateCreateValidationAndCustomLimit(t *testing.T) {
	repo := newFakeEmailTemplateRepository()
	svc := newTestEmailTemplateService(repo, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"})

	invalids := []SaveEmailTemplateInput{
		{Name: "", Subject: "ok", HTMLBody: "<p>ok</p>"},
		{Name: strings.Repeat("a", maxEmailTemplateNameLen+1), Subject: "ok", HTMLBody: "<p>ok</p>"},
		{Name: "ok", Subject: "bad\nsubject", HTMLBody: "<p>ok</p>"},
		{Name: "ok", Subject: strings.Repeat("a", maxEmailTemplateSubjectLen+1), HTMLBody: "<p>ok</p>"},
		{Name: "ok", Subject: "ok", HTMLBody: "   "},
		{Name: "ok", Subject: "ok", HTMLBody: strings.Repeat("a", maxEmailTemplateHTMLBytes+1)},
	}
	for _, input := range invalids {
		if _, err := svc.CreateEmailTemplate(context.Background(), "user-1", input); !errors.Is(err, ErrEmailTemplateValidation) {
			t.Fatalf("expected validation for %+v, got %v", input, err)
		}
	}

	if _, err := svc.ListEmailTemplates(context.Background(), "user-1"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for i := 0; i < maxEmailTemplatesPerSpace; i++ {
		template := EmailTemplate{ID: "custom-" + strconv.Itoa(i), Name: "n", Subject: "s", HTMLBody: "<p>x</p>"}
		if _, ok, err := repo.CreateEmailTemplate(context.Background(), "user-1", "acct-1", template, maxEmailTemplatesPerSpace); err != nil || !ok {
			t.Fatalf("seed custom rows: %v", err)
		}
	}
	if _, err := svc.CreateEmailTemplate(context.Background(), "user-1", validEmailTemplateInput()); !errors.Is(err, ErrEmailTemplateLimitReached) {
		t.Fatalf("expected limit reached after 50 custom templates, got %v", err)
	}
}

func TestEmailTemplateRepositoryCreateLimitIsAtomicInFake(t *testing.T) {
	repo := newFakeEmailTemplateRepository()
	const attempts = 20
	var wg sync.WaitGroup
	created := make(chan bool, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			template := EmailTemplate{ID: "custom-" + strconv.Itoa(index), Name: "n", Subject: "s", HTMLBody: "<p>x</p>"}
			_, ok, err := repo.CreateEmailTemplate(context.Background(), "user-1", "acct-1", template, 1)
			if err != nil {
				t.Errorf("create: %v", err)
				return
			}
			created <- ok
		}(i)
	}
	wg.Wait()
	close(created)

	successes := 0
	for ok := range created {
		if ok {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one create under limit 1, got %d", successes)
	}
}

func TestEmailTemplateCRUDProtectionAndNotFound(t *testing.T) {
	repo := newFakeEmailTemplateRepository()
	svc := newTestEmailTemplateService(repo, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"})
	created, err := svc.CreateEmailTemplate(context.Background(), "user-1", validEmailTemplateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" || created.IsBuiltIn {
		t.Fatalf("unexpected created template: %+v", created)
	}

	updated, err := svc.UpdateEmailTemplate(context.Background(), "user-1", created.ID, SaveEmailTemplateInput{Name: "更新", Subject: "更新标题", HTMLBody: "<p>updated</p>"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "更新" || updated.Subject != "更新标题" || updated.HTMLBody != "<p>updated</p>" {
		t.Fatalf("unexpected update: %+v", updated)
	}
	if err := svc.DeleteEmailTemplate(context.Background(), "user-1", builtInMarketingTemplateID); !errors.Is(err, ErrEmailTemplateBuiltInProtected) {
		t.Fatalf("expected built-in protected, got %v", err)
	}
	if err := svc.DeleteEmailTemplate(context.Background(), "user-1", created.ID); err != nil {
		t.Fatalf("delete custom: %v", err)
	}
	if _, err := svc.GetEmailTemplate(context.Background(), "user-1", created.ID); !errors.Is(err, ErrEmailTemplateNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
	if _, err := svc.UpdateEmailTemplate(context.Background(), "user-1", "missing", validEmailTemplateInput()); !errors.Is(err, ErrEmailTemplateNotFound) {
		t.Fatalf("expected update not found, got %v", err)
	}
	if err := svc.DeleteEmailTemplate(context.Background(), "user-1", "missing"); !errors.Is(err, ErrEmailTemplateNotFound) {
		t.Fatalf("expected delete not found, got %v", err)
	}
}

func TestEmailTemplateTestUsesSavedContentOnly(t *testing.T) {
	repo := newFakeEmailTemplateRepository()
	smtpRepo := newFakeSMTPRepository()
	sender := &fakeSMTPSender{}
	svc := newTestEmailTemplateService(repo, smtpRepo, &fakeAdminAccountResolver{id: "acct-1"})
	svc.smtpSender = sender
	created, err := svc.CreateEmailTemplate(context.Background(), "user-1", SaveEmailTemplateInput{Name: "活动", Subject: "已保存标题", HTMLBody: "<p>saved body</p>"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	smtpRepo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls)}

	if err := svc.TestEmailTemplate(context.Background(), "user-1", created.ID, "recipient@example.com"); err != nil {
		t.Fatalf("test template: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected one send, got %d", len(sender.sent))
	}
	if sender.sent[0].Subject != "已保存标题" || sender.sent[0].HTMLBody != "<p>saved body</p>" {
		t.Fatalf("expected saved subject/body only, got %+v", sender.sent[0])
	}
}

func TestEmailTemplateSMTPErrorMapping(t *testing.T) {
	repo := newFakeEmailTemplateRepository()
	smtpRepo := newFakeSMTPRepository()
	sender := &fakeSMTPSender{err: ErrSMTPSendFailed}
	svc := newTestEmailTemplateService(repo, smtpRepo, &fakeAdminAccountResolver{id: "acct-1"})
	svc.smtpSender = sender
	created, err := svc.CreateEmailTemplate(context.Background(), "user-1", validEmailTemplateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.TestEmailTemplate(context.Background(), "user-1", created.ID, "recipient@example.com"); !errors.Is(err, ErrSMTPMissingConfig) {
		t.Fatalf("expected missing smtp config, got %v", err)
	}
	smtpRepo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls)}
	if err := svc.TestEmailTemplate(context.Background(), "user-1", created.ID, "bad\nrecipient@example.com"); !errors.Is(err, ErrEmailTemplateInvalidEmail) {
		t.Fatalf("expected invalid email, got %v", err)
	}
	if err := svc.TestEmailTemplate(context.Background(), "user-1", created.ID, "recipient@example.com"); !errors.Is(err, ErrSMTPSendFailed) {
		t.Fatalf("expected send failed, got %v", err)
	}
}

func TestEmailTemplatePersistenceErrorsAreStable(t *testing.T) {
	repo := newFakeEmailTemplateRepository()
	repo.err = errors.New("db down")
	svc := newTestEmailTemplateService(repo, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"})
	if _, err := svc.ListEmailTemplates(context.Background(), "user-1"); !errors.Is(err, ErrEmailTemplatePersistence) {
		t.Fatalf("expected persistence, got %v", err)
	}
}
