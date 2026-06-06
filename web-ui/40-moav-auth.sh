#!/bin/sh
# Generates the dashboard basic-auth config consumed by nginx.conf
# (include /etc/nginx/moav-auth.conf). Runs on every container start via the
# official nginx image's /docker-entrypoint.d/ hook.
#
# Credentials come from MOAV_DASHBOARD_USER / MOAV_DASHBOARD_PASS. We prefer the
# values in the mounted .env file (/etc/moav/.env) over the baked-in container
# env, so a plain restart re-applies a password the user just set in the
# dashboard (matching how proxy-core re-reads .env).
set -eu

AUTH_FILE=/etc/nginx/moav-auth.conf
HTPASSWD=/etc/nginx/.htpasswd
ENVF=/etc/moav/.env

USER_VAL="${MOAV_DASHBOARD_USER:-}"
PASS_VAL="${MOAV_DASHBOARD_PASS:-}"

if [ -f "$ENVF" ]; then
    fu=$(grep -E '^MOAV_DASHBOARD_USER=' "$ENVF" 2>/dev/null | tail -1 | cut -d= -f2- | tr -d '"' || true)
    fp=$(grep -E '^MOAV_DASHBOARD_PASS=' "$ENVF" 2>/dev/null | tail -1 | cut -d= -f2- | tr -d '"' || true)
    [ -n "${fu:-}" ] && USER_VAL="$fu"
    [ -n "${fp:-}" ] && PASS_VAL="$fp"
fi

# A password alone enables auth; default the username to "moav".
if [ -n "$PASS_VAL" ]; then
    [ -z "$USER_VAL" ] && USER_VAL="moav"
    htpasswd -nbB "$USER_VAL" "$PASS_VAL" > "$HTPASSWD"
    cat > "$AUTH_FILE" <<EOF
auth_basic "moav-client dashboard";
auth_basic_user_file $HTPASSWD;
EOF
    echo "[moav-auth] dashboard basic-auth ENABLED (user '$USER_VAL')"
else
    echo "auth_basic off;" > "$AUTH_FILE"
    echo "[moav-auth] dashboard basic-auth disabled (set a dashboard password to enable)"
fi
