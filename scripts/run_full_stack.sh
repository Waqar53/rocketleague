#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

cleanup() {
  for pid in ${PIDS:-}; do
    kill "$pid" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT INT TERM

PIDS=""

cd "$ROOT_DIR/backend"
GAME_ADDR=:9003 MATCH_DURATION_SEC=300 go run ./cmd/gameserver > /tmp/velocity_gameserver.log 2>&1 &
PIDS+=" $!"
MATCHMAKER_ADDR=:9001 GAME_WS_ADDR=ws://localhost:9003/ws go run ./cmd/matchmaker > /tmp/velocity_matchmaker.log 2>&1 &
PIDS+=" $!"
GATEWAY_ADDR=:9000 MATCHMAKER_HTTP=http://localhost:9001 go run ./cmd/gateway > /tmp/velocity_gateway.log 2>&1 &
PIDS+=" $!"
TELEMETRY_ADDR=:9002 go run ./cmd/telemetry > /tmp/velocity_telemetry.log 2>&1 &
PIDS+=" $!"

cd "$ROOT_DIR"
python3 -m http.server 5173 --directory "$ROOT_DIR/client" > /tmp/velocity_web.log 2>&1 &
PIDS+=" $!"

for i in {1..60}; do
  if curl -sf http://localhost:9000/health >/dev/null \
    && curl -sf http://localhost:9001/health >/dev/null \
    && curl -sf http://localhost:9002/health >/dev/null \
    && curl -sf http://localhost:9003/health >/dev/null \
    && curl -sf http://localhost:5173 >/dev/null; then
    break
  fi
  sleep 0.5
done

echo "Project Velocity is running"
echo "Play client: http://localhost:5173"
echo "Gateway API: http://localhost:9000"
echo "Telemetry metrics: http://localhost:9002/metrics"
echo "Press Ctrl+C to stop"

wait
