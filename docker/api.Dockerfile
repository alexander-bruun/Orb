FROM golang:latest AS builder
WORKDIR /src
COPY pkg/ ./pkg/
COPY services/api/ ./services/api/
WORKDIR /src/services/api
RUN GOWORK=off GONOSUMCHECK=github.com/alexander-bruun/orb/* \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -o /bin/api ./cmd/main.go

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/api /api
ENTRYPOINT ["/api"]
