package admin_accounts

import (
	"context"
	"log"
	"strings"
	"time"
)

type accountRepository interface {
	EnsureSchema(ctx context.Context) error
	AssignLegacyRows(ctx context.Context) error
	List(ctx context.Context, userID string) ([]Account, error)
	Current(ctx context.Context, userID string) (*Account, error)
	CurrentID(ctx context.Context, userID string) (string, error)
	UpsertAndSwitch(ctx context.Context, userID string, input UpsertInput) (Account, error)
	Switch(ctx context.Context, userID string, accountID string) (*Account, error)
	Update(ctx context.Context, userID string, accountID string, displayName string) (*Account, error)
	DeleteWorkspace(ctx context.Context, userID string, accountID string) (*DeleteResult, error)
	ClaimDueCleanupJobs(ctx context.Context, limit int) ([]CleanupJob, error)
	CompleteCleanupJob(ctx context.Context, id string) error
	MarkCleanupJobRetry(ctx context.Context, id string, attempt int, err error) error
}

type WorkspaceCleanup interface {
	CleanupDeletedWorkspace(ctx context.Context, payload WorkspaceCleanupPayload) error
}

type Service struct {
	repository accountRepository
	cleanup    WorkspaceCleanup
}

func NewService(repository accountRepository) *Service { return &Service{repository: repository} }

func (s *Service) SetWorkspaceCleanup(cleanup WorkspaceCleanup) { s.cleanup = cleanup }

func (s *Service) EnsureSchema(ctx context.Context) error { return s.repository.EnsureSchema(ctx) }

func (s *Service) AssignLegacyRows(ctx context.Context) error {
	return s.repository.AssignLegacyRows(ctx)
}

func (s *Service) List(ctx context.Context, userID string) ([]Account, error) {
	return s.repository.List(ctx, userID)
}

func (s *Service) Current(ctx context.Context, userID string) (*Account, error) {
	return s.repository.Current(ctx, userID)
}

func (s *Service) CurrentID(ctx context.Context, userID string) (string, error) {
	return s.repository.CurrentID(ctx, userID)
}

func (s *Service) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	id, err := s.CurrentID(ctx, userID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(id) == "" {
		return "", requestError(ErrorNoCurrentAccount)
	}
	return id, nil
}

func (s *Service) UpsertAndSwitch(ctx context.Context, userID string, input UpsertInput) (Account, error) {
	input.Platform = strings.TrimSpace(input.Platform)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.Identity = strings.TrimSpace(input.Identity)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.AuthMethod = strings.TrimSpace(input.AuthMethod)
	if input.Platform == "" || input.Identity == "" {
		return Account{}, requestError(ErrorRequest)
	}
	return s.repository.UpsertAndSwitch(ctx, userID, input)
}

func (s *Service) Switch(ctx context.Context, userID string, accountID string) (*Account, error) {
	account, err := s.repository.Switch(ctx, userID, strings.TrimSpace(accountID))
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, requestError(ErrorNotFound)
	}
	return account, nil
}

func (s *Service) Update(ctx context.Context, userID string, accountID string, req UpdateRequest) (*Account, error) {
	account, err := s.repository.Update(ctx, userID, strings.TrimSpace(accountID), req.DisplayName)
	if err != nil {
		return nil, err
	}
	if account == nil {
		return nil, requestError(ErrorNotFound)
	}
	return account, nil
}

func (s *Service) DeleteWorkspace(ctx context.Context, userID string, accountID string, req DeleteRequest) (DeleteResponse, error) {
	if req.Confirmation != DeleteConfirmation {
		return DeleteResponse{}, requestError(ErrorRequest)
	}
	result, err := s.repository.DeleteWorkspace(ctx, userID, strings.TrimSpace(accountID))
	if err != nil {
		return DeleteResponse{}, err
	}
	if result == nil {
		return DeleteResponse{}, requestError(ErrorNotFound)
	}

	response := DeleteResponse{
		DeletedID:             result.DeletedID,
		HasCurrent:            result.CurrentAdminAccountID != "",
		CurrentAdminAccountID: result.CurrentAdminAccountID,
		CleanupComplete:       true,
	}
	if s.cleanup != nil {
		err := s.runCleanupJob(ctx, CleanupJob{
			ID:                     result.CleanupJobID,
			UserID:                 userID,
			AdminAccountID:         result.DeletedID,
			AttachmentStoragePaths: append([]string(nil), result.AttachmentStoragePaths...),
			UpstreamSiteIDs:        append([]string(nil), result.UpstreamSiteIDs...),
		})
		if err != nil {
			response.CleanupComplete = false
			response.CleanupPending = true
		}
	} else {
		response.CleanupComplete = false
		response.CleanupPending = true
	}
	return response, nil
}

func (s *Service) StartCleanupWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		s.ProcessDueCleanupJobs(ctx, 10)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.ProcessDueCleanupJobs(ctx, 10)
			}
		}
	}()
}

func (s *Service) ProcessDueCleanupJobs(ctx context.Context, limit int) {
	if s.cleanup == nil {
		return
	}
	jobs, err := s.repository.ClaimDueCleanupJobs(ctx, limit)
	if err != nil {
		log.Printf("admin account cleanup job claim failed: %v", err)
		return
	}
	for _, job := range jobs {
		if err := s.runCleanupJob(ctx, job); err != nil {
			continue
		}
	}
}

func (s *Service) runCleanupJob(ctx context.Context, job CleanupJob) error {
	if s.cleanup == nil {
		return nil
	}
	err := s.cleanup.CleanupDeletedWorkspace(ctx, WorkspaceCleanupPayload{
		UserID:                 job.UserID,
		AdminAccountID:         job.AdminAccountID,
		AttachmentStoragePaths: append([]string(nil), job.AttachmentStoragePaths...),
		UpstreamSiteIDs:        append([]string(nil), job.UpstreamSiteIDs...),
	})
	if err != nil {
		log.Printf("admin account workspace cleanup failed job_id=%s user_id=%s admin_account_id=%s err=%v", job.ID, job.UserID, job.AdminAccountID, err)
		if job.ID != "" {
			if markErr := s.repository.MarkCleanupJobRetry(ctx, job.ID, job.Attempts, err); markErr != nil {
				log.Printf("admin account cleanup job retry mark failed job_id=%s err=%v", job.ID, markErr)
			}
		}
		return err
	}
	if job.ID != "" {
		if err := s.repository.CompleteCleanupJob(ctx, job.ID); err != nil {
			log.Printf("admin account cleanup job complete failed job_id=%s err=%v", job.ID, err)
			return err
		}
	}
	return nil
}
