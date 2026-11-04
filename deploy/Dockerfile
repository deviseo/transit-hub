# ---- 阶段 1：前端构建 ----
# 在构建机自己的平台上运行（产物与平台无关），在 Apple 芯片上构建 amd64 镜像时不用模拟。
FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
ARG VITE_API_BASE_URL=/api
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
RUN npm run build

# ---- 阶段 2：后端构建 ----
# 同样在构建机的平台上运行，交叉编译成目标平台（TARGETOS/TARGETARCH 由 BuildKit 提供）。
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS backend-builder
ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/transithub-api ./cmd/api

# ---- 阶段 3：运行时 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
RUN mkdir -p /app/data

COPY --from=backend-builder /out/transithub-api /app/transithub-api
COPY --from=frontend-builder /src/frontend/dist /app/public

ENV PORT=10621
ENV PUBLIC_DIR=/app/public
# 在线更新下载的新版本放在这里；部署时挂载成持久卷，重建容器后仍然用更新后的版本。
ENV DATA_DIR=/app/data
# 业务时区：仪表盘「今日」营收/成本的日期边界按北京时间切分，与午夜快照调度（固定 Asia/Shanghai）一致。
# 镜像已安装 tzdata，Go 运行时会读取 TZ；不设置时容器默认 UTC，北京时间 0–8 点的「今日」会变成前一天。
ENV TZ=Asia/Shanghai
EXPOSE 10621

ENTRYPOINT ["/app/transithub-api"]
