package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repository *Repository
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type TokenResponse struct {
	Strategy    string `json:"strategy"`
	Subject     string `json:"subject"`
	AccessToken string `json:"accessToken"`
}

type CurrentUser struct {
	ID string
}

type RequestError struct {
	Status  int
	Message string
}

func (e *RequestError) Error() string {
	return e.Message
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	return s.repository.EnsureSchema(ctx)
}

// BootstrapAdmin 在启动时检查是否需要创建首个管理员账号。
// 规则：用户表为空时使用 email/password 创建管理员；已有用户时不做任何事。
func (s *Service) BootstrapAdmin(ctx context.Context, email, password string) error {
	count, err := s.repository.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("bootstrap admin: count users: %w", err)
	}
	if count > 0 {
		log.Printf("[auth] %d users exist, skipping admin bootstrap", count)
		return nil
	}

	// 没有用户，必须有管理员凭据
	email = normalizeEmail(email)
	password = strings.TrimSpace(password)
	if email == "" || password == "" {
		return fmt.Errorf("bootstrap admin: ADMIN_EMAIL and ADMIN_PASSWORD are required when no users exist")
	}

	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("bootstrap admin: hash password: %w", err)
	}

	if err := s.repository.CreateUser(ctx, email, hash); err != nil {
		return fmt.Errorf("bootstrap admin: create user: %w", err)
	}

	log.Printf("[auth] admin account created for %s", email)
	return nil
}

func (s *Service) Login(ctx context.Context, dto LoginRequest) (TokenResponse, error) {
	email := normalizeEmail(dto.Email)
	password := strings.TrimSpace(dto.Password)
	if email == "" || password == "" {
		return TokenResponse{}, requestError(http.StatusBadRequest, "auth.errors.invalidLogin")
	}
	passwordHash, err := s.repository.PasswordHashByEmail(ctx, email)
	if err != nil {
		return TokenResponse{}, err
	}
	if passwordHash == "" || !verifyPassword(passwordHash, password) {
		return TokenResponse{}, requestError(http.StatusUnauthorized, "auth.errors.invalidCredentials")
	}
	return s.createSession(ctx, "login", email)
}

func (s *Service) CurrentUser(ctx context.Context, accessToken string) (CurrentUser, error) {
	token := strings.TrimSpace(accessToken)
	if token == "" {
		return CurrentUser{}, requestError(http.StatusUnauthorized, "auth.errors.unauthorized")
	}
	userID, err := s.repository.UserIDBySessionToken(ctx, hashValue(token))
	if err != nil {
		return CurrentUser{}, err
	}
	if userID == "" {
		return CurrentUser{}, requestError(http.StatusUnauthorized, "auth.errors.unauthorized")
	}
	return CurrentUser{ID: userID}, nil
}

func (s *Service) createSession(ctx context.Context, strategy string, email string) (TokenResponse, error) {
	token, err := randomToken(32)
	if err != nil {
		return TokenResponse{}, err
	}
	userID, err := s.repository.UserIDByEmail(ctx, email)
	if err != nil {
		return TokenResponse{}, err
	}
	if err := s.repository.CreateSession(ctx, userID, hashValue(token), time.Now().Add(7*24*time.Hour)); err != nil {
		return TokenResponse{}, err
	}
	return TokenResponse{Strategy: strategy, Subject: email, AccessToken: token}, nil
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyPassword(passwordHash string, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) == nil
}

func randomToken(bytesCount int) (string, error) {
	data := make([]byte, bytesCount)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func requestError(status int, message string) *RequestError {
	return &RequestError{Status: status, Message: message}
}
