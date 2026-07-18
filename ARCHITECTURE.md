# Food App — Architecture

Personal food + weight tracker for Jim. Sibling app to `~/projects/journal` and
`~/projects/finance` — same stack, same patterns, same bearer-key auth. The
fundamental goal: support weight loss and health improvement with near-zero
logging friction, grounded nutrition data, and a rich enough database that more
sophisticated analyses can be run later.

**Single user (Jim). No accounts, no profiles.** One bearer API key like journal.

## Core loop

1. Jim describes food in casual natural language — typed, dictated (Web Speech
   API, same as journal), or photographed (label, package, barcode, or plate),
   optionally with a hint like "I had half of this".
2. Claude parses it into food items, looks up nutrition (USDA FoodData Central,
   Open Food Facts for barcodes, AI estimate as fallback), and classifies each
   item against `docs/diet-framework.md`.
3. The PWA shows a **draft preview card** — items, grams, calories, macros —
   Jim adjusts portions with fraction chips if needed and taps Save.
4. Today screen leads with **calories remaining**, **protein to go**, and the
   day's **quality score**. Trends show daily bars for week/month plus the
   weight chart.

## Tech stack

- Go 1.25, chi router, pgx/v5, PostgreSQL (Render-hosted in production)
- Vanilla JS single-file PWA in `pwa/` served by the Go binary
- Anthropic API (vision-capable model, default `claude-sonnet-5`) via a
  hand-rolled client — copy the pattern from `~/projects/journal/internal/ai/claude.go`
  and the tool-loop pattern from `~/projects/finance/internal/finance/assistant.go`
- Cloudflare R2 for photos — copy `~/projects/journal/internal/storage/r2.go`
  verbatim (same env var names, so credentials are reusable)
- USDA FoodData Central REST API (free key from api.data.gov)
- Open Food Facts REST API (no key) for barcode lookups

## Repository structure

```
├── cmd/server/          # entry point, env config, DI (mirror journal/finance)
├── internal/
│   ├── api/             # HTTP handlers, bearer auth + logging middleware, PWA serving
│   ├── db/              # pgx store + migrations (run automatically at startup)
│   ├── food/            # domain types, Store interface, service, score math
│   ├── ai/              # Anthropic client + meal-parsing agent loop
│   ├── nutrition/       # USDA FDC client, Open Food Facts client
│   └── storage/         # R2 client (copied from journal)
├── pwa/                 # single-file vanilla JS PWA (index.html, manifest, sw.js)
├── docs/                # diet-framework.md (AI classification source of truth)
└── Dockerfile           # multi-stage golang:1.25-alpine → alpine (like finance)
```

## Domain invariants (do not break)

- **No floats in stored data or math.** Calories are integer kcal. Macro and
  micro amounts are integer **milligrams** (`protein_mg BIGINT`; 32 g protein =
  32000). Grams of food weight are integer grams. Weight is integer grams
  (`weight_g`; 187.4 lb = 85004 g — the PWA converts lb↔g for display).
  Fractions are integer percent (50 = half).
- **Items store full-portion nutrition; the eaten amount is derived.** Each
  item has `fraction_pct` (default 100). As-eaten value =
  `value * fraction_pct / 100` (integer division). This lets the user re-adjust
  a portion ("actually I had ¾") without another AI call.
- **The day is a user-chosen DATE, not derived from the timestamp.** Default is
  the device-local today; late-night entries can be filed to yesterday. All
  summaries group by `day`.
- **Quality tier is stored per item; the composite score is computed, never
  stored.** Tiers: `hard_yes`, `soft_yes`, `neutral`, `soft_no`, `hard_no`
  (`neutral` = not addressed by the framework). Day-to-day views show only
  the composite score; tapping the score badge opens the day's per-item tier
  breakdown (tier, reason, calorie share). Tiers stay in the DB for later
  analysis.
- **Keep every AI parse's raw output** (`meals.ai_raw JSONB`) and every
  nutrition source's full nutrient payload (`meal_items.micros JSONB`) — the
  point is a database rich enough for future analyses.
