# syntax=docker/dockerfile:1

FROM alpine:3.23

WORKDIR /app
# Add /app to PATH
ENV PATH="/app:${PATH}"

# Install ca-certificates for TLS and netcat for health checks
RUN apk add --no-cache ca-certificates netcat-openbsd

# Pre-compiled binary is provided in build context as anytype-linux-{arch}
# TARGETARCH is set automatically by docker buildx (amd64 or arm64)
ARG TARGETARCH
COPY anytype-linux-${TARGETARCH} /app/anytype
RUN chmod +x /app/anytype

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
