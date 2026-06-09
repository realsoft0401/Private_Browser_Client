#!/usr/bin/env bash
set -euo pipefail

# Mac / Docker Desktop 本地测试配置生成脚本。
#
# 设计来源：
# - Mac 本地测试主要验证浏览器界面、mixed-port 代理链路、WebVNC/CDP；
# - Docker Desktop 场景通常不能把宿主 `/dev/net/tun` 稳定传给浏览器容器；
# - 因此本脚本固定生成 `tun.enable=false` 的测试配置，避免本地 run 因 TUN 能力缺失失败。
#
# 使用方式：
#   scripts/clash_tun_false_for_mac.sh /Users/lining/Documents/analysis_ins/proxy/ClashVerge_1.yaml .tmp/ClashVerge_1.mac.yaml

if [[ $# -ne 2 ]]; then
  printf 'Usage: %s <input.yaml> <output.yaml>\n' "$0" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
"${script_dir}/clash_set_tun.sh" "$1" "$2" false
