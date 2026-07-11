# SMTP 设置与测试邮件发送一期开发规格

## 1. 目标与边界

本规格用于指导 Claude Code 在现有 TransitHub 代码库中实现一期 SMTP 设置管理与测试邮件发送能力。实现必须扩展现有 settings 模块，不允许创建新的后端业务模块、新的前端路由或新的认证角色。

一期目标：管理员在现有后台「系统设置」页面中为当前工作区配置 SMTP 服务器参数，安全保存 SMTP 密码，并使用已保存配置实际发送一封静态 HTML 测试邮件。

二期明确延期：邮件模板、验证码、密码重置、队列、附件、CC/BCC、重试、连接池、用户通知触发器、SMTP 连接探测专用端点等均不在一期实现范围内。

## 2. 当前代码集成点

后端必须基于以下现有路径集成：

- `backend/internal/modules/settings/types.go`：新增 SMTP DTO、领域类型、稳定错误 key 常量。
- `backend/internal/modules/settings/repository.go`：在现有 `Repository` 上新增 SMTP 查询与保存方法。不要在 `EnsureSchema` 中创建 `smtp_settings` 表。
- `backend/internal/modules/settings/service.go`：在现有 `Service` 上新增 SMTP 获取、保存、测试邮件发送、密码加解密与校验逻辑。继续使用 `AdminAccountResolver.RequireCurrentID` 获取当前 workspace。
- `backend/internal/modules/settings/handler.go`：在 `RegisterRoutes` 中注册 SMTP API；handler 继续从 `authctx.UserID` 读取用户身份。
- `backend/internal/httpserver/server.go`：组装 settings service 时注入 SMTP 加密 key 或相关配置。`server.protectedPath('/api/settings')` 已保护 `/api/settings` 前缀，保持现有 bearer auth 边界。
- `backend/internal/config/config.go` 与 `backend/internal/config/config_test.go`：读取并测试 `SMTP_ENCRYPTION_KEY`，但该变量在应用启动时是可选项。
- `backend/internal/database/migrations/runner.go`：迁移已通过 `go:embed *.sql` 按文件名排序执行。新增 SQL 文件必须是下一个迁移 `000003_create_smtp_settings.sql`。

前端必须基于以下现有路径集成：

- `frontend/src/modules/admin/views/SettingsView.vue`：扩展现有 `/admin/settings` 页面，在当前 settings UI 内新增邮件 tab 或邮件 section，使用现有内联反馈风格。
- `frontend/src/modules/admin/api/settings.ts`：扩展现有 settings API client。
- `frontend/src/modules/admin/types/settings.ts`：新增 SMTP 类型。
- `frontend/src/locales/zh-CN.ts` 与 `frontend/src/locales/en-US.ts`：新增稳定 i18n 文案 key，两种语言都必须补齐。

部署文档集成点：

- `deploy/docker-compose.prod.yml`、`README.md`、`README_CN.md`：更新文档说明 `SMTP_ENCRYPTION_KEY` 的用途和生成方式，但不得要求现有部署必须立即配置该变量。
- 不要编辑真实 `backend/.env`，不要在仓库中写入真实密钥。

## 3. 权限、租户与兼容原则

SMTP 设置作用域必须是 `(user_id, admin_account_id)`。`user_id` 来自现有 bearer auth 注入的 `authctx.UserID`；`admin_account_id` 来自现有 `AdminAccountResolver.RequireCurrentID(ctx, userID)`。如果当前用户没有当前 admin workspace，返回 409。

现有项目没有角色模型。不要发明 `super_admin`、`owner`、`email_admin` 等新角色；一期可执行边界是现有受保护后台 API、bearer auth 和当前 workspace 作用域。

应用必须兼容已有线上部署：

- `SMTP_ENCRYPTION_KEY` 缺失时应用仍然正常启动。
- `SMTP_ENCRYPTION_KEY` 缺失时与 SMTP 无关的功能仍然可用。
- 不改变现有 settings API 的字段、状态码、路由或语义。
- 不修改发布版本号，不新增 release/version 相关改动。

## 4. 数据库迁移

新增迁移文件：`backend/internal/database/migrations/000003_create_smtp_settings.sql`。

必须使用迁移创建表，不允许把建表逻辑加到 `settings.Repository.EnsureSchema`。

