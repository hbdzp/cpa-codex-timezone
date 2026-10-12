#!/usr/bin/env bash
# End-to-end check: real CPA + this plugin + a mock Codex upstream.
#
#   CPA_BIN=/path/to/cli-proxy-api ./scripts/e2e/run.sh
#
# Build CPA from https://github.com/router-for-me/CLIProxyAPI (v8.0.26) with
#   go build -o cli-proxy-api ./cmd/server
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$ROOT/.e2e"
CPA_BIN="${CPA_BIN:-cli-proxy-api}"
BASE="http://127.0.0.1:48317"
CLIENT="Authorization: Bearer e2e-client-key"
MGMT="Authorization: Bearer e2e-management-key"
PLUGIN="$(ls "$ROOT"/bin/codex-timezone.* | head -n1)"

command -v "$CPA_BIN" >/dev/null || { echo "CPA binary not found: set CPA_BIN" >&2; exit 1; }

rm -rf "$WORK" && mkdir -p "$WORK/plugins"
cp "$ROOT/scripts/e2e/config.yaml" "$WORK/config.yaml"
cp "$PLUGIN" "$WORK/plugins/"
cd "$WORK"

python3 "$ROOT/scripts/e2e/mock_upstream.py" --log upstream.jsonl &
MOCK_PID=$!
"$CPA_BIN" -config config.yaml -local-model >cpa.log 2>&1 &
CPA_PID=$!
trap 'kill $CPA_PID $MOCK_PID 2>/dev/null || true' EXIT

for _ in $(seq 1 60); do
  curl -sf -o /dev/null "$BASE/v0/resource/plugins/codex-timezone/ui" && break
  sleep 0.5
done

fail() { echo "FAIL: $*" >&2; tail -n 30 cpa.log >&2; exit 1; }
post() { curl -sf "$BASE$1" -H "$CLIENT" -H 'Content-Type: application/json' -d "$2" >/dev/null || fail "POST $1"; }
last_body() { tail -n1 upstream.jsonl; }

post /v1/responses '{"model":"gpt-5.5","stream":false,"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <current_date>2000-01-01</current_date>\n  <timezone>America/Los_Angeles</timezone>\n</environment_context>"}]}]}'
last_body | grep -q '<timezone>Asia/Shanghai</timezone>' || fail "Codex environment_context timezone not rewritten"
last_body | grep -q '2000-01-01' && fail "current_date not rewritten"
echo "ok   Codex <environment_context> rewritten"

post /v1/chat/completions '{"model":"gpt-5.5","messages":[{"role":"user","content":"what time is it?"}]}'
last_body | grep -q '"role": "developer"' || fail "developer note not injected for chat completions"
echo "ok   developer note injected for chat completions"

post /v1/chat/completions '{"model":"o3-mock","messages":[{"role":"user","content":"untouched"}]}'
last_body | grep -q 'Current timezone' && fail "non-matching model was modified"
echo "ok   non-matching model passed through"

[ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/v0/management/codex-timezone/status")" = 401 ] || fail "status must require the management key"
curl -sf -X PATCH "$BASE/v8/management/config/plugins/configs/codex-timezone" -H "$MGMT" -H 'Content-Type: application/json' \
  -d '{"timezone":"Europe/London"}' >/dev/null || fail "PATCH config"
for _ in $(seq 1 20); do
  curl -sf "$BASE/v0/management/codex-timezone/status" -H "$MGMT" | grep -q '"timezone":"Europe/London"' && break
  sleep 0.5
done
curl -sf "$BASE/v0/management/codex-timezone/status" -H "$MGMT" | grep -q '"timezone":"Europe/London"' || fail "hot reload did not apply"
echo "ok   config saved through management API and hot-reloaded"

echo "all e2e checks passed"
