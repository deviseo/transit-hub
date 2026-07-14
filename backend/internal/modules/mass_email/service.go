package mass_email

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/mail"
	"strings"
	"time"

	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/modules/upstream"
)

const (
	defaultPageSize    = 20
	maxUsersPageSize   = 100
	maxItemsPageSize   = 100
	maxBatchPageSize   = 100
	maxSelectedUserIDs = 1000
	maxBatchRecipients = 10000
	maxUserSearchRunes = 100
)

type AdminSessionProvider interface {
	RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error)
}

type Sub2APIUsersClient interface {
	FetchSub2APIAdminUsersPage(session upstream.Session, query upstream.Sub2APIAdminUsersQuery) (upstream.Sub2APIAdminUsersPage, error)
	FetchSub2APIAdminUser(session upstream.Session, userID string) (upstream.Sub2APIAdminUser, error)
}

type SettingsProvider interface {
	SnapshotEmailTemplateForWorkspace(ctx context.Context, userID string, adminAccountID string, id string) (settings.EmailTemplateSnapshot, error)
	ValidateSMTPReadyForWorkspace(ctx context.Context, userID string, adminAccountID string) error
	SendSavedSMTPEmailForWorkspace(ctx context.Context, userID string, adminAccountID string, recipientEmail string, subject string, htmlBody string) error
}

type batchRepository interface {
	EnsureSchema(ctx context.Context) error
	GetByRequestID(ctx context.Context, userID string, adminAccountID string, requestID string) (Batch, error)
	HasActiveBatch(ctx context.Context, userID string, adminAccountID string) (bool, error)
	CreateBatch(ctx context.Context, batch Batch, items []BatchItem) (Batch, bool, error)
	Get(ctx context.Context, userID string, adminAccountID string, id string) (Batch, error)
	List(ctx context.Context, userID string, adminAccountID string, page int, pageSize int) ([]Batch, int, error)
	ListItems(ctx context.Context, userID string, adminAccountID string, batchID string, page int, pageSize int) ([]BatchItem, int, error)
	CancelBatch(ctx context.Context, userID string, adminAccountID string, batchID string) (Batch, error)
	RecoverStaleSending(ctx context.Context, staleBefore time.Time) ([]string, error)
	ClaimPendingItems(ctx context.Context, limit int) ([]BatchItem, error)
	GetBatchByID(ctx context.Context, id string) (Batch, error)
	CompleteItem(ctx context.Context, itemID string, status string, errorKey string) error
	FinalizeBatch(ctx context.Context, batchID string) error
}

type Service struct {
	repository batchRepository
	sessions   AdminSessionProvider
	users      Sub2APIUsersClient
	settings   SettingsProvider
}

func NewService(repository batchRepository, sessions AdminSessionProvider, users Sub2APIUsersClient, settingsProvider SettingsProvider) *Service {
	return &Service{repository: repository, sessions: sessions, users: users, settings: settingsProvider}
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	return s.repository.EnsureSchema(ctx)
}

func (s *Service) ListUsers(ctx context.Context, userID string, adminAccountID string, query UserQuery) (UsersPage, error) {
	session, err := s.sessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return UsersPage{}, mapUpstreamError(err)
	}
	page, err := s.users.FetchSub2APIAdminUsersPage(session, upstream.Sub2APIAdminUsersQuery{
		Page:      clampInt(query.Page, 1, math.MaxInt, 1),
		PageSize:  clampInt(query.PageSize, 1, maxUsersPageSize, defaultPageSize),
		Status:    strings.TrimSpace(query.Status),
		Role:      strings.TrimSpace(query.Role),
		Search:    normalizeUserSearch(query.Search),
		SortBy:    strings.TrimSpace(query.SortBy),
		SortOrder: strings.TrimSpace(query.SortOrder),
		Timezone:  strings.TrimSpace(query.Timezone),
	})
	if err != nil {
		return UsersPage{}, mapUpstreamError(err)
	}
	items := make([]UserDTO, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, userDTO(item))
	}
	return UsersPage{Items: items, Total: page.Total, Page: page.Page, PageSize: page.PageSize, Pages: page.Pages}, nil
}

