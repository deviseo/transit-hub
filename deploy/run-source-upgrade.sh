#!/usr/bin/env bash

set -Eeuo pipefail

UPGRADE_SCRIPT='/opt/transithub/update-source.sh'
STATE_DIR='/var/lib/transithub'
STATUS_FILE="$STATE_DIR/upgrade-status.json"
STATUS_NEXT="$STATE_DIR/upgrade-status.json.next"
LOG_FILE="$STATE_DIR/upgrade.log"
LOCK_FILE='/run/lock/transithub-maintenance.lock'

started_at=''

write_status() {
    local state="$1"
    local finished_at="${2:-}"
    local exit_code="${3:-}"

    if [[ -n "$finished_at" ]]; then
        printf '{"state":"%s","startedAt":"%s","finishedAt":"%s","exitCode":%s}\n' \
            "$state" "$started_at" "$finished_at" "$exit_code" >"$STATUS_NEXT"
    else
        printf '{"state":"%s","startedAt":"%s"}\n' \
            "$state" "$started_at" >"$STATUS_NEXT"
    fi
    chmod 0644 "$STATUS_NEXT"
    mv -f "$STATUS_NEXT" "$STATUS_FILE"
}

on_signal() {
    local signal="$1"
    local exit_code="$2"
    printf '升级失败：收到 %s 信号，已停止执行。\n' "$signal" | tee -a "$LOG_FILE" >&2
    write_status 'failed' "$(date -u +'%Y-%m-%dT%H:%M:%SZ')" "$exit_code"
    exit "$exit_code"
}

install -d -m 0755 "$STATE_DIR"
: >"$LOG_FILE"
chmod 0644 "$LOG_FILE"
started_at="$(date -u +'%Y-%m-%dT%H:%M:%S.%NZ')"
write_status 'running'

trap 'rm -f "$STATUS_NEXT"' EXIT
trap 'on_signal INT 130' INT
trap 'on_signal TERM 143' TERM
trap 'on_signal HUP 129' HUP

if ! command -v flock >/dev/null 2>&1; then
    printf '升级失败：服务器缺少 flock 命令。\n' | tee -a "$LOG_FILE" >&2
    write_status 'failed' "$(date -u +'%Y-%m-%dT%H:%M:%S.%NZ')" 127
    exit 127
fi

if ! exec 9>"$LOCK_FILE"; then
    printf '升级失败：无法打开维护锁文件。\n' | tee -a "$LOG_FILE" >&2
    write_status 'failed' "$(date -u +'%Y-%m-%dT%H:%M:%S.%NZ')" 73
    exit 73
fi
if ! flock -n 9; then
    printf '升级失败：后台服务重启或其他维护任务正在执行。\n' | tee -a "$LOG_FILE" >&2
    write_status 'failed' "$(date -u +'%Y-%m-%dT%H:%M:%S.%NZ')" 75
    exit 75
fi

set +e
"$UPGRADE_SCRIPT" 2>&1 | tee -a "$LOG_FILE"
pipeline_status=("${PIPESTATUS[@]}")
set -e

exit_code="${pipeline_status[0]}"
if [[ "$exit_code" -eq 0 && "${pipeline_status[1]}" -ne 0 ]]; then
    exit_code="${pipeline_status[1]}"
fi

finished_at="$(date -u +'%Y-%m-%dT%H:%M:%S.%NZ')"
if [[ "$exit_code" -eq 0 ]]; then
    write_status 'succeeded' "$finished_at" 0
    exit 0
fi

write_status 'failed' "$finished_at" "$exit_code"
exit "$exit_code"
