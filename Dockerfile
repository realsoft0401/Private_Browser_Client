# 第一阶段负责编译 Go 服务。
#
# 设计来源：
# - 当前项目使用 go-sqlite3，必须启用 CGO；
# - 因此构建镜像需要 gcc，运行镜像保留在 Debian slim，避免 Alpine/musl 和 glibc 动态库不匹配；
# - docs、Settings、public 作为运行时事实文件复制到最终镜像，保证 Swagger 入口和配置能随镜像一起部署。
FROM golang:1.25-bookworm AS builder

WORKDIR /src

# 容器构建时需要下载 Go module。
#
# 国内网络直接访问 proxy.golang.org 容易 TLS 超时，所以默认走 goproxy.cn；
# 后续如果部署在海外 CI，可以用 `--build-arg GOPROXY=https://proxy.golang.org,direct` 覆盖。
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

RUN apt-get update \
  && apt-get install -y --no-install-recommends gcc libc6-dev \
  && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build \
  -ldflags="-s -w -X private_browser_client/Settings.BuildEnv=docker" \
  -o /out/private_browser_client .

# 第二阶段只保留运行服务需要的文件。
FROM debian:bookworm-slim AS runtime

WORKDIR /app

RUN apt-get update \
  && apt-get install -y --no-install-recommends ca-certificates tzdata lsof \
  && rm -rf /var/lib/apt/lists/*

ENV ENV=docker

COPY --from=builder /out/private_browser_client /app/private_browser_client
COPY Settings /app/Settings
COPY docs /app/docs
COPY public /app/public

RUN mkdir -p /app/data

EXPOSE 3300

CMD ["/app/private_browser_client"]
