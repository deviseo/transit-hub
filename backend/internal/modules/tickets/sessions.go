package tickets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	sessionKeyPrefix          = "tickets:client:session:"
	workspaceSessionKeyPrefix = "tickets:client:workspace:"
)

// SessionStore 把工单会话存在 Redis：只存令牌的哈希，七天后自动过期。
type SessionStore struct {
	client *redis.Client
}

func NewSessionStore(client *redis.Client) *SessionStore {
	return &SessionStore{client: client}
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func workspaceSessionKey(ws Workspace) string {
	return workspaceSessionKeyPrefix + ws.UserID + ":" + ws.AdminAccountID
}

// Create 保存会话，返回给客户端的令牌。
func (s *SessionStore) Create(ctx context.Context, session ClientSession) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	data, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return "", errors.New("session already expired")
	}
	hash := hashToken(token)
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, sessionKeyPrefix+hash, data, ttl)
	// 记下工作区有哪些会话，删除工作区时一起删掉。
	pipe.SAdd(ctx, workspaceSessionKey(session.Workspace), hash)
	pipe.Expire(ctx, workspaceSessionKey(session.Workspace), ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}
	return token, nil
}

// Get 按令牌读取会话；没有或已过期时返回 nil。
func (s *SessionStore) Get(ctx context.Context, token string) (*ClientSession, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := s.client.Get(ctx, sessionKeyPrefix+hashToken(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var session ClientSession
	if err := json.Unmarshal(raw, &session); err != nil {
		return nil, err
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, nil
	}
	return &session, nil
}

// DeleteWorkspace 删除工作区的所有工单会话。
func (s *SessionStore) DeleteWorkspace(ctx context.Context, ws Workspace) error {
	key := workspaceSessionKey(ws)
	hashes, err := s.client.SMembers(ctx, key).Result()
	if err != nil {
		return err
	}
	keys := []string{key}
	for _, hash := range hashes {
		keys = append(keys, sessionKeyPrefix+hash)
	}
	return s.client.Del(ctx, keys...).Err()
}
