package mass_email

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"transithub/backend/internal/modules/settings"
)

func TestCreateLeadBatchKeepsLeadIDsAddsAdLabelAndSkipsBadAddresses(t *testing.T) {
	repo := newFakeRepo()
	service := newTestService(repo, &fakeUsers{}, nil)
	batch, err := service.CreateLeadBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{
		TemplateID:    "tpl-1",
		RequestID:     "req-1",
		PublicBaseURL: "https://hub.example.com/api",
		AdLabel:       true,
	}, []LeadRecipient{
		{LeadID: "lead-1", Email: "a@example.com"},
		{LeadID: "lead-2", Email: "A@example.com"},
		{LeadID: "lead-3", Email: "not-an-email"},
		{LeadID: "", Email: "b@example.com"},
		{LeadID: "lead-4", Email: "c@example.com"},
	})
	if err != nil {
		t.Fatalf("create lead batch: %v", err)
	}
	if batch.SelectionMode != SelectionModeLeads || batch.RecipientCount != 2 || batch.SkippedCount != 3 {
		t.Fatalf("unexpected batch: %+v", batch)
	}
	if batch.TemplateSubject != AdLabel+"Subject" {
		t.Fatalf("subject should carry the ad label, got %q", batch.TemplateSubject)
	}
	items := repo.items[batch.ID]
	if len(items) != 2 || items[0].LeadID != "lead-1" || items[1].LeadID != "lead-4" || items[0].UpstreamUserID != "" {
		t.Fatalf("items should keep their lead IDs: %+v", items)
	}
}

func TestCreateBatchDoesNotAcceptLeadsFromTheAPI(t *testing.T) {
	service := newTestService(newFakeRepo(), &fakeUsers{}, nil)
	_, err := service.CreateBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", RequestID: "req-1", SelectionMode: SelectionModeLeads})
	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("leads must only come from the leads module, got %v", err)
	}
}

func TestWithAdLabelDoesNotDoubleLabel(t *testing.T) {
	for input, want := range map[string]string{
		"春季优惠":       AdLabel + "春季优惠",
		"  春季优惠  ":   AdLabel + "春季优惠",
		"【广告】春季优惠":   "【广告】春季优惠",
		"[AD] Sale":  "[AD] Sale",
		"AD: Sale":   "AD: Sale",
		"Advantages": AdLabel + "Advantages",
	} {
		if got := withAdLabel(input); got != want {
			t.Errorf("withAdLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestProcessOneMarksRejectedRecipientAsFailed(t *testing.T) {
	repo := newFakeRepo()
	batch := Batch{ID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", TemplateSubject: "S", TemplateHTML: "<p>x</p>", Status: BatchStatusRunning, RecipientCount: 1}
	repo.batches[batch.ID] = batch
	settingsProvider := &fakeSettings{sendErr: settings.ErrSMTPRecipientRejected}
	service := NewService(repo, &fakeSessions{}, &fakeUsers{}, settingsProvider)

	service.processOne(context.Background(), BatchItem{ID: "item-1", BatchID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", LeadID: "lead-1", RecipientEmail: "gone@example.com"})

	if completed := repo.completed["item-1"]; completed.status != ItemStatusFailed || completed.errorKey != string(ErrRecipientRejected) {
		t.Fatalf("a rejected address is a definite failure, got %#v", completed)
	}
	// 其他发送错误照旧是「不确定」。
	settingsProvider.sendErr = fmt.Errorf("%w: timeout", settings.ErrSMTPSendFailed)
	service.processOne(context.Background(), BatchItem{ID: "item-2", BatchID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", RecipientEmail: "slow@example.com"})
	if completed := repo.completed["item-2"]; completed.status != ItemStatusUncertain {
		t.Fatalf("other failures stay uncertain, got %#v", completed)
	}
}

func TestBatchForRequestFindsAnEarlierBatch(t *testing.T) {
	repo := newFakeRepo()
	service := newTestService(repo, &fakeUsers{}, nil)
	if _, ok := service.BatchForRequest(context.Background(), "user-1", "admin-1", "req-1"); ok {
		t.Fatal("no batch yet")
	}
	created, err := service.CreateLeadBatch(context.Background(), "user-1", "admin-1", CreateBatchRequest{TemplateID: "tpl-1", RequestID: "req-1"}, []LeadRecipient{{LeadID: "lead-1", Email: "a@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	found, ok := service.BatchForRequest(context.Background(), "user-1", "admin-1", " req-1 ")
	if !ok || found.ID != created.ID {
		t.Fatalf("expected the earlier batch, got %+v %t", found, ok)
	}
}
