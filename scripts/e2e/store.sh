#!/usr/bin/env bash
# Plugin store check: a fresh CPA trusts a self-hosted registry, installs the
# plugin from it, then upgrades to a newer version without restarting.
#
#   CPA_BIN=/path/to/cli-proxy-api ./scripts/e2e/store.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$ROOT/.e2e-store"
STORE="$WORK/store"
CPA_BIN="${CPA_BIN:-cli-proxy-api}"
STORE_URL="http://127.0.0.1:47400"
BASE="http://127.0.0.1:48318/v8/management"
MGMT="Authorization: Bearer e2e-management-key"

command -v "$CPA_BIN" >/dev/null || { echo "CPA binary not found: set CPA_BIN" >&2; exit 1; }

rm -rf "$WORK" && mkdir -p "$STORE"
for v in 0.1.0 0.1.1; do
  make -C "$ROOT" --no-print-directory package VERSION="$v" DIST_DIR="$STORE" >/dev/null
done
python3 "$ROOT/scripts/store/gen_registry.py" --dist "$STORE" --version 0.1.0 --base-url "$STORE_URL" --out "$STORE/registry.json" >/dev/null

sed -e 's/48317/48318/' -e '/^plugins:/,$d' "$ROOT/scripts/e2e/config.yaml" > "$WORK/config.yaml"
printf 'plugins:\n  enabled: true\n  dir: "plugins"\n' >> "$WORK/config.yaml"
cd "$WORK"

python3 -m http.server 47400 --bind 127.0.0.1 --directory "$STORE" >http.log 2>&1 &
HTTP_PID=$!
"$CPA_BIN" -config config.yaml -local-model >cpa.log 2>&1 &
CPA_PID=$!
trap 'kill $CPA_PID $HTTP_PID 2>/dev/null || true' EXIT

for _ in $(seq 1 60); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/config" -H "$MGMT")" = 200 ] && break
  sleep 0.5
done

fail() { echo "FAIL: $*" >&2; tail -n 30 cpa.log >&2; exit 1; }
api() { curl -sf -X "$1" "$BASE$2" -H "$MGMT" -H 'Content-Type: application/json' ${3:+-d "$3"}; }
entry() { api GET /plugins/store | python3 -c "import json,sys; p=[p for p in json.load(sys.stdin)['plugins'] if p['id']=='codex-timezone']; print(json.dumps(p[0]) if p else '')"; }
wait_version() {
  for _ in $(seq 1 20); do
    curl -sf "http://127.0.0.1:48318/v0/management/codex-timezone/status" -H "$MGMT" 2>/dev/null | grep -q "\"version\":\"$1\"" && return 0
    sleep 0.5
  done
  return 1
}

api PUT /config/plugins/store-sources "[\"$STORE_URL/registry.json\"]" >/dev/null || fail "add store source"
api PUT /config/plugins/store-auth "[{\"match\":\"$STORE_URL/\",\"apply-to\":[\"registry\",\"artifact\"],\"type\":\"none\",\"allow-insecure\":true}]" >/dev/null || fail "add allow-insecure rule"
sleep 1

E="$(entry)"; [ -n "$E" ] || fail "plugin not listed in store"
SOURCE_ID="$(echo "$E" | python3 -c 'import json,sys; print(json.load(sys.stdin)["source_id"])')"
echo "ok   store lists codex-timezone from $SOURCE_ID"

api POST "/plugins/store/codex-timezone/install?source=$SOURCE_ID" >/dev/null || fail "install 0.1.0"
wait_version 0.1.0 || fail "0.1.0 not loaded after install"
echo "ok   installed 0.1.0 from store and hot-loaded"

python3 "$ROOT/scripts/store/gen_registry.py" --dist "$STORE" --version 0.1.1 --base-url "$STORE_URL" --merge "$STORE/registry.json" --out "$STORE/registry.json" >/dev/null
entry | grep -q '"update_available": true' || fail "update not detected"
api POST "/plugins/store/codex-timezone/install?source=$SOURCE_ID" >/dev/null || fail "install 0.1.1"
wait_version 0.1.1 || fail "0.1.1 not loaded after upgrade"
echo "ok   upgraded to 0.1.1 through the store without restart"

echo "all store checks passed"
