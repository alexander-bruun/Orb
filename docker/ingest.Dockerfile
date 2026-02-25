FROM golang:latest AS builder
WORKDIR /src
COPY pkg/ ./pkg/
COPY cmd/ingest/ ./cmd/ingest/
WORKDIR /src/cmd/ingest
RUN GOWORK=off GONOSUMCHECK=github.com/alexander-bruun/orb/* \
    CGO_ENABLED=0 GOOS=linux \
    /bin/sh -c "go mod tidy && go build -trimpath -o /bin/ingest ."

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/ingest /ingest
ENTRYPOINT ["/ingest"]
