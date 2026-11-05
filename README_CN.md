# TransitHub

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8.svg)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3.5+-4FC08D.svg)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-336791.svg)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D.svg)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg)](https://www.docker.com/)

**面向 sub2api / new-api 自托管 API 服务的多上游运营管理中心。**

[English](README.md) | 中文

</div>

## 重要说明

使用本项目之前，请先阅读以下内容：

- **上游平台规则风险**：TransitHub 用于帮助管理员连接和操作上游后台平台。请确认你的使用方式符合所有上游平台的服务条款。
- **合规使用**：请仅在你所在国家或地区法律法规允许的范围内使用本项目。禁止用于绕过授权、滥用上游服务，或操作你无权管理的账号。
- **自部署责任**：你需要自行保护管理员凭据、数据库备份、网络访问权限和部署密钥。
- **免责声明**：本项目仅用于技术学习。你需要自行确保使用方式符合适用法律法规和上游平台规则。因使用本项目导致的服务中断、账号限制、数据丢失或其他直接/间接损失，作者不承担责任。

## 赞助商

<table>
<thead>
<tr>
<th align="center" valign="middle" width="130">名称</th>
<th align="left" valign="middle" width="78%">描述</th>
</tr>
</thead>
<tbody>
<tr>
<td align="center" valign="middle" width="130"><a href="https://www.recycleai.vip/"><img src="docs/assets/sponsors/recycleai-logo.png" alt="RecycleAI logo" height="28"><br><strong>RecycleAI</strong></a></td>
<td valign="middle" width="78%">面向 AI 资源复用与智能服务循环的创新平台，致力于提升算力、模型与应用能力之间的协同效率。</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://www.xiongxiongai.online"><img src="docs/assets/sponsors/xiongxiongai-logo.png" alt="熊熊AI logo" height="30"><br><strong>熊熊AI</strong></a></td>
<td valign="middle" width="78%">以亲和体验、稳定接入和轻量化智能工具为核心的 AI 服务品牌，为用户提供更顺手、更可靠的智能应用入口。</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://console.qqqrouter.ai"><img src="docs/assets/sponsors/qqqrouter-logo.png" alt="qqqRouter logo" height="24"><br><strong>qqqRouter</strong></a></td>
<td valign="middle" width="78%">面向多模型接入、请求路由与用量治理的 AI 基础设施平台，帮助团队构建更灵活、更可控的模型调用链路。</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://sparkcode.top"><img src="docs/assets/sponsors/sparkcode-logo.png" alt="SparkCode logo" height="30"><br><strong>SparkCode</strong></a></td>
<td valign="middle" width="78%">稳定高效的 API 中转服务商，提供 Claude Code、Codex、Gemini（含 NanoBanana 系列模型）等主流 AI 编程模型接入；支持包月与按量计费、高并发调用、充值开票、企业专属对接与技术支持，并提供长期有效的邀请返佣福利。</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://uuapi.net"><img src="docs/assets/sponsors/uuapi.svg" alt="UU API logo" height="30"><br><strong>UU API</strong></a></td>
<td valign="middle" width="78%">面向全球开发者与企业的 AI 算力网关，一站式接入 ChatGPT、Claude、Gemini 等主流大模型；坚持渠道透明、官方直连或一手渠道，已为数千位开发者和上百家企业提供稳定 AI 基础设施服务，让每一次调用都值得。</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://hk.getelucid.com/"><img src="docs/assets/sponsors/ElucidRelay.png" alt="ElucidRelay logo" height="30"><br><strong>ElucidRelay</strong></a></td>
<td valign="middle" width="78%">高吞吐 API relay，通过单一 OpenAI-compatible endpoint 稳定接入 OpenAI、Claude、Gemini 等海外主流模型；适合希望一站式启动服务的站点运营者、代理商和下游平台，兼顾稳定容量与有竞争力的价格。</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://songsongai.com/"><img src="docs/assets/sponsors/songsongai.png" alt="songsongAi logo" height="30"><br><strong>songsongAi</strong></a></td>
<td valign="middle" width="78%">面向高品质智能应用体验的 AI 服务品牌，聚焦优质模型能力、稳定服务交付与简洁高效的使用路径，为个人创作者和团队提供值得信赖的智能生产力入口。</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://web.ymocode.com"><img src="docs/assets/sponsors/yimous.png" alt="Yimo-US logo" height="22"><br><strong>Yimo-US</strong></a></td>
<td valign="middle" width="78%">专注于稳定接入与多上游聚合的 API 服务品牌，面向高可用访问、资源整合与持续服务体验打造专业化接入能力。</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://wahaapi.top/"><img src="docs/assets/sponsors/wahaapi.jpg" alt="哇哈AI logo" height="30"><br><strong>哇哈AI</strong></a></td>
<td valign="middle" width="78%">通过单一 OpenAI 兼容 API 接入 Claude、GPT、Gemini 等模型。一把密钥，调用所有模型。仅按实际使用的 token 付费，余额透明，充值即时到账。完全兼容现有 OpenAI 代码，可自由切换。</td>
</tr>
</tbody>
</table>

## 项目概览

TransitHub 是一个自部署的后台运营中心，用于管理多个上游站点和管理员工作区。它专注于中转站的经营分析：上下游成本与营收、用户活跃与留存、销售数据，并提供余额与倍率监控和通知。

项目由 Go 后端和 Vue 3 管理前端组成，使用 PostgreSQL 和 Redis。

## 功能特性

- **管理员工作区管理** - 在多个管理员账号/工作区之间切换，并隔离工作区数据。
- **上游站点管理** - 添加、同步、查看和管理上游站点，并缓存关键指标。
- **仪表盘指标** - 查看实时和历史运营数据，包括余额、成本、趋势、分组用量和上游下钻明细。
- **分组倍率** - 只读的上游分组倍率列表，支持搜索、筛选、涨跌和历史记录。
- **用户活跃分析** - 同步自有 sub2api/new-api 站点的用户与每日用量，找出不活跃和用量下降的用户，做同比环比，并统计充值、消耗和用户余额。
- **群发邮件** - 按选中用户或活跃分群发送邮件，带退订链接和工作区级退订名单。
- **通知渠道** - 配置钉钉、飞书和 Telegram 机器人，用于余额预警和倍率变化通知。
- **系统设置** - 管理刷新间隔、通知策略和运行时展示配置。

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go 1.25, net/http, pgx |
| 前端 | Vue 3.5, Vite, TypeScript, TailwindCSS, vue-i18n |
| 数据库 | PostgreSQL 16+ |
| 缓存 / 会话 | Redis 7+ |
| 部署 | Docker, Docker Compose |

## 部署

### Docker Compose

生产部署文件位于 `deploy/` 目录。

```bash
git clone https://github.com/deviseo/transit-hub.git transit-hub
cd transit-hub

# 先编辑 deploy/docker-compose.prod.yml：
# - 镜像 tag（默认使用 deviseo/transithub:v1.2.0）
# - 替换所有 change-this-* 占位值
# - DATABASE_URL 和 POSTGRES_PASSWORD 中的数据库密码
# - ADMIN_EMAIL / ADMIN_PASSWORD

docker compose -f deploy/docker-compose.prod.yml up -d
```

访问地址：

```text
http://YOUR_SERVER_IP:10621
```

生产 compose 包含：

- `app`：TransitHub 应用容器。
- `postgres`：PostgreSQL 数据库。
- `redis`：用于管理员会话、缓存和定时任务。

默认持久化数据存放在仓库根目录的 `data/`：

```text
data/postgres
data/redis
data/app        # 在线更新的版本、凭据加密的密钥和工单图片
```

`SMTP_ENCRYPTION_KEY` 是可选环境变量，仅当需要在「系统设置 - 邮件设置」中保存 SMTP 密码或发送测试邮件时才需要配置。缺失该变量不会阻止应用启动，也不影响任何非 SMTP 功能。生成方式：

```bash
openssl rand -base64 32
```

该值必须是 base64 编码的 32 字节随机值，且一经设置需要长期稳定保存；更换 key 后，旧的 SMTP 密码密文将无法解密，需要重新填写并保存密码。

### 更新

点击后台顶栏的版本号检查更新。

**在线更新（仅系统管理员）**：有新版本时，在弹窗里点「更新到 vX.Y.Z」。服务器会从本仓库的 GitHub Release 下载对应 CPU 架构的更新包，用 Release 里的 `checksums.txt` 校验 SHA-256，安装到 `/app/data/updates/`，然后自动重启到新版本；新版本起来后页面自动刷新。新版本第一次启动失败时，服务会自动退回原来的版本。

- 请保留 `deploy/docker-compose.prod.yml` 里 `../data/app:/app/data` 这个挂载。不挂载也能在线更新，但重建容器后会回到镜像自带的版本。v1.0.0 之前部署的，请给 `app` 服务补上这条 `volumes`。
- 在线更新需要运行在 Linux（amd64 或 arm64）上；开启了「系统设置」里的出站代理时，下载会走代理。
- 镜像版本更新时以镜像为准：拉取了比在线更新版本更新的镜像后，运行的是镜像里的版本。

**Docker 更新**：把 `deploy/docker-compose.prod.yml` 里的镜像版本改成新版本（用 `latest` 的跳过这一步），然后运行：

```bash
docker compose -f deploy/docker-compose.prod.yml pull
docker compose -f deploy/docker-compose.prod.yml up -d
```

### 凭据加密

保存的登录凭据都是加密存储的：你自己站点的管理员会话、上游站点的登录状态（cookie、token、Admin Key）、通知机器人的密钥，以及出站代理地址。只拿到数据库或备份，看不到这些凭据。

- 密钥在第一次启动时自动生成，保存在 `data/app/credential.key`（也就是 `../data/app:/app/data` 这个挂载里）。想自己管理密钥，可以设置环境变量 `CREDENTIAL_ENCRYPTION_KEY`（base64 编码的 32 字节，`openssl rand -base64 32` 生成），它优先于密钥文件。
- 备份 `data/postgres` 时要连同 `data/app/credential.key` 一起备份。没有密钥，保存的凭据就读不出来，只能重新登录站点、重新填写机器人和代理。
- 数据目录不是持久卷时，TransitHub 继续不加密保存（否则重建容器后密钥丢失），并在版本号弹窗里提示，直到挂载好为止。
- 旧版本保存的凭据会在启动约一分钟后自动改成加密保存。v1.0.2 之前的版本读不懂加密后的凭据，所以不要再降级到 v1.0.2 之前。

### TransitLink 客户端配置

「客户端」页面会在 `/api/client/v1/sites/<代码>` 发布一份只读配置，供 TransitLink 桌面客户端读取。用户的电脑需要能访问到它，所以请把 `/api/client/` 公开出去（例如通过反向代理）；后台和其他 `/api/` 路径可以继续只在内网访问。在该页面把「TransitHub 对外地址」填成公网 URL，再把生成的 `transitlink://connect?config=...` 链接发给用户。

- **推荐配置**：给 Claude Code、Codex、Gemini CLI、Grok Build、OpenCode、DeepSeek Harness 分别设置推荐的分组和模型，用户一键配置时默认用它们，也可以自己改（后四个工具要 TransitLink 0.5.0 或更高版本才会读取）。
- **工单**：在同一页面打开后，用户可以在 TransitLink 里给你提交问题，你在「工单」页面回复。换取工单会话前，TransitHub 会用用户自己的登录去你的站点核实身份，只有你的用户能提交。工单接口也在 `/api/client/` 下。新工单推送给哪些机器人，在「系统设置 → 策略」里选。
- **TransitLink 最低版本**：低于这个版本的客户端会提示用户更新。

### 开发依赖服务

本地开发只启动 PostgreSQL 和 Redis：

```bash
docker compose -f deploy/docker-compose.yml up -d
```

这会在本地开放 `5432` 和 `6379` 端口。

### 构建 Docker 镜像

由于 Dockerfile 放在 `deploy/`，但构建上下文需要使用仓库根目录，请使用：

```bash
docker build -f deploy/Dockerfile -t deviseo/transithub:v1.2.0 .
```

### 发布包

在线更新安装的是 GitHub Release 里附带的更新包，用下面的命令构建：

```bash
scripts/build-release.sh v1.2.0
```

它会在 `dist/release/v1.2.0/` 生成 `transithub_v1.2.0_linux_amd64.tar.gz`、`transithub_v1.2.0_linux_arm64.tar.gz` 和 `checksums.txt`，三个文件都要上传到 Release。版本号必须和 `backend/internal/config/config.go` 里的 `defaultAppVersion` 一致，服务端靠它确认新版本已经启动。

## 本地开发

### 后端

```bash
cd backend
go test ./...
go run ./cmd/api
```

常用环境变量：

```env
PORT=10621
DATABASE_URL=postgres://postgres:postgres@localhost:5432/transithub?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379/0
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=transithub
ALLOW_PUBLIC_REGISTER=true
```

### 前端

```bash
cd frontend
npm install
npm run dev
```

构建检查：

```bash
npm run build
```

## 验证命令

提交变更前建议运行：

```bash
cd backend
go test ./...
go vet ./...
go build ./...

cd ../frontend
npm run build

cd ..
docker compose -f deploy/docker-compose.yml config
docker compose -f deploy/docker-compose.prod.yml config
```

## 项目结构

```text
transit-hub/
├── backend/                  # Go 后端服务
│   ├── cmd/api/              # API 入口
│   ├── internal/config/      # 运行配置
│   ├── internal/database/    # PostgreSQL、Redis、迁移
│   ├── internal/httpserver/  # HTTP 服务组装和中间件
│   └── internal/modules/     # 领域模块
│       ├── admin_accounts/
│       ├── auth/
│       ├── dashboard/
│       ├── group_rates/
│       ├── mass_email/
│       ├── my_sites/
│       ├── settings/
│       ├── system/
│       ├── upstream/
│       └── user_activity/
├── frontend/                 # Vue 3 管理前端
│   └── src/modules/          # 前端业务模块
├── deploy/                   # Dockerfile 和 Compose 文件
├── development-docs/         # 开发说明和实现规划
└── data/                     # 持久化运行数据
```

## 核心工作流

- 工作区隔离的多上游运营：每个 admin workspace 独立管理，可连接多个 sub2api/new-api 上游并执行同步与账号管理。
- 仪表盘：结合自有站点营收与上游成本，展示每日利润、余额和趋势。
- 分组倍率：跟踪最新上游倍率，支持搜索、筛选和涨跌历史。
- 用户活跃分析：找出不活跃和用量下降的用户，并据此定向群发邮件。

## Star History 星际历史

<a href="https://github.com/deviseo/transit-hub/stargazers">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/star-history-dark.svg" />
    <source media="(prefers-color-scheme: light)" srcset="docs/assets/star-history-light.svg" />
    <img alt="TransitHub Star 趋势图" src="docs/assets/star-history-light.svg" />
  </picture>
</a>

## License

本项目采用 GNU Lesser General Public License v3.0（LGPL-3.0-only）协议，详见 [LICENSE](LICENSE)。

---

<div align="center">

**如果 TransitHub 对你的工作有帮助，欢迎点一个 Star。**

</div>
