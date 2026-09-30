# ── Stage 1: frontend ────────────────────────────────────────────────────────
FROM node:22-alpine AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# sekce Novinky čte ../CHANGELOG.md
COPY CHANGELOG.md /app/CHANGELOG.md
RUN npm run build

# ── Stage 2: Go binary (pure Go, no CGO) ─────────────────────────────────────
FROM golang:1.26-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /nanofaktura ./cmd/server/

# ── Stage 3: runtime ─────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=go-builder /nanofaktura /app/nanofaktura
COPY --from=frontend-builder /app/web/dist /app/dist

ENV NANOFAKTURA_STATIC_DIR=/app/dist \
    NANOFAKTURA_LISTEN_ADDR=:8080 \
    NANOFAKTURA_DB_DRIVER=postgres

EXPOSE 8080
ENTRYPOINT ["/app/nanofaktura"]
