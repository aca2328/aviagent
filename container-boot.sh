#!/usr/bin/env bash
# Keep aviagent running: starts it at login and restarts it if the
# container crashes. Runs under launchd (com.aviagent.boot.plist,
# KeepAlive=true) as a polling supervisor loop — Apple's `container start
# --attach` refuses to attach to an already-running container, so a
# blocking foreground process (the usual launchd-supervision pattern)
# isn't an option here; KeepAlive instead relaunches this loop if it
# ever exits, and the loop itself notices and heals a crashed container
# within one poll interval.
set -uo pipefail
export PATH="/usr/local/bin:$PATH"
cd "$(dirname "$0")"

for i in $(seq 1 60); do
  container system status 2>/dev/null | grep -q '^status *running' && break
  sleep 1
done

is_running() {
  container inspect aviagent 2>/dev/null \
    | python3 -c "import json,sys; d=json.load(sys.stdin); sys.exit(0 if d[0]['status']['state']=='running' else 1)" 2>/dev/null
}

while true; do
  if ! is_running; then
    if container inspect aviagent >/dev/null 2>&1; then
      container start aviagent
    else
      ./container-run.sh up
    fi
  fi
  sleep 15
done
