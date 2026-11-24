#
# MasterDNS sidecar
# Downloads the upstream masterking32/MasterDnsVPN client binary and runs it
# against the m.t7d.my MoaV tunnel. Exposes SOCKS5 on 5300.
#
# Config files are mounted at /etc/masterdns/ by docker-compose
# (./data/sidecar-configs/masterdns/). moav-client generates them on startup
# from the parameters in data/beezhan-t7d/masterdns-instructions.txt.
#
FROM debian:bookworm-slim AS fetch

ARG MASTERDNS_VERSION=v2026.11.18.210322-7198b45

RUN apt-get update && apt-get install -y --no-install-recommends \
      curl tar ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /work
RUN set -eux; \
    ARCH=$(uname -m); \
    case "$ARCH" in \
      x86_64)  PKG="MasterDnsVPN_Client_Linux_AMD64" ;; \
      aarch64) PKG="MasterDnsVPN_Client_Linux_ARM64" ;; \
      *) echo "unsupported arch $ARCH"; exit 1 ;; \
    esac; \
    BASE="https://github.com/masterking32/MasterDnsVPN/releases/download/${MASTERDNS_VERSION}"; \
    echo "fetching $BASE/${PKG}.tar.gz"; \
    curl -fL --retry 3 --retry-delay 2 "$BASE/${PKG}.tar.gz" -o pkg.tar.gz; \
    # Verify against the SHA256SUMS.txt of the same release (as the MoaV server does).
    curl -fsSL --retry 3 --retry-delay 2 "$BASE/SHA256SUMS.txt" -o SHA256SUMS.txt; \
    EXPECTED=$(awk -v f="${PKG}.tar.gz" '{n=split($2,a,"/"); if (a[n]==f) {print $1; exit}}' SHA256SUMS.txt); \
    [ -n "$EXPECTED" ] || { echo "no checksum for ${PKG}.tar.gz in SHA256SUMS.txt" >&2; exit 1; }; \
    ACTUAL=$(sha256sum pkg.tar.gz | cut -d' ' -f1); \
    [ "$EXPECTED" = "$ACTUAL" ] || { echo "SHA256 mismatch for ${PKG}.tar.gz: expected $EXPECTED, got $ACTUAL" >&2; exit 1; }; \
    echo "SHA256 OK ($ACTUAL)"; \
    rm -f SHA256SUMS.txt; \
    tar xzf pkg.tar.gz; \
    ls -la; \
    # the upstream archive contains either a single binary or a folder; flatten.
    if [ -d "$PKG" ]; then \
      cp "$PKG"/* /work/; \
    fi; \
    # find any executable that looks like the client.
    for f in MasterDnsVPN_Client masterdns_client client masterdns_vpn; do \
      if [ -f "$f" ]; then chmod +x "$f"; mv "$f" /work/masterdns; break; fi; \
    done; \
    if [ ! -f /work/masterdns ]; then \
      # last resort — first executable file in the archive.
      cand=$(find . -maxdepth 2 -type f -perm -u+x | head -1); \
      [ -n "$cand" ] && cp "$cand" /work/masterdns; \
    fi; \
    test -x /work/masterdns

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=fetch /work/masterdns /usr/local/bin/masterdns
VOLUME ["/etc/masterdns"]
EXPOSE 5300/tcp
WORKDIR /etc/masterdns
ENTRYPOINT ["/bin/sh","-c","echo '[masterdns] waiting for /etc/masterdns/client_config.toml ...'; while [ ! -s /etc/masterdns/client_config.toml ]; do sleep 1; done; echo '[masterdns] starting'; exec /usr/local/bin/masterdns client_config.toml"]