func (s *Service) CreateBatch(ctx context.Context, userID string, adminAccountID string, req CreateBatchRequest) (BatchDTO, error) {
	req, err := normalizeCreateRequest(req)
	if err != nil {
		return BatchDTO{}, err
	}
	if existing, err := s.repository.GetByRequestID(ctx, userID, adminAccountID, req.RequestID); err == nil {
		return batchDTO(existing), nil
	}
	// This cheap precheck blocks obvious queue flooding before session/template/SMTP
	// and recipient resolution. It is not the correctness boundary; CreateBatch still
	// relies on the partial unique index to reject concurrent creators atomically.
	hasActive, err := s.repository.HasActiveBatch(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[mass-email] active batch precheck failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
		return BatchDTO{}, ErrPersistence
	}
	if hasActive {
		return BatchDTO{}, ErrActiveBatchExists
	}
	session, err := s.sessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return BatchDTO{}, mapUpstreamError(err)
	}
	template, err := s.settings.SnapshotEmailTemplateForWorkspace(ctx, userID, adminAccountID, req.TemplateID)
	if err != nil {
		return BatchDTO{}, mapSettingsError(err)
	}
	if err := s.settings.ValidateSMTPReadyForWorkspace(ctx, userID, adminAccountID); err != nil {
		return BatchDTO{}, mapSettingsError(err)
	}
	recipients, skipped, err := s.resolveRecipients(ctx, session, req)
	if err != nil {
		return BatchDTO{}, err
	}
	batchID, err := newID()
	if err != nil {
		return BatchDTO{}, ErrPersistence
	}
	status := BatchStatusQueued
	if len(recipients) == 0 {
		status = BatchStatusFailed
	}
	batch := Batch{
		ID:              batchID,
		UserID:          userID,
		AdminAccountID:  adminAccountID,
		RequestID:       req.RequestID,
		TemplateID:      template.ID,
		TemplateName:    template.Name,
		TemplateSubject: template.Subject,
		TemplateHTML:    template.HTMLBody,
		SelectionMode:   req.SelectionMode,
		Filters:         req.Filters,
		Status:          status,
		RecipientCount:  len(recipients),
		SkippedCount:    skipped,
	}
	items := make([]BatchItem, 0, len(recipients))
	for _, recipient := range recipients {
		itemID, err := newID()
		if err != nil {
			return BatchDTO{}, ErrPersistence
		}
		items = append(items, BatchItem{
			ID:              itemID,
			BatchID:         batchID,
			UserID:          userID,
			AdminAccountID:  adminAccountID,
			UpstreamUserID:  recipient.UpstreamUserID,
			RecipientEmail:  recipient.Email,
			NormalizedEmail: normalizeEmailKey(recipient.Email),
			Username:        recipient.Username,
			Status:          ItemStatusPending,
		})
	}
	created, _, err := s.repository.CreateBatch(ctx, batch, items)
	if err != nil {
		if errors.Is(err, ErrActiveBatchExists) {
			return BatchDTO{}, ErrActiveBatchExists
		}
		log.Printf("[mass-email] create batch failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
		return BatchDTO{}, ErrPersistence
	}
	return batchDTO(created), nil
}

func (s *Service) ListBatches(ctx context.Context, userID string, adminAccountID string, page int, pageSize int) (BatchPage, error) {
	page, pageSize = normalizePage(page, pageSize, maxBatchPageSize)
	batches, total, err := s.repository.List(ctx, userID, adminAccountID, page, pageSize)
	if err != nil {
		return BatchPage{}, ErrPersistence
	}
	items := make([]BatchDTO, 0, len(batches))
	for _, batch := range batches {
		items = append(items, batchDTO(batch))
	}
	return BatchPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages(total, pageSize)}, nil
}

func (s *Service) GetBatch(ctx context.Context, userID string, adminAccountID string, id string) (BatchDTO, error) {
	batch, err := s.repository.Get(ctx, userID, adminAccountID, strings.TrimSpace(id))
	if err != nil {
		return BatchDTO{}, mapNotFound(err)
	}
	return batchDTO(batch), nil
}

func (s *Service) ListItems(ctx context.Context, userID string, adminAccountID string, batchID string, page int, pageSize int) (ItemPage, error) {
	if _, err := s.repository.Get(ctx, userID, adminAccountID, strings.TrimSpace(batchID)); err != nil {
		return ItemPage{}, mapNotFound(err)
	}
	page, pageSize = normalizePage(page, pageSize, maxItemsPageSize)
	items, total, err := s.repository.ListItems(ctx, userID, adminAccountID, batchID, page, pageSize)
	if err != nil {
		return ItemPage{}, ErrPersistence
	}
	dtos := make([]BatchItemDTO, 0, len(items))
	for _, item := range items {
		dtos = append(dtos, itemDTO(item))
	}
	return ItemPage{Items: dtos, Total: total, Page: page, PageSize: pageSize, Pages: pages(total, pageSize)}, nil
}

func (s *Service) CancelBatch(ctx context.Context, userID string, adminAccountID string, id string) (BatchDTO, error) {
	batch, err := s.repository.CancelBatch(ctx, userID, adminAccountID, strings.TrimSpace(id))
	if err != nil {
		return BatchDTO{}, mapNotFound(err)
	}
	return batchDTO(batch), nil
}

