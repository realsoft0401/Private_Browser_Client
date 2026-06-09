#!/usr/bin/env bash
set -euo pipefail

# Private_Browser_Client 全接口冒烟测试脚本。
#
# 设计来源：
# - 2026-06-09 阶段 4 要求 Server 对接前先把 Client 全接口和异常路径测试矩阵固定下来；
# - 环境包生命周期动作会改变 Docker、SQLite、备份包和 browser-data/profile，因此默认只执行只读接口；
# - 需要执行 run/stop/backup/restore/revalidate/delete/import-package 时，必须显式打开对应环境变量。
#
# 使用示例：
#   EDGE_BASE_URL=http://192.168.10.119:3300 ./scripts/edge_api_smoke.sh
#   EDGE_BASE_URL=http://192.168.10.119:3300 ENV_ID=xxx RUN_MUTATING=1 ./scripts/edge_api_smoke.sh
#
# 维护边界：
# - 该脚本不替代 Go 单元测试，也不直接读取 SQLite 或环境包目录；
# - 所有事实都从 HTTP API 获取，保持和未来 Server 调用方式一致；
# - 默认不上传包、不删除环境包、不启动容器，避免测试误伤商业资产。

EDGE_BASE_URL="${EDGE_BASE_URL:-http://127.0.0.1:3300}"
ENV_ID="${ENV_ID:-}"
IMPORT_PACKAGE="${IMPORT_PACKAGE:-}"
RUN_MUTATING="${RUN_MUTATING:-0}"
RUN_IMPORT="${RUN_IMPORT:-0}"
RUN_DELETE="${RUN_DELETE:-0}"

pass_count=0
fail_count=0

log() {
  printf '\n[%s] %s\n' "$(date '+%H:%M:%S')" "$*"
}

request() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local url="${EDGE_BASE_URL}${path}"
  local response

  if [[ -n "$body" ]]; then
    response="$(curl -fsS -X "$method" "$url" -H 'accept: application/json' -H 'Content-Type: application/json' --data "$body")"
  else
    response="$(curl -fsS -X "$method" "$url" -H 'accept: application/json')"
  fi
  printf '%s' "$response"
}

check_json_code() {
  local name="$1"
  local response="$2"
  local expected="${3:-1000}"
  local code
  code="$(printf '%s' "$response" | jq -r '.code // empty')"
  if [[ "$code" == "$expected" ]]; then
    printf 'PASS %-34s code=%s\n' "$name" "$code"
    pass_count=$((pass_count + 1))
  else
    printf 'FAIL %-34s expected=%s actual=%s response=%s\n' "$name" "$expected" "$code" "$response"
    fail_count=$((fail_count + 1))
  fi
}

check_http_json() {
  local name="$1"
  local method="$2"
  local path="$3"
  local body="${4:-}"
  local response
  if response="$(request "$method" "$path" "$body")"; then
    check_json_code "$name" "$response" "1000"
  else
    printf 'FAIL %-34s curl failed\n' "$name"
    fail_count=$((fail_count + 1))
  fi
}

check_health() {
  local response
  if response="$(curl -fsS "${EDGE_BASE_URL}/health")"; then
    if [[ -n "$response" ]]; then
      printf 'PASS %-34s reachable\n' "health"
      pass_count=$((pass_count + 1))
    else
      printf 'FAIL %-34s empty response\n' "health"
      fail_count=$((fail_count + 1))
    fi
  else
    printf 'FAIL %-34s curl failed\n' "health"
    fail_count=$((fail_count + 1))
  fi
}

require_jq() {
  if ! command -v jq >/dev/null 2>&1; then
    printf 'jq is required. Install jq before running this smoke test.\n' >&2
    exit 2
  fi
}

wait_task() {
  local task_id="$1"
  local name="$2"
  local attempt
  for attempt in $(seq 1 120); do
    local response
    response="$(request GET "/api/v1/edge/tasks/${task_id}")"
    local status
    status="$(printf '%s' "$response" | jq -r '.data.status // empty')"
    case "$status" in
      success)
        printf 'PASS %-34s task=%s\n' "$name" "$task_id"
        pass_count=$((pass_count + 1))
        return 0
        ;;
      failed)
        printf 'FAIL %-34s task=%s response=%s\n' "$name" "$task_id" "$response"
        fail_count=$((fail_count + 1))
        return 1
        ;;
    esac
    sleep 1
  done
  printf 'FAIL %-34s task timeout task=%s\n' "$name" "$task_id"
  fail_count=$((fail_count + 1))
  return 1
}

