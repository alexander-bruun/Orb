FROM golang:1.26.1-bookworm AS go-builder
WORKDIR /src

# Cache module downloads separately from source changes
COPY services/go.mod services/go.sum ./services/
RUN cd services && GOWORK=off go mod download

# .git needed for build-tag stamping
COPY .git .git
COPY services/ services/

WORKDIR /src/services
RUN BUILD_TAG=$(git -C /src describe --tags --always 2>/dev/null || echo "unknown") && \
    BUILD_SHA=$(git -C /src rev-parse --short HEAD 2>/dev/null || echo "unknown") && \
    GOWORK=off CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
      -ldflags "-s -w -X main.buildTag=$BUILD_TAG -X main.buildSHA=$BUILD_SHA" \
      -o /bin/api ./cmd/main.go

FROM oven/bun:alpine AS ui-builder
WORKDIR /app

# Cache bun install layer — invalidate only when lockfile or manifest changes
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile

COPY web/ ./
RUN bun run build

FROM alpine:3.21
RUN apk add --no-cache nginx ffmpeg postgresql-client && \
    mkdir -p /run/nginx

COPY --from=go-builder /bin/api /opt/api
COPY --from=ui-builder /app/build /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/http.d/default.conf
COPY docker/entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /opt/api /docker-entrypoint.sh

EXPOSE 80 8080 9090 1900/udp
ENTRYPOINT ["/docker-entrypoint.sh"]
