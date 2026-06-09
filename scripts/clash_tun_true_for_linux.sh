#!/usr/bin/env bash
set -euo pipefail

# x86 Linux 商用节点 TUN 配置生成脚本。
#
# 设计来源：
# - 119 节点测试确认：原始 `tun.enable=true` 配置需要 Edge Client 容器本身也能看到 `/dev/net/tun`；
# - Linux 节点完整 TUN/DNS 保护必须配套 Docker 参数：
#   `--cap-add NET_ADMIN --device /dev/net/tun:/dev/net/tun`；
# - 本脚本固定生成 `tun.enable=true` 的代理配置，并在当前宿主机可见时顺手检查 `/dev/net/tun`。
#
# 使用方式：
#   scripts/clash_tun_true_for_linux.sh /path/ClashVerge_1.yaml .tmp/ClashVerge_1.linux.yaml

if [[ $# -ne 2 ]]; then
  printf 'Usage: %s <input.yaml> <output.yaml>\n' "$0" >&2
  exit 2
fi

if [[ ! -e /dev/net/tun ]]; then
  cat >&2 <<'WARN'
warning: current host cannot see /dev/net/tun.
Linux node should run:
  sudo modprobe tun
  docker run ... --cap-add NET_ADMIN --device /dev/net/tun:/dev/net/tun ...
WARN
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"${script_dir}/clash_set_tun.sh" "$1" "$2" true