表结构必须是专用表 `smtp_settings`：

```sql
CREATE TABLE IF NOT EXISTS smtp_settings (
  user_id text NOT NULL,
  admin_account_id text NOT NULL,
  host text NOT NULL,
  port integer NOT NULL,
  username text NOT NULL DEFAULT '',
  password_ciphertext text NOT NULL DEFAULT '',
  from_email text NOT NULL,
  from_name text NOT NULL DEFAULT '',
  tls_mode text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, admin_account_id),
  CONSTRAINT smtp_settings_tls_mode_check CHECK (tls_mode IN ('implicit', 'starttls')),
  CONSTRAINT smtp_settings_port_check CHECK (port BETWEEN 1 AND 65535)
);
```

禁止字段：

- 不要添加 `enabled`。一期只要保存了配置就代表可用于测试邮件。
- 不要添加 `skip_tls_verification` 或任何跳过证书校验字段。
- 不要保存明文密码字段。

Repository 保存时使用 `(user_id, admin_account_id)` upsert。GET 没有记录时返回空配置 DTO，而不是自动插入空行。

## 5. 配置与加密

新增配置项：`SMTP_ENCRYPTION_KEY`。

要求：

- app boot 时可选；缺失不 panic。
- 值必须是 base64 编码的 32 字节随机值，用作 AES-256-GCM key。
- 如果设置了但 base64 解码失败或解码后不是 32 字节，应用必须启动失败并给出不包含密钥内容的错误。理由：显式配置错误应尽早暴露。
- GET 永远不需要 `SMTP_ENCRYPTION_KEY`；已有记录即使缺 key，也必须返回全部非敏感字段以及正确的 `passwordConfigured=true`。缺 key 时禁止保存非空密码，也禁止为测试邮件解密已保存密码。

加密行为：

- 保存非空 password 时必须用 AES-256-GCM 加密，`password_ciphertext` 固定保存为 `v1:<base64.StdEncoding(nonce+ciphertext)>`。
- AES-GCM additional data 固定使用 `userID + "\x00" + adminAccountID`，解密时使用相同值，防止密文被复制到另一个 workspace 后仍可使用。
- 解密只在测试邮件发送前发生，明文密码只保存在局部变量中，不写日志，不返回给前端。
- GET 和 PUT 响应永远不返回 password 明文。
- PUT 中 `password` 字段省略、空字符串或纯空白时，保留现有 `password_ciphertext`。
- 一期没有清空密码功能；不要通过空 password 清空密文。需要更换密码时提交非空新密码。

配置与 key 解析测试必须覆盖：

- 缺失 `SMTP_ENCRYPTION_KEY` 时 `Load()` 成功，原值字段为空。
- `Load()` 原样读取非空环境变量，不在 config 层解析密钥。
- settings 层 key parser/cipher constructor 正确解析有效 base64 32 字节 key。
- settings 层拒绝非 base64 或非 32 字节 key，且错误信息不包含原始 key。
- `httpserver.New` 注入显式非法 key 时启动失败；空 key 不影响启动。

保持现有 `Load() Config` 签名不变：`Config` 保存环境变量原值，由 settings 模块提供独立的 key 解析/密码 cipher 构造函数。空值构造为“加密能力不可用”状态；显式非空但非法的值在 `httpserver.New` 组装 settings service 时触发启动失败。不要新增第二套配置加载入口。

## 6. 后端类型与 API 合同

所有 JSON 字段使用 camelCase。

### 6.1 GET /api/settings/smtp

返回当前 `(user_id, admin_account_id)` 的 SMTP 配置。

响应 200：

```json
{
  "host": "smtp.example.com",
  "port": 587,
  "username": "mailer@example.com",
  "fromEmail": "mailer@example.com",
  "fromName": "TransitHub",
  "tlsMode": "starttls",
  "passwordConfigured": true,
  "updatedAt": "2026-07-10T12:00:00Z"
}
```

无记录时响应 200，返回空默认对象：

```json
{
  "host": "",
  "port": 587,
  "username": "",
  "fromEmail": "",
  "fromName": "",
  "tlsMode": "starttls",
  "passwordConfigured": false,
  "updatedAt": null
}
```

### 6.2 PUT /api/settings/smtp