- **The AI never writes to the database.** The parse endpoints return a draft;
  only an explicit save request persists anything. Server-side validation
  recomputes all derived numbers and sanity-checks AI output before returning
  a draft (see "Validation" below).

## Quality score

Deterministic Go code (`internal/food/score.go`), not AI:

- Tier values: `hard_yes=100, soft_yes=75, neutral=50, soft_no=25, hard_no=0`.
- Day score = calorie-weighted mean of tier values over the day's as-eaten
  items, rounded to an integer 0–100:
  `score = Σ(eatenCalories_i × tierValue_i) / Σ(eatenCalories_i)`
- Items with 0 as-eaten calories (black coffee, water) are excluded from the
  weighting. A day with no calorie-bearing items has a null score.
- Integer math throughout: multiply first, divide last.

Worked example: 500 kcal salmon+greens dinner (hard_yes) + 300 kcal white-flour
roll (soft_no) + 200 kcal Greek yogurt (soft_yes) →
`(500×100 + 300×25 + 200×75) / 1000 = 72`.

## Database schema (migration 001)

```sql
CREATE TABLE meals (
    id            BIGSERIAL PRIMARY KEY,
    day           DATE NOT NULL,
    eaten_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    slot          TEXT CHECK (slot IN ('breakfast','lunch','dinner','snack')),
    description   TEXT NOT NULL DEFAULT '',      -- raw user input (typed/dictated text or photo hint)
    input_kind    TEXT NOT NULL DEFAULT 'text'
                  CHECK (input_kind IN ('text','voice','photo','barcode','manual')),
    photo_key     TEXT,                          -- R2 object key
    photo_url     TEXT,                          -- R2 public URL
    ai_model      TEXT,                          -- model that produced the parse
    ai_raw        JSONB,                         -- full raw AI response for later analysis
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX meals_day_idx ON meals(day);

CREATE TABLE meal_items (
    id            BIGSERIAL PRIMARY KEY,
    meal_id       BIGINT NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
    position      INT NOT NULL DEFAULT 0,
    name          TEXT NOT NULL,
    brand         TEXT,
    quantity      TEXT NOT NULL DEFAULT '',      -- human description: "1 cup", "2 slices"
    grams         INT,                            -- estimated full-portion weight, NULL if unknown
    fraction_pct  INT NOT NULL DEFAULT 100 CHECK (fraction_pct BETWEEN 1 AND 100),
    -- full-portion nutrition (as-eaten = value * fraction_pct / 100)
    calories      INT NOT NULL DEFAULT 0,        -- kcal
    protein_mg    BIGINT NOT NULL DEFAULT 0,
    carbs_mg      BIGINT NOT NULL DEFAULT 0,
    fat_mg        BIGINT NOT NULL DEFAULT 0,
    fiber_mg      BIGINT NOT NULL DEFAULT 0,
    sat_fat_mg    BIGINT NOT NULL DEFAULT 0,
    sugar_mg      BIGINT NOT NULL DEFAULT 0,
    sodium_mg     BIGINT NOT NULL DEFAULT 0,
    micros        JSONB,                          -- full nutrient payload from source
    tier          TEXT NOT NULL DEFAULT 'neutral'
                  CHECK (tier IN ('hard_yes','soft_yes','neutral','soft_no','hard_no')),
    tier_reason   TEXT NOT NULL DEFAULT '',
    source        TEXT NOT NULL DEFAULT 'ai'
                  CHECK (source IN ('usda','off','ai','label','manual')),
    source_ref    TEXT,                           -- FDC id or barcode
    confidence    TEXT NOT NULL DEFAULT 'medium'
                  CHECK (confidence IN ('high','medium','low'))
);
CREATE INDEX meal_items_meal_idx ON meal_items(meal_id);

CREATE TABLE weights (
    id         BIGSERIAL PRIMARY KEY,
    day        DATE NOT NULL UNIQUE,              -- one weigh-in per day, upsert
    weight_g   INT NOT NULL,
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE settings (                            -- single row, id=1
    id                 INT PRIMARY KEY CHECK (id = 1),
    calorie_target     INT NOT NULL DEFAULT 1800,  -- kcal/day
    protein_target_mg  BIGINT NOT NULL DEFAULT 165000, -- 165 g/day (target band 150-180)
    weight_target_g    INT,                        -- optional goal weight
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO settings (id) VALUES (1);
```

