# ==============================================================================
# SwanPanel - Multi-Stage Production Container Build
# https://github.com/thienvu18/swan-panel
#
# Stage 1: Build modern React SPA with Vite & Node 22
# Stage 2: Compile pure Go 1.27 static binary (CGO_ENABLED=0)
# Stage 3: Lightweight Alpine runtime with strongSwan, iptables, & nftables
# ==============================================================================

# ------------------------------------------------------------------------------
# Stage 1: Frontend SPA Builder
# ------------------------------------------------------------------------------
FROM node:22-alpine AS frontend-builder

WORKDIR /build/ui

# Install dependencies first for efficient layer caching
COPY ui/package.json ui/package-lock.json ./
RUN npm ci --prefer-offline --no-audit

# Copy UI source and build production assets
COPY ui/ ./
RUN npm run build

# ------------------------------------------------------------------------------
# Stage 2: Go Backend Builder (Pure Go, Statically Linked)
# ------------------------------------------------------------------------------
FROM golang:alpine AS backend-builder

WORKDIR /build

# Install CA certificates and git
RUN apk add --no-cache ca-certificates git

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy compiled React SPA assets from Stage 1 into ui/dist for embed.go
COPY --from=frontend-builder /build/ui/dist ./ui/dist

# Compile pure static binary with symbols stripped for minimal size
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -extldflags '-static'" \
    -o /build/swan-panel \
    ./cmd/server

# ------------------------------------------------------------------------------
# Stage 3: Lightweight Runtime Image
# ------------------------------------------------------------------------------
FROM alpine:latest AS runtime

LABEL maintainer="Thai Thien Vu <thienvu18@gmail.com>"
LABEL org.opencontainers.image.title="SwanPanel"
LABEL org.opencontainers.image.description="Modern Zero-Trust Web Control Plane for strongSwan VPN"
LABEL org.opencontainers.image.url="https://github.com/thienvu18/swan-panel"
LABEL org.opencontainers.image.source="https://github.com/thienvu18/swan-panel"
LABEL org.opencontainers.image.licenses="AGPL-3.0"

# Install strongSwan, firewall tooling, and runtime utilities
RUN apk add --no-cache \
    strongswan \
    iptables \
    ip6tables \
    nftables \
    iproute2 \
    ca-certificates \
    tzdata \
    curl \
    bash

# Setup application, state, and runtime socket directories
RUN mkdir -p /var/lib/swan-panel /etc/swan-panel /var/run /etc/ipsec.d && \
    chmod 750 /var/lib/swan-panel

# Copy statically compiled binary and entrypoint
COPY --from=backend-builder /build/swan-panel /usr/local/bin/swan-panel
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/swan-panel /usr/local/bin/docker-entrypoint.sh

# Default environment configuration
ENV PORT=8080 \
    DNS_PORT=53 \
    ENABLE_DNS=true \
    DB_PATH=/var/lib/swan-panel/swanpanel.db \
    VICI_SOCKET=/var/run/charon.vici \
    ENVIRONMENT=production \
    LOG_LEVEL=info

# Expose VPN IKEv2/IPsec (UDP 500, 4500), Split-DNS (UDP/TCP 53), Web UI (TCP 8080)
EXPOSE 8080/tcp 53/udp 53/tcp 500/udp 4500/udp

# Mount points for database persistence and custom configs
VOLUME ["/var/lib/swan-panel", "/etc/swan-panel"]

# Health check against SwanPanel API
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD curl -fs http://127.0.0.1:${PORT}/api/health || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/local/bin/swan-panel"]
