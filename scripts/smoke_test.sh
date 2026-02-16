#!/usr/bin/env bash
set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:9000}"

json_get() {
  python3 -c 'import json,sys; print(json.load(sys.stdin).get(sys.argv[1],""))' "$1"
}

wait_for_gateway() {
  for _ in {1..40}; do
    if curl -sf "$GATEWAY_URL/health" >/dev/null; then
      return 0
    fi
    sleep 0.5
  done
  echo "[smoke] gateway health check timed out at $GATEWAY_URL/health"
  return 1
}

wait_for_gateway

echo "[smoke] creating guest player A"
A_AUTH=$(curl -sS -X POST "$GATEWAY_URL/v1/auth/guest" -H 'Content-Type: application/json' -d '{"display_name":"PilotA"}')
A_TOKEN=$(echo "$A_AUTH" | json_get token)

echo "[smoke] creating guest player B"
B_AUTH=$(curl -sS -X POST "$GATEWAY_URL/v1/auth/guest" -H 'Content-Type: application/json' -d '{"display_name":"PilotB"}')
B_TOKEN=$(echo "$B_AUTH" | json_get token)

A_JOIN=$(curl -sS -X POST "$GATEWAY_URL/v1/matchmaking/join" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $A_TOKEN" \
  -d '{"region":"us-east","playlist":"ranked-1v1","mmr":1200}')
A_TICKET=$(echo "$A_JOIN" | json_get ticket_id)

B_JOIN=$(curl -sS -X POST "$GATEWAY_URL/v1/matchmaking/join" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $B_TOKEN" \
  -d '{"region":"us-east","playlist":"ranked-1v1","mmr":1210}')
B_TICKET=$(echo "$B_JOIN" | json_get ticket_id)

if [[ -z "$A_TICKET" || -z "$B_TICKET" ]]; then
  echo "[smoke] failed to create tickets"
  echo "A_JOIN=$A_JOIN"
  echo "B_JOIN=$B_JOIN"
  exit 1
fi

poll_status() {
  local token="$1"
  local ticket="$2"
  curl -sS "$GATEWAY_URL/v1/matchmaking/poll?ticket_id=$ticket" -H "Authorization: Bearer $token"
}

echo "[smoke] polling for match assignment"
for i in {1..20}; do
  A_POLL=$(poll_status "$A_TOKEN" "$A_TICKET")
  A_STATUS=$(echo "$A_POLL" | json_get status)
  if [[ "$A_STATUS" == "matched" ]]; then
    MATCH_ID=$(echo "$A_POLL" | python3 -c 'import json,sys; print((json.load(sys.stdin).get("assignment") or {}).get("match_id",""))')
    SERVER_ADDR=$(echo "$A_POLL" | python3 -c 'import json,sys; print((json.load(sys.stdin).get("assignment") or {}).get("server_addr",""))')
    echo "[smoke] matched: match_id=$MATCH_ID server=$SERVER_ADDR"
    exit 0
  fi
  sleep 0.5
  echo "[smoke] waiting... status=$A_STATUS"
done

echo "[smoke] timeout waiting for match"
exit 1
