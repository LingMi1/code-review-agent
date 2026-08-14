# Code Review Agent — Dockerfile
# 多阶段构建：编译 Go 二进制 → 最小运行时镜像
FROM golang:1.25-alpine AS builder

WORKDIR /src

# 先复制依赖文件用于缓存
COPY go.mod go.sum ./

RUN go mod download

COPY . .

# 编译（CGO_ENABLED=0 纯静态二进制，配合 modernc.org/sqlite）
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /app/server ./cmd/server/

# --- 运行时镜像 ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata wget
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/server .

# 数据目录
RUN mkdir -p /app/data && chown 65532:65532 /app/data

# 非 root 用户
USER 65532:65532

EXPOSE 8080

# 健康检查（独立 docker run 时也生效）
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["/app/server"]
