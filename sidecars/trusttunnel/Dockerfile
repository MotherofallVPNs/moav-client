#
# TrustTunnel sidecar — runs the upstream CLI client in SOCKS5 mode.
#
# Pulls the prebuilt Linux client from github.com/TrustTunnel/TrustTunnelClient
# releases (Apache-2.0). The client supports a [listener.socks] SOCKS5 proxy —
# moav-client's configgen rewrites the bundle's [listener.tun] stanza to a
# SOCKS5 listener on 0.0.0.0:5600, so no TUN device or NET_ADMIN is needed.
#
# Config is mounted at /etc/trusttunnel/client.toml (written by moav-client on
# startup from sidecars.trusttunnel.config.source_path).
#
FROM debian:bookworm-slim AS fetch
ARG TRUSTTUNNEL_CLIENT_VERSION=v1.3.0
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /dl
# Match the binary by exact name: the tarball also ships trusttunnel_client.sig,
# which a 'trusttunnel_client*' glob can pick first. --version proves it runs here.
# SHA256s are the release's per-asset digests; a version bump must add its own.
RUN arch="$(uname -m)"; \
    case "$arch" in \
      x86_64)  A=linux-x86_64 ;; \
      aarch64) A=linux-aarch64 ;; \
      armv7l)  A=linux-armv7 ;; \
      *) echo "unsupported arch $arch"; exit 1 ;; \
    esac; \
    case "${TRUSTTUNNEL_CLIENT_VERSION}/${A}" in \
      v1.3.0/linux-x86_64)  EXPECTED=99e764bb2c080f7a60c441b51ad75fa7ec00e6c8a80afe36030af37201ba32f7 ;; \
      v1.3.0/linux-aarch64) EXPECTED=837ec64520d546cdf8e7a327614589f64dc6c4f30e408e3788de7bfaef05b52b ;; \
      v1.3.0/linux-armv7)   EXPECTED=19a0c479df4b2bce0ea3ea09777fc78be2918d0f7e8360279e1eee543284ac01 ;; \
      *) echo "no pinned SHA256 for TrustTunnel client ${TRUSTTUNNEL_CLIENT_VERSION} ${A}; add it to sidecars/trusttunnel/Dockerfile" >&2; exit 1 ;; \
    esac; \
    curl -fL "https://github.com/TrustTunnel/TrustTunnelClient/releases/download/${TRUSTTUNNEL_CLIENT_VERSION}/trusttunnel_client-${TRUSTTUNNEL_CLIENT_VERSION}-${A}.tar.gz" -o tt.tar.gz \
    && ACTUAL="$(sha256sum tt.tar.gz | cut -d' ' -f1)" \
    && { [ "$EXPECTED" = "$ACTUAL" ] || { echo "SHA256 mismatch for trusttunnel_client-${TRUSTTUNNEL_CLIENT_VERSION}-${A}.tar.gz: expected $EXPECTED, got $ACTUAL" >&2; exit 1; }; } \
    && echo "SHA256 OK ($ACTUAL)" \
    && tar xzf tt.tar.gz \
    && bin="$(find . -type f -name trusttunnel_client | head -1)" \
    && test -n "$bin" \
    && cp "$bin" /trusttunnel_client \
    && chmod +x /trusttunnel_client \
    && /trusttunnel_client --version

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates libstdc++6 socat \
    && rm -rf /var/lib/apt/lists/*
COPY --from=fetch /trusttunnel_client /usr/local/bin/trusttunnel_client
VOLUME ["/etc/trusttunnel"]
EXPOSE 5600/tcp
# The client binds its SOCKS5 listener to 127.0.0.1:5601 (loopback — it
# refuses a non-loopback bind without auth). socat republishes it on
# 0.0.0.0:5600 for proxy-core to reach over moav-net. socat runs in the
# background; the client is the foreground process so a crash restarts the
# container. Wait for client.toml like the singbox/xray/amneziawg sidecars:
# on a cold `compose up` this container starts before proxy-core writes it.
ENTRYPOINT ["/bin/sh","-c","\
echo '[trusttunnel] waiting for /etc/trusttunnel/client.toml (written by moav-client from sidecars.trusttunnel.config.source_path) ...'; \
while [ ! -s /etc/trusttunnel/client.toml ]; do sleep 1; done; \
echo '[trusttunnel] config detected, launching client (SOCKS5 via socat :5600 -> :5601)'; \
socat TCP-LISTEN:5600,fork,reuseaddr TCP:127.0.0.1:5601 & \
exec /usr/local/bin/trusttunnel_client --config /etc/trusttunnel/client.toml"]
