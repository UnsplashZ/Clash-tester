# Dockerfile
# 多阶段构建

# Stage 1: Build Golang Binary
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 安装 git (go mod download 可能需要)
RUN apk add --no-cache git

# 复制依赖文件并下载
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
# CGO_ENABLED=0 静态编译，减小体积且更兼容
RUN CGO_ENABLED=0 GOOS=linux go build -o clash-tester cmd/main.go

# Stage 2: Runtime Image
FROM alpine:latest

WORKDIR /app

# 安装基础依赖
# ca-certificates: HTTPS 请求必需
# tzdata: 时区设置必需
# curl, gzip: 下载 mihomo 必需
RUN apk add --no-cache ca-certificates tzdata curl gzip

# 复制编译好的二进制文件
COPY --from=builder /app/clash-tester /app/clash-tester

# 复制 entrypoint 脚本
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# 自动下载 Mihomo (Clash Meta) 核心
# 使用 alpha 版本通常更新修复快，也可以改为 release
# 这里通过 uname -m 判断架构自动下载
ARG MIHOMO_VERSION=v1.18.1
RUN ARCH=$(uname -m) && \
    case "$ARCH" in \
        x86_64) MIHOMO_ARCH="amd64" ;; \
        aarch64) MIHOMO_ARCH="arm64" ;; \
        *) echo "Unsupported architecture: $ARCH"; exit 1 ;; \
    esac && \
    echo "Downloading Mihomo for $MIHOMO_ARCH..." && \
    curl -L -o mihomo.gz "https://github.com/MetaCubeX/mihomo/releases/download/${MIHOMO_VERSION}/mihomo-linux-${MIHOMO_ARCH}-${MIHOMO_VERSION}.gz" && \
    gzip -d mihomo.gz && \
    chmod +x mihomo

# 设置时区为上海
ENV TZ=Asia/Shanghai

# 定义数据卷挂载点
VOLUME /data

# 启动命令
ENTRYPOINT ["/app/entrypoint.sh"]
