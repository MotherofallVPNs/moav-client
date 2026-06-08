# Xray-core sidecar — handles the VLESS transports sing-box can't speak
# (xhttp / splithttp). proxy-core generates data/xray.json only when the bundle
# has such endpoints; otherwise this container idles in the compose wait-loop.
#
# Uses the OFFICIAL XTLS release binary (github.com/XTLS/Xray-core), pinned via
# XRAY_VERSION, matching the MoaV server's Dockerfile.xray. Falls back to a
# source compile only when the prebuilt asset is unavailable (GitHub blocked or
# no asset for the target arch).

FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git wget unzip ca-certificates

ARG XRAY_REPO=https://github.com/XTLS/Xray-core.git
ARG XRAY_VERSION=v26.5.9
ARG TARGETARCH

# Go-module fetch resilience for the source-compile fallback only. Pipe (|)
# falls through on ANY error (incl. Google CDN 403s); GOSUMDB=off drops the
# sum.golang.org dependency (integrity still checked against committed go.sum).
ARG GOPROXY="https://proxy.golang.org|https://goproxy.cn|direct"
ARG GOSUMDB=off
ENV GOPROXY=${GOPROXY} GOSUMDB=${GOSUMDB}

# 1) Download the prebuilt release binary. 2) On any failure, compile from source.
RUN set -eux; \
    case "${TARGETARCH:-amd64}" in \
        amd64) XR_ARCH="64" ;; \
        arm64) XR_ARCH="arm64-v8a" ;; \
        *)     XR_ARCH="" ;; \
    esac; \
    got=0; \
    if [ -n "$XR_ARCH" ] && \
       wget -qO /tmp/xray.zip "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/Xray-linux-${XR_ARCH}.zip" && \
       unzip -j /tmp/xray.zip xray -d / ; then \
        got=1; \
        echo "xray ${XRAY_VERSION}: installed from prebuilt release (Xray-linux-${XR_ARCH})"; \
    fi; \
    if [ "$got" -ne 1 ]; then \
        echo "xray ${XRAY_VERSION}: prebuilt unavailable, compiling from source..."; \
        git clone --depth 1 -b "${XRAY_VERSION}" "${XRAY_REPO}" /src; \
        cd /src; \
        CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /xray ./main; \
    fi; \
    rm -f /tmp/xray.zip; \
    /xray version

# Runtime stage — /bin/sh is required by the compose inline entrypoint.
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /xray /usr/local/bin/xray
RUN chmod +x /usr/local/bin/xray
RUN mkdir -p /etc/xray
# Sanity-check the binary is runnable at build time.
RUN /usr/local/bin/xray version
ENTRYPOINT ["/usr/local/bin/xray"]
CMD ["run", "-c", "/etc/xray/xray.json"]
