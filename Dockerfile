# syntax=docker/dockerfile:1.7

# -------- build stage --------
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Cache modules separately from source.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG CMD=indexer

# Build statically linked binary.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" \
        -o /out/indexer ./cmd/${CMD}

# -------- runtime stage --------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S indexer \
 && adduser -S -G indexer indexer

WORKDIR /app

COPY --from=builder /out/indexer /app/indexer
COPY configs/config.example.yaml /app/configs/config.example.yaml

USER indexer

ENTRYPOINT ["/app/indexer"]
CMD ["-config=/app/configs/config.yaml"]
