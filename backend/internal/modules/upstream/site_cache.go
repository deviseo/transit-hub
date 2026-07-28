package upstream

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Redis key 约定：
//   - siteKeyPrefix + siteID 存储单个站点的完整 JSON（包含 UserID 和 Session）。
//   - userSitesKeyPrefix + userID 维护该用户拥有的站点 ID 集合，
//     用于快速按用户列出站点，避免全量扫描。
const (
	siteKeyPrefix      = "upstream:site:"
	userSitesKeyPrefix = "upstream:user:"
	userSitesKeySuffix = ":sites"
)

// SiteCache 抽象上游站点的运行时缓存层，由 Redis 实现。
// 站点的持久化仍由 PostgreSQL（SiteRepository）负责，SiteCache 仅保存运行时状态。
type SiteCache interface {
	Get(ctx context.Context, id string) (*Site, error)
	Set(ctx context.Context, site *Site) error
	Delete(ctx context.Context, id string, userID string) error
	ListByUser(ctx context.Context, userID string) ([]*Site, error)
	// Flush 清除所有上游站点缓存。启动时调用，确保 Redis 与 PostgreSQL 一致。
	Flush(ctx context.Context) error
}

// RedisSiteCache 使用 Redis 存储上游站点运行时状态。
// Site 结构体的 UserID 和 Session 字段标记了 json:"-"，
// 因此使用 sitePayload 包装结构绕过标签限制，保证完整序列化。
type RedisSiteCache struct {
	client *redis.Client
}

func NewRedisSiteCache(client *redis.Client) *RedisSiteCache {
	return &RedisSiteCache{client: client}
}

// sitePayload 是 Site 在 Redis 中的序列化格式。
// 嵌入 Site 后通过额外字段覆盖 json:"-" 标记的 UserID 和 Session，
// 确保这些关键字段不会在序列化时丢失。
type sitePayload struct {
	ID                string       `json:"id"`
	UserID            string       `json:"userId"`
	AdminAccountID    string       `json:"adminAccountId"`
	Name              string       `json:"name"`
	BaseURL           string       `json:"baseUrl"`
	Platform          Platform     `json:"platform"`
	RequestedPlatform Platform     `json:"requestedPlatform"`
	Account           string       `json:"account"`
	Remark            string       `json:"remark"`
	RechargeRate      float64      `json:"rechargeRate"`
	Status            Status       `json:"status"`
	ErrorKey          *string      `json:"errorKey"`
	Metrics           Metrics      `json:"metrics"`
	Settings          SiteSettings `json:"settings"`
	LastSyncedAt      *int64       `json:"lastSyncedAt"`
	Session           *Session     `json:"session,omitempty"`
}

func toPayload(site *Site) sitePayload {
	return sitePayload{
		ID:                site.ID,
		UserID:            site.UserID,
		AdminAccountID:    site.AdminAccountID,
		Name:              site.Name,
		BaseURL:           site.BaseURL,
		Platform:          site.Platform,
		RequestedPlatform: site.RequestedPlatform,
		Account:           site.Account,
		Remark:            site.Remark,
		RechargeRate:      site.RechargeRate,
		Status:            site.Status,
		ErrorKey:          site.ErrorKey,
		Metrics:           site.Metrics,
		Settings:          site.Settings,
		LastSyncedAt:      site.LastSyncedAt,
		Session:           site.Session,
	}
}

func fromPayload(p sitePayload) *Site {
	return &Site{
		ID:                p.ID,
		UserID:            p.UserID,
		AdminAccountID:    p.AdminAccountID,
		Name:              p.Name,
		BaseURL:           p.BaseURL,
		Platform:          p.Platform,
		RequestedPlatform: p.RequestedPlatform,
		Account:           p.Account,
		Remark:            p.Remark,
		RechargeRate:      p.RechargeRate,
		Status:            p.Status,
		ErrorKey:          p.ErrorKey,
		Metrics:           p.Metrics,
		Settings:          p.Settings,
		LastSyncedAt:      p.LastSyncedAt,
		Session:           p.Session,
	}
}

func siteKey(id string) string {
	return siteKeyPrefix + id
}

func userSitesKey(userID string) string {
	return userSitesKeyPrefix + userID + userSitesKeySuffix
}

// Get 从 Redis 读取单个站点，不存在时返回 (nil, nil)。
func (c *RedisSiteCache) Get(ctx context.Context, id string) (*Site, error) {
	raw, err := c.client.Get(ctx, siteKey(id)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var p sitePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return nil, err
	}
	return fromPayload(p), nil
}

// Set 写入站点到 Redis，同时把站点 ID 添加到该用户的站点集合中。
// 使用 Pipeline 保证两个操作在同一个 RTT 完成。
func (c *RedisSiteCache) Set(ctx context.Context, site *Site) error {
	payload, err := json.Marshal(toPayload(site))
	if err != nil {
		return err
	}
	pipe := c.client.TxPipeline()
	pipe.Set(ctx, siteKey(site.ID), payload, 0)
	if strings.TrimSpace(site.UserID) != "" {
		pipe.SAdd(ctx, userSitesKey(site.UserID), site.ID)
	}
	_, err = pipe.Exec(ctx)
	return err
}

// Delete 从 Redis 删除站点及其在用户集合中的引用。
func (c *RedisSiteCache) Delete(ctx context.Context, id string, userID string) error {
	pipe := c.client.TxPipeline()
	pipe.Del(ctx, siteKey(id))
	if strings.TrimSpace(userID) != "" {
		pipe.SRem(ctx, userSitesKey(userID), id)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ListByUser 返回指定用户的所有站点。
// 先从用户集合获取站点 ID 列表，再批量读取站点 JSON。
func (c *RedisSiteCache) ListByUser(ctx context.Context, userID string) ([]*Site, error) {
	ids, err := c.client.SMembers(ctx, userSitesKey(userID)).Result()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// 使用 Pipeline 批量获取，减少 RTT。
	pipe := c.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(ids))
	for i, id := range ids {
		cmds[i] = pipe.Get(ctx, siteKey(id))
	}
	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	sites := make([]*Site, 0, len(ids))
	for _, cmd := range cmds {
		raw, err := cmd.Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			return nil, err
		}
		var p sitePayload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
		sites = append(sites, fromPayload(p))
	}
	return sites, nil
}

// Flush 清除所有上游站点相关的 Redis key。
// 使用 SCAN 遍历匹配的 key 并批量删除，避免阻塞 Redis 的 KEYS 命令。
// 在进程启动时调用，确保 Redis 缓存与 PostgreSQL 持久层完全一致。
func (c *RedisSiteCache) Flush(ctx context.Context) error {
	patterns := []string{siteKeyPrefix + "*", userSitesKeyPrefix + "*"}
	for _, pattern := range patterns {
		iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
		for iter.Next(ctx) {
			if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
				return err
			}
		}
		if err := iter.Err(); err != nil {
			return err
		}
	}
	return nil
}
