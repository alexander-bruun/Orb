FROM node:20-alpine AS builder
WORKDIR /app
COPY web/ui/package*.json ./
RUN npm ci
COPY web/ui/ ./
RUN npm run build

FROM nginx:alpine
COPY --from=builder /app/build /usr/share/nginx/html
COPY docker/nginx.conf /etc/nginx/conf.d/default.conf
