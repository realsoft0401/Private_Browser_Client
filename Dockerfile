# 第一阶段负责编译 Go 服务。
#
# 设计来源：
# - 当前项目使用 go-sqlite3，必须启用 CGO；
# - 因此构建镜像需要 gcc，运行镜像保留在 Debian slim，避免 Alpine/musl 和 glibc 动态库不匹配；
# - docs、Settings、public 作为运行时事实文件复制到最终镜像，保证 Swagger 入口和配置能随镜像一起部署。
FROM golang:1.22-bookworm AS builder

WORKDIR /src

# 容器构建时需要下载 Go module。
#
# 国内网络直接访问 proxy.golang.org 容易 TLS 超时，因此这里固定走可用的国内 Go 代理；
# 清华 Debian 镜像继续保留，但当前清华提供的 goproxy 路径会对项目依赖返回 404，
# 导致 go mod download 回退到 direct 拉 GitHub，镜像构建会长时间卡住。
# 后续如果部署在海外 CI，可以用 `--build-arg GOPROXY=https://proxy.golang.org,direct` 覆盖。
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
ARG GOSUMDB=sum.golang.google.cn
ENV GOSUMDB=${GOSUMDB}

# 默认使用清华 Debian 镜像；如果后续海外构建更快，可通过 build-arg 覆盖回官方源。
ARG DEBIAN_MIRROR=mirrors.tuna.tsinghua.edu.cn
RUN sed -i "s|http://deb.debian.org/debian|http://${DEBIAN_MIRROR}/debian|g" /etc/apt/sources.list.d/debian.sources \
  && sed -i "s|http://deb.debian.org/debian-security|http://${DEBIAN_MIRROR}/debian-security|g" /etc/apt/sources.list.d/debian.sources \
  && apt-get update \
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

ARG DEBIAN_MIRROR=mirrors.tuna.tsinghua.edu.cn
RUN sed -i "s|http://deb.debian.org/debian|http://${DEBIAN_MIRROR}/debian|g" /etc/apt/sources.list.d/debian.sources \
  && sed -i "s|http://deb.debian.org/debian-security|http://${DEBIAN_MIRROR}/debian-security|g" /etc/apt/sources.list.d/debian.sources \
  && apt-get update \
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
