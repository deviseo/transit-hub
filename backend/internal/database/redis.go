package database

import (
	"context"

	"github.com/redis/go-redis/v9"
)

// ConnectRedis 建立 Redis 连接并做一次 Ping 健康检查。
//
// 仪表盘的 admin 会话（access/refresh token、到期时间、刷新时间）保存在 Redis 中，
// 后台刷新协程也依赖它来扫描临期会话。连接参数统一通过标准的 redis URL 解析，
// 例如 redis://127.0.0.1:6379/0。
func ConnectRedis(ctx context.Context, redisURL string) (*redis.Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}
