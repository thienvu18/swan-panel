# ==============================================================================
# SwanPanel - Multi-Stage Production Container Build
# https://github.com/thienvu18/swan-panel
#
# Stage 1: Build modern React SPA with Vite & Node 26 (Alpine 3.23)
# Stage 2: Compile CGO-enabled Go with musl into a static Linux binary
# Stage 3: Lightweight Alpine 3.23 runtime with strongSwan, iptables, & nftables
# ==============================================================================

# ------------------------------------------------------------------------------
# Stage 1: Frontend SPA Builder
# ------------------------------------------------------------------------------
FROM node:26-alpine3.23 AS frontend-builder

WORKDIR /build/ui

# Install dependencies first for efficient layer caching
COPY ui/package.json ui/package-lock.json ./
RUN npm ci --prefer-offline --no-audit

# Copy UI source and build production assets
COPY ui/ ./
RUN npm run build

# ------------------------------------------------------------------------------
# Stage 2: Go Backend Builder (CGO SQLite, statically linked with musl)
# ------------------------------------------------------------------------------
FROM golang:1.27-alpine3.23 AS backend-builder

WORKDIR /build

# SQLite driver github.com/mattn/go-sqlite3 requires CGO and a native C compiler.
# Alpine uses musl, allowing a static release binary without glibc dependency.
RUN apk add --no-cache ca-certificates git gcc musl-dev binutils file

# Cache Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy compiled React SPA assets from Stage 1 into ui/dist for embed.go
COPY --from=frontend-builder /build/ui/dist ./ui/dist

# A release must embed the actual React bundle, never an empty directory
# or the development placeholder. Abort image builds without JavaScript.
RUN test -s ui/dist/index.html \
    && find ui/dist/assets -type f -name '*.js' | grep -q .

# Compile with the official GORM SQLite driver; omit loadable SQLite extensions.
# Static linking avoids requiring an external libc or libsqlite3 in the runtime.
RUN CGO_ENABLED=1 CC=gcc GOOS=linux go build \
    -trimpath \
    -tags "netgo,osusergo,sqlite_omit_load_extension" \
    -ldflags="-s -w -linkmode external -extldflags '-static'" \
    -o /build/swan-panel \
    ./cmd/server \
    && file /build/swan-panel \
    && file /build/swan-panel | grep -q 'statically linked' \
    && ! readelf -l /build/swan-panel | grep -q INTERP

# ------------------------------------------------------------------------------
# Stage 3: Lightweight Runtime Image
# ------------------------------------------------------------------------------
FROM alpine:3.23 AS runtime

LABEL maintainer="Thai Thien Vu <thienvu18@gmail.com>"
LABEL org.opencontainers.image.title="SwanPanel"
LABEL org.opencontainers.image.description="Modern Zero-Trust Web Control Plane for strongSwan VPN"
LABEL org.opencontainers.image.url="https://github.com/thienvu18/swan-panel"
LABEL org.opencontainers.image.source="https://github.com/thienvu18/swan-panel"
LABEL org.opencontainers.image.licenses="AGPL-3.0"

# Install strongSwan, firewall tooling, and runtime utilities.
# Alpine 3.23's iptables package supplies ip6tables; there is no separate
# ip6tables package for this release.
RUN apk add --no-cache \
    strongswan \
    iptables \
    nftables \
    iproute2 \
    ca-certificates \
    tzdata \
    curl \
    bash \
    tini

# Setup application, state, and runtime socket directories
RUN mkdir -p /var/lib/swan-panel /etc/swan-panel /var/run /etc/ipsec.d \
    /etc/swanctl/x509ca /etc/swanctl/x509 /etc/swanctl/rsa /etc/swanctl/x509crl && \
    chmod 750 /var/lib/swan-panel && chmod 700 /etc/swanctl/rsa

# Copy statically compiled binary and entrypoint
COPY --from=backend-builder /build/swan-panel /usr/local/bin/swan-panel
COPY deploy/docker/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
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

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/local/bin/swan-panel"]