请求：

```json
{
  "host": "smtp.example.com",
  "port": 587,
  "username": "mailer@example.com",
  "password": "new-secret-or-omitted",
  "fromEmail": "mailer@example.com",
  "fromName": "TransitHub",
  "tlsMode": "starttls"
}
```

响应 200：返回与 GET 相同的安全配置对象，包含 `passwordConfigured`，不包含 `password`。

保存规则：

- `host` 必填，trim 后长度 1 到 255。
- `port` 必填，范围 1 到 65535。推荐 UI 默认 `587`。
- `username` 可空，trim 后最多 255。
- `password` 可省略；省略、空字符串或纯空白表示保留已有密文。
- 如果无已有密文且 `password` 为空，也允许保存无密码 SMTP 配置，用于无认证服务器。
- trim 后 `username` 为空但 `password` 非空时返回 400 validation；一期不保存无法使用的孤立密码。
- `fromEmail` 必填，必须是有效邮箱地址，最多 320。
- `fromName` 可空，trim 后最多 120。
- `tlsMode` 只能是 `implicit` 或 `starttls`。
- 明确拒绝 `none`，因为 SMTP 发送必须使用 TLS 1.2+。
- 禁止任何 `skipTlsVerification`、`skipTLSVerification`、`insecure` 字段进入领域模型。

### 6.3 POST /api/settings/smtp/test-email

请求：

```json
{
  "recipientEmail": "admin@example.com"
}
```

行为：

- 必须读取已保存的 SMTP 配置，不允许从请求体传 SMTP host/port/password。
- 必须实际发送邮件，不是 connection-only ping。
- 邮件内容为固定静态 HTML 测试消息，不包含用户输入内容。
- 收件人只来自 `recipientEmail`，并做邮箱格式校验。
- 发件人来自已保存 `fromEmail` 与 `fromName`。

响应 200：

```json
{
  "success": true,
  "message": "admin.settings.smtp.testEmailSuccess"
}
```

固定测试邮件合同：

- Subject: `TransitHub SMTP Test`
- HTML body: `<p>TransitHub SMTP test email was sent successfully.</p>`

## 7. HTTP 状态码与错误 key

必须使用稳定 i18n message key；不要返回底层 SMTP 错误、数据库错误、密钥内容或密码内容。

状态码语义：

- 400：请求体错误或输入校验失败。
- 401：未认证，由现有 auth middleware / handler 语义处理。
- 409：当前没有 admin workspace，沿用 `admin.adminAccounts.errors.noCurrentAccount`。
- 502：SMTP 连接、握手、认证、MAIL/RCPT/DATA 或发送失败。
- 503：`SMTP_ENCRYPTION_KEY` 不可用，或解密已保存密码失败。
- 500：持久化、查询或其他非预期内部错误。

固定边界：当前 workspace 尚未保存 SMTP 记录、已保存配置缺少必填字段，或 `username` 非空但 `passwordConfigured=false` 时，测试邮件接口返回 400 和 `admin.settings.smtp.errors.missingConfig`。已存在密文但 key 缺失或密文无法解密时才返回 503。

必须新增并只返回以下 SMTP 错误/结果 key；handler 必须为每个 service sentinel 固定唯一的状态码与 key 映射：

- `admin.settings.smtp.errors.validation`
- `admin.settings.smtp.errors.missingConfig`
- `admin.settings.smtp.errors.invalidTlsMode`
- `admin.settings.smtp.errors.invalidEmail`
- `admin.settings.smtp.errors.encryptionKeyUnavailable`
- `admin.settings.smtp.errors.decryptFailed`
- `admin.settings.smtp.errors.sendFailed`
- `admin.settings.smtp.errors.persistence`
- `admin.settings.smtp.testEmailSuccess`
- `admin.settings.smtp.saveSuccess`

