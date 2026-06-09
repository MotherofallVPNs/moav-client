#
# AmneziaWG sidecar
#
# Builds amnezia-vpn/amneziawg-go (userspace WireGuard with Amnezia
# obfuscation: Jc/Jmin/Jmax/S1/S2/H1..H4) and amnezia-vpn/amneziawg-tools
# (the `awg` / `awg-quick` CLIs needed to load the obfuscation params).
# Then runs `awg-quick up` against /etc/amneziawg/awg0.conf and exposes
# a SOCKS5 listener on 5500 that egresses through the configured awg0
# interface (kernel default route).
#
# Requires NET_ADMIN + /dev/net/tun on the container (set in docker-compose).
# Config is mounted at /etc/amneziawg/awg0.conf — moav-client writes this on
# startup from source_path declared in config.yaml -> sidecars.amneziawg.config.
#
FROM golang:1.25-bookworm AS gobuild
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
# Go-module fetch resilience — fall through proxies on a 403 (CDN rate-limit).
ARG GOPROXY="https://proxy.golang.org|https://goproxy.cn|direct"
ARG GOSUMDB=off
ENV GOPROXY=${GOPROXY} GOSUMDB=${GOSUMDB}
WORKDIR /src
RUN git clone --depth=1 https://github.com/amnezia-vpn/amneziawg-go.git \
    && cd amneziawg-go \
    && CGO_ENABLED=0 GOOS=linux go build -o /out/amneziawg-go .

FROM debian:bookworm AS toolsbuild
RUN apt-get update && apt-get install -y --no-install-recommends \
      git build-essential ca-certificates pkg-config \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN mkdir -p /out \
    && git clone --depth=1 https://github.com/amnezia-vpn/amneziawg-tools.git \
    && cd amneziawg-tools/src \
    && make \
    && { cp wg /out/awg || cp awg /out/awg; } \
    && ls -la /out

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      iproute2 iptables ca-certificates microsocks bash \
    && rm -rf /var/lib/apt/lists/*
COPY --from=gobuild /out/amneziawg-go /usr/local/bin/amneziawg-go
COPY --from=toolsbuild /out/awg /usr/local/bin/awg
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
VOLUME ["/etc/amneziawg"]
EXPOSE 5500/tcp
ENTRYPOINT ["/entrypoint.sh"]
