#!/usr/bin/env bash

set -euo pipefail

# build-client-image.sh 负责统一收口 Private_Browser_Client 的正式 Docker 构建流程。
#
# 设计来源：
# - 当前用户明确要求把 Client 的 Dockerfile 构建链路标准化，而不是每次手敲一长串 buildx 参数；
# - 项目协作规范已经要求“基础镜像入口 + apt 源 + Go proxy”三层都要能一起控制；
# - 因此这里把平台、镜像名、tag、load/push、国内源覆盖统一收进一个脚本，避免后续 README、
#   本地构建、客户环境构建各写一套命令，最终把正式口径写散。
#
# 职责边界：
# - 这个脚本只负责“构建镜像”；
# - 不负责运行容器，不负责发布，不负责修改配置文件；
# - 默认走 buildx；如果需要单架构本地调试，可用 `--load` 把结果装回本机 Docker。

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKERFILE="${PROJECT_ROOT}/Dockerfile"

IMAGE_NAME="private-browser-client"
IMAGE_TAG="latest"
PLATFORM="linux/amd64"
OUTPUT_MODE="load"

DOCKERHUB_MIRROR="${DOCKERHUB_MIRROR:-docker.m.daocloud.io}"
DEBIAN_MIRROR="${DEBIAN_MIRROR:-mirrors.tuna.tsinghua.edu.cn}"
GOPROXY_VALUE="${GOPROXY:-https://goproxy.cn,direct}"
GOSUMDB_VALUE="${GOSUMDB:-sum.golang.google.cn}"

usage() {
  cat <<'EOF'
Usage:
  ./scripts/build-client-image.sh [options]

Options:
  --platform <value>     Target platform, default: linux/amd64
  --image <value>        Image name, default: private-browser-client
  --tag <value>          Image tag, default: latest
  --push                 Build and push image
  --load                 Build and load image to local Docker, default
  --output <load|push>   Explicit output mode
  --dockerfile <path>    Custom Dockerfile path
  --help                 Show help

Environment overrides:
  DOCKERHUB_MIRROR
  DEBIAN_MIRROR
  GOPROXY
  GOSUMDB

Examples:
  ./scripts/build-client-image.sh
  ./scripts/build-client-image.sh --platform linux/amd64 --image private-browser-client --tag amd64
  ./scripts/build-client-image.sh --platform linux/arm64 --image private-browser-client --tag arm64
  ./scripts/build-client-image.sh --platform linux/amd64 --image repo/private-browser-client --tag 0.2.0 --push
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --platform)
      PLATFORM="${2:?missing value for --platform}"
      shift 2
      ;;
    --image)
      IMAGE_NAME="${2:?missing value for --image}"
      shift 2
      ;;
    --tag)
      IMAGE_TAG="${2:?missing value for --tag}"
      shift 2
      ;;
    --push)
      OUTPUT_MODE="push"
      shift
      ;;
    --load)
      OUTPUT_MODE="load"
      shift
      ;;
    --output)
      OUTPUT_MODE="${2:?missing value for --output}"
      shift 2
      ;;
    --dockerfile)
      DOCKERFILE="${2:?missing value for --dockerfile}"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ "${OUTPUT_MODE}" != "load" && "${OUTPUT_MODE}" != "push" ]]; then
  echo "invalid --output value: ${OUTPUT_MODE}, expected load or push" >&2
  exit 1
fi

if [[ ! -f "${DOCKERFILE}" ]]; then
  echo "dockerfile not found: ${DOCKERFILE}" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker command not found" >&2
  exit 1
fi

if ! docker buildx version >/dev/null 2>&1; then
  echo "docker buildx is required but not available" >&2
  exit 1
fi

FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"

echo "==> project root: ${PROJECT_ROOT}"
echo "==> dockerfile:   ${DOCKERFILE}"
echo "==> image:        ${FULL_IMAGE}"
echo "==> platform:     ${PLATFORM}"
echo "==> output:       ${OUTPUT_MODE}"
echo "==> mirror:       ${DOCKERHUB_MIRROR}"
echo "==> apt mirror:   ${DEBIAN_MIRROR}"
echo "==> goproxy:      ${GOPROXY_VALUE}"
echo "==> gosumdb:      ${GOSUMDB_VALUE}"

BUILD_ARGS=(
  --build-arg "DOCKERHUB_MIRROR=${DOCKERHUB_MIRROR}"
  --build-arg "DEBIAN_MIRROR=${DEBIAN_MIRROR}"
  --build-arg "GOPROXY=${GOPROXY_VALUE}"
  --build-arg "GOSUMDB=${GOSUMDB_VALUE}"
)

OUTPUT_ARGS=(--load)
if [[ "${OUTPUT_MODE}" == "push" ]]; then
  OUTPUT_ARGS=(--push)
fi

docker buildx build \
  --platform "${PLATFORM}" \
  -f "${DOCKERFILE}" \
  -t "${FULL_IMAGE}" \
  "${BUILD_ARGS[@]}" \
  "${OUTPUT_ARGS[@]}" \
  "${PROJECT_ROOT}"
