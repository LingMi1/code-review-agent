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

RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/server .

# 数据目录
RUN mkdir -p /app/data

EXPOSE 8080

CMD ["/app/server"]
