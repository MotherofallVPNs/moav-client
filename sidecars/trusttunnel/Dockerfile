#
# TrustTunnel sidecar — placeholder until upstream ships a Linux binary.
#
# TrustTunnel is an HTTP/2 + HTTP/3 (QUIC) tunnel with username/password
# authentication. The upstream client (https://trusttunnel.org/) currently
# only releases for macOS/iOS/Android and the desktop CLI is closed source.
#
# This sidecar accepts a binary mounted at /usr/local/bin/trusttunnel-client
# and a config at /etc/trusttunnel/client.toml. With both present it runs
# the client; otherwise it exposes the SOCKS5 port as TCP-RST so the moav
# probe surfaces status=error and the balancer rolls over to another peer.
#
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates socat \
    && rm -rf /var/lib/apt/lists/*
VOLUME ["/etc/trusttunnel", "/usr/local/bin"]
EXPOSE 5600/tcp
ENTRYPOINT ["/bin/sh","-c","\
if [ -x /usr/local/bin/trusttunnel-client ] && [ -s /etc/trusttunnel/client.toml ]; then \
  echo '[trusttunnel] launching upstream client'; \
  exec /usr/local/bin/trusttunnel-client /etc/trusttunnel/client.toml; \
else \
  echo '[trusttunnel] no client binary at /usr/local/bin/trusttunnel-client — port 5600 will refuse connections'; \
  echo '[trusttunnel] mount the upstream Linux build and a client.toml to enable.'; \
  exec sleep infinity; \
fi"]
