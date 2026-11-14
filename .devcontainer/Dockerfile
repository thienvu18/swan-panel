# Development-only: Go, Node, Air, strongSwan and nftables in one network namespace.
# Never copy the Air development tool into the production image.
FROM golang:1.27-alpine3.23 AS go-tools
RUN CGO_ENABLED=0 go install github.com/air-verse/air@v1.67.4

FROM node:26-alpine3.23
COPY --from=go-tools /usr/local/go /usr/local/go
COPY --from=go-tools /go/bin/air /usr/local/bin/air

ENV PATH="/usr/local/go/bin:/usr/local/bin:${PATH}" \
    GOPATH=/go \
    CGO_ENABLED=1 \
    GOFLAGS="-buildvcs=false"

RUN apk add --no-cache \
    bash build-base ca-certificates curl git iproute2 iptables \
    jq nftables openssl procps strongswan supervisor tini \
    && mkdir -p /workspace/swan-panel /var/log/swanpanel

WORKDIR /workspace/swan-panel
ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/bin/sh", ".devcontainer/start.sh"]