func (s *Service) recoverStaleSending(ctx context.Context, staleAfter time.Duration) {
	if staleAfter <= 0 {
		staleAfter = 5 * time.Minute
	}
	if batchIDs, err := s.repository.RecoverStaleSending(ctx, time.Now().Add(-staleAfter)); err != nil {
		log.Printf("[mass-email] recover stale sending failed err=%v", err)
	} else if len(batchIDs) > 0 {
		for _, batchID := range batchIDs {
			if err := s.repository.FinalizeBatch(ctx, batchID); err != nil {
				log.Printf("[mass-email] finalize recovered batch failed batch_id=%s err=%v", batchID, err)
			}
		}
		log.Printf("[mass-email] recovered stale sending batches count=%d", len(batchIDs))
	}
}

func (s *Service) processOne(ctx context.Context, item BatchItem) {
	batch, err := s.repository.GetBatchByID(ctx, item.BatchID)
	if err != nil {
		_ = s.repository.CompleteItem(ctx, item.ID, ItemStatusFailed, string(ErrNotFound))
		return
	}
	if batch.Status == BatchStatusCancelling || batch.Status == BatchStatusCancelled {
		_ = s.repository.CompleteItem(ctx, item.ID, ItemStatusCancelled, "")
		return
	}
	if err := s.settings.SendSavedSMTPEmailForWorkspace(ctx, item.UserID, item.AdminAccountID, item.RecipientEmail, batch.TemplateSubject, batch.TemplateHTML); err != nil {
		// 发送调用已经发起后无法可靠判断远端是否投递成功，因此统一落入 uncertain，不做自动重试。
		_ = s.repository.CompleteItem(ctx, item.ID, ItemStatusUncertain, string(ErrSendFailed))
		return
	}
	_ = s.repository.CompleteItem(ctx, item.ID, ItemStatusSent, "")
}

func (s *Service) resolveRecipients(ctx context.Context, session upstream.Session, req CreateBatchRequest) ([]recipientCandidate, int, error) {
	if req.SelectionMode == SelectionModeSelected {
		return s.resolveSelectedRecipients(ctx, session, req.UserIDs)
	}
	return s.resolveAllRecipients(ctx, session, req.Filters)
}

func (s *Service) resolveSelectedRecipients(ctx context.Context, session upstream.Session, ids []string) ([]recipientCandidate, int, error) {
	recipients := make([]recipientCandidate, 0, len(ids))
	seen := map[string]struct{}{}
	skipped := 0
	for _, id := range ids {
		user, err := s.users.FetchSub2APIAdminUser(session, id)
		if err != nil {
			return nil, 0, mapUpstreamError(err)
		}
		candidate, ok := candidateFromUser(user)
		if !ok {
			skipped++
			continue
		}
		key := normalizeEmailKey(candidate.Email)
		if _, exists := seen[key]; exists {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		recipients = append(recipients, candidate)
	}
	return recipients, skipped, nil
}

func (s *Service) resolveAllRecipients(ctx context.Context, session upstream.Session, filters BatchFilters) ([]recipientCandidate, int, error) {
	seen := map[string]struct{}{}
	var recipients []recipientCandidate
	skipped := 0
	maxAllowedPages := pages(maxBatchRecipients, maxUsersPageSize)
	search := normalizeUserSearch(filters.Search)
	for page := 1; ; page++ {
		result, err := s.users.FetchSub2APIAdminUsersPage(session, upstream.Sub2APIAdminUsersQuery{Page: page, PageSize: maxUsersPageSize, Status: filters.Status, Role: filters.Role, Search: search, SortBy: "created_at", SortOrder: "desc"})
		if err != nil {
			return nil, 0, mapUpstreamError(err)
		}
		if (result.TotalKnown && result.Total > maxBatchRecipients) || (result.PagesKnown && result.Pages > maxAllowedPages) || (!result.TotalKnown && !result.PagesKnown && page > maxAllowedPages && len(result.Items) > 0) {
			return nil, 0, ErrRecipientLimitReached
		}
		for _, user := range result.Items {
			candidate, ok := candidateFromUser(user)
			if !ok {
				skipped++
				continue
			}
			key := normalizeEmailKey(candidate.Email)
			if _, exists := seen[key]; exists {
				skipped++
				continue
			}
			if len(recipients) >= maxBatchRecipients {
				return nil, 0, ErrRecipientLimitReached
			}
			seen[key] = struct{}{}
			recipients = append(recipients, candidate)
		}
		if len(result.Items) == 0 || (result.PagesKnown && page >= result.Pages) || (!result.PagesKnown && len(result.Items) < maxUsersPageSize) {
			break
		}
	}
	return recipients, skipped, nil
}

func normalizeCreateRequest(req CreateBatchRequest) (CreateBatchRequest, error) {
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.Filters.Status = strings.TrimSpace(req.Filters.Status)
	req.Filters.Role = strings.TrimSpace(req.Filters.Role)
	req.Filters.Search = normalizeUserSearch(req.Filters.Search)
	if req.TemplateID == "" || req.RequestID == "" {
		return CreateBatchRequest{}, ErrInvalidRequest
	}
	switch req.SelectionMode {
	case SelectionModeSelected:
		if len(req.UserIDs) == 0 || len(req.UserIDs) > maxSelectedUserIDs {
			return CreateBatchRequest{}, ErrInvalidSelection
		}
		seen := map[string]struct{}{}
		ids := make([]string, 0, len(req.UserIDs))
		for _, id := range req.UserIDs {
			trimmed := strings.TrimSpace(id)
			if trimmed == "" {
				return CreateBatchRequest{}, ErrInvalidSelection
			}
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			ids = append(ids, trimmed)
		}
		req.UserIDs = ids
	case SelectionModeAll:
		req.UserIDs = nil
	default:
		return CreateBatchRequest{}, ErrInvalidSelection
	}
	return req, nil
}

func normalizeUserSearch(value string) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if len(runes) > maxUserSearchRunes {
		return string(runes[:maxUserSearchRunes])
	}
	return trimmed
}

