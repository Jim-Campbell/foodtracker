# Phase 2 — Core API: domain types, CRUD, summaries, score math

Read `CLAUDE.md` and `ARCHITECTURE.md` in full first. Phase 1 (scaffold) is
already built. This phase makes the whole app usable via `curl` — meals,
weights, settings, day/range summaries, and the quality score — with **no AI
involved yet** (`input_kind: "manual"` meals created directly).

## Reference implementations

- `~/projects/finance/internal/finance/` — how types / Store interface /
  service split is laid out
- `~/projects/finance/internal/db/store.go` — pgx query patterns
- `~/projects/finance/internal/api/handler.go` — JSON handler idioms,
  error responses

## Tasks

1. `internal/food/types.go`: `Meal`, `MealItem`, `Weight`, `Settings`,
   `DaySummary`, `RangeDay`, `ParseResult` mirroring the schema and API
   shapes in ARCHITECTURE.md exactly. JSON tags in snake_case matching the
   API doc. All amounts int64/int — **no float fields**.
2. `internal/food/score.go`:
   - `EatenValue(v int64, fractionPct int) int64` → `v * int64(fractionPct) / 100`.
   - `DayScore(items []MealItem) (int, bool)` — calorie-weighted mean of tier
     values (hard_yes=100, soft_yes=75, neutral=50, soft_no=25, hard_no=0)
     using **as-eaten** calories as weights; zero-calorie items excluded;
     `false` when no calorie-bearing items. Integer math, multiply first.
   - `ValidateItem(it MealItem) []string` — enum checks, range checks, and the
     Atwater check from ARCHITECTURE.md → Validation (±30%; returns warnings,
     never errors, except invalid enums/negative amounts which are errors).
3. `internal/food/store.go`: `Store` interface (CRUD for meals+items in a
   transaction, weights upsert-by-day, settings get/put, day + range queries).
   `internal/db/store.go` implements it with pgx. Range summary should be one
   SQL query with `GROUP BY day` for the sums; compute scores in Go from a
   second query of the day's items (or a single joined query — keep it simple
   and correct over clever).
4. `internal/food/service.go`: thin service wiring validation + score
   computation into responses. `DaySummary` includes `calories_remaining`
   (may be negative) and `protein_remaining_mg` computed against settings.
5. `internal/api/`: implement every endpoint in ARCHITECTURE.md → API
   **except** `/api/parse`, `/api/photos`, `/api/analyze-photo`, and
   `/api/export` (later phases). `PUT /api/meals/{id}` deletes and re-inserts
   items in the same transaction. Dates are `YYYY-MM-DD` strings end to end.
6. Tests (`internal/food/score_test.go`, `internal/food/service_test.go` with
   a fake store):
   - The worked example from ARCHITECTURE.md → Quality score (must equal 72).
   - Fraction math: 610-kcal item at fraction 50 contributes 305; at 33 → 201
     (integer division).
   - Zero-calorie-only day → no score.
   - Atwater warning triggers (e.g. claims 1000 kcal but macros say 400).
   - Remaining-vs-target arithmetic, including negative remaining.

## Out of scope

No AI, no nutrition APIs, no photos, no PWA changes.

## Acceptance checklist

- `go build ./... && go test ./...` passes.
- Against a scratch DB, with `curl` (key `x`):
  - `POST /api/meals` with two items (one hard_yes ~500 kcal, one soft_no
    ~300 kcal, plus a 200-kcal soft_yes) then `GET /api/day/<day>` → totals
    match hand-added numbers and `score` is 72.
  - Set an item's `fraction_pct` to 50 via `PUT /api/meals/{id}` → day
    calories drop by half that item's calories.
  - `POST /api/weights` twice for the same day → one row, latest value.
  - `GET /api/range?start=&end=` returns only days that have data.
  - `PUT /api/settings` with a new calorie target changes
    `calories_remaining` on `GET /api/day/...`.
- `dropdb food_smoke` at the end.