| 场景 | HTTP | message key |
|---|---:|---|
| JSON/字段通用校验失败，或 username 为空但 password 非空 | 400 | `admin.settings.smtp.errors.validation` |
| 未保存配置、已保存配置缺必填字段，或 username 非空但未保存 password | 400 | `admin.settings.smtp.errors.missingConfig` |
| tlsMode 非法 | 400 | `admin.settings.smtp.errors.invalidTlsMode` |
| fromEmail 或 recipientEmail 非法 | 400 | `admin.settings.smtp.errors.invalidEmail` |
| key 缺失且当前操作需要加密或解密 | 503 | `admin.settings.smtp.errors.encryptionKeyUnavailable` |
| 密文版本、base64、认证标签或 additional data 校验失败 | 503 | `admin.settings.smtp.errors.decryptFailed` |
| SMTP 连接、TLS、认证或发送阶段失败 | 502 | `admin.settings.smtp.errors.sendFailed` |
| 数据库查询或写入失败 | 500 | `admin.settings.smtp.errors.persistence` |

handler 日志要求：

- 允许记录 SMTP 失败类别、host、port、tlsMode、workspace id，但不要记录 username、password、ciphertext、recipientEmail 的完整值或底层错误中可能包含的认证秘密。
- 对外统一返回稳定 key，例如 SMTP 失败返回 `admin.settings.smtp.errors.sendFailed`。
- 解密失败返回 503，不要把它伪装成密码错误或 SMTP 认证失败。

## 8. SMTP 发送实现要求

TLS 要求：

- `tlsMode = implicit`：用于 465 等隐式 TLS 端口，先建立 TLS 连接再执行 SMTP。
- `tlsMode = starttls`：用于 587 等 STARTTLS 端口，先明文 EHLO，必须要求服务器支持 STARTTLS，然后升级 TLS。
- STARTTLS 升级成功后必须丢弃升级前的 capability 状态并再次执行 EHLO，之后才能 AUTH、MAIL、RCPT 或 DATA。
- 两种模式都必须设置 `MinVersion: tls.VersionTLS12`。
- 必须使用系统根证书校验服务端证书，不允许跳过证书验证。
- `ServerName` 应来自 `host`，不要从用户输入的 from email 推导。
- SMTP sender 必须提供仅供包内测试使用的 TLS 配置或根证书池注入点。生产构造路径不得传入自定义根证书池，必须保持系统根、`ServerName: host`、`MinVersion: tls.VersionTLS12` 和 `InsecureSkipVerify=false`。

超时要求：

- SMTP dial、TLS 握手、SMTP 命令和整体发送必须有超时控制。建议整体超时 10 秒到 15 秒。
- 不要使用无超时的全局连接。

邮件头安全：

- 对 `fromName`、`fromEmail`、`recipientEmail`、subject 等进入邮件头的值做 CRLF sanitization。
- 如果字段包含 `\r` 或 `\n`，直接 400 拒绝，不要尝试替换。
- 使用标准库或成熟 MIME 头编码方式处理 display name，避免手写不安全头拼接。

认证：

- 如果 username 非空且 password 可用，使用 SMTP AUTH（优先 PLAIN 或现有 Go 标准库可支持方式）。
- 如果 username 为空且 password 为空，允许不认证发送。
- 保存接口已经拒绝 username 为空但 password 非空的组合；sender 不得静默忽略这种非法状态。
- 如果 username 非空但从未保存 password，测试邮件返回 400 和 `admin.settings.smtp.errors.missingConfig`。
- 如果 password 密文存在但 key 缺失或解密失败，测试邮件返回 503；不要尝试无认证降级。

## 9. 前端 UX 规格

在 `SettingsView.vue` 现有页面内新增邮件相关入口，不新增 `/admin/email`、`/admin/smtp` 等页面。

布局要求：

- 可在现有 tabs 中新增 `email` tab，或在现有 channels tab 内新增独立 SMTP 邮件 section。推荐新增 `email` tab，因为 SMTP 是系统邮件能力，不属于机器人通知渠道。
- tab/section 使用 `Mail` icon（`lucide-vue-next`）。
- 复用现有卡片、按钮、输入框、inline error/success、loading spinner 模式。
- 使用现有 dark-mode design tokens，例如 `bg-card`、`text-foreground`、`text-muted-foreground`、`border-border`、`bg-surface`，不要硬编码只适合亮色模式的颜色。

字段：

- Host：文本输入，必填。
- Port：数字输入，必填，默认 587。
- TLS Mode：只允许 `starttls` 与 `implicit`，不展示 `none`。
- Username：文本输入，可空。
- Password：密码输入，可空；placeholder 根据 `passwordConfigured` 显示「留空以保留已保存密码」或「输入 SMTP 密码」。
- From Email：邮箱输入，必填。
- From Name：文本输入，可空。
- Test Recipient：邮箱输入，只用于测试邮件，不保存到 SMTP 配置。

