# syntax=docker/dockerfile:1

# ---------- 1) 构建前端 ----------
FROM node:22-bookworm AS frontend
WORKDIR /src/frontend
RUN corepack enable pnpm
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm run build

# ---------- 2) 构建后端（前端已内嵌进二进制） ----------
FROM golang:1.26-bookworm AS backend
WORKDIR /src
COPY backend/ backend/
COPY --from=frontend /src/frontend/dist/ backend/internal/webui/dist/
WORKDIR /src/backend
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags "-s -w" -o /out/gitdash .

# ---------- 3) 运行时 ----------
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git curl tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 10001 --create-home gitdash

ENV GITDASH_DATA=/data \
    GITDASH_HTTP_ADDR=:8080 \
    GITDASH_SSH_ADDR=:2222

COPY --from=backend /out/gitdash /usr/local/bin/gitdash

RUN mkdir -p /data && chown -R gitdash:gitdash /data

USER gitdash
WORKDIR /home/gitdash
EXPOSE 8080 2222
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -fsS http://127.0.0.1:8080/api/health >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/gitdash", "serve"]
