#!/bin/sh
# Point nginx's /api upstream at the configured API_PORT. The single source is
# .env (same var proxy-core and the compose mapping read); nginx.conf ships with
# the default :8088, so we only rewrite when the operator overrode it. Runs
# before 40-moav-auth.sh on every container start (docker-entrypoint.d).
set -e
PORT="${API_PORT:-8088}"
CONF=/etc/nginx/conf.d/default.conf
if [ "$PORT" != "8088" ] && [ -f "$CONF" ]; then
    sed -i "s#proxy-core:8088#proxy-core:${PORT}#g" "$CONF"
    echo "[moav] api upstream -> proxy-core:${PORT}"
fi
