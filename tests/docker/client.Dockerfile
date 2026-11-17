FROM alpine:3.23
RUN apk add --no-cache strongswan bash iproute2 iputils ca-certificates
RUN mkdir -p /etc/swanctl/conf.d /etc/swanctl/x509ca /etc/swanctl/x509 /etc/swanctl/rsa
