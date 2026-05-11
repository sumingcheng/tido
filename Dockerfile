# --- builder ---
FROM golang:1.25-alpine AS builder

WORKDIR /src

# 优先复制 go.mod/go.sum 利用层缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 静态链接（modernc.org/sqlite 是纯 Go，不需要 CGO）
ARG VERSION=docker
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.Version=${VERSION}" \
    -o /out/tido ./cmd/tido

# 预创建空 /data 目录，runtime stage 复制时带上正确的属主，
# 让 docker 首次创建 volume 时继承 nonroot:nonroot 权限。
RUN mkdir -p /empty-data

# --- runtime ---
FROM gcr.io/distroless/static-debian12:nonroot

ENV TIDO_HOME=/data

# distroless nonroot UID/GID = 65532；--chown 让 volume 首次初始化时拥有可写权限
COPY --from=builder --chown=65532:65532 /empty-data /data
COPY --from=builder /out/tido /tido

VOLUME ["/data"]
EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/tido", "-http", ":8080"]
