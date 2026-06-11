#!/usr/bin/env bash
set -euo pipefail

# x86 Linux / 商用节点启动 Edge Client 容器，启用完整 TUN 和 UDP discovery 能力。
#
# 设计来源：
# - 192.168.10.119 实测确认：宿主机有 `/dev/net/tun` 还不够，Edge Client 容器自己也必须挂载该设备；
# - 浏览器环境包里如果 `proxy/clash.yaml` 使用 `tun.enable=true`，Client 会先检查自己容器内是否可见 TUN；
# - Node Server 通过 UDP discovery 自动发现 Client 时，Docker bridge 网络下的 255.255.255.255 广播
#   不一定能穿到宿主机所在局域网；119 实测后统一改为 `--network host`，让 beacon 从宿主机网络栈发出；
# - 因此 Linux 商用节点必须通过 host 网络、NET_ADMIN 和 /dev/net/tun 启动 Client，并把 Docker API
#   配成容器内可访问的 `http://127.0.0.1:2375`。
#
# 可覆盖参数：
#   IMAGE=crpi-.../private_browser_edge_server:0.1.9-amd64 scripts/docker_run_linux_tun.sh
#   DATA_DIR=/Business/data scripts/docker_run_linux_tun.sh
#   ADVERTISE_HOST=192.168.10.119 scripts/docker_run_linux_tun.sh

IMAGE="${IMAGE:-crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge_server:0.1.9-amd64}"
CONTAINER_NAME="${CONTAINER_NAME:-private-browser-edge-server}"
SERVER_PORT="${SERVER_PORT:-3300}"
DATA_DIR="${DATA_DIR:-/Business/data}"
CONFIG_FILE="${DATA_DIR}/config-docker-host.yaml"
ADVERTISE_HOST="${ADVERTISE_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}')}"
ADVERTISE_BASE_URL="${ADVERTISE_BASE_URL:-http://${ADVERTISE_HOST}:${SERVER_PORT}}"

if [[ ! -e /dev/net/tun ]]; then
  cat >&2 <<'ERROR'
host /dev/net/tun not found.
Fix on Linux host first:
  sudo modprobe tun
Then run this script again.
ERROR
  exit 1
fi

if [[ -z "${ADVERTISE_HOST}" ]]; then
  cat >&2 <<'ERROR'
cannot infer ADVERTISE_HOST.
Fix:
  ADVERTISE_HOST=192.168.10.119 scripts/docker_run_linux_tun.sh
ERROR
  exit 1
fi

mkdir -p "$DATA_DIR"

cat > "$CONFIG_FILE" <<EOF
name: private-browser-client
mode: production
version: 0.1.9
server:
  host: 0.0.0.0
  port: ${SERVER_PORT}
  read_timeout_seconds: 15
  write_timeout_seconds: 15
docker:
  api_url: http://127.0.0.1:2375
status_sync:
  enabled: true
  interval_seconds: 5
  watchdog_seconds: 15
  stale_seconds: 30
discovery:
  enabled: true
  broadcast_address: 255.255.255.255
  port: 43000
  interval_seconds: 5
  magic: PRIVATE_BROWSER_CLIENT_DISCOVERY
  protocol_version: 1
  group: default
  advertise_host: "${ADVERTISE_HOST}"
  advertise_base_url: "${ADVERTISE_BASE_URL}"
EOF

docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

docker run -d \
  --name "$CONTAINER_NAME" \
  --label bv.project=private-browser-client \
  --label bv.role=edge-service \
  --restart unless-stopped \
  --network host \
  -v "${DATA_DIR}:/app/data" \
  -v "${CONFIG_FILE}:/app/Settings/config-docker.yaml:ro" \
  --cap-add NET_ADMIN \
  --device /dev/net/tun:/dev/net/tun \
  "$IMAGE"

printf 'started %s on %s using image %s\n' "$CONTAINER_NAME" "$ADVERTISE_BASE_URL" "$IMAGE"
printf 'data dir: %s\n' "$DATA_DIR"
printf 'config file: %s\n' "$CONFIG_FILE"
docker exec "$CONTAINER_NAME" ls -l /dev/net/tun
