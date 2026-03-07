FROM golang:1.26.1-bookworm AS go-builder
WORKDIR /src
COPY services/ ./services/
WORKDIR /src/services
RUN GOWORK=off GONOSUMCHECK=github.com/alexander-bruun/orb/* \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -o /bin/api ./cmd/main.go

FROM oven/bun:latest AS ui-builder
WORKDIR /app
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM nginx:alpine
RUN mkdir -p /opt
COPY --from=go-builder /bin/api /opt/api
COPY --from=ui-builder /app/build /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
COPY docker/entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /opt/api /docker-entrypoint.sh
EXPOSE 80 8080 9090 1900/udp
ENTRYPOINT ["/docker-entrypoint.sh"]
