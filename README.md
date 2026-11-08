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

TransitHub is a self-hosted admin and operations hub for managing multiple upstream sites and admin workspaces. It focuses on business analysis for relay operators: upstream and downstream cost and revenue, user activity and retention, and sales, together with balance and multiplier monitoring and notifications.

The project is built as a Go backend with a Vue 3 admin frontend, backed by PostgreSQL and Redis.

## Features

- **Admin workspace management** - Switch between admin accounts/workspaces and isolate workspace-scoped data.
- **Upstream site management** - Add, sync, inspect, and manage upstream sites with cached metrics.
- **Dashboard metrics** - View live and historical operation data, including balances, costs, trends, group usage, and upstream drilldowns.
- **Group multiplier tracking** - A read-only list of upstream group multipliers with search, filters, changes, and history.
- **User activity analytics** - Sync your own sub2api/new-api users and daily usage to find inactive or declining users, compare periods, and track top-ups, spend, and user balances.
- **Sales analysis** - Credits by channel, paying and repeat users, conversion, concentration and the top payers, with a period-over-period comparison. Leave out credits that are not sales, one entry at a time or a whole channel such as admin top-ups.
- **Top spenders** - Your biggest users, suggested by 7-day and lifetime spend or added by hand, with their balance and how many days it will last. A bot alerts you when one is about to run out, and you can copy a ready-made reminder to send them.
- **Mass email** - Email selected users or an activity segment, with an unsubscribe link and a per-workspace suppression list.
- **Leads** - Keep email addresses of potential users with their source, add them one at a time or import a batch, and email them through the same queue with unsubscribe links, an ad label in the subject and a 24-hour sending limit.
- **TransitLink client** - Publish a site link for the TransitLink desktop client with your branding, recommended groups and models, notice, and support contacts. Users open the link to connect and set up Claude Code, Codex, Gemini CLI, Grok Build, OpenCode, DeepSeek Harness, Pi, Oh My Pi, Kimi Code, MiniMax Code and OpenClaw in one click.
- **Notification channels** - Configure DingTalk, Feishu, and Telegram bots for balance warnings and multiplier changes.
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
# - image tag (defaults to deviseo/transithub:v1.6.0)
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
data/app        # online updates, the credential encryption key and ticket images
```

SMTP passwords are stored encrypted. By default they use a key derived from the credential encryption key (see Credential encryption below, `data/app/credential.key`), so nothing needs to be configured. To manage the SMTP key separately, set the optional `SMTP_ENCRYPTION_KEY` environment variable: it then takes precedence, and passwords saved before it was set can still be read. Generate a value with:

```bash
openssl rand -base64 32
```

This must be a base64-encoded 32-byte value, and it must be kept stable once set: changing it makes SMTP passwords saved with the old key unreadable, so they have to be entered and saved again. If the data directory isn't a persistent volume and no key is set, SMTP passwords can't be saved.

### Updating

Click the version number in the admin console's top bar to check for updates.

**Online update (system admin only).** When a newer release is available, open the dialog and choose **Update to vX.Y.Z**. The server downloads the package for its CPU from this repository's GitHub Releases, checks its SHA-256 against the release's `checksums.txt`, installs it under `/app/data/updates/`, and restarts itself into the new version. The page reloads by itself once the new version is up. If the new version fails on its first start, the server goes back to the version it was running before.

- Keep the `../data/app:/app/data` volume from `deploy/docker-compose.prod.yml`. Without it an online update still works, but recreating the container brings back the version inside the image. Deployments created before v1.0.0 should add this `volumes` entry to the `app` service.
- Online updates need Linux on amd64 or arm64. Downloads go through the outbound proxy from **System Settings** when one is enabled.
- A newer image always takes precedence: if you pull an image that is newer than the online-installed version, the image's version runs.

**Docker.** Change the image tag in `deploy/docker-compose.prod.yml` (skip this if you use `latest`), then run:

```bash
docker compose -f deploy/docker-compose.prod.yml pull
docker compose -f deploy/docker-compose.prod.yml up -d
```

### Credential Encryption

Saved credentials are encrypted at rest: your site's admin session, upstream site logins (cookies, tokens, Admin Keys), notification bot secrets and the outbound proxy URL. A database dump or backup alone doesn't reveal them.

- The key is generated on first start as `data/app/credential.key`, inside the `../data/app:/app/data` volume. To manage it yourself, set `CREDENTIAL_ENCRYPTION_KEY` to a base64-encoded 32-byte value (`openssl rand -base64 32`); it takes precedence over the file.
- Back up `data/app/credential.key` together with `data/postgres`. Without the key, saved credentials can't be read: you'd have to log in to your sites again and re-enter bot and proxy settings.
- If the data directory isn't a persistent volume, TransitHub keeps storing credentials unencrypted, because a generated key would be lost when the container is recreated. The version dialog shows a warning until it's fixed.
- Credentials saved by earlier versions are encrypted automatically about a minute after startup. Versions before v1.0.2 can't read encrypted credentials, so don't downgrade below v1.0.2.

### TransitLink Client Config

The **Client** page publishes a read-only config at `/api/client/v1/sites/<code>` for the TransitLink desktop client. Your users' computers must be able to reach it, so expose `/api/client/` publicly (for example through your reverse proxy); the admin console and the other `/api/` paths can stay private. Set **Public TransitHub address** on that page to the public URL, then share the generated `transitlink://connect?config=...` link with your users.