Migration 002 adds **favorites** — reusable meal templates. Items are a JSONB
snapshot (same shape as `meal_items`), not references, so editing or deleting
the original meal never mutates a favorite. Migration 003 makes names unique
(case-insensitive); `POST /api/favorites` upserts by name, so re-favoriting
replaces the template instead of duplicating it. The PWA decides whether a
meal "is favorited" by matching item content (name/calories/fraction
fingerprint) against the favorites list, shows ★ on matching meal rows, and
renders the edit-dialog button as a ★ Favorited toggle (tap to unfavorite):

```sql
CREATE TABLE favorites (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    items      JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

## API

All under `/api`, bearer-key auth (`Authorization: Bearer $FOOD_API_KEY`)
exactly like journal/finance. JSON in/out. `/api/health` is unauthenticated.

```
GET    /api/health

POST   /api/parse           {text, day}                       → NDJSON stream (see below), ends in ParseResult
POST   /api/photos          multipart image                   → {key, url}   (upload to R2)
POST   /api/analyze-photo   {key, hint, day}                  → NDJSON stream (see below); hint carries
                                                                 "half of this", "just the salmon", etc.

POST   /api/meals           {day, slot, description, input_kind, photo_key,
                             photo_url, ai_model, ai_raw, items:[Item]}   → Meal (saves a confirmed draft)
GET    /api/meals?day=YYYY-MM-DD                              → [Meal with items]
GET    /api/meals/{id}                                        → Meal with items
PUT    /api/meals/{id}      same shape as POST                → Meal (replaces items)
DELETE /api/meals/{id}

GET    /api/day/{date}      → {day, calories, protein_mg, carbs_mg, fat_mg, fiber_mg,
                               score, calorie_target, protein_target_mg,
                               calories_remaining, protein_remaining_mg, meals:[...]}
GET    /api/range?start=&end=  → [{day, calories, protein_mg, carbs_mg, fat_mg, score,
                                   over_target bool}]   -- one row per day with data

POST   /api/weights         {day, weight_g, note}             → upsert by day
GET    /api/weights?start=&end=                               → [{day, weight_g, note}]
DELETE /api/weights/{day}

POST   /api/favorites       {name, items:[Item]}               → Favorite (named meal template; upserts by name)
GET    /api/favorites                                          → [Favorite], ordered by name
PUT    /api/favorites/{id}  {name}                             → 204 (rename; 400 if the name is taken)
DELETE /api/favorites/{id}

GET    /api/settings
PUT    /api/settings        {calorie_target, protein_target_mg, weight_target_g}

