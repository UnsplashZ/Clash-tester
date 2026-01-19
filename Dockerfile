# Dockerfile
# Stage 1: Build Golang Binary
FROM golang:1.23-bullseye AS builder

WORKDIR /app

# 安装 git
RUN apt-get update && apt-get install -y git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 静态编译
RUN CGO_ENABLED=0 GOOS=linux go build -o clash-tester cmd/main.go

# Stage 2: Runtime Image (Debian Slim)
FROM debian:bullseye-slim

WORKDIR /app

# 安装基础依赖
# ca-certificates: HTTPS 证书
# curl, gzip: 下载工具
# tzdata: 时区
RUN apt-get update && apt-get install -y ca-certificates curl gzip tzdata && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/clash-tester /app/clash-tester
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# 自动下载 Mihomo (使用 compatible 模式以防万一)
ARG MIHOMO_VERSION=v1.19.19
RUN ARCH=$(uname -m) && \
    case "$ARCH" in \
        x86_64) MIHOMO_ARCH="amd64-compatible" ;; \
        aarch64) MIHOMO_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH"; exit 1 ;; \
    esac && \
    echo "Downloading Mihomo for $MIHOMO_ARCH..." && \
    curl -L -o mihomo.gz "https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VERSION}/mihomo-linux-${MIHOMO_ARCH}-${MIHOMO_VERSION}.gz" && \
    gzip -d mihomo.gz && \
    chmod +x mihomo

# 设置时区
ENV TZ=Asia/Shanghai

VOLUME /data

ENTRYPOINT ["/app/entrypoint.sh"]