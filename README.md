# TransitHub

<div align="center">

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8.svg)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3.5+-4FC08D.svg)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-336791.svg)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D.svg)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg)](https://www.docker.com/)

**A multi-upstream operations hub for self-hosted API services built with sub2api or new-api.**

English | [中文](README_CN.md)

</div>

## Important Notice

Please read the following carefully before using this project:

- **Provider policy risk**: TransitHub helps administrators connect to and operate upstream admin platforms. Make sure your use complies with the terms of service of every upstream platform you connect.
- **Compliant use only**: Use this project only in compliance with the laws and regulations of your country or region. Do not use it to bypass authorization, abuse upstream services, or operate accounts you do not control.
- **Self-hosting responsibility**: You are responsible for protecting admin credentials, database backups, network access, and deployment secrets in your own environment.
- **Disclaimer**: This project is for technical learning only. You are responsible for complying with applicable laws and upstream platform policies. The authors assume no liability for service interruptions, account restrictions, data loss, or any direct or indirect damages caused by using this project.

## Sponsors

<table>
<thead>
<tr>
<th align="center" valign="middle" width="130">Name</th>
<th align="left" valign="middle" width="78%">Description</th>
</tr>
</thead>
<tbody>
<tr>
<td align="center" valign="middle" width="130"><a href="https://www.recycleai.vip/"><img src="docs/assets/sponsors/recycleai-logo.png" alt="RecycleAI logo" height="28"><br><strong>RecycleAI</strong></a></td>
<td valign="middle" width="78%">An innovation platform for AI resource reuse and intelligent service circulation, improving coordination across compute, model, and application capabilities.</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://www.xiongxiongai.online"><img src="docs/assets/sponsors/xiongxiongai-logo.png" alt="XiongXiongAI logo" height="30"><br><strong>XiongXiongAI</strong></a></td>
<td valign="middle" width="78%">An AI service brand centered on approachable experiences, stable access, and lightweight intelligent tools for reliable everyday usage.</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://console.qqqrouter.ai"><img src="docs/assets/sponsors/qqqrouter-logo.png" alt="qqqRouter logo" height="24"><br><strong>qqqRouter</strong></a></td>
<td valign="middle" width="78%">An AI infrastructure platform for multi-model access, request routing, and usage governance, helping teams build more flexible and controllable model invocation workflows.</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://sparkcode.top"><img src="docs/assets/sponsors/sparkcode-logo.png" alt="SparkCode logo" height="30"><br><strong>SparkCode</strong></a></td>
<td valign="middle" width="78%">A stable and efficient API relay provider for mainstream AI coding models including Claude Code, Codex, and Gemini with NanoBanana-series models; supports monthly plans, usage-based billing, high-concurrency calls, invoiced top-ups, dedicated enterprise onboarding, technical support, and a long-term invitation rebate program.</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://uuapi.net"><img src="docs/assets/sponsors/uuapi.svg" alt="UU API logo" height="30"><br><strong>UU API</strong></a></td>
<td valign="middle" width="78%">An AI compute gateway for global developers and enterprises, offering one-stop access to ChatGPT, Claude, Gemini, and other major models through transparent official or first-hand channels, with reliable infrastructure built to make every call worthwhile.</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://hk.getelucid.com/"><img src="docs/assets/sponsors/ElucidRelay.png" alt="ElucidRelay logo" height="30"><br><strong>ElucidRelay</strong></a></td>
<td valign="middle" width="78%">An API relay providing stable, high-throughput access to major overseas models including OpenAI, Claude, Gemini, and more through a single OpenAI-compatible endpoint; built for resellers and downstream platforms that need reliable volume and competitive pricing.</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://songsongai.com/"><img src="docs/assets/sponsors/songsongai.png" alt="songsongAi logo" height="30"><br><strong>songsongAi</strong></a></td>
<td valign="middle" width="78%">An AI service brand for polished intelligent application experiences, focused on premium model access, reliable delivery, and streamlined workflows for creators and teams.</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://web.ymocode.com"><img src="docs/assets/sponsors/yimous.png" alt="Yimo-US logo" height="22"><br><strong>Yimo-US</strong></a></td>
<td valign="middle" width="78%">An API service brand focused on stable access and multi-upstream aggregation, designed for high-availability access, resource integration, and consistent service experiences.</td>
</tr>
<tr>
<td align="center" valign="middle" width="130"><a href="https://wahaapi.top/"><img src="docs/assets/sponsors/wahaapi.jpg" alt="Waha AI logo" height="30"><br><strong>Waha AI</strong></a></td>
<td valign="middle" width="78%">Access Claude, GPT, Gemini and more through a single OpenAI-compatible API. One key, all models. Pay only for the tokens you use, with transparent balances and instant top-ups. Fully compatible with your existing OpenAI code — switch freely.</td>
</tr>
</tbody>
</table>

## Overview

TransitHub is a self-hosted admin and operations hub for managing multiple upstream sites and admin workspaces. It focuses on practical operator workflows: connecting upstream platforms, tracking balances and group multipliers, monitoring dashboard metrics, configuring notifications, and running scheduled group-rate campaigns that can automatically restore original multipliers after an activity ends.

The project is built as a Go backend with a Vue 3 admin frontend, backed by PostgreSQL and Redis.

## Features

- **Admin workspace management** - Switch between admin accounts/workspaces and isolate workspace-scoped data.
- **Upstream site management** - Add, sync, inspect, and manage upstream sites with cached metrics.
- **Dashboard metrics** - View live and historical operation data, including balances, costs, trends, group usage, and upstream drilldowns.
- **Group multiplier tracking** - Track group multiplier snapshots, changes, history, platform tags, and custom group types.
- **Group-rate campaigns** - Create immediate or scheduled pricing activities, update selected admin groups, retain activity records, and restore original multipliers at the configured end time.
- **Auto pricing support** - Configure mapped group auto-pricing rules based on upstream multiplier changes.
- **Notification channels** - Configure DingTalk, Feishu, and Telegram bots for balance warnings, multiplier changes, auto-pricing, and campaign notifications.
- **System settings** - Manage refresh intervals, notification strategy, and runtime display settings.

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go 1.25, net/http, pgx |
| Frontend | Vue 3.5, Vite, TypeScript, TailwindCSS, vue-i18n |
| Database | PostgreSQL 16+ |
| Cache / Session | Redis 7+ |
| Deployment | Docker, Docker Compose |

## Deployment

### Docker Compose

Production compose files live under `deploy/`.

```bash
git clone https://github.com/deviseo/transit-hub.git transit-hub
cd transit-hub

# Edit deploy/docker-compose.prod.yml first:
# - image tag (defaults to deviseo/transithub:v0.1.8)
# - replace every change-this-* placeholder
# - database password in both DATABASE_URL and POSTGRES_PASSWORD
# - ADMIN_EMAIL / ADMIN_PASSWORD

docker compose -f deploy/docker-compose.prod.yml up -d
```

Open:

```text
http://YOUR_SERVER_IP:10621
```

The production compose file includes:

- `app`: TransitHub application container.
- `postgres`: PostgreSQL database.
- `redis`: Redis for admin sessions, cache, and scheduled tasks.

Persistent data is stored under the repository root `data/` directory by default:

```text
data/postgres
data/redis
data/ticket-uploads
```

`data/ticket-uploads` holds uploaded ticket images (mounted into the `app` container at `TICKET_UPLOAD_DIR`, default `/app/data/ticket-uploads`). It is not served as a public static directory; make sure this volume is present before recreating the `app` container, otherwise uploaded images will be lost even though their metadata remains in the database.

`SMTP_ENCRYPTION_KEY` is an optional environment variable, only required if you want to save an SMTP password or send test emails from **System Settings > Email Settings**. Its absence does not prevent the application from starting and does not affect any non-SMTP feature. Generate a value with:

```bash
openssl rand -base64 32
```

This must be a base64-encoded 32-byte value, and it must be kept stable long-term once set. Rotating the key makes any previously saved SMTP password ciphertext undecryptable, requiring the password to be re-entered and saved.

### Development Services

For local development dependencies only:

```bash
docker compose -f deploy/docker-compose.yml up -d
```

This starts PostgreSQL and Redis on local ports `5432` and `6379`.

### Build Docker Image

Because the Dockerfile is stored in `deploy/` but expects the repository root as build context, build with:

```bash
docker build -f deploy/Dockerfile -t deviseo/transithub:v0.1.8 .
```

## Local Development

### Backend

```bash
cd backend
go test ./...
go run ./cmd/api
```

Important environment variables:

```env
PORT=10621
DATABASE_URL=postgres://postgres:postgres@localhost:5432/transithub?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379/0
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=transithub
ALLOW_PUBLIC_REGISTER=true
# Local lottery debugging only; keep false or unset in production
LOTTERY_ALLOW_PRIVATE_SUB2API_TARGETS=false
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

Build check:

```bash
npm run build
```

## Verification

Recommended checks before submitting changes:

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

## Project Structure

```text
transit-hub/
├── backend/                  # Go backend service
│   ├── cmd/api/              # API entrypoint
│   ├── internal/config/      # Runtime configuration
│   ├── internal/database/    # PostgreSQL, Redis, migrations
│   ├── internal/httpserver/  # HTTP server assembly and middleware
│   └── internal/modules/     # Domain modules
│       ├── admin_accounts/
│       ├── auth/
│       ├── dashboard/
│       ├── group_rate_campaigns/
│       ├── group_rates/
│       ├── my_sites/
│       ├── settings/
│       ├── system/
│       └── upstream/
├── frontend/                 # Vue 3 admin frontend
│   └── src/modules/          # Feature modules
├── deploy/                   # Dockerfile and compose files
├── development-docs/         # Development notes and implementation plans
└── data/                     # Persistent runtime data
```

## Core Workflows

- Workspace-isolated operations keep each admin workspace scoped while connecting multiple sub2api/new-api upstreams for synchronization and account management.
- Group multiplier workflows track current upstream rates, support search and filtering, map own groups to upstream groups, and organize campaign-based adjustments.
- Mapped-group automatic pricing can run manually or after sync with configurable strategies, execution status, and notifications.
- Operational panels cover dashboard metrics, connection health, tickets, and email/template administration for day-to-day support.

## Star History

<a href="https://github.com/deviseo/transit-hub/stargazers">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/star-history-dark.svg" />
    <source media="(prefers-color-scheme: light)" srcset="docs/assets/star-history-light.svg" />
    <img alt="TransitHub star history chart" src="docs/assets/star-history-light.svg" />
  </picture>
</a>

## License

This project is licensed under the GNU Lesser General Public License v3.0 (LGPL-3.0-only). See [LICENSE](LICENSE) for details.

---

<div align="center">

**If TransitHub helps your workflow, consider giving it a star.**

</div>
