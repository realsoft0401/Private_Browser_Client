# 第一阶段负责编译 Go 服务。
#
# 设计来源：
# - 当前新 Client 仍然依赖 `github.com/mattn/go-sqlite3`，构建阶段必须开启 CGO；
# - 为了避免 Alpine/musl 和 glibc 动态库差异，这里继续采用 Debian 系多阶段镜像；
# - `Settings`、`docs`、`public` 都属于运行时事实文件，必须随镜像一起复制，不能只留二进制；
# - 根据当前仓库协作规范，正式构建链路不仅要切 apt / GOPROXY，还要把最前面的 `FROM`
#   一并纳入国内可访问镜像前缀，避免 build 卡在 Docker Hub 元数据获取阶段。
#
# 职责边界：
# - `DOCKERHUB_MIRROR` 只负责基础镜像入口；
# - `DEBIAN_MIRROR` 只负责容器内 apt 源；
# - `GOPROXY/GOSUMDB` 只负责 Go 依赖下载；
# - 如果后续海外 CI 或客户环境直连官方源更稳定，可通过 `--build-arg` 显式覆盖。
ARG DOCKERHUB_MIRROR=docker.m.daocloud.io
FROM ${DOCKERHUB_MIRROR}/library/golang:1.22-bookworm AS builder

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

ARG GOSUMDB=sum.golang.google.cn
ENV GOSUMDB=${GOSUMDB}

ARG DEBIAN_MIRROR=mirrors.tuna.tsinghua.edu.cn
RUN if [ "${DEBIAN_MIRROR}" = "deb.debian.org" ]; then DEBIAN_SCHEME="http"; else DEBIAN_SCHEME="https"; fi \
  && sed -i "s|http://deb.debian.org/debian|${DEBIAN_SCHEME}://${DEBIAN_MIRROR}/debian|g" /etc/apt/sources.list.d/debian.sources \
  && sed -i "s|http://deb.debian.org/debian-security|${DEBIAN_SCHEME}://${DEBIAN_MIRROR}/debian-security|g" /etc/apt/sources.list.d/debian.sources \
  && apt-get update \
  && apt-get install -y --no-install-recommends gcc libc6-dev \
  && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 这里固定编译 Linux 目标，具体 amd64/arm64 由 buildx 在外层通过 TARGETOS/TARGETARCH 注入。
#
# 这样做的原因是：
# - 同一份 Dockerfile 需要同时服务本地 amd64 和后续 arm64；
# - CGO 构建必须跟随目标平台，而不能把 GOARCH 写死在 Dockerfile 里；
# - 真正的“打哪种架构镜像”应该由外层构建脚本统一控制。
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=1 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
  -o /out/private_browser_client .

FROM ${DOCKERHUB_MIRROR}/library/debian:bookworm-slim AS runtime

WORKDIR /app

ARG DEBIAN_MIRROR=mirrors.tuna.tsinghua.edu.cn
RUN if [ "${DEBIAN_MIRROR}" = "deb.debian.org" ]; then DEBIAN_SCHEME="http"; else DEBIAN_SCHEME="https"; fi \
  && sed -i "s|http://deb.debian.org/debian|${DEBIAN_SCHEME}://${DEBIAN_MIRROR}/debian|g" /etc/apt/sources.list.d/debian.sources \
  && sed -i "s|http://deb.debian.org/debian-security|${DEBIAN_SCHEME}://${DEBIAN_MIRROR}/debian-security|g" /etc/apt/sources.list.d/debian.sources \
  && apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tzdata lsof sqlite3 \
  && rm -rf /var/lib/apt/lists/*

ENV ENV=docker

COPY --from=builder /out/private_browser_client /app/private_browser_client
COPY Settings /app/Settings
COPY docs /app/docs
COPY public /app/public

RUN mkdir -p /app/data

EXPOSE 3300

CMD ["/app/private_browser_client"]