run_task_action() {
  local name="$1"
  local method="$2"
  local path="$3"
  local body="${4:-}"
  local response
  if ! response="$(request "$method" "$path" "$body")"; then
    printf 'FAIL %-34s curl failed\n' "$name"
    fail_count=$((fail_count + 1))
    return 1
  fi
  check_json_code "${name} create-task" "$response" "1000"
  local task_id
  task_id="$(printf '%s' "$response" | jq -r '.data.taskId // empty')"
  if [[ -z "$task_id" ]]; then
    printf 'FAIL %-34s no taskId response=%s\n' "$name" "$response"
    fail_count=$((fail_count + 1))
    return 1
  fi
  wait_task "$task_id" "$name"
}

main() {
  require_jq

  log "Read-only Edge checks on ${EDGE_BASE_URL}"
  check_health
  check_http_json "device-info" GET "/api/v1/edge/device-info"
  check_http_json "docker-status" GET "/api/v1/edge/docker/status"
  check_http_json "docker-images" GET "/api/v1/edge/docker/images"
  check_http_json "docker-containers" GET "/api/v1/edge/docker/containers"
  check_http_json "browser-envs-list" GET "/api/v1/edge/browser-envs"
  check_http_json "rebuild-candidates" GET "/api/v1/edge/browser-envs-rebuild/candidates"

  if [[ -n "$ENV_ID" ]]; then
    log "Read-only env checks for ${ENV_ID}"
    check_http_json "browser-env-detail" GET "/api/v1/edge/browser-envs/${ENV_ID}"
    check_http_json "browser-env-cdp-test" GET "/api/v1/edge/browser-envs/${ENV_ID}/cdp-test"
    check_http_json "browser-env-vnc-info" GET "/api/v1/edge/browser-envs/${ENV_ID}/vnc-info"
  fi

  if [[ "$RUN_MUTATING" == "1" && -n "$ENV_ID" ]]; then
    log "Mutating lifecycle checks for ${ENV_ID}"
    run_task_action "run" POST "/api/v1/edge/browser-envs/${ENV_ID}/run" '{}'
    run_task_action "stop" POST "/api/v1/edge/browser-envs/${ENV_ID}/stop" '{}'
    run_task_action "revalidate" POST "/api/v1/edge/browser-envs/${ENV_ID}/revalidate" ''
    check_http_json "backup" POST "/api/v1/edge/browser-envs/${ENV_ID}/backup"
    check_http_json "restore" POST "/api/v1/edge/browser-envs/${ENV_ID}/restore"
  else
    log "Skipping mutating lifecycle checks. Set RUN_MUTATING=1 and ENV_ID=... to enable."
  fi

  if [[ "$RUN_IMPORT" == "1" && -n "$IMPORT_PACKAGE" ]]; then
    log "Import package check ${IMPORT_PACKAGE}"
    local response
    if response="$(curl -fsS -X POST "${EDGE_BASE_URL}/api/v1/edge/browser-envs/import-package" -H 'accept: application/json' -F "file=@${IMPORT_PACKAGE};type=application/x-gzip")"; then
      check_json_code "import-package" "$response" "1000"
    else
      printf 'FAIL %-34s curl failed\n' "import-package"
      fail_count=$((fail_count + 1))
    fi
  else
    log "Skipping import-package. Set RUN_IMPORT=1 and IMPORT_PACKAGE=/path/file.tar.gz to enable."
  fi

  if [[ "$RUN_DELETE" == "1" && -n "$ENV_ID" ]]; then
    log "Delete check for ${ENV_ID}"
    run_task_action "delete" DELETE "/api/v1/edge/browser-envs/${ENV_ID}" ''
  else
    log "Skipping delete. Set RUN_DELETE=1 and ENV_ID=... to enable."
  fi

  log "Result: pass=${pass_count} fail=${fail_count}"
  if [[ "$fail_count" -gt 0 ]]; then
    exit 1
  fi
}

main "$@"
