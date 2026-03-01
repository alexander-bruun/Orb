FROM oven/bun:latest AS builder
WORKDIR /app
COPY web/ui/package.json web/ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ui/ ./
RUN bun run build

FROM nginx:alpine
COPY --from=builder /app/build /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
