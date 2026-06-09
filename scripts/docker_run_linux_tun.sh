#!/usr/bin/env bash
set -euo pipefail

# x86 Linux / 商用节点启动 Edge Client 容器，启用完整 TUN 能力。
#
# 设计来源：
# - 192.168.10.119 实测确认：宿主机有 `/dev/net/tun` 还不够，Edge Client 容器自己也必须挂载该设备；
# - 浏览器环境包里如果 `proxy/clash.yaml` 使用 `tun.enable=true`，Client 会先检查自己容器内是否可见 TUN；
# - 因此 Linux 商用节点必须通过 `--cap-add NET_ADMIN --device /dev/net/tun:/dev/net/tun` 启动 Client。
#
# 可覆盖参数：
#   IMAGE=crpi-.../private_browser_edge_server:0.1.8-amd64 scripts/docker_run_linux_tun.sh
#   DATA_DIR=/Business/data scripts/docker_run_linux_tun.sh

IMAGE="${IMAGE:-crpi-6s60spbjvluac8j8.cn-shanghai.personal.cr.aliyuncs.com/ln0216/private_browser_edge_server:0.1.8-amd64}"
CONTAINER_NAME="${CONTAINER_NAME:-private-browser-client}"
HOST_PORT="${HOST_PORT:-3300}"
DATA_DIR="${DATA_DIR:-/Business/data}"

if [[ ! -e /dev/net/tun ]]; then
  cat >&2 <<'ERROR'
host /dev/net/tun not found.
Fix on Linux host first:
  sudo modprobe tun
Then run this script again.
ERROR
  exit 1
fi

mkdir -p "$DATA_DIR"

docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

docker run -d \
  --name "$CONTAINER_NAME" \
  --label bv.project=private-browser-client \
  --label bv.role=edge-service \
  --restart unless-stopped \
  -p "${HOST_PORT}:3300" \
  -v "${DATA_DIR}:/app/data" \
  --add-host=host.docker.internal:host-gateway \
  --cap-add NET_ADMIN \
  --device /dev/net/tun:/dev/net/tun \
  "$IMAGE"

printf 'started %s on http://0.0.0.0:%s using image %s\n' "$CONTAINER_NAME" "$HOST_PORT" "$IMAGE"
printf 'data dir: %s\n' "$DATA_DIR"
docker exec "$CONTAINER_NAME" ls -l /dev/net/tun
