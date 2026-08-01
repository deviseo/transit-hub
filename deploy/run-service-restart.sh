#!/usr/bin/env bash

set -Eeuo pipefail

STATE_DIR='/var/lib/transithub'
STATUS_FILE="$STATE_DIR/restart-status.json"
STATUS_NEXT="$STATE_DIR/restart-status.json.next"
LOG_FILE="$STATE_DIR/restart.log"
LOCK_FILE='/run/lock/transithub-maintenance.lock'
SERVICE='transithub-api.service'
HEALTH_URL='http://127.0.0.1:10621/api/health'
HEALTH_TIMEOUT_SECONDS=60

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

finish_failed() {
    local exit_code="$1"
    local message="$2"

    trap - ERR
    printf '%s\n' "$message" | tee -a "$LOG_FILE" >&2
    write_status 'failed' "$(date -u +'%Y-%m-%dT%H:%M:%S.%NZ')" "$exit_code"
    exit "$exit_code"
}

append_service_diagnostics() {
    printf '\n固定后台服务状态：\n' >>"$LOG_FILE"
    systemctl status "$SERVICE" --no-pager -l >>"$LOG_FILE" 2>&1 || true
    printf '\n固定后台服务最近日志：\n' >>"$LOG_FILE"
    journalctl -u "$SERVICE" -n 50 --no-pager >>"$LOG_FILE" 2>&1 || true
}

on_error() {
    local exit_code=$?
    finish_failed "$exit_code" "后台服务重启失败（第 $1 行）：$2"
}

on_signal() {
    finish_failed "$2" "后台服务重启失败：收到 $1 信号，已停止执行。"
}

install -d -m 0755 "$STATE_DIR"
: >"$LOG_FILE"
chmod 0644 "$LOG_FILE"
started_at="$(date -u +'%Y-%m-%dT%H:%M:%S.%NZ')"
write_status 'running'

trap 'rm -f "$STATUS_NEXT"' EXIT
trap 'on_error "$LINENO" "$BASH_COMMAND"' ERR
trap 'on_signal INT 130' INT
trap 'on_signal TERM 143' TERM
trap 'on_signal HUP 129' HUP

if ! command -v flock >/dev/null 2>&1; then
    finish_failed 127 '后台服务重启失败：服务器缺少 flock 命令。'
fi

if ! exec 9>"$LOCK_FILE"; then
    finish_failed 73 '后台服务重启失败：无法打开维护锁文件。'
fi
if ! flock -n 9; then
    finish_failed 75 '后台服务重启失败：源码升级或其他维护任务正在执行。'
fi

# 给 POST /api/system/restart 的 202 响应留出发送时间。
sleep 1
set +e
systemctl restart "$SERVICE" >>"$LOG_FILE" 2>&1
restart_exit_code=$?
set -e
if (( restart_exit_code != 0 )); then
    append_service_diagnostics
    finish_failed "$restart_exit_code" '后台服务重启失败：systemctl restart 执行失败，实际输出见上方日志。'
fi

set +e
systemctl is-active --quiet "$SERVICE" >>"$LOG_FILE" 2>&1
active_exit_code=$?
set -e
if (( active_exit_code != 0 )); then
    append_service_diagnostics
    finish_failed "$active_exit_code" '后台服务重启失败：服务未进入 active 状态，实际输出见上方日志。'
fi

health_deadline=$((SECONDS + HEALTH_TIMEOUT_SECONDS))
health_response=''
while (( SECONDS < health_deadline )); do
    set +e
    health_response="$(curl -fsS --max-time 2 "$HEALTH_URL" 2>&1)"
    health_exit_code=$?
    set -e
    if (( health_exit_code == 0 )) && \
        [[ "$health_response" =~ \"status\"[[:space:]]*:[[:space:]]*\"ok\" ]]; then
        printf '%s\n' "$health_response" | tee -a "$LOG_FILE"
        write_status 'succeeded' "$(date -u +'%Y-%m-%dT%H:%M:%S.%NZ')" 0
        printf '后台服务重启成功：服务已恢复并通过健康检查。\n' | tee -a "$LOG_FILE"
        exit 0
    fi
    sleep 1
done

health_failure="后台服务重启失败：${HEALTH_TIMEOUT_SECONDS} 秒内健康检查未恢复。"
if [[ -n "$health_response" ]]; then
    health_failure+=$'\n'"最后一次健康检查输出：$health_response"
fi
append_service_diagnostics
finish_failed 1 "$health_failure"
