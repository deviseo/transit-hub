# ---- 阶段 1：前端构建 ----
FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
ARG VITE_API_BASE_URL=/api
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
RUN npm run build

# ---- 阶段 2：后端构建 ----
FROM golang:1.25-alpine AS backend-builder
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -o /out/transithub-api ./cmd/api

# ---- 阶段 3：运行时 ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
RUN mkdir -p /app/data

COPY --from=backend-builder /out/transithub-api /app/transithub-api
COPY --from=frontend-builder /src/frontend/dist /app/public

ENV PORT=10621
ENV PUBLIC_DIR=/app/public
# 工单图片附件本地存储目录：/app/data 下的子目录，不在 PUBLIC_DIR（/app/public）之下，
# 不会被当作公开静态资源暴露；docker-compose.prod.yml 需要把这个路径挂载成持久化 volume，
# 否则容器重建后数据库里的附件 metadata 还在但图片文件会丢失。
ENV TICKET_UPLOAD_DIR=/app/data/ticket-uploads
EXPOSE 10621

ENTRYPOINT ["/app/transithub-api"]
