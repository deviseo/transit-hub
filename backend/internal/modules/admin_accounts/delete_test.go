package admin_accounts

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type deleteFakeAccountRepository struct {
	userID       string
	accountID    string
	deleteCalls  int
	deleteResult *DeleteResult
	deleteErr    error
	completed    []string
	retries      []string
	claimedJobs  []CleanupJob
}

func (f *deleteFakeAccountRepository) EnsureSchema(ctx context.Context) error     { return nil }
func (f *deleteFakeAccountRepository) AssignLegacyRows(ctx context.Context) error { return nil }
func (f *deleteFakeAccountRepository) List(ctx context.Context, userID string) ([]Account, error) {
	return nil, nil
}
func (f *deleteFakeAccountRepository) Current(ctx context.Context, userID string) (*Account, error) {
	return nil, nil
}
func (f *deleteFakeAccountRepository) CurrentID(ctx context.Context, userID string) (string, error) {
	return "", nil
}
func (f *deleteFakeAccountRepository) UpsertAndSwitch(ctx context.Context, userID string, input UpsertInput) (Account, error) {
	return Account{}, nil
}
func (f *deleteFakeAccountRepository) Switch(ctx context.Context, userID string, accountID string) (*Account, error) {
	return nil, nil
}
func (f *deleteFakeAccountRepository) Update(ctx context.Context, userID string, accountID string, displayName string) (*Account, error) {
	return nil, nil
}
func (f *deleteFakeAccountRepository) DeleteWorkspace(ctx context.Context, userID string, accountID string) (*DeleteResult, error) {
	f.deleteCalls++
	f.userID = userID
	f.accountID = accountID
	return f.deleteResult, f.deleteErr
}
func (f *deleteFakeAccountRepository) ClaimDueCleanupJobs(ctx context.Context, limit int) ([]CleanupJob, error) {
	jobs := append([]CleanupJob(nil), f.claimedJobs...)
	f.claimedJobs = nil
	return jobs, nil
}
func (f *deleteFakeAccountRepository) CompleteCleanupJob(ctx context.Context, id string) error {
	f.completed = append(f.completed, id)
	return nil
}
func (f *deleteFakeAccountRepository) MarkCleanupJobRetry(ctx context.Context, id string, attempt int, err error) error {
	f.retries = append(f.retries, id)
	return nil
}

type deleteFakeWorkspaceCleanup struct {
	called  bool
	payload WorkspaceCleanupPayload
	err     error
}

func (f *deleteFakeWorkspaceCleanup) CleanupDeletedWorkspace(ctx context.Context, payload WorkspaceCleanupPayload) error {
	f.called = true
	f.payload = payload
	return f.err
}

func TestDeleteWorkspaceRequiresExactConfirmation(t *testing.T) {
	repo := &deleteFakeAccountRepository{}
	service := NewService(repo)

	_, err := service.DeleteWorkspace(context.Background(), "user-1", "account-1", DeleteRequest{Confirmation: " delete workspace "})
	if !errors.Is(err, requestError(ErrorRequest)) {
		t.Fatalf("expected request error for non-exact confirmation, got %v", err)
	}
	if repo.deleteCalls != 0 {
		t.Fatalf("repository was called despite invalid confirmation: %+v", repo)
	}
}

