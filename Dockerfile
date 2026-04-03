# syntax=docker/dockerfile:1

# =============================================================================
# Build stage
# =============================================================================
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
# - build-base: gcc, musl-dev (required for CGO/tantivy linking)
# - curl: for downloading the tantivy library
# - make: to use Makefile build
RUN apk add --no-cache build-base curl make

# Copy dependency files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build arguments for version info (pass via --build-arg)
ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG GIT_STATE=unknown
ARG TARGETARCH

# Build a statically-linked binary via Makefile
RUN CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH="${TARGETARCH}" \
    BUILD_TAGS="noheic" \
    EXTRA_LDFLAGS="-linkmode external -extldflags '-static'" \
    OUTPUT=/app/anytype \
    VERSION="${VERSION}" \
    COMMIT="${COMMIT}" \
    BUILD_TIME="${BUILD_TIME}" \
    GIT_STATE="${GIT_STATE}" \
    make build

# =============================================================================
# Production stage
# =============================================================================
FROM alpine:3.23 AS production

WORKDIR /app

# Install ca-certificates for TLS and netcat for health checks
RUN apk add --no-cache ca-certificates netcat-openbsd

# Copy binary from builder
COPY --from=builder /app/anytype /app/anytype

# Note: Running as root to avoid volume permission issues in docker-compose

# gRPC (31010), gRPC-Web (31011), API (31012)
EXPOSE 31010 31011 31012

# Persistent data volumes
VOLUME ["/root/.anytype", "/root/.config/anytype"]

# Health check: verify gRPC port is accepting connections
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD nc -z 127.0.0.1 31010 || exit 1

# Run the embedded server in foreground
ENTRYPOINT ["/app/anytype"]
CMD ["serve"]
