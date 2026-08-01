#!/usr/bin/env bash

set -Eeuo pipefail

PROJECT_DIR='/opt/transithub'
BACKUP_DIR="$PROJECT_DIR/backups"
BACKUP_FILE="$BACKUP_DIR/transithub.dump"
BACKUP_NEXT="$PROJECT_DIR/.transithub.dump.next"
CONTAINER_BACKUP='/tmp/transithub.dump.next'
POSTGRES_CONTAINER='transithub-postgres'
SERVICE='transithub-api.service'
HEALTH_URL='http://127.0.0.1:10621/api/health'

on_error() {
    local exit_code=$?
    printf '升级失败（第 %s 行，退出码 %s）：%s\n' "$1" "$exit_code" "$2" >&2
    if [[ "$2" == *systemctl* || "$2" == *curl* || "$2" == *health_response* ]]; then
        print_service_diagnostics
    fi
    exit "$exit_code"
}

print_service_diagnostics() {
    sudo systemctl status "$SERVICE" --no-pager -l >&2 || true
    sudo journalctl -u "$SERVICE" -n 100 --no-pager >&2 || true
}

prepare_go_environment() {
    export HOME="${HOME:-/root}"
    export GOPATH="${GOPATH:-$HOME/go}"
    export GOMODCACHE="${GOMODCACHE:-$GOPATH/pkg/mod}"
    export GOCACHE="${GOCACHE:-$HOME/.cache/go-build}"
    install -d -m 0755 "$GOPATH" "$GOMODCACHE" "$GOCACHE"
}

wait_for_health() {
    local attempts=60
    local attempt
    local health_response
    for ((attempt = 1; attempt <= attempts; attempt++)); do
        if health_response="$(curl -fsS "$HEALTH_URL" 2>/dev/null)"; then
            if printf '%s' "$health_response" | grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"'; then
                printf '%s\n' "$health_response"
                return 0
            fi
        fi
        sleep 1
    done

    printf '升级失败：健康接口在 %s 秒内未返回 status=ok。\n' "$attempts" >&2
    return 1
}

cleanup() {
    sudo docker exec "$POSTGRES_CONTAINER" rm -f "$CONTAINER_BACKUP" >/dev/null 2>&1 || true
    sudo rm -f "$BACKUP_NEXT" >/dev/null 2>&1 || true
}

on_signal() {
    printf '升级失败：收到 %s 信号，已停止执行。\n' "$1" >&2
    exit "$2"
}

trap 'on_error "$LINENO" "$BASH_COMMAND"' ERR
trap cleanup EXIT
trap 'on_signal INT 130' INT
trap 'on_signal TERM 143' TERM
trap 'on_signal HUP 129' HUP

cd "$PROJECT_DIR"
git fetch origin main
git switch --detach origin/main

sudo mkdir -p "$BACKUP_DIR"
sudo docker exec "$POSTGRES_CONTAINER" rm -f "$CONTAINER_BACKUP"
sudo docker exec "$POSTGRES_CONTAINER" pg_dump \
    --username=postgres \
    --dbname=transithub \
    --format=custom \
    --file="$CONTAINER_BACKUP"
sudo rm -f "$BACKUP_NEXT"
sudo docker cp "$POSTGRES_CONTAINER:$CONTAINER_BACKUP" "$BACKUP_NEXT"
sudo test -s "$BACKUP_NEXT"
sudo mv -f "$BACKUP_NEXT" "$BACKUP_FILE"
sudo find "$BACKUP_DIR" -maxdepth 1 -type f -name '*.dump' ! -name 'transithub.dump' -delete
sudo docker exec "$POSTGRES_CONTAINER" rm -f "$CONTAINER_BACKUP"

cd "$PROJECT_DIR/frontend"
npm ci --registry=https://registry.npmmirror.com
npm run build

cd "$PROJECT_DIR/backend"
prepare_go_environment
GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0 go build \
    -o "$PROJECT_DIR/transithub-api.next" \
    ./cmd/api
test -x "$PROJECT_DIR/transithub-api.next"
mv -f "$PROJECT_DIR/transithub-api.next" "$PROJECT_DIR/transithub-api"

sudo systemctl restart "$SERVICE"
sudo systemctl is-active --quiet "$SERVICE"

health_response="$(wait_for_health)"
printf '%s\n' "$health_response"

sudo journalctl -u "$SERVICE" -n 100 --no-pager
printf '升级成功：源码已更新，服务已重启并通过健康检查。\n'