func candidateFromUser(user upstream.Sub2APIAdminUser) (recipientCandidate, bool) {
	email := strings.TrimSpace(user.Email)
	if email == "" || strings.ContainsAny(email, "\r\n") {
		return recipientCandidate{}, false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Name != "" || parsed.Address != email {
		return recipientCandidate{}, false
	}
	return recipientCandidate{UpstreamUserID: strings.TrimSpace(user.ID), Email: parsed.Address, Username: strings.TrimSpace(user.Username)}, true
}

func normalizeEmailKey(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func mapSettingsError(err error) error {
	switch {
	case errors.Is(err, settings.ErrEmailTemplateNotFound):
		return ErrTemplateNotFound
	case errors.Is(err, settings.ErrSMTPMissingConfig), errors.Is(err, settings.ErrSMTPEncryptionKeyUnavailable), errors.Is(err, settings.ErrSMTPDecryptFailed):
		return ErrSMTPNotReady
	default:
		return ErrPersistence
	}
}

func mapUpstreamError(err error) error {
	message := err.Error()
	if strings.Contains(message, upstream.ErrorAuth) {
		return ErrUpstreamAuth
	}
	return fmt.Errorf("%w", ErrUpstreamRequest)
}

func mapNotFound(err error) error {
	if errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "no rows") {
		return ErrNotFound
	}
	return ErrPersistence
}

func userDTO(user upstream.Sub2APIAdminUser) UserDTO {
	return UserDTO{ID: user.ID, Email: user.Email, Username: user.Username, Role: user.Role, Status: user.Status, CreatedAt: user.CreatedAt}
}

func batchDTO(batch Batch) BatchDTO {
	return BatchDTO{
		ID: batch.ID, RequestID: batch.RequestID, TemplateID: batch.TemplateID, TemplateName: batch.TemplateName,
		TemplateSubject: batch.TemplateSubject, SelectionMode: batch.SelectionMode, Filters: batch.Filters,
		Status: batch.Status, RecipientCount: batch.RecipientCount, SkippedCount: batch.SkippedCount,
		SentCount: batch.SentCount, FailedCount: batch.FailedCount, UncertainCount: batch.UncertainCount,
		CancelledCount: batch.CancelledCount, CreatedAt: batch.CreatedAt, UpdatedAt: batch.UpdatedAt,
		StartedAt: batch.StartedAt, FinishedAt: batch.FinishedAt, CancelledAt: batch.CancelledAt,
	}
}

func itemDTO(item BatchItem) BatchItemDTO {
	return BatchItemDTO{ID: item.ID, BatchID: item.BatchID, UpstreamUserID: item.UpstreamUserID, RecipientEmail: item.RecipientEmail, Username: item.Username, Status: item.Status, ErrorKey: item.ErrorKey, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, ClaimedAt: item.ClaimedAt, SentAt: item.SentAt, FinishedAt: item.FinishedAt}
}

func normalizePage(page int, pageSize int, maxPageSize int) (int, int) {
	return clampInt(page, 1, math.MaxInt, 1), clampInt(pageSize, 1, maxPageSize, defaultPageSize)
}

func clampInt(value int, min int, max int, fallback int) int {
	if value < min {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func pages(total int, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
