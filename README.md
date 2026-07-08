# Food — a personal food + weight tracker

Personal food and weight tracker for Jim, built to support weight loss and
health improvement with near-zero logging friction. Sibling app to
`~/projects/journal` and `~/projects/finance` — same stack, same
bearer-key-auth PWA pattern, same "server is the source of truth" philosophy.
Single user, no accounts.

Meals are logged in casual natural language — typed, dictated, or
photographed (label, package, barcode, or plate) — and Claude parses them
into items, looks up nutrition (USDA FoodData Central, Open Food Facts for
barcodes, AI estimate as a last resort), and scores each item against
`docs/diet-framework.md`. Nothing the AI produces is trusted blindly: every
draft is shown for confirmation before it's saved, and every derived number
(as-eaten macros, day totals, the composite quality score) is recomputed in
Go, never taken from the model's arithmetic.

## Core loop

1. Describe what you ate — type it, dictate it, or snap a photo, optionally
   with a hint ("I had half of this").
2. Claude parses it into items, resolves nutrition, and classifies each item
   against the diet framework.
3. The PWA shows a draft preview — items, grams, calories, macros — adjust
   portions with fraction chips if needed, then Save.
4. Today leads with **calories remaining**, **protein to go**, and the day's
   **quality score**; Trends shows daily bars for the week/month plus the
   weight chart.

## Screenshots

_(placeholder — add screenshots of Today, the draft dialog, and Trends here)_

## Running locally

```sh
createdb food
cp .env.example .env   # edit DATABASE_URL, FOOD_API_KEY, ANTHROPIC_API_KEY, FDC_API_KEY
go run ./cmd/server     # migrations run automatically
open http://localhost:8082
```

The PWA asks for the API key once (Add to Home Screen on iPhone for the full
app feel) and stores it in localStorage.

Run the smoke test any time to sanity-check a change end-to-end against a
scratch DB (never the cloud DB):

```sh
scripts/smoke.sh
```

## Environment

| Variable | Required | Notes |
|---|---|---|
| `DATABASE_URL` | yes | Postgres connection string |
| `FOOD_API_KEY` | yes | bearer key the PWA sends as `Authorization: Bearer …` |
| `ANTHROPIC_API_KEY` | for AI parsing | unset → `/api/parse` and `/api/analyze-photo` return 503, Ask-style features hidden |
| `FDC_API_KEY` | for AI parsing | free key from https://api.data.gov/signup/ |
| `AI_MODEL` | no | defaults to `claude-sonnet-5` |
| `PORT` | no | defaults to `8082` |
| `R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET`, `R2_PUBLIC_URL` | for photos | all five must be set together; unset → camera button hidden (`/api/health` reports `photos:false`) |

## Data model highlights

- **No floats anywhere.** Calories are integer kcal; macros/micros are
  integer milligrams; body weight is integer grams; portions are integer
  percent. See `CLAUDE.md` → "Domain invariants" for the full list.
- **Items store full-portion nutrition**; the as-eaten amount is derived
  (`value * fraction_pct / 100`), so re-adjusting a portion never needs
  another AI call.
- **Quality score is computed, never stored** — a calorie-weighted mean of
  per-item tiers, worked example and formula in `ARCHITECTURE.md` → "Quality
  score".
- **Backup**: `GET /api/export` streams the entire DB (settings, weights,
  every meal with items) as one JSON document — Render's disk is ephemeral,
  so this download is how a copy survives. Settings → Export in the PWA.

## Further reading

- [`ARCHITECTURE.md`](ARCHITECTURE.md) — full design: schema, API surface, AI
  parsing pipeline, quality-score algorithm, PWA layout.
- [`docs/diet-framework.md`](docs/diet-framework.md) — the food-quality
  classification the AI parser embeds verbatim in its system prompt.
- [`DEPLOY.md`](DEPLOY.md) — step-by-step Render deployment.
- [`CLAUDE.md`](CLAUDE.md) — instructions and conventions for working on this
  codebase with Claude Code.
