# CLAUDE.md - Food App

## Project Overview

Personal food + weight tracker for Jim, built to support weight loss and
health improvement. Sibling app to `~/projects/journal` and
`~/projects/finance` — same structure, patterns, and bearer-key auth.
Natural-language and photo meal logging flows through Claude, nutrition is
grounded in USDA FoodData Central / Open Food Facts, and every item is
scored against `docs/diet-framework.md`.

**Read ARCHITECTURE.md before making changes** — it holds the full design:
schema, API surface, AI parsing pipeline, quality-score algorithm, and PWA
layout. Build-phase prompts live in `build-prompts/`.

## Tech Stack

- Go 1.25, chi router, pgx/v5, PostgreSQL (Render-hosted in production)
- Vanilla JS single-file PWA in `pwa/` served by the Go binary
- Anthropic API via hand-rolled client (no SDK) — vision-capable model,
  default `claude-sonnet-5`
- Cloudflare R2 for photos (client copied from journal, same env var names)
- USDA FoodData Central + Open Food Facts REST APIs

## Reference implementations (read these, copy their patterns)

- `~/projects/journal/internal/storage/r2.go` — R2 client, copy near-verbatim
- `~/projects/journal/internal/ai/claude.go` — Anthropic HTTP client shape
- `~/projects/finance/internal/finance/assistant.go` — agentic tool loop
- `~/projects/finance/internal/db/` — migrations runner + pgx store layout
- `~/projects/journal/pwa/index.html` — Web Speech API mic usage
- `~/projects/finance/pwa/index.html` — dialog/`S`-state/`render()` conventions

## Domain invariants (do not break)

- **No floats anywhere in stored data or math.** Calories: integer kcal.
  Macros/micros: integer milligrams (`protein_mg`). Body weight: integer
  grams. Portions: integer percent (`fraction_pct`, 50 = half). Multiply
  before dividing.
- Items store **full-portion** nutrition; as-eaten = `value * fraction_pct / 100`.
  Adjusting a portion never requires an AI call.
- `day` is a user-chosen DATE (default device-local today), not derived from
  the timestamp. All summaries group by `day`.
- Quality tiers (`hard_yes/soft_yes/neutral/soft_no/hard_no`) are stored per
  item; the composite day score (calorie-weighted mean of tier values
  100/75/50/25/0, zero-calorie items excluded) is **computed, never stored**.
  Day-to-day views show only the composite score; the per-item tier breakdown
  lives behind a tap on the score badge.
- The AI never writes to the DB. Parse endpoints return drafts; only explicit
  saves persist. Totals are always recomputed in Go/SQL — never trust AI
  arithmetic. Keep raw AI output in `meals.ai_raw` and full nutrient payloads
  in `meal_items.micros`.
- Imports of AI numbers are sanity-checked (Atwater ±30% calories-vs-macros
  check) before a draft is shown.

## Verifying changes

- `go build ./... && go test ./...`
- Unit tests cover score math and summary totals (`internal/food/*_test.go`)
  and nutrition clients against recorded fixtures (`internal/nutrition/*_test.go`).
- Smoke test against a scratch DB (never the cloud DB):
  `createdb food_smoke && DATABASE_URL=postgres://localhost:5432/food_smoke?sslmode=disable FOOD_API_KEY=x go run ./cmd/server`
  — `dropdb food_smoke` when done.

## Deployment

Render, from the `Dockerfile` (multi-stage `golang:1.25-alpine` → `alpine`,
`GOTOOLCHAIN=local`). Migrations run automatically at startup. Never point a
local server at the production `DATABASE_URL`.

## Environment

Required: `DATABASE_URL`, `FOOD_API_KEY`, `ANTHROPIC_API_KEY`, `FDC_API_KEY`.
Optional: `PORT` (default 8082), `AI_MODEL` (text parses), `AI_VISION_MODEL`
(photo parses; defaults to Sonnet), and the five `R2_*` vars
(photo features off until all are set). See ARCHITECTURE.md → Environment.