GET    /api/export          → full-DB JSON download (meals+items+weights+favorites+settings)
```

The two parse endpoints stream progress so the PWA can narrate the wait
instead of showing a blank spinner. The response is
`application/x-ndjson` — one JSON object per line, flushed as the AI loop
runs: any number of `{"type":"progress","message":"Looking up “feta
cheese”…"}` lines, then exactly one `{"type":"result","result":<ParseResult>}`
or `{"type":"error","error":"..."}`. Request-validation failures (4xx/503)
are still plain JSON errors before the stream starts. The PWA reads the
stream with `fetch` + `ReadableStream` (`apiStream`) and updates the spinner
status line per progress event.

`ParseResult`:

```json
{
  "items": [{ "name": "...", "brand": null, "quantity": "1 cup", "grams": 140,
              "fraction_pct": 100, "calories": 220, "protein_mg": 8000, "...": 0,
              "micros": {}, "tier": "hard_yes", "tier_reason": "whole grains",
              "source": "usda", "source_ref": "173735", "confidence": "high" }],
  "notes": "Assumed cooked brown rice; say 'dry' if it was measured dry.",
  "ai_model": "claude-sonnet-5",
  "ai_raw": { ... }
}
```

The client edits the draft (fraction chips, delete an item, fix grams) and
POSTs it to `/api/meals` unchanged in shape.

## AI meal parsing (`internal/ai`)

One agentic Claude conversation per parse, mirroring the tool-loop in
`~/projects/finance/internal/finance/assistant.go`:

- **System prompt** embeds `docs/diet-framework.md` verbatim, the tier
  definitions, the unit conventions (kcal, milligrams), and instructions:
  parse casually described meals; prefer USDA lookups for whole foods and
  common dishes, **batching every lookup into one turn** (a typical parse is
  two model calls: lookups, then `record_meal`); use the barcode tool when
  digits are visible/known; estimate from knowledge only when lookups fail;
  keep `notes` to at most one short sentence and only for non-obvious
  assumptions (empty when the read was straightforward); for plate photos
  estimate portions from visual cues; a "half of this" style hint sets
  `fraction_pct`, not scaled-down nutrition values.
- **Tools exposed to Claude:**
  - `usda_search(query, page_size)` → top matches from FDC `/v1/foods/search`
    with per-100g core nutrients (implemented in `internal/nutrition/fdc.go`;
    prefer Foundation and SR Legacy data types, fall back to Branded).
  - `off_barcode(code)` → Open Food Facts `/api/v2/product/{code}.json` product
    name, brand, serving size, per-100g and per-serving nutriments
    (`internal/nutrition/off.go`).
  - `record_meal(items, notes)` → terminal tool; ends the loop and yields the
    draft.
  - `web_search` (Anthropic's server tool, executed API-side; `AI_WEB_SEARCH=off`
    disables) → chains/restaurants skip USDA and use the published nutrition
    directly; any other brand name (packaged goods, store brands, local
    items) escalates to web search when USDA has no confident match, before
    any estimate. Items get `source: "web"` with the URL in `source_ref`.
    A `pause_turn` stop reason is resumed by replaying the conversation
    unchanged.
- Vision parses (`/api/analyze-photo`) put the image (fetched from R2, base64)
  in the first user message with the hint text. Same loop; Claude may read a
  nutrition label directly (source `label`) or extract barcode digits and call
  `off_barcode`.
- Max ~8 tool-use rounds, then force `record_meal`. Model from `AI_MODEL` env
  (default `claude-sonnet-5`).
- **Tool results are slim; `micros` is attached server-side.** Lookup results
  sent to the model exclude per-nutrient lists (an FDC food can carry 100+
  nutrient entries — as tokens they made parses slow and expensive, and the
  model would then re-type them into `record_meal`). Instead the parser caches
  each lookup's full payload during the parse and `finish()` attaches it as
  the item's `micros` by matching `source:source_ref` — richer data than a
  model transcription, at zero token cost. `record_meal` has no `micros`
  field.

### Validation (server-side, deterministic)

Before returning a draft, Go validates every item: tier is a known enum;
`fraction_pct` in 1..100; all amounts non-negative; **Atwater check** —
`protein_g*4 + carbs_g*4 + fat_g*9` must be within ±30% of `calories` (else
clamp confidence to `low` and append a warning to `notes`). Never trust AI
arithmetic for totals — totals are always recomputed in Go/SQL.

## PWA (`pwa/index.html`, single file — follow journal/finance conventions)

`S` mutable state + `render()`; one reusable `<dialog>`; bottom tab nav.
Colors/typography: clean, large type, thumb-reachable controls; dark mode via
`prefers-color-scheme`.

- **Today (home).** Top: big calories-remaining ring (over budget adds an
  inner red overage ring), protein-to-go bar (progress toward target),
  quality score badge, and a macro row — small pie of calorie share
  (protein 4 / carbs 4 / fat 9 kcal per gram) with a legend carrying gram
  totals plus the fiber total. Below: the day's meals
  grouped by slot, each row tappable to edit. Bottom: an always-visible
  log bar — text input, 🎤 mic button (Web Speech API, copy journal's
  `webkitSpeechRecognition` usage), 📷 camera button (file input without a
  `capture` attribute so iOS offers Photo Library / Take Photo), ⭐ favorites,
  ⚖️ weight quick-entry. Day switcher (‹ today ›) to log to yesterday.
- **Log flow.** Input → spinner → **draft preview card** in the dialog: item
  list with name, grams, calories, macros; per-item fraction chips
  (¼ ½ ¾ All) and a whole-meal fraction row; delete-item ✕; editable grams
  (recompute proportionally client-side: nutrition scales linearly with
  grams). Save → POST /api/meals → Today refreshes. Photos: picking a shot
  opens a hint step (photo preview + optional free-text hint, pre-filled from
  the log input) before Analyze; then client-side canvas downscale to ≤1600px
  JPEG, upload, and the vision parse with the hint.
- **Duplicate & favorites.** The edit dialog offers ⧉ Duplicate (opens a new
  draft with the same items for the currently viewed day) and ☆ Favorite
  (names the meal and saves it as a template). The log bar's ⭐ opens the
  favorites list — tap one to open it as a pre-filled draft (adjust fraction
  chips, Save), ✕ deletes a favorite.
- **Trends.** Week and Month toggles: per-day calorie bars (colored against
  target, score dot inside the bar top, value label above); a macro stack
  chart (grams, protein on the bottom, carbs, fat; protein target as the
  dashed line, protein-gram label above each stack); weight line chart with
  optional goal line; simple inline SVG, no chart library. Month grid follows
  the finance convention of being honest about the in-progress day/month —
  averages count only completed days.
- **Settings.** Calorie target, protein target (entered in grams, stored mg),
  goal weight (entered lb, stored grams), export-JSON download link, build/version.
- PWA installability: `manifest.json`, minimal `sw.js` (network-first for the
  shell with cache fallback for offline — deploys show on the next reload with
  no version bump; network-only for API), apple-touch-icon. API key stored in
  `localStorage` with a first-run prompt, same as journal.

## Exercise (migration 005, phase E1)

Cardio / strength / yoga / meditation tracking, added alongside the food
tracker post-launch. **Fully independent of the calorie domain** — a session
never credits or debits `DaySummary`/`RangeDay` calories or touches the meals
tables; this separation is an invariant every later exercise phase must
preserve.

```sql
CREATE TABLE exercise_sessions (
    id            BIGSERIAL PRIMARY KEY,
    day           DATE NOT NULL,               -- user-chosen, independent of performed_at
    performed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    type          TEXT NOT NULL CHECK (type IN ('cardio','strength','yoga','meditation')),
    activity      TEXT,                        -- cardio: Run/Bike/Hike/Swim/Row/Other
    location      TEXT,                        -- strength + yoga
    style         TEXT,                        -- yoga: Vinyasa/Hot/Other
    duration_min  INT,                         -- cardio/yoga/meditation; NULL for strength
    note          TEXT NOT NULL DEFAULT '',
    input_kind    TEXT NOT NULL DEFAULT 'tap'
                  CHECK (input_kind IN ('tap','text','voice','import')),
    ai_raw        JSONB,                       -- raw AI output when NL-parsed (exercise phase 4)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX exercise_sessions_day_idx ON exercise_sessions(day);
```

`activity`/`location`/`style` are free-text (not DB enums) so "Other" and
future values need no migration; the **service** validates the field the type
requires: cardio needs `activity` + `duration_min > 0`; yoga needs `location`
+ `style` + `duration_min > 0`; meditation needs `duration_min > 0`; strength
needs `location` and must **not** have `duration_min` set (no duration field
for it yet). `duration_min` is integer minutes — no floats, same as the food
domain. **Multiple sessions per day per type are normal** (yoga twice, a hike
after a swim) — saves always append, never upsert/dedupe by (day, type).

`settings` gained four weekly-target columns, all **Monday–Sunday weeks**:
`cardio_weekly_target` (default 3), `strength_weekly_target` (default 2),
`yoga_weekly_target` (default 2), `meditation_weekly_days` (default 7 — a
days-per-week target, not a session count).

API — same bearer auth, JSON in/out as the rest of `/api`:

```
POST   /api/exercise            {day,type,activity,location,style,duration_min,note,input_kind} → 201 ExerciseSession
GET    /api/exercise?start=&end=                                → [ExerciseSession]  (day range, inclusive; both required)
GET    /api/exercise/{id}                                       → ExerciseSession
PUT    /api/exercise/{id}       same body shape as POST         → ExerciseSession
DELETE /api/exercise/{id}                                       → 204
```

No weekly-rollup endpoint — the home card and trends (exercise phases 2–3)
read raw sessions via the range route and aggregate client-side. The four
targets are included in `GET /api/settings` / accepted by `PUT /api/settings`.
`GET /api/export` includes `exercise: [ExerciseSession]`.

### PWA (phase E2): Training card + tap-log

On Today, under the calorie hero, a **Training · this week** card shows one
row per practice — this is the whole reminder-to-move, no nudge sentence.
Cardio/Strength/Yoga rows show an icon, a subtitle (`today`/`today ×N`,
`last: <Wkday>`, or `last: <Wkday> · last wk`/`· N wks ago` falling back
across the Mon–Sun week boundary, or `none yet`), a dot per session this
week up to the weekly target (extra dots beyond target, a dashed dot for
today when nothing's logged yet), and `n/target` with a ✓/green treatment
when met. Meditation is a 7-dot Mon–Sun daily row (`daysDone/7`) instead of
a session count. Tapping a row (or its "today" dashed dot) opens a quick-log
bottom sheet — chip pickers with sensible defaults, 2–3 taps, no AI call;
saving always appends (`POST /api/exercise`, `input_kind: 'tap'`, `day` =
the currently-viewed day), never overwrites. Tapping a filled dot reopens
that session for edit/delete. The PWA fetches the current week plus an
8-week lookback once per Today load (`GET /api/exercise?start=&end=`) and
recomputes the card from that cache as the viewed day changes. Settings
gained a "Weekly training targets" group for the four columns above. Trends
(exercise phase 3) and NL logging (exercise phase 4) are not built yet.

## Environment

Required: `DATABASE_URL`, `FOOD_API_KEY`, `ANTHROPIC_API_KEY`, `FDC_API_KEY`.
Optional: `PORT` (default 8082), `AI_MODEL` (default `claude-sonnet-5`; text
parses), `AI_VISION_MODEL` (default `claude-sonnet-5`; photo parses stay on
Sonnet even when `AI_MODEL` is Haiku — vision OCR like barcode digits is
where smaller models misread), `AI_WEB_SEARCH` (default on; `off` disables
the web-search tool),
`R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET`,
`R2_PUBLIC_URL` (photo features disabled until all five are set — the PWA
hides the camera button when `/api/health` reports `photos: false`).
Open Food Facts needs no key (send a descriptive `User-Agent`).

## Deployment

Render web service from the `Dockerfile` (multi-stage `golang:1.25-alpine` →
`alpine`, `GOTOOLCHAIN=local` — same caveats as finance). Migrations run at
startup. Production Postgres on Render. Smoke test locally against a scratch
DB, never the cloud DB:

```
createdb food_smoke
DATABASE_URL=postgres://localhost:5432/food_smoke?sslmode=disable \
  FOOD_API_KEY=x ANTHROPIC_API_KEY=... FDC_API_KEY=DEMO_KEY \
  go run ./cmd/server
dropdb food_smoke   # when done
```
