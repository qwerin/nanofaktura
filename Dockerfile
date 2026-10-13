# Base images are pinned to a patch version and digest (reproducible, patched
# builds). Update both together, e.g. `docker buildx imagetools inspect golang:1.26.9-alpine3.24`.

# ── Stage 1: frontend ────────────────────────────────────────────────────────
FROM node:24.21.0-alpine3.24@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS frontend-builder
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# sekce Novinky čte ../CHANGELOG.md
COPY CHANGELOG.md /app/CHANGELOG.md
RUN npm run build

# ── Stage 2: Go binary (pure Go, no CGO) ─────────────────────────────────────
FROM golang:1.26.9-alpine3.24@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /nanofaktura ./cmd/server/ \
 && mkdir -p /data

# ── Stage 3: runtime ─────────────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
WORKDIR /app
COPY --from=go-builder /nanofaktura /app/nanofaktura
COPY --from=frontend-builder /app/web/dist /app/dist
# data directory (secret.key, attachments) writable by the nonroot user; mount a volume here
COPY --from=go-builder --chown=65532:65532 /data /data

ENV NANOFAKTURA_STATIC_DIR=/app/dist \
    NANOFAKTURA_LISTEN_ADDR=:8080 \
    NANOFAKTURA_DB_DRIVER=postgres \
    NANOFAKTURA_DATA_DIR=/data

VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/app/nanofaktura"]
