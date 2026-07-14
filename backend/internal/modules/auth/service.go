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

const defaultEmailCode = "123456"

type Service struct {
	repository *Repository
}

type EmailCodeRequest struct {
	Email string `json:"email"`
}

type EmailCodeResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type PasswordLogin struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

type APIKeyLogin struct {
	APIKey string `json:"apiKey"`
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

func (s *Service) RequestEmailCode(ctx context.Context, dto EmailCodeRequest) (EmailCodeResponse, error) {
	email := normalizeEmail(dto.Email)
	if email == "" {
		return EmailCodeResponse{}, requestError(http.StatusBadRequest, "auth.errors.emailRequired")
	}

	// 当前阶段验证码固定为 123456，但仍写入验证码表；以后接入真实邮件时只需替换生成和发送逻辑。
	if err := s.repository.SaveEmailCode(ctx, email, hashValue(defaultEmailCode), time.Now().Add(10*time.Minute)); err != nil {
		return EmailCodeResponse{}, err
	}
	return EmailCodeResponse{Success: true, Code: defaultEmailCode}, nil
}

func (s *Service) Register(ctx context.Context, dto RegisterRequest) (TokenResponse, error) {
	email := normalizeEmail(dto.Email)
	password := strings.TrimSpace(dto.Password)
	code := strings.TrimSpace(dto.Code)
	if email == "" || password == "" || code == "" {
		return TokenResponse{}, requestError(http.StatusBadRequest, "auth.errors.invalidRegister")
	}
	if code != defaultEmailCode {
		return TokenResponse{}, requestError(http.StatusBadRequest, "auth.errors.invalidCode")
	}

	// 验证码表记录用于模拟真实邮箱验证码生命周期，防止未请求验证码就直接注册。
	verification, err := s.repository.LatestEmailCode(ctx, email)
	if err != nil {
		return TokenResponse{}, err
	}
	if verification == nil || verification.CodeHash != hashValue(defaultEmailCode) || time.Now().After(verification.ExpiresAt) {
		return TokenResponse{}, requestError(http.StatusBadRequest, "auth.errors.invalidCode")
	}
	// 密码和验证码分开处理：验证码是临时固定值，密码必须用带盐哈希保存，避免明文或快速哈希落库。
	passwordHash, err := hashPassword(password)
	if err != nil {
		return TokenResponse{}, err
	}
	if err := s.repository.CreateUser(ctx, email, passwordHash); err != nil {
		if isUniqueViolation(err) {
			return TokenResponse{}, requestError(http.StatusConflict, "auth.errors.emailExists")
		}
		return TokenResponse{}, err
	}
	if err := s.repository.ConsumeEmailCode(ctx, verification.ID); err != nil {
		return TokenResponse{}, err
	}
	return s.createSession(ctx, "register", email)
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

func (s *Service) LoginWithPassword(dto PasswordLogin) (TokenResponse, bool) {
	if strings.TrimSpace(dto.Account) == "" || strings.TrimSpace(dto.Password) == "" {
		return TokenResponse{}, false
	}
	return TokenResponse{Strategy: "password", Subject: dto.Account, AccessToken: "pending-implementation"}, true
}

func (s *Service) LoginWithAPIKey(dto APIKeyLogin) (TokenResponse, bool) {
	if strings.TrimSpace(dto.APIKey) == "" {
		return TokenResponse{}, false
	}
	return TokenResponse{Strategy: "api-key", Subject: dto.APIKey, AccessToken: "pending-implementation"}, true
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
