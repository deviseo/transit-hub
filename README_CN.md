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

TransitHub 是一个自部署的后台运营中心，用于管理多个上游站点和管理员工作区。它关注真实运营工作流：连接上游平台、追踪余额和分组倍率、查看仪表盘指标、配置通知，并运行可定时恢复原倍率的分组活动调价。

项目由 Go 后端和 Vue 3 管理前端组成，使用 PostgreSQL 和 Redis。

## 功能特性

- **管理员工作区管理** - 在多个管理员账号/工作区之间切换，并隔离工作区数据。
- **上游站点管理** - 添加、同步、查看和管理上游站点，并缓存关键指标。
- **仪表盘指标** - 查看实时和历史运营数据，包括余额、成本、趋势、分组用量和上游下钻明细。
- **分组倍率追踪** - 记录分组倍率快照、变动、历史、平台标签和自定义分组类型。
- **活动调价** - 创建立即或定时的调价活动，更新选中的 admin 分组，并在活动结束后恢复原倍率。
- **自动调价支持** - 基于上游倍率变化，为映射分组配置自动调价规则。
- **通知渠道** - 配置钉钉、飞书和 Telegram 机器人，用于余额预警、倍率变化、自动调价和活动通知。
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
# - 镜像 tag（默认使用 deviseo/transithub:v0.1.8）
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
data/ticket-uploads
```

`data/ticket-uploads` 存放工单图片附件（挂载到 `app` 容器的 `TICKET_UPLOAD_DIR`，默认 `/app/data/ticket-uploads`），不会作为公开静态目录对外暴露。重建 `app` 容器前请确认该 volume 已存在，否则图片文件会丢失（数据库里的附件 metadata 不受影响）。

`SMTP_ENCRYPTION_KEY` 是可选环境变量，仅当需要在「系统设置 - 邮件设置」中保存 SMTP 密码或发送测试邮件时才需要配置。缺失该变量不会阻止应用启动，也不影响任何非 SMTP 功能。生成方式：

```bash
openssl rand -base64 32
```

该值必须是 base64 编码的 32 字节随机值，且一经设置需要长期稳定保存；更换 key 后，旧的 SMTP 密码密文将无法解密，需要重新填写并保存密码。

### 开发依赖服务

本地开发只启动 PostgreSQL 和 Redis：

```bash
docker compose -f deploy/docker-compose.yml up -d
```

这会在本地开放 `5432` 和 `6379` 端口。

### 构建 Docker 镜像

由于 Dockerfile 放在 `deploy/`，但构建上下文需要使用仓库根目录，请使用：

```bash
docker build -f deploy/Dockerfile -t deviseo/transithub:v0.1.8 .
```

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
# 仅限本地抽奖联调；线上必须保持 false 或不设置
LOTTERY_ALLOW_PRIVATE_SUB2API_TARGETS=false
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
│       ├── group_rate_campaigns/
│       ├── group_rates/
│       ├── my_sites/
│       ├── settings/
│       ├── system/
│       └── upstream/
├── frontend/                 # Vue 3 管理前端
│   └── src/modules/          # 前端业务模块
├── deploy/                   # Dockerfile 和 Compose 文件
├── development-docs/         # 开发说明和实现规划
└── data/                     # 持久化运行数据
```

## 核心工作流

- 工作区隔离的多上游运营：每个 admin workspace 独立管理，可连接多个 sub2api/new-api 上游并执行同步与账号管理。
- 分组倍率工作流：跟踪最新上游倍率，支持搜索和筛选、分组关联，以及按活动组织调价。
- 已对接分组自动调价：支持手动或同步后执行，可配置策略、查看执行状态并发送通知。
- 日常运维面板：覆盖仪表盘指标、连接健康、工单和邮件/模板管理。

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
