#
# Psiphon sidecar
# Builds the Psiphon tunnel-core ConsoleClient from source and runs it
# against a server-list / config-file mounted at /etc/psiphon/. Exposes
# SOCKS5 on 5400 (matches the address the SidecarManager publishes).
#
FROM golang:1.26-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN git clone --depth=1 https://github.com/Psiphon-Labs/psiphon-tunnel-core.git \
    && cd psiphon-tunnel-core/ConsoleClient \
    && CGO_ENABLED=0 GOOS=linux go build -o /out/psiphon-console-client .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/psiphon-console-client /usr/local/bin/psiphon-tunnel-core
VOLUME ["/etc/psiphon", "/var/lib/psiphon"]
EXPOSE 5400/tcp
WORKDIR /var/lib/psiphon
ENTRYPOINT ["/bin/sh","-c","echo '[psiphon] waiting for /etc/psiphon/psiphon.config ...'; while [ ! -s /etc/psiphon/psiphon.config ]; do sleep 1; done; echo '[psiphon] starting'; exec /usr/local/bin/psiphon-tunnel-core -config /etc/psiphon/psiphon.config -dataRootDirectory /var/lib/psiphon -listenInterface eth0"]