状态：

- `passwordConfigured` 来自 GET/PUT 响应，显示为轻量状态文本或 badge。
- 保存与测试必须有独立 loading/error/success 状态，互不覆盖。
- 测试按钮必须在存在未保存的脏配置时禁用，并显示 i18n 提示：必须先保存当前 SMTP 设置再发送测试邮件。
- dirty 只比较标准化后的已保存 SMTP 字段，并将非空 password 输入视为 dirty；`recipientEmail` 是测试参数，不参与 dirty 比较。
- 保存成功后必须以 PUT 响应替换本地 baseline、清空 password 输入，使测试按钮可用。
- 保存成功后清空前端 password 输入值，只保留后端返回的 `passwordConfigured`；不得把 password 写入 localStorage、sessionStorage、URL 或日志。
- GET 初始加载失败只影响 SMTP section，不应破坏 strategy/channels 现有功能。

前端 API 类型建议：

```ts
export type SmtpTlsMode = 'implicit' | 'starttls'

export type SmtpSettings = {
  host: string
  port: number
  username: string
  fromEmail: string
  fromName: string
  tlsMode: SmtpTlsMode
  passwordConfigured: boolean
  updatedAt: string | null
}

export type SaveSmtpSettingsPayload = {
  host: string
  port: number
  username: string
  password?: string
  fromEmail: string
  fromName: string
  tlsMode: SmtpTlsMode
}

export type TestSmtpEmailPayload = {
  recipientEmail: string
}

export type TestSmtpEmailResponse = {
  success: boolean
  message: string
}
```

前端 API 方法：

- `getSmtpSettings(): Promise<SmtpSettings>` -> `GET /settings/smtp`
- `saveSmtpSettings(payload): Promise<SmtpSettings>` -> `PUT /settings/smtp`
- `testSmtpEmail(payload): Promise<TestSmtpEmailResponse>` -> `POST /settings/smtp/test-email`

## 10. i18n 要求

`zh-CN.ts` 与 `en-US.ts` 都必须新增同构 key。key 必须稳定，后端返回的 message key 必须能被前端直接 `t(error.message)` 显示。

必须使用以下 key 结构：

```ts
admin: {
  settings: {
    tabs: {
      email: '邮件设置'
    },
    smtp: {
      title: 'SMTP 邮件设置',
      description: '配置用于发送系统邮件的 SMTP 服务器。',
      host: 'SMTP 主机',
      port: '端口',
      tlsMode: 'TLS 模式',
      tlsStarttls: 'STARTTLS (587)',
      tlsImplicit: '隐式 TLS (465)',
      username: '用户名',
      password: '密码',
      passwordConfigured: '已保存密码',
      passwordNotConfigured: '未保存密码',
      passwordKeepPlaceholder: '留空以保留已保存密码',
      passwordNewPlaceholder: '输入 SMTP 密码',
      fromEmail: '发件邮箱',
      fromName: '发件名称',
      testRecipient: '测试收件人',
      saveSuccess: 'SMTP 设置已保存',
      testEmail: '发送测试邮件',
      testEmailSuccess: '测试邮件已发送',
      dirtyBeforeTest: '请先保存当前 SMTP 设置再发送测试邮件',
      errors: {
        validation: '请检查 SMTP 设置。',
        missingConfig: '请先保存 SMTP 设置。',
        invalidTlsMode: 'TLS 模式无效。',
        invalidEmail: '邮箱地址无效。',
        encryptionKeyUnavailable: '服务器未配置 SMTP 加密密钥。',
        decryptFailed: '无法读取已保存的 SMTP 密码。',
        sendFailed: '测试邮件发送失败。',
        persistence: 'SMTP 设置保存失败。'
      }
    }
  }
}
```

英文 locale 使用同样 key，文案自然翻译即可。

## 11. 实施顺序

建议 Claude Code 按以下顺序实施，避免安全和合同漂移：

