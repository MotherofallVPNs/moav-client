#!/bin/sh
set -e

CONF=/etc/sni-spoof/sni-spoof.json
echo "[sni-spoof] waiting for $CONF ..."
while [ ! -s "$CONF" ]; do sleep 1; done
echo "[sni-spoof] config detected"

# Fan out — one sni-spoofing process per mapping. We use Python because
# debian-slim's busybox doesn't have jq.
python3 - "$CONF" <<'PY'
import json, os, signal, subprocess, sys, time
mappings = json.load(open(sys.argv[1]))

procs = []
for m in mappings:
    listen = m["listen"]                  # ":40443"
    connect = m["connect"]                # "1.2.3.4:443"
    fake_sni = m["fake_sni"]              # "hcaptcha.com"
    utls = m.get("utls", "chrome")
    args = [
        "/usr/local/bin/sni-spoofing",
        "-listen", listen,
        "-connect", connect,
        "-fake-sni", fake_sni,
        "-utls", utls,
    ]
    for extra in ("fake_repeat", "fake_delay", "sni_chunk"):
        if v := m.get(extra):
            args.extend([f"-{extra.replace('_','-')}", str(v)])
    print(f"[sni-spoof] spawn  listen={listen}  →  {connect}  (fake SNI={fake_sni}, utls={utls})", flush=True)
    p = subprocess.Popen(args)
    procs.append(p)

def shutdown(*_):
    for p in procs: p.terminate()
    for p in procs:
        try: p.wait(timeout=5)
        except subprocess.TimeoutExpired: p.kill()
    sys.exit(0)

signal.signal(signal.SIGTERM, shutdown)
signal.signal(signal.SIGINT, shutdown)

# Supervisor loop — if any child dies, restart the lot.
while True:
    time.sleep(2)
    for p in procs:
        if p.poll() is not None:
            print(f"[sni-spoof] child {p.args[2]} died (code={p.returncode}); restarting all", flush=True)
            for q in procs: q.kill()
            os.execv(sys.executable, [sys.executable, *sys.argv])
PY
