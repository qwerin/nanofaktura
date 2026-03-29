# ── Stage 1: Build frontend ──────────────────────────────────────────────────
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# ── Stage 2: Build Go binary (CGO required for SQLite) ───────────────────────
FROM golang:1.25-bookworm AS go-builder
WORKDIR /app

# Download dependencies first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o bin/nanofaktura ./cmd/server/

# ── Stage 3: Minimal runtime image ───────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=go-builder  /app/bin/nanofaktura ./nanofaktura
COPY --from=frontend-builder /app/web/dist   ./dist

ENV NANOFAKTURA_STATIC_DIR=/app/dist
ENV NANOFAKTURA_LISTEN_ADDR=:8080
ENV NANOFAKTURA_DB_DRIVER=postgres

EXPOSE 8080

ENTRYPOINT ["/app/nanofaktura"]
