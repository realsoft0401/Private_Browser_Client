#!/usr/bin/env bash
set -euo pipefail

# Mac / Docker Desktop 启动 Edge Client 容器。
#
# 设计来源：
# - Mac 本地主要用于编译后界面和接口 smoke 测试；
# - Docker Desktop 通常不能稳定把宿主 `/dev/net/tun` 暴露给 Edge Client 容器；
# - 因此这份启动脚本不挂 TUN、不加 NET_ADMIN。需要测试代理时，应配合 tun.enable=false 的 Clash 配置。
#
# 可覆盖参数：
#   IMAGE=private-browser-client:local scripts/docker_run_mac.sh
#   DATA_DIR=/Users/lining/Documents/Browser_virtualization/Private_Browser_Client/data scripts/docker_run_mac.sh

IMAGE="${IMAGE:-private-browser-client:local}"
CONTAINER_NAME="${CONTAINER_NAME:-private-browser-client}"
HOST_PORT="${HOST_PORT:-3300}"
DATA_DIR="${DATA_DIR:-$(pwd)/data}"

mkdir -p "$DATA_DIR"

docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true

docker run -d \
  --name "$CONTAINER_NAME" \
  --label bv.project=private-browser-client \
  --label bv.role=edge-service \
  --restart unless-stopped \
  -p "${HOST_PORT}:3300" \
  -v "${DATA_DIR}:/app/data" \
  "$IMAGE"

printf 'started %s on http://127.0.0.1:%s using image %s\n' "$CONTAINER_NAME" "$HOST_PORT" "$IMAGE"
printf 'data dir: %s\n' "$DATA_DIR"