func TestDeleteWorkspaceScopesByAuthenticatedUserAndAccount(t *testing.T) {
	repo := &deleteFakeAccountRepository{deleteResult: &DeleteResult{DeletedID: "account-1", CurrentAdminAccountID: "account-2"}}
	service := NewService(repo)

	response, err := service.DeleteWorkspace(context.Background(), "user-1", " account-1 ", DeleteRequest{Confirmation: DeleteConfirmation})
	if err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	if repo.userID != "user-1" || repo.accountID != "account-1" {
		t.Fatalf("expected scoped delete user/account, got user=%q account=%q", repo.userID, repo.accountID)
	}
	if response.DeletedID != "account-1" || !response.HasCurrent || response.CurrentAdminAccountID != "account-2" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestDeleteWorkspaceReturnsNotFoundForUnownedAccount(t *testing.T) {
	repo := &deleteFakeAccountRepository{}
	cleanup := &deleteFakeWorkspaceCleanup{}
	service := NewService(repo)
	service.SetWorkspaceCleanup(cleanup)

	_, err := service.DeleteWorkspace(context.Background(), "user-1", "account-elsewhere", DeleteRequest{Confirmation: DeleteConfirmation})
	if !errors.Is(err, requestError(ErrorNotFound)) {
		t.Fatalf("expected not found, got %v", err)
	}
	if cleanup.called {
		t.Fatal("cleanup should not run for unowned or missing account")
	}
}

func TestDeleteWorkspaceCurrentFallbackAndLastWorkspace(t *testing.T) {
	t.Run("fallback", func(t *testing.T) {
		repo := &deleteFakeAccountRepository{deleteResult: &DeleteResult{DeletedID: "account-1", CurrentAdminAccountID: "account-2"}}
		service := NewService(repo)
		response, err := service.DeleteWorkspace(context.Background(), "user-1", "account-1", DeleteRequest{Confirmation: DeleteConfirmation})
		if err != nil {
			t.Fatalf("delete workspace: %v", err)
		}
		if !response.HasCurrent || response.CurrentAdminAccountID != "account-2" {
			t.Fatalf("expected fallback current account, got %+v", response)
		}
	})

	t.Run("last workspace clears pointer", func(t *testing.T) {
		repo := &deleteFakeAccountRepository{deleteResult: &DeleteResult{DeletedID: "account-1"}}
		service := NewService(repo)
		response, err := service.DeleteWorkspace(context.Background(), "user-1", "account-1", DeleteRequest{Confirmation: DeleteConfirmation})
		if err != nil {
			t.Fatalf("delete workspace: %v", err)
		}
		if response.HasCurrent || response.CurrentAdminAccountID != "" {
			t.Fatalf("expected cleared current pointer, got %+v", response)
		}
	})
}

func TestDeleteWorkspaceDoesNotCleanupWhenRepositoryFails(t *testing.T) {
	repo := &deleteFakeAccountRepository{deleteErr: errors.New("rollback")}
	cleanup := &deleteFakeWorkspaceCleanup{}
	service := NewService(repo)
	service.SetWorkspaceCleanup(cleanup)

	_, err := service.DeleteWorkspace(context.Background(), "user-1", "account-1", DeleteRequest{Confirmation: DeleteConfirmation})
	if err == nil {
		t.Fatal("expected repository error")
	}
	if cleanup.called {
		t.Fatal("cleanup must only run after a committed repository delete")
	}
}

func TestDeleteWorkspaceCleanupCallbackPayloadAndAdditiveResponse(t *testing.T) {
	repo := &deleteFakeAccountRepository{deleteResult: &DeleteResult{
		CleanupJobID:           "job-1",
		DeletedID:              "account-1",
		AttachmentStoragePaths: []string{"a.png", "b.png"},
		UpstreamSiteIDs:        []string{"site-1"},
	}}
	cleanup := &deleteFakeWorkspaceCleanup{err: errors.Join(errors.New("file cleanup failed"), errors.New("redis cleanup failed"))}
	service := NewService(repo)
	service.SetWorkspaceCleanup(cleanup)

	response, err := service.DeleteWorkspace(context.Background(), "user-1", "account-1", DeleteRequest{Confirmation: DeleteConfirmation})
	if err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	if !cleanup.called {
		t.Fatal("expected cleanup callback")
	}
	wantPayload := WorkspaceCleanupPayload{
		UserID:                 "user-1",
		AdminAccountID:         "account-1",
		AttachmentStoragePaths: []string{"a.png", "b.png"},
		UpstreamSiteIDs:        []string{"site-1"},
	}
	if !reflect.DeepEqual(cleanup.payload, wantPayload) {
		t.Fatalf("unexpected cleanup payload: %+v", cleanup.payload)
	}
	if response.DeletedID != "account-1" || response.CleanupComplete || !response.CleanupPending {
		t.Fatalf("expected safe pending cleanup status in successful response, got %+v", response)
	}
	if len(repo.retries) != 1 || repo.retries[0] != "job-1" {
		t.Fatalf("expected failed cleanup job to be persisted for retry, got %+v", repo.retries)
	}
}

func TestDeleteWorkspaceCompletesCleanupJobAfterImmediateSuccess(t *testing.T) {
	repo := &deleteFakeAccountRepository{deleteResult: &DeleteResult{CleanupJobID: "job-1", DeletedID: "account-1"}}
	service := NewService(repo)
	service.SetWorkspaceCleanup(&deleteFakeWorkspaceCleanup{})

	response, err := service.DeleteWorkspace(context.Background(), "user-1", "account-1", DeleteRequest{Confirmation: DeleteConfirmation})
	if err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	if !response.CleanupComplete || response.CleanupPending {
		t.Fatalf("expected cleanup complete response, got %+v", response)
	}
	if len(repo.completed) != 1 || repo.completed[0] != "job-1" {
		t.Fatalf("expected cleanup job completion, got %+v", repo.completed)
	}
}

func TestProcessDueCleanupJobsCompletesRestartedJob(t *testing.T) {
	repo := &deleteFakeAccountRepository{claimedJobs: []CleanupJob{{ID: "job-1", UserID: "user-1", AdminAccountID: "account-1", AttachmentStoragePaths: []string{"a.png"}, UpstreamSiteIDs: []string{"site-1"}, Attempts: 1}}}
	cleanup := &deleteFakeWorkspaceCleanup{}
	service := NewService(repo)
	service.SetWorkspaceCleanup(cleanup)

	service.ProcessDueCleanupJobs(context.Background(), 1)
	if !cleanup.called {
		t.Fatal("expected restarted cleanup job to run")
	}
	if len(repo.completed) != 1 || repo.completed[0] != "job-1" {
		t.Fatalf("expected restarted cleanup job completion, got %+v", repo.completed)
	}
}
