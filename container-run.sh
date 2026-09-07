#!/usr/bin/env bash
# Build and run aviagent with Apple's `container` CLI (replaces docker-compose).
# Usage: ./container-run.sh {up|down|logs}
set -euo pipefail

IMAGE="aviagent"
NAME="aviagent"
PORT="${PORT:-8088}"
ENV_FILE="${ENV_FILE:-.env}"
CMD="${1:-up}"

case "$CMD" in
  up)
    container build -t "$IMAGE" .
    container rm -f "$NAME" >/dev/null 2>&1 || true
    mkdir -p data/sessions
    args=(-d --name "$NAME" -p "$PORT:8088" -e GIN_MODE=release \
      -v "$(pwd)/config.yaml:/etc/aviagent/config.yaml:ro" \
      -v "$(pwd)/data/sessions:/web/data/sessions")
    [ -f "$ENV_FILE" ] && args+=(--env-file "$ENV_FILE")
    container run "${args[@]}" "$IMAGE"
    echo "aviagent running at http://localhost:$PORT"
    ;;
  down)
    container rm -f "$NAME" >/dev/null 2>&1 || true
    ;;
  logs)
    container logs -f "$NAME"
    ;;
  *)
    echo "Usage: $0 {up|down|logs}" >&2
    exit 1
    ;;
esac
