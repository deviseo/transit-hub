package config

import (
	"bufio"
	"os"
	"strings"
	"time"
)

const (
	defaultPort       = "10621"
	defaultRedisURL   = "redis://127.0.0.1:6379/0"
	defaultPublicDir  = "/app/public"
	defaultAppVersion = "v0.1.6"
)

type Config struct {
	// 基础运行配置
	Port        string
	DatabaseURL string
	RedisURL    string
	CORSOrigins []string
	PublicDir   string // 前端静态文件目录，生产默认 /app/public

	// 初始化管理员
	AdminEmail    string
	AdminPassword string

	// 公开注册开关，false 时禁用 /api/auth/register 和 /api/auth/email-code
	AllowPublicRegister bool

	// 版本展示：开源版仅用于系统信息 API 和前端纯展示，不依赖任何远程授权/更新服务。
	// 发布版本号由代码内置，不能由部署环境变量覆盖，避免部署用户随意修改后台展示版本。
	AppVersion string

	// 活动调价（group_rate_campaigns）默认值，仅作为创建活动时未填字段的兜底，不强制覆盖用户在页面上的选择。
	GroupRateCampaignNotifyEnabled     bool
	GroupRateCampaignDefaultNotifyBots []string
	GroupRateCampaignStartTemplate     string
	GroupRateCampaignEndTemplate       string
	GroupRateCampaignSchedulerInterval time.Duration

	// 工单图片附件本地存储目录：只存文件本身，metadata 落库；目录不存在时自动创建。
	TicketUploadDir string

	// SMTP 密码加密密钥：base64 编码的 32 字节 AES-256-GCM key，由 settings 模块解析和校验。
	// 应用启动时是可选项，缺失不影响启动；这里只原样读取环境变量原值，不做任何解析或校验。
	SMTPEncryptionKey string
}

func Load() Config {
	loadEnvFiles(
		envFile{path: "backend/.env"},
		envFile{path: ".env"},
	)

	return Config{
		Port:        envOrDefault("PORT", defaultPort),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		// 仪表盘 admin 会话与令牌刷新调度依赖 Redis。未显式配置时回退到本地默认地址，
		// 便于开发环境直接通过 docker-compose 中的 redis 服务连通。
		RedisURL:    envOrDefault("REDIS_URL", defaultRedisURL),
		CORSOrigins: splitOrigins(os.Getenv("CORS_ORIGINS")),
		PublicDir:   envOrDefault("PUBLIC_DIR", defaultPublicDir),

		AdminEmail:    os.Getenv("ADMIN_EMAIL"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),

		AllowPublicRegister: envOrDefault("ALLOW_PUBLIC_REGISTER", "true") == "true",

		AppVersion: defaultAppVersion,

		GroupRateCampaignNotifyEnabled:     envOrDefault("GROUP_RATE_CAMPAIGN_NOTIFY_ENABLED", "false") == "true",
		GroupRateCampaignDefaultNotifyBots: splitOrigins(os.Getenv("GROUP_RATE_CAMPAIGN_DEFAULT_NOTIFY_BOT_IDS")),
		GroupRateCampaignStartTemplate:     os.Getenv("GROUP_RATE_CAMPAIGN_START_NOTIFY_TEMPLATE"),
		GroupRateCampaignEndTemplate:       os.Getenv("GROUP_RATE_CAMPAIGN_END_NOTIFY_TEMPLATE"),
		GroupRateCampaignSchedulerInterval: envDuration("GROUP_RATE_CAMPAIGN_SCHEDULER_INTERVAL", 60*time.Second),

		TicketUploadDir: envOrDefault("TICKET_UPLOAD_DIR", "data/ticket-uploads"),

		SMTPEncryptionKey: os.Getenv("SMTP_ENCRYPTION_KEY"),
	}
}

type envFile struct {
	path         string
	overrideKeys map[string]struct{}
}

func loadEnvFiles(files ...envFile) {
	for _, envFile := range files {
		file, err := os.Open(envFile.path)
		if err != nil {
			continue
		}

		func() {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				setEnvLine(scanner.Text(), envFile.overrideKeys)
			}
		}()
	}
}

func setEnvLine(line string, overrideKeys map[string]struct{}) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return
	}

	key, value, ok := strings.Cut(trimmed, "=")
	if !ok {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	_, shouldOverride := overrideKeys[key]
	if _, exists := os.LookupEnv(key); exists && !shouldOverride {
		return
	}
	os.Setenv(key, strings.Trim(strings.TrimSpace(value), `"'`))
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// envDuration 解析形如 "60s"、"5m" 的环境变量；缺失或不合法时回退到 fallback，不中断启动。
func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitOrigins(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		origin := strings.TrimSpace(part)
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
