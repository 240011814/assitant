# Build stage：固定在 runner 原生平台编译，用 GOOS/GOARCH 交叉出目标平台二进制，避免 arm64 走 QEMU 模拟
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# 安装 git
RUN apk add --no-cache git

# 复制依赖文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源码
COPY . .

# 静态编译（TARGETOS/TARGETARCH 由 buildx 按目标平台自动注入；timetzdata 内嵌时区库，运行时不再需要 tzdata 包）
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags timetzdata -o main .

# Runtime stage
FROM alpine:latest

WORKDIR /app

# 内嵌 tzdata，只需时区环境变量；本阶段无 RUN，双平台构建全程无需 QEMU
ENV TZ=Asia/Shanghai

# 复制可执行文件（--chmod 保证执行权限，避免 RUN chmod 走模拟）
COPY --chmod=0755 --from=builder /app/main .

# 暴露端口
EXPOSE 8080

# 启动
CMD ["./main"]