- **Recommended setup**: set a recommended group and model for each tool TransitLink can set up: Claude Code, Codex, Gemini CLI, Grok Build, OpenCode, DeepSeek Harness, Pi, Oh My Pi, Kimi Code, MiniMax Code and OpenClaw. One-click setup uses them by default and users can change them. Gemini CLI through DeepSeek Harness need TransitLink 0.5.0 or later; Pi through OpenClaw need 0.6.0 or later.
- **Tickets**: turn them on in the same page and TransitLink users can send you questions, which you answer on the **Tickets** page. Before opening a ticket session, TransitHub checks the user's own login against your site, so only your users can file tickets. The ticket API also lives under `/api/client/`. Choose the bots that hear about new tickets in **System Settings > Strategy**.
- **Minimum TransitLink version**: older clients show a banner asking their users to update.

### Development Services

For local development dependencies only:

```bash
docker compose -f deploy/docker-compose.yml up -d
```

This starts PostgreSQL and Redis on local ports `5432` and `6379`.

### Build Docker Image

Because the Dockerfile is stored in `deploy/` but expects the repository root as build context, build with:

```bash
docker build -f deploy/Dockerfile -t deviseo/transithub:v1.6.0 .
```

### Release Packages

Online updates install the packages attached to a GitHub Release. Build them with:

```bash
scripts/build-release.sh v1.6.0
```

This writes `transithub_v1.6.0_linux_amd64.tar.gz`, `transithub_v1.6.0_linux_arm64.tar.gz` and `checksums.txt` to `dist/release/v1.6.0/`; upload all three to the release. The version must match `defaultAppVersion` in `backend/internal/config/config.go`, because the server uses it to confirm that the new version started.

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
│       ├── group_rates/
│       ├── mass_email/
│       ├── my_sites/
│       ├── settings/
│       ├── system/
│       ├── upstream/
│       └── user_activity/
├── frontend/                 # Vue 3 admin frontend
│   └── src/modules/          # Feature modules
├── deploy/                   # Dockerfile and compose files
├── development-docs/         # Development notes and implementation plans
└── data/                     # Persistent runtime data
```

## Core Workflows

- Workspace-isolated operations keep each admin workspace scoped while connecting multiple sub2api/new-api upstreams for synchronization and account management.
- The dashboard combines revenue from your own site with upstream cost to show daily profit, balances, and trends.
- The group multiplier list tracks current upstream rates with search, filters, and change history.
- User activity analysis finds inactive and declining users and feeds targeted mass email campaigns.

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
