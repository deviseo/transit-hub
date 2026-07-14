package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// embedSessionTTL 是 iframe 短期会话的有效期：iframe 打开后在此时长内复用同一会话，
// 避免每次读写工单都重新调用 Sub2API /auth/me（文档建议优先做短期 session）。过期后前端需要
// 重新走一次会话初始化（重新携带 URL 中 Sub2API 追加的 token 参数）。
const embedSessionTTL = 30 * time.Minute

const embedSessionKeyPrefix = "tickets:embed:session:"
const embedWorkspaceIndexKeyPrefix = "tickets:embed:workspace:"

// EmbedSessionStore 把 iframe 会话短期存放在 Redis：不写入 PostgreSQL、不含任何 Sub2API token，
// 满足"不保存 Sub2API token"的安全边界。
type EmbedSessionStore struct {
	client *redis.Client
}

func NewEmbedSessionStore(client *redis.Client) *EmbedSessionStore {
	return &EmbedSessionStore{client: client}
}

func (s *EmbedSessionStore) Save(ctx context.Context, token string, session EmbedSession) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	sessionKey := embedSessionKey(token)
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, sessionKey, payload, embedSessionTTL)
	indexKey := embedWorkspaceIndexKey(session.UserID, session.AdminAccountID)
	if strings.TrimSpace(session.UserID) != "" && strings.TrimSpace(session.AdminAccountID) != "" {
		pipe.SAdd(ctx, indexKey, sessionKey)
		pipe.Expire(ctx, indexKey, embedSessionTTL)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// Get 读取一个 embed session；不存在或已过期时返回 (nil, nil)，由调用方统一映射为"会话无效"错误。
func (s *EmbedSessionStore) Get(ctx context.Context, token string) (*EmbedSession, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := s.client.Get(ctx, embedSessionKey(token)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var session EmbedSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// DeleteWorkspace 删除指定 workspace 的 iframe 短期会话。新会话通过 workspace index 精确删除；
// 为兼容旧版本未写入 index 的会话，会安全扫描 tickets:embed:session:* 并只删除 JSON 中
// UserID/AdminAccountID 完全匹配的 key，避免误删其他 workspace 的会话。
func (s *EmbedSessionStore) DeleteWorkspace(ctx context.Context, userID string, adminAccountID string) error {
	userID = strings.TrimSpace(userID)
	adminAccountID = strings.TrimSpace(adminAccountID)
	if userID == "" || adminAccountID == "" {
		return nil
	}
	indexKey := embedWorkspaceIndexKey(userID, adminAccountID)
	indexedKeys, err := s.client.SMembers(ctx, indexKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	deleteSet := make(map[string]struct{}, len(indexedKeys))
	for _, key := range indexedKeys {
		if strings.HasPrefix(key, embedSessionKeyPrefix) {
			deleteSet[key] = struct{}{}
		}
	}

	iter := s.client.Scan(ctx, 0, embedSessionKeyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		raw, err := s.client.Get(ctx, key).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			return err
		}
		if embedSessionBelongsToWorkspace(raw, userID, adminAccountID) {
			deleteSet[key] = struct{}{}
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}

	keys := make([]string, 0, len(deleteSet)+1)
	for key := range deleteSet {
		keys = append(keys, key)
	}
	keys = append(keys, indexKey)
	return s.client.Del(ctx, keys...).Err()
}

func embedSessionKey(token string) string {
	return embedSessionKeyPrefix + token
}

func embedWorkspaceIndexKey(userID string, adminAccountID string) string {
	return embedWorkspaceIndexKeyPrefix + strings.TrimSpace(userID) + ":" + strings.TrimSpace(adminAccountID)
}

func embedSessionBelongsToWorkspace(raw string, userID string, adminAccountID string) bool {
	var session EmbedSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return false
	}
	return session.UserID == userID && session.AdminAccountID == adminAccountID
}
