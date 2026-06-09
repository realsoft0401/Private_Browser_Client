#!/usr/bin/env bash
set -euo pipefail

# Clash TUN 配置生成脚本。
#
# 设计来源：
# - 2026-06-09 远端 119 测试确认：同一份 Clash Verge 配置在 Mac Docker Desktop 和 x86 Linux 节点上的
#   TUN 能力不一样，不能再靠人工手改 `tun.enable`；
# - Mac 本地通常没有可直接传给浏览器容器的 `/dev/net/tun`，本地界面 smoke 应使用 `tun.enable=false`；
# - x86 Linux 商用节点如果要完整 TUN/DNS 保护，必须使用 `tun.enable=true`，并让 Edge Client 容器具备
#   `--cap-add NET_ADMIN --device /dev/net/tun:/dev/net/tun`。
#
# 职责边界：
# - 只读取输入 YAML，生成一份新的输出 YAML；
# - 不修改原始代理文件，不校验代理账号有效性，不启动 Docker；
# - 只处理顶层 `tun:` 段里的 `enable:` 字段，避免误伤 rules/proxy-groups/sniffer 等其它位置的 enable。

usage() {
  cat >&2 <<'USAGE'
Usage:
  scripts/clash_set_tun.sh <input.yaml> <output.yaml> <true|false>

Examples:
  scripts/clash_set_tun.sh /path/ClashVerge_1.yaml .tmp/ClashVerge_1.mac.yaml false
  scripts/clash_set_tun.sh /path/ClashVerge_1.yaml .tmp/ClashVerge_1.linux.yaml true
USAGE
}

if [[ $# -ne 3 ]]; then
  usage
  exit 2
fi

input_file="$1"
output_file="$2"
tun_value="$3"

case "$tun_value" in
  true|false) ;;
  *)
    printf 'tun value must be true or false, got: %s\n' "$tun_value" >&2
    exit 2
    ;;
esac

if [[ ! -f "$input_file" ]]; then
  printf 'input yaml not found: %s\n' "$input_file" >&2
  exit 1
fi

mkdir -p "$(dirname "$output_file")"

tmp_file="${output_file}.tmp.$$"
cleanup() {
  rm -f "$tmp_file"
}
trap cleanup EXIT

awk -v desired="$tun_value" '
BEGIN {
  in_tun = 0
  seen_tun = 0
  changed = 0
}

function print_tun_block_if_missing() {
  if (seen_tun == 0) {
    print ""
    print "tun:"
    print "  enable: " desired
    changed = 1
  }
}

/^[[:space:]]*#/ {
  print
  next
}

/^tun:[[:space:]]*$/ {
  in_tun = 1
  seen_tun = 1
  print
  next
}

in_tun == 1 && /^[^[:space:]][^:]*:/ {
  if (changed == 0) {
    print "  enable: " desired
    changed = 1
  }
  in_tun = 0
  print
  next
}

in_tun == 1 && /^[[:space:]]+enable:[[:space:]]*(true|false)[[:space:]]*$/ {
  sub(/enable:[[:space:]]*(true|false)/, "enable: " desired)
  changed = 1
  print
  next
}

{
  print
}

END {
  if (in_tun == 1 && changed == 0) {
    print "  enable: " desired
    changed = 1
  }
  print_tun_block_if_missing()
}
' "$input_file" > "$tmp_file"

mv "$tmp_file" "$output_file"
trap - EXIT

printf 'generated %s with tun.enable=%s\n' "$output_file" "$tun_value"
