package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// embedSessionTTL 是 iframe 短期会话的有效期：iframe 打开后在此时长内复用同一会话，
// 避免每次读写工单都重新调用 Sub2API /auth/me（文档建议优先做短期 session）。过期后前端需要
// 重新走一次会话初始化（重新携带 URL 中 Sub2API 追加的 token 参数）。
const embedSessionTTL = 30 * time.Minute

const embedSessionKeyPrefix = "tickets:embed:session:"

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
	return s.client.Set(ctx, embedSessionKeyPrefix+token, payload, embedSessionTTL).Err()
}

// Get 读取一个 embed session；不存在或已过期时返回 (nil, nil)，由调用方统一映射为"会话无效"错误。
func (s *EmbedSessionStore) Get(ctx context.Context, token string) (*EmbedSession, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := s.client.Get(ctx, embedSessionKeyPrefix+token).Result()
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
