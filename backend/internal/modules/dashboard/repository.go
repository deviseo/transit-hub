package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Redis key 约定：
//   - sessionKeyPrefix + userID + ":" + adminAccountID 存储单个 workspace 的 admin 会话 JSON。
//   - activeSetKey 维护当前持有会话的 userID:adminAccountID 集合，供后台刷新协程高效遍历，
//     避免对整个 keyspace 做 SCAN。
const (
	sessionKeyPrefix = "dashboard:admin:session:"
	activeSetKey     = "dashboard:admin:active"
)

// Repository 封装仪表盘 admin 会话在 Redis 中的读写。
type Repository struct {
	client *redis.Client
}

type ActiveSessionRef struct {
	UserID         string
	AdminAccountID string
}

func NewRepository(client *redis.Client) *Repository {
	return &Repository{client: client}
}

func sessionKey(userID string, adminAccountID string) string {
	return sessionKeyPrefix + userID + ":" + adminAccountID
}

func activeMember(userID string, adminAccountID string) string {
	return userID + ":" + adminAccountID
}

// Get 读取用户的 admin 会话，不存在时返回 (nil, nil)。
func (r *Repository) Get(ctx context.Context, userID string, adminAccountID string) (*AdminSession, error) {
	raw, err := r.client.Get(ctx, sessionKey(userID, adminAccountID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var session AdminSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// Save 写入会话并把 userID 登记到活跃集合。
// 会话长期有效（依赖后台刷新维持令牌），因此不设置 TTL。
func (r *Repository) Save(ctx context.Context, userID string, adminAccountID string, session AdminSession) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	pipe := r.client.TxPipeline()
	pipe.Set(ctx, sessionKey(userID, adminAccountID), payload, 0)
	pipe.SAdd(ctx, activeSetKey, activeMember(userID, adminAccountID))
	_, err = pipe.Exec(ctx)
	return err
}

// Delete 删除会话并从活跃集合移除，用于退出当前 admin 账户。
func (r *Repository) Delete(ctx context.Context, userID string, adminAccountID string) error {
	pipe := r.client.TxPipeline()
	pipe.Del(ctx, sessionKey(userID, adminAccountID))
	pipe.SRem(ctx, activeSetKey, activeMember(userID, adminAccountID))
	_, err := pipe.Exec(ctx)
	return err
}

// ActiveSessions 返回当前持有会话的所有 userID/adminAccountID，供后台刷新协程遍历。
func (r *Repository) ActiveSessions(ctx context.Context) ([]ActiveSessionRef, error) {
	members, err := r.client.SMembers(ctx, activeSetKey).Result()
	if err != nil {
		return nil, err
	}
	refs := make([]ActiveSessionRef, 0, len(members))
	for _, member := range members {
		parts := strings.SplitN(member, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		refs = append(refs, ActiveSessionRef{UserID: parts[0], AdminAccountID: parts[1]})
	}
	return refs, nil
}
