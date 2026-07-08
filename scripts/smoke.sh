#!/usr/bin/env bash
# End-to-end smoke test against a scratch DB. Safe to re-run: drops any
# leftover food_smoke DB first (a stale DB from a previous failed run
# otherwise makes createdb fail — the finance lesson).
#
# Usage: scripts/smoke.sh
# Requires: createdb/dropdb/psql on PATH, curl, jq, go.

set -uo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

DB_NAME="food_smoke"
DB_URL="postgres://localhost:5432/${DB_NAME}?sslmode=disable"
API_KEY="smoketestkey"
PORT="8199"
BASE="http://localhost:${PORT}/api"
DAY="$(date +%Y-%m-%d)"
SERVER_PID=""
FAILED=0

BIN="$(mktemp -d)/food-smoke-server"

cleanup() {
  if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null
    wait "$SERVER_PID" 2>/dev/null
  fi
  rm -f "$BIN"
  dropdb --if-exists "$DB_NAME" >/dev/null 2>&1
}
trap cleanup EXIT

pass() { echo "PASS: $1"; }
fail() { echo "FAIL: $1"; FAILED=1; }

# jq-based assertion: $1 = label, $2 = json, $3 = jq filter, $4 = expected value
assert_eq() {
  local label="$1" json="$2" filter="$3" expected="$4" got
  got="$(echo "$json" | jq -r "$filter" 2>/dev/null)"
  if [ "$got" = "$expected" ]; then
    pass "$label"
  else
    fail "$label (expected $expected, got $got)"
  fi
}

echo "== setting up scratch DB =="
dropdb --if-exists "$DB_NAME" >/dev/null 2>&1
if createdb "$DB_NAME"; then
  pass "createdb $DB_NAME"
else
  fail "createdb $DB_NAME"
  exit 1
fi

echo "== building server =="
if go build -o "$BIN" ./cmd/server; then
  pass "go build"
else
  fail "go build"
  exit 1
fi

echo "== starting server on :$PORT =="
# Build a real binary (not `go run`, which execs a child process under a
# different PID that survives `kill $SERVER_PID` and leaks a DB connection,
# blocking the next dropdb) so cleanup can actually terminate the server.
DATABASE_URL="$DB_URL" FOOD_API_KEY="$API_KEY" PORT="$PORT" \
  "$BIN" >/tmp/food-smoke-server.log 2>&1 &
SERVER_PID=$!

ready=0
for i in $(seq 1 30); do
  if curl -s -o /dev/null -w '%{http_code}' "$BASE/health" 2>/dev/null | grep -q 200; then
    ready=1
    break
  fi
  sleep 0.5
done
if [ "$ready" = "1" ]; then
  pass "server started"
else
  fail "server started (see /tmp/food-smoke-server.log)"
  cat /tmp/food-smoke-server.log
  exit 1
fi

auth=(-H "Authorization: Bearer $API_KEY")

echo "== health =="
health="$(curl -s "$BASE/health")"
assert_eq "health.ok" "$health" '.ok' "true"

echo "== settings (defaults) =="
settings="$(curl -s "${auth[@]}" "$BASE/settings")"
assert_eq "settings.calorie_target default" "$settings" '.calorie_target' "1800"
assert_eq "settings.protein_target_mg default" "$settings" '.protein_target_mg' "165000"

echo "== manual meal: score-72 worked example =="
# 500 kcal hard_yes + 300 kcal soft_no + 200 kcal soft_yes
# score = (500*100 + 300*25 + 200*75) / 1000 = 72 (ARCHITECTURE.md worked example)
meal_body=$(cat <<JSON
{
  "day": "$DAY", "slot": "dinner", "description": "smoke test meal", "input_kind": "manual",
  "items": [
    {"name":"Salmon and greens","quantity":"1 plate","fraction_pct":100,"calories":500,
     "protein_mg":40000,"carbs_mg":10000,"fat_mg":20000,"tier":"hard_yes","source":"manual","confidence":"high"},
    {"name":"White roll","quantity":"1 roll","fraction_pct":100,"calories":300,
     "protein_mg":6000,"carbs_mg":50000,"fat_mg":6000,"tier":"soft_no","source":"manual","confidence":"high"},
    {"name":"Greek yogurt","quantity":"1 cup","fraction_pct":100,"calories":200,
     "protein_mg":15000,"carbs_mg":15000,"fat_mg":6000,"tier":"soft_yes","source":"manual","confidence":"high"}
  ]
}
JSON
)
meal="$(curl -s -X POST "${auth[@]}" -H "Content-Type: application/json" -d "$meal_body" "$BASE/meals")"
meal_id="$(echo "$meal" | jq -r '.id')"
if [ -n "$meal_id" ] && [ "$meal_id" != "null" ]; then
  pass "create meal"
else
  fail "create meal (response: $meal)"
fi

echo "== day summary =="
day="$(curl -s "${auth[@]}" "$BASE/day/$DAY")"
assert_eq "day.calories" "$day" '.calories' "1000"
assert_eq "day.score" "$day" '.score' "72"

echo "== weight upsert =="
weight_body="{\"day\":\"$DAY\",\"weight_g\":85003,\"note\":\"smoke\"}"
weight="$(curl -s -X POST "${auth[@]}" -H "Content-Type: application/json" -d "$weight_body" "$BASE/weights")"
assert_eq "weight.weight_g" "$weight" '.weight_g' "85003"

weights="$(curl -s "${auth[@]}" "$BASE/weights?start=$DAY&end=$DAY")"
assert_eq "weights list length" "$weights" '. | length' "1"

echo "== range summary =="
range="$(curl -s "${auth[@]}" "$BASE/range?start=$DAY&end=$DAY")"
assert_eq "range[0].score" "$range" '.[0].score' "72"

echo "== export =="
export_headers="$(curl -s -D - -o /tmp/food-smoke-export.json "${auth[@]}" "$BASE/export")"
if echo "$export_headers" | grep -qi 'Content-Disposition: attachment; filename=food-export-'; then
  pass "export Content-Disposition header"
else
  fail "export Content-Disposition header (headers: $export_headers)"
fi
export_json="$(cat /tmp/food-smoke-export.json)"
if echo "$export_json" | jq -e . >/dev/null 2>&1; then
  pass "export is valid JSON"
else
  fail "export is valid JSON"
fi
assert_eq "export.meals length" "$export_json" '.meals | length' "1"
assert_eq "export.weights length" "$export_json" '.weights | length' "1"
assert_eq "export.settings.calorie_target" "$export_json" '.settings.calorie_target' "1800"
rm -f /tmp/food-smoke-export.json

echo "=========================="
if [ "$FAILED" = "0" ]; then
  echo "ALL PASS"
  exit 0
else
  echo "SOME TESTS FAILED"
  exit 1
fi
