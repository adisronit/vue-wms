# syntax=docker/dockerfile:1

# ---- builder ---------------------------------------------------------
# Compiles both binaries and regenerates Swagger docs from source, so the
# generated docs/ output never needs to be committed to stay reproducible
# — `docker compose build` always regenerates it from the current code.
FROM golang:1.23-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go install github.com/swaggo/swag/cmd/swag@v1.16.4
RUN swag init -g cmd/server/main.go -o docs --parseInternal --parseDependency

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/wms-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/wms-worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/wms-migrate ./cmd/migrate

# ---- server runtime ----------------------------------------------------
# Small runtime image: no Go toolchain, no swag CLI, just the static
# binary and CA certificates.
FROM alpine:3.20 AS server
RUN apk add --no-cache ca-certificates wget
COPY --from=builder /out/wms-server /usr/local/bin/wms-server

EXPOSE 8080
# -O /dev/null (not --spider, which sends a HEAD request the health route
# doesn't register) against 127.0.0.1 (not localhost, which can resolve
# to ::1 first and get refused since the server only listens on IPv4).
HEALTHCHECK --interval=15s --timeout=5s --start-period=20s --retries=5 \
    CMD wget -q -O /dev/null "http://127.0.0.1:${APP_PORT:-8080}/api/v1/health" || exit 1

ENTRYPOINT ["/usr/local/bin/wms-server"]

# ---- worker runtime ----------------------------------------------------
FROM alpine:3.20 AS worker
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/wms-worker /usr/local/bin/wms-worker

ENTRYPOINT ["/usr/local/bin/wms-worker"]

# ---- migrate runtime ----------------------------------------------------
# One-shot schema migration + RBAC seed runner (see cmd/migrate). Runs to
# completion and exits; not a long-lived service.
FROM alpine:3.20 AS migrate
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/wms-migrate /usr/local/bin/wms-migrate

ENTRYPOINT ["/usr/local/bin/wms-migrate"]