1. 新增迁移 `000003_create_smtp_settings.sql`，运行迁移相关测试或启动路径确认 embed 顺序。
2. 在 config 中新增 `SMTP_ENCRYPTION_KEY` 读取、解析和测试，保证缺失可启动、显式非法会失败。
3. 在 settings types 中定义 SMTP DTO、TLS mode、错误 sentinel。
4. 在 settings service 中实现输入校验、CRLF 拒绝、AES-GCM 加解密、workspace 解析、保存/读取逻辑。
5. 在 repository 中实现 `GetSMTPSettings`、`SaveSMTPSettings`，使用 upsert 和 `password_ciphertext` 保留语义。
6. 实现 SMTP sender，可放在 settings 模块内部小文件中；使用接口便于 fake SMTP 测试，但不要抽成新业务模块。
7. 在 handler 中注册三个 API，并映射精确状态码。
8. 在 `httpserver/server.go` 中把 config 注入 settings service。
9. 扩展前端 types/api/SettingsView，添加 email tab 或 section、独立状态、dirty 检测和测试动作。
10. 补齐 zh-CN/en-US i18n。
11. 更新 `deploy/docker-compose.prod.yml`、`README.md`、`README_CN.md` 的部署说明，不编辑 `backend/.env`。
12. 执行后端、前端、手工 QA，修复发现的问题。

## 12. 测试要求

后端必须新增或更新以下测试：

- Config/wiring tests：`Load()` 对 `SMTP_ENCRYPTION_KEY` 缺失与原值读取；settings key parser 对合法、非法 base64、非 32 字节的处理；`httpserver.New` 对空 key 可启动、显式非法 key 启动失败且错误脱敏。
- Crypto unit tests：AES-GCM 加密后不含明文；同一明文多次加密产生不同密文；正确 key 可解密；错误 key 或损坏密文失败且不泄漏明文。
- Validation tests：host、port、fromEmail、recipientEmail、tlsMode、CRLF header injection、password preserve 语义。
- Repository/service tests：按 `(user_id, admin_account_id)` 隔离；保存新密码、PUT 空密码保留密文、GET 不返回密码、无 workspace 返回 409 对应错误。
- SMTP behavior tests：使用本地 fake SMTP server，不依赖真实外部 SMTP 服务。覆盖 STARTTLS、implicit TLS、认证成功、认证失败/发送失败映射 502、缺 key/decrypt failure 映射 503。fake server 的测试证书必须加入测试专用根证书池，禁止通过 `InsecureSkipVerify` 让测试通过。
- Handler contract tests：三个路由的 JSON 字段、状态码和错误 key，至少覆盖 400、409、502、503、500 中可行场景。

前端必须覆盖或手动验证：

- TypeScript 类型通过。
- `passwordConfigured` 控制 placeholder/状态显示。
- 保存和测试 loading/error/success 独立。
- 脏配置禁止测试。
- 后端错误 key 可以在中英文 locale 中显示。

必须运行：

```bash
gofmt
go test ./...
go vet ./...
go build ./...
npm run typecheck
npm run build
```

如果仓库的实际命令需要在 `backend/` 或 `frontend/` 子目录执行，按项目现有脚本调整，但验收报告必须写明实际执行目录和命令。

## 13. 手工 QA

一期完成后必须手工验证以下路径：

1. 未配置 `SMTP_ENCRYPTION_KEY` 启动应用：登录后台，确认非 SMTP 设置页面功能仍可用；保存非空 SMTP 密码返回 503 或前端显示对应不可用提示。
2. 配置合法 `SMTP_ENCRYPTION_KEY` 启动应用：保存 STARTTLS 587 配置，刷新页面后密码不回显且 `passwordConfigured=true`。
3. 修改 host/fromEmail 但不保存：测试按钮禁用，并显示「先保存」提示。
4. 保存后对本地 fake 或真实测试 SMTP 服务器发送测试邮件，确认实际收到静态 HTML 测试邮件。
5. 使用 465 implicit TLS 手工发送成功。
6. 使用 587 STARTTLS 手工发送成功。
7. 尝试 `tlsMode=none` API 请求，确认 400。
8. 尝试带 CRLF 的 fromName/fromEmail/recipientEmail，确认 400，且日志不包含注入内容。
9. 使用错误 SMTP 密码，确认响应 502，前端显示泛化失败，不泄漏认证错误细节。
10. 切换 workspace，确认不同 `(user_id, admin_account_id)` 看到不同 SMTP 配置。

