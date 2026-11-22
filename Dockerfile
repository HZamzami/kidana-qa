# syntax=docker/dockerfile:1.7
#
# Targets:
#   dev   – all tooling (Go, templ, sqlc, goose, air, Tailwind, golangci-lint). Used by compose for local work.
#   build – compiles assets + a static binary.
#   prod  – minimal distroless image that runs the binary as non-root.

ARG GO_VERSION=1.27.4

# ---------------------------------------------------------------- dev
FROM golang:${GO_VERSION}-bookworm AS dev

ARG TARGETARCH=amd64
ARG TEMPL_VERSION=v0.3.1118
ARG SQLC_VERSION=1.31.1
ARG GOOSE_VERSION=v3.30.1
ARG AIR_VERSION=v1.67.4
ARG TAILWIND_VERSION=v4.3.5
ARG GOLANGCI_VERSION=2.15.0

ENV GOBIN=/usr/local/bin \
    GOFLAGS=-buildvcs=false \
    GOCACHE=/go-cache \
    GOMODCACHE=/go-mod \
    GOPATH=/go-mod/gopath \
    GOLANGCI_LINT_CACHE=/go-cache/golangci \
    HOME=/tmp

RUN go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION} \
 && go install github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION} \
 && go install github.com/air-verse/air@${AIR_VERSION} \
 && curl -fsSL "https://github.com/sqlc-dev/sqlc/releases/download/v${SQLC_VERSION}/sqlc_${SQLC_VERSION}_linux_${TARGETARCH}.tar.gz" \
      | tar -xz -C /usr/local/bin sqlc \
 && TW_ARCH=$([ "$TARGETARCH" = "arm64" ] && echo arm64 || echo x64) \
 && curl -fsSL -o /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${TW_ARCH}" \
 && chmod +x /usr/local/bin/tailwindcss \
 && curl -fsSL "https://github.com/golangci/golangci-lint/releases/download/v${GOLANGCI_VERSION}/golangci-lint-${GOLANGCI_VERSION}-linux-${TARGETARCH}.tar.gz" \
      | tar -xz --strip-components=1 -C /usr/local/bin "golangci-lint-${GOLANGCI_VERSION}-linux-${TARGETARCH}/golangci-lint" \
 && apt-get update && apt-get install -y --no-install-recommends postgresql-client && rm -rf /var/lib/apt/lists/* \
 && rm -rf /go-cache /go-mod \
 # Caches are named volumes; world-writable so the container can run as the host UID.
 && mkdir -p /go-cache /go-mod /data/uploads && chmod 1777 /go-cache /go-mod /data /data/uploads

WORKDIR /app
EXPOSE 8080
CMD ["air", "-c", ".air.toml"]

# ---------------------------------------------------------------- build
FROM dev AS build
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN templ generate \
 && tailwindcss -i web/styles/input.css -o web/static/css/app.css --minify \
 && go build -trimpath -ldflags="-s -w -X main.version=$(date -u +%Y%m%d%H%M)" -o /out/kidana-qa ./cmd/server \
 && mkdir -p /out/data/uploads

# ---------------------------------------------------------------- prod
FROM gcr.io/distroless/static-debian12:nonroot AS prod
COPY --from=build /out/kidana-qa /kidana-qa
# Pre-create the uploads dir owned by the nonroot user so a fresh named volume inherits it.
COPY --from=build --chown=65532:65532 /out/data /data
ENV APP_ENV=production HTTP_ADDR=:8080 UPLOAD_DIR=/data/uploads
VOLUME ["/data/uploads"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/kidana-qa"]
