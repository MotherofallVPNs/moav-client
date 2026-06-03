#
# SNI Spoofing sidecar
# Wraps aleskxyz/SNI-Spoofing-Go. The upstream binary is one-process-per-mapping
# (listen :port → connect host:port with a decoy SNI), so we ship a tiny
# entrypoint that reads /etc/sni-spoof/mappings.json (written by proxy-core
# from per-endpoint sources[].spoof config) and spawns one instance per row.
#
# Profile: --profile sni-spoof
# Capabilities: NET_ADMIN — the upstream tool sets BPF / raw sockets
#                          for low-level CH injection.
#
FROM golang:1.25-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN git clone --depth=1 https://github.com/aleskxyz/SNI-Spoofing-Go.git \
    && cd SNI-Spoofing-Go \
    && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/sni-spoofing .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates iptables iproute2 python3 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/sni-spoofing /usr/local/bin/sni-spoofing
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
VOLUME ["/etc/sni-spoof"]
ENTRYPOINT ["/entrypoint.sh"]