## 14. 部署文档要求

文档更新必须说明：

- `SMTP_ENCRYPTION_KEY` 是可选环境变量；仅当需要保存 SMTP 密码或发送测试邮件时必需。
- 缺失该变量不会阻止应用启动，也不会影响非 SMTP 功能。
- 生成方式示例：

```bash
openssl rand -base64 32
```

- 该值必须长期稳定保存。更换 key 后，旧的 SMTP 密码密文无法解密，需要重新输入并保存密码。
- 不要把真实 key 提交到仓库。
- `docker-compose.prod.yml` 可以增加注释掉的示例项，或在 README 中说明添加到 `app.environment`；不要强制现有部署定义该变量。

## 15. 禁止事项

实现期间不得做以下事情：

- 不创建 Go/Vue/TypeScript/config/deploy/README 之外的无关改动。
- 不新增 phase-two 模板、验证码、密码重置、队列、附件、CC/BCC、重试、连接池。
- 不支持 TLS none。
- 不支持 skip TLS verification。
- 不回显 password。
- 不保存 plaintext secrets。
- 不新增 SMTP connection-only endpoint。
- 不新增后台页面路由。
- 不新增 auth role 或权限模型。
- 不改 release/version。
- 不做无关重构。

## 16. 验收标准

满足以下全部条件才算一期完成：

- 三个 API 路由存在并受现有 `/api/settings` bearer auth 保护。
- SMTP 设置严格按 `(user_id, admin_account_id)` 隔离。
- 数据库存在 `smtp_settings` 专用表，由 `000003_create_smtp_settings.sql` 创建。
- `tlsMode` 只接受 `implicit` 和 `starttls`，并强制 TLS 1.2+。
- `SMTP_ENCRYPTION_KEY` 缺失时应用可启动；保存非空密码或测试需解密密码时返回 503。
- 密码永远不返回前端，PUT 空密码保留已有密文。
- 测试邮件使用已保存配置实际发送静态 HTML 邮件。
- CRLF header injection 被拒绝。
- 外部错误、日志和 HTTP 响应不泄漏密码、密文或 SMTP 认证细节。
- 前端在现有 settings 页面内提供邮件设置 UI、保存、测试收件人、dirty 禁测和中英文文案。
- 所有要求的测试、build、vet/typecheck/build 与手工 QA 均完成并有结果记录。

## 17. 对抗式 Review Checklist

后续验收审查时必须逐项挑战：

- 是否真的扩展了 settings 模块，而不是新增了平行模块或新页面？
- 是否所有 repository 查询都带 `user_id` 和 `admin_account_id`？是否存在跨 workspace 泄漏？
- 是否把 `smtp_settings` 建表误放进 `EnsureSchema`？
- 是否存在 `enabled`、`skip_tls_verification`、`tls none` 或隐藏的不安全 fallback？
- `SMTP_ENCRYPTION_KEY` 缺失是否真的不影响启动和非 SMTP 功能？
- 非法 key 是否尽早失败，且错误不包含 key 原文？
- 密码是否可能出现在 JSON 响应、日志、测试快照、前端状态持久化或错误消息中？
- PUT 空 password 是否真的保留密文，而不是清空或写入空密文？
- 测试邮件是否读取已保存配置，而不是使用请求体 SMTP 参数？
- 测试邮件是否真的发送 DATA，而不是只 dial/EHLO/STARTTLS？
- STARTTLS 模式是否要求服务器支持 STARTTLS，且升级后才认证和发送？
- TLS 配置是否设置 `MinVersion: tls.VersionTLS12`，且没有 `InsecureSkipVerify`？
- 所有邮件头输入是否拒绝 CRLF？
- 502/503/500 是否区分正确？解密失败是否没有被误报为 SMTP 认证失败？
- 前端是否在脏配置时禁止测试？保存成功后 baseline 是否刷新？
- `zh-CN.ts` 与 `en-US.ts` 是否都有同构 key？后端返回 key 是否能被前端翻译？
- 测试是否使用本地 fake SMTP server，未依赖真实外部服务？
- README/docker-compose 是否只做可选说明，没有强制旧部署新增变量？
- 是否没有实现任何二期能力或无关重构？
