# Phase 3 — AI pipeline: natural-language parsing, USDA + Open Food Facts

Read `CLAUDE.md` and `ARCHITECTURE.md` in full first (especially → "AI meal
parsing"). Phases 1–2 are built. This phase delivers `POST /api/parse`: casual
text in, validated nutrition draft out. Photos come in phase 4 — but build the
parse loop so a vision message can be dropped in later (the loop takes a
prepared first user message, not just a text string).

## Reference implementations

- `~/projects/journal/internal/ai/claude.go` — hand-rolled Anthropic HTTP
  client (messages API, no SDK). Copy its shape; add tool-use support.
- `~/projects/finance/internal/finance/assistant.go` — the agentic tool loop:
  send messages → if `stop_reason` is `tool_use`, execute tools, append
  `tool_result` blocks, repeat. Mirror this structure closely.

## Tasks

1. `internal/nutrition/fdc.go` — USDA FoodData Central client:
   - `POST https://api.nal.usda.gov/fdc/v1/foods/search?api_key=$FDC_API_KEY`
     with `{"query": q, "pageSize": n, "dataType": ["Foundation","SR Legacy","Branded"]}`.
   - Return per result: fdcId, description, dataType, brand (Branded only),
     serving info, and per-100g core nutrients mapped by nutrient number —
     energy kcal `1008`, protein `1003`, fat `1004`, carbs `1005`, fiber
     `1079`, saturated fat `1258`, sugars `2000`, sodium `1093`. Convert to
     the app's integer units (kcal, mg per 100 g). Keep the raw nutrient list
     too (for `micros`).
   - 10s timeout; on non-200 return an error the tool layer converts into a
     tool_result telling Claude the lookup failed (so it falls back to
     estimating, never crashes the parse).
2. `internal/nutrition/off.go` — Open Food Facts client:
   - `GET https://world.openfoodfacts.org/api/v2/product/{barcode}.json` with
     `User-Agent: food-app/1.0 (personal tracker)`.
   - Return product name, brands, serving_size, and nutriments (`*_100g` and
     `*_serving` keys), converted to integer units; keep raw nutriments JSON.
3. Tests for both clients using `httptest.Server` fixtures — record one real
   response each (FDC search for "salmon", OFF product for a known barcode
   like `0016000275270`) into `testdata/*.json` and assert the unit
   conversions (grams→mg, kJ vs kcal — OFF's `energy-kcal_100g` is already
   kcal; do not use `energy_100g`, which is kJ).
4. `internal/ai/client.go` — Anthropic messages client with tool-use and
   image content-block support (base64 source blocks), model from `AI_MODEL`
   env, default `claude-sonnet-5`, `max_tokens` 4096.
5. `internal/ai/parser.go` — the parse loop per ARCHITECTURE.md:
   - System prompt embeds `docs/diet-framework.md` **via `go:embed`** (never
     paraphrase it), the tier enum + `neutral` rule, unit conventions
     (integer kcal / mg / grams / fraction_pct), and behavior rules: prefer
     `usda_search` for whole foods and common dishes; barcodes go to
     `off_barcode`; estimate only when lookups fail or the food is a
     composite homemade dish; state every assumption in `notes`; hints like
     "I had half" set `fraction_pct`, never pre-scaled nutrition; when the
     user is ambiguous, pick the most likely reading and note the assumption
     — never ask questions (this is a one-shot parse, not a chat).
   - Tools: `usda_search`, `off_barcode`, `record_meal` (terminal; schema =
     ParseResult items + notes). Hard cap 8 rounds, then send a final user
     message demanding `record_meal`.
   - Return ParseResult with `ai_model` and `ai_raw` (the full final
     assistant turn + tool trace, JSON-marshaled).
6. Service + handler: `POST /api/parse {text, day}` → run loop → run
   `ValidateItem` on every item (Atwater warnings appended to `notes`,
   confidence clamped to `low` on failures) → return the draft. Return 503
   with a clear message when `ANTHROPIC_API_KEY` is unset (and health should
   now report `"ai": true/false` accordingly).
7. Parser unit test with a **fake** AI transport (scripted tool_use →
   tool_result → record_meal exchange) asserting the loop mechanics, the
   round cap, and validation wiring. No live-API tests in `go test`.
8. A manual live check documented in the prompt output (not a Go test):
   `curl -s -X POST -H "Authorization: Bearer x" -d '{"text":"two scrambled eggs, a slice of whole wheat toast with olive oil, and a bowl of blueberries","day":"2026-07-07"}' localhost:8082/api/parse | jq`
   — sane items, eggs/berries/whole-grain tiers correct, USDA sources present.

## Out of scope

Photos, barcode-from-image, PWA.

## Acceptance checklist

- `go build ./... && go test ./...` passes; nutrition + parser tests green.
- The live `curl` above returns 3–4 items with plausible grams/calories,
  correct tiers (all hard_yes here), `source: "usda"` on most items, and
  non-empty `notes` stating portion assumptions.
- A parse of "half a bag of Lays and a beer" yields tier `hard_no` items
  (ultra-processed, alcohol) and `fraction_pct: 50` on the chips.
- Unset `ANTHROPIC_API_KEY` → `/api/parse` returns 503, health shows `"ai": false`.
