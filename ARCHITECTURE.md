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
2. Claude **parses** the text into `(food, quantity)` pairs and **picks** the
   best FoodData Central entry per food; the app looks the nutrition up and
   does the arithmetic — the model never emits a macro value (see "Resolution
   layer"). Each food is classified against `docs/diet-framework.md`.
3. The PWA shows a **draft preview card** — one row per food with grams,
   calories, macros, and a confirm-the-match line (which database entry backed
   it, one-tap alternatives). Jim adjusts per-food portions and taps Save.
4. Save **appends the foods to the day's slot** (breakfast/lunch/dinner/snack).
   The slot is the *meal*; the *food* is the first-class unit (see "Data
   model"). Today screen leads with **calories remaining**, **protein to go**,
   the **quality score**, and a **saturated-fat** ceiling; an outlier line
   surfaces on an off day. Trends show daily bars for week/month plus weight.

## Data model — Food and Meal

The stored, first-class unit is the **Food** (`meal_items` row): a single item
with its own nutrition, portion, quality tier, resolution provenance, photo,
and logging metadata. A **Meal** is the **(day, slot) container** — one
Breakfast/Lunch/Dinner/Snack per day (unique on `(day, COALESCE(slot,''))`),
holding foods but carrying no nutrition of its own. Logging *appends* foods to
a slot; opening a slot shows the flat list of every food in it regardless of
when or how it was added. This matches how the analysis export has always
grouped — by `(date, slot)`. (Migration 012 moved the app to this model;
before it, a "meal" was a single logging entry with N items.)

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
- **Keep every AI parse's raw output** (`meal_items.ai_raw JSONB`, per food
  since migration 012) and every nutrition source's full nutrient payload
  (`meal_items.micros JSONB`) — the point is a database rich enough for future
  analyses.
- **The model parses; the code does the arithmetic.** For a food resolved
  through FoodData Central or a barcode, the model returns the chosen entry's
  id and a portion in grams — never calorie/macro numbers. Go computes every
  amount as `per_100g × grams / 100` from the looked-up entry (see "Resolution
  layer"). The model only supplies numbers for the last-resort LLM-estimate /
  label / web tiers.
- **The AI never writes to the database.** The parse endpoints return a draft;
  only an explicit save request persists anything. Server-side validation
  recomputes derived numbers and sanity-checks the draft first (see
  "Validation").

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

## Inclusion score

A second, **independent** score (`internal/food/inclusion.go`,
build-prompts/inclusion-spec-20260726.md). Composition (above) asks *of the
calories I ate, what tier were they?* Inclusion asks *did I eat the handful of
foods with the strongest evidence behind them?* Calorie-trivial foods (kale,
berries) can never move a calorie-weighted mean, so this is a structurally
different construct, not a recalibration of the first one. **The two scores
are never merged, averaged, or combined into one number** — no code path may
touch `DayScore`, `TierValue`, or `DaySummary.Score` to compute this.

Five fixed components, each with a weekly target (`ComponentCatalog`, the
single source of truth for ids/labels/emoji/targets/serving text):

| id | label | target/wk | default g/serving |
|---|---|---|---|
| `leafy_greens` | 🥬 Leafy greens | 7 | 30 |
| `berries` | 🫐 Berries | 5 | 75 |
| `legumes` | 🫘 Legumes | 5 | 90 |
| `fatty_fish` | 🐟 Fatty fish | 3 | 100 |
| `cruciferous` | 🥦 Cruciferous | 4 | 85 |

Nuts/seeds is deliberately excluded (permanently-satisfied target, pure
noise); EVOO/fermented/whole-grains are phase-2, not built yet.

**Tag layer** — `food_component_tags` maps a logged food to the component(s)
it counts toward. Keyed on `normalized_name` (not `fdc_id` as in the original
spec draft) since many logged foods never resolve to an FDC entry; `fdc_id` is
kept as a nullable secondary key. Lookup order: fdc_id exact hit ->
normalized_name hit -> untagged. `grams_per_serving` (not `servings_per_ref`)
keeps the math integer and lets a composite dish carry its own reference.
Multi-tag is expected (kale is both `leafy_greens` and `cruciferous`).

**Tag capture (phase 2)** — "tag once, persist, reuse forever." The
`record_meal` tool schema (`internal/ai/tools.go`) carries an optional
`components` array per item; the parser's system prompt encodes the component
rules (kale double-counts, whole fruit and starchy veg get nothing, nuts/seeds
never will, fatty fish is only salmon/sardines/mackerel/herring/anchovies) in
a string constant (`componentRulesPrompt`) shared verbatim with the batch
suggester below, so the two never drift. `canonical_lookup`'s result is
extended with the food's existing tags so a repeat food's proposal is a copy,
not a re-decision. `MealItem.Components` (`[]ItemComponent{component_id,
grams_per_serving, source}`) rides the draft through the confirm sheet — the
PWA's chip picker (draft card and edit-food sheet share one `componentRow()`)
sets `source: "user"` the moment Jim touches a chip. On save,
`Service.accreteComponentTags` upserts each item's components into
`food_component_tags` (`Store.UpsertComponentTag`, keyed
`(normalized_name, component_id)`); the DB upsert's `WHERE` clause is the
"never overwritten by an AI proposal" invariant — an incoming `ai` row is
dropped when the existing row is already `user`-owned, while an incoming
`user` row always wins. `SanitizeItemComponents` (shared by the parser's
`finish()` and the service save path) drops an unknown `component_id`,
clamps `grams_per_serving` to 1–2000, and collapses duplicates, so a
malformed proposal degrades to omission rather than a 500.

**Backfill** — Settings → "Tag foods" (`GET /api/component-tags/candidates`,
`Service.TagCandidates`) lists distinct foods from the last 30 days of
`meal_items` plus every `canonical_foods` row, untagged first, times-logged
descending, each with the same chip picker. "Suggest tags" posts every
untagged name in one batch to `POST /api/component-tags/suggest`
(`Parser.SuggestComponentTags`, one Claude call, `componentRulesPrompt`
again) and renders the result as unconfirmed dashed chips — nothing persists
until Jim taps one, same AI-never-writes invariant as everywhere else.

**Math** — all integer, servings tracked as `servings_x100` (hundredths):
`gramsEaten = grams * fraction_pct / 100`, `servingsX100 = gramsEaten * 100 /
gramsPerServing`. A per-meal cap (`MaxServingsX100PerMeal = 200`, i.e. 2.00
servings) limits one `(day, slot)` container's contribution to a component —
the construct is exposure frequency, not volume, so one huge salad can't
satisfy the weekly target. Per component, `progressPct = min(servingsX100 /
target, 100)` — the 1.0 cap is non-negotiable, otherwise six servings of
berries paper over zero fatty fish. `InclusionScore = sum(progressPct) /
len(components)`.

**Window** — the rolling last 7 calendar days inclusive of `end`
(`Service.InclusionWindow`), never a calendar week; food has unlimited slots
and fully recoverable misses, so nothing is ever written off.
`ExpiringWhole` reports whole servings logged on `end-6`, the ones aging out
at the start of tomorrow. `Service.InclusionWeeks` is the one place calendar
weeks (Mon–Sun) apply to food — the Trends retrospective only.

A food with no grams (`Grams == nil` or `<= 0`) can't contribute; it's counted
into `UnmeasuredFoods` instead of silently dropped.

**Tunable targets (phase 5)** — `Settings.ComponentTargets` (`component_targets`
JSONB, migration 014) overrides `ComponentCatalog`'s default per-component
target; `{}` means "use the defaults." `SanitizeComponentTargets` drops
unknown ids and values outside 1..21 before a save ever reaches the store, the
same shape as `SanitizeItemComponents`. `ComputeInclusion` takes the resolved
override map as its 5th argument — a missing or non-positive entry falls back
to the catalog default, so `nil` reproduces phase-1 behavior exactly. Targets
are the *only* tuning knob (spec §3: priority is expressed through
thresholds, not weights) — there is no per-component weight to sanitize
alongside it.

**Nudges (phase 5, in-app only)** — there is no push infrastructure in this
app (`pwa/sw.js` is cache-only: no VAPID keypair, no subscription table, no
`push`/`notificationclick` handler, no server-side scheduler), so both nudges
render on the Today card instead of pushing. The honesty constraint (spec §7)
is enforced structurally: `SupplyNeeds(w InclusionWindow)` and
`DecisionCandidate(w InclusionWindow)` (`internal/food/inclusion.go`) take an
`InclusionWindow` as their only argument — never `DaySummary`, a day score,
calories, or macros — so a reviewer can confirm the constraint from the
signature alone. Both are computed once in `Service.InclusionWindow` (never in
`InclusionWeeks` — nudges are Today-only) and ride along as
`InclusionWindow.SupplyNeeds`/`.DecisionCandidate` in the `/api/inclusion`
response. `SupplyNeeds` ranks components with `ProgressPct < 100` by relative
gap `(target - servings) / target` descending, top 3. `DecisionCandidate`
picks the single largest relative gap among components not `Met`, tying
toward one with `ExpiringWhole > 0` (a serving aging out within 24h — action
today prevents the loss). The relative gap is computed from `ServingsX100`
directly, not from the already-rounded `ProgressPct`, to avoid distorting the
ranking.

The PWA (`NUDGE_COPY`, `supplyNudgeLine`/`decisionNudgeLine`/`nudgeLineHTML` in
`pwa/index.html`) decides only *whether* to surface a candidate — using the
device clock, `Settings.SupplyNudgeDOW`/`NudgeStartHour`/`NudgeEndHour`, a
per-day localStorage dismissal, and `Settings.NudgesEnabled` — never
score/calorie/macro data. Tier 1 (supply) fires only on the configured
weekday; tier 2 (decision) only inside the configured device-local hour
window, and never while tier 1 is showing. Both render as one line directly
under the `FOOD · LAST 7 DAYS` card, dismissible for the day.

## Database schema

The initial schema is below (migration 001). Migrations 009–012 evolved it
substantially — see **Schema evolution** after the favorites table. Most
notably `meals` is now the (day, slot) container (its per-food columns —
`description`, `input_kind`, `photo_*`, `ai_*`, `eaten_at` — moved onto
`meal_items`), and `meal_items` gained resolution-provenance columns.

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
    fraction_pct  INT NOT NULL DEFAULT 100 CHECK (fraction_pct BETWEEN 1 AND 300),
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

Migration 002 adds **favorites** — reusable templates. Items are a JSONB
snapshot (same shape as `meal_items`), not references, so editing or deleting
the original never mutates a favorite. Migration 003 makes names unique
(case-insensitive); `POST /api/favorites` upserts by name. Migration 012 adds a
`kind` (`food` | `meal`): a single food or a whole slot's collection. The
favorites sheet splits into **Foods** and **Meals** sections; the PWA marks a
food/meal "favorited" by matching item content (name/calories/fraction
fingerprint) against the list.

```sql
CREATE TABLE favorites (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL DEFAULT 'meal'      -- 'food' (single) | 'meal' (collection), migration 012
               CHECK (kind IN ('food','meal')),
    items      JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Schema evolution (migrations 009–014)

- **009 — resolution provenance** on `meal_items`: `fdc_id`, `fdc_data_type`
  (`Branded`|`Foundation`|`SR Legacy`|`Survey (FNDDS)`), `resolution_tier`
  (1–4), `portion_source` (`weighed`|`package_unit`|`estimated`), `tier_source`
  (`table`|`cascade`). All nullable — legacy rows honestly read "unknown".
- **010 — `settings.sat_fat_target_mg`** (default 14000): the saturated-fat
  ceiling, tracked like the calorie target.
- **011 — `canonical_foods`**: the accretion cache. A resolved food's per-100g
  nutrition + tier + provenance are written here keyed on a normalized name and
  reused on later logs, so a food never re-resolves; corrections overwrite the
  entry (propagate forward, history untouched). Has `protected`/`protected_reason`
  columns as groundwork for the backlog "protected foods" feature.
- **012 — food-first-class**: moved `description`/`input_kind`/`photo_*`/`ai_*`/
  `eaten_at` off `meals` onto `meal_items`; merged entries sharing a
  `(day, slot)` into one container; added the unique index
  `(day, COALESCE(slot,''))` and `favorites.kind`. The existing ~2 weeks of data
  were flattened in place.
- **013 — `food_component_tags`**: the inclusion-score tag layer (see
  **Inclusion score** above). `(normalized_name, component_id)` unique;
  `fdc_id` nullable secondary key; `grams_per_serving` 1–2000;
  `tag_source` `ai`|`user`.
- **014 — tunable inclusion targets + nudge settings** on `settings`:
  `component_targets` JSONB (default `{}`, overrides `ComponentCatalog`'s
  per-component target), `supply_nudge_dow` INT (default 6, Saturday),
  `nudge_start_hour`/`nudge_end_hour` INT (default 11/13), `nudges_enabled`
  BOOLEAN (default true). See **Inclusion score** → Tunable targets/Nudges
  above.

## API

All under `/api`, bearer-key auth (`Authorization: Bearer $FOOD_API_KEY`)
exactly like journal/finance. JSON in/out. `/api/health` is unauthenticated.

```
GET    /api/health

POST   /api/parse           {text, day}                       → NDJSON stream (see below), ends in ParseResult
POST   /api/photos          multipart image                   → {key, url}   (upload to R2)
POST   /api/analyze-photo   {key, hint, day}                  → NDJSON stream (see below); hint carries
                                                                 "half of this", "just the salmon", etc.

POST   /api/foods           {day, slot, items:[Food]}         → Meal (appends foods to the
                                                                 day's slot container; each food
                                                                 carries its own photo/input_kind/ai_raw)
GET    /api/foods/{id}                                        → Food
PUT    /api/foods/{id}      {day, slot, ...Food}              → Food (edits; a new slot moves it)
DELETE /api/foods/{id}                                        → 204 (prunes the container if emptied)
GET    /api/meals?day=YYYY-MM-DD                              → [Meal (container) with foods]
GET    /api/meals/{id}                                        → Meal with foods
DELETE /api/meals/{id}                                        → 204 (clears the whole slot)

GET    /api/day/{date}      → {day, calories, protein_mg, carbs_mg, fat_mg, fiber_mg, sat_fat_mg,
                               score, calorie_target, protein_target_mg, sat_fat_target_mg,
                               calories_remaining, protein_remaining_mg,
                               anomaly:{headline,reasons}|null, meals:[...]}
GET    /api/range?start=&end=  → [{day, calories, protein_mg, carbs_mg, fat_mg, score,
                                   over_target bool}]   -- one row per day with data

POST   /api/weights         {day, weight_g, note}             → upsert by day
GET    /api/weights?start=&end=                               → [{day, weight_g, note}]
DELETE /api/weights/{day}

POST   /api/favorites       {name, kind, items:[Food]}         → Favorite (food|meal; upserts by name)
GET    /api/favorites                                          → [Favorite], ordered by name
PUT    /api/favorites/{id}  {name}                             → 204 (rename; 400 if the name is taken)
DELETE /api/favorites/{id}

GET    /api/settings
PUT    /api/settings        {calorie_target, protein_target_mg, sat_fat_target_mg, weight_target_g}

GET    /api/export          → full-DB JSON download (meals+items+weights+favorites+settings)
GET    /api/export/range     → {start, end} earliest/latest logged day ({} if empty)
GET    /api/export/analysis?start=&end=  → reshaped LLM-ready JSON download (dates
                               default to the full logged range)

GET    /api/components                  → ComponentDef catalog (ids, labels, emoji, targets, serving text)
GET    /api/inclusion?end=YYYY-MM-DD    → InclusionWindow, rolling 7 days ending `end` (default today)
GET    /api/inclusion/weeks?start=&end= → [InclusionWeek], completed Mon-Sun weeks only, newest first
GET    /api/component-tags              → [ComponentTag], newest updated first
PUT    /api/component-tags   {normalized_name, fdc_id?, tags:[{component_id, grams_per_serving}]}
                                         → 204 (replaces that food's whole tag set, tag_source "user")
DELETE /api/component-tags/{name}       → 204 (clears a food's tags)
GET    /api/component-tags/candidates   → [TagCandidate], distinct logged foods (last 30 days +
                                           canonical_foods), untagged first, times-logged descending —
                                           Settings → "Tag foods" backfill screen (phase 2 §4)
POST   /api/component-tags/suggest {names:[string]} → [{name, components}], one Claude call proposing
                                           tags for a batch of names; 503 when AI isn't configured.
                                           Nothing is persisted — the PWA confirms each chip before
                                           it PUTs /api/component-tags.
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

`ParseResult` (calorie/macro amounts are *computed by Go* for usda/off items,
not emitted by the model):

```json
{
  "items": [{ "name": "brown rice", "brand": null, "quantity": "1 cup", "grams": 195,
              "fraction_pct": 100, "calories": 216, "protein_mg": 5000, "...": 0,
              "micros": {}, "tier": "hard_yes", "tier_reason": "whole grains",
              "tier_source": "table", "source": "usda", "source_ref": "168880",
              "fdc_data_type": "SR Legacy", "resolution_tier": 2,
              "portion_source": "estimated", "confidence": "high",
              "match_description": "Rice, brown, long-grain, cooked",
              "alternatives": [{ "fdc_id": 169703, "description": "Rice, white, cooked",
                                 "fdc_data_type": "SR Legacy", "per_100g": { ... } }],
              "components": [{ "component_id": "leafy_greens", "grams_per_serving": 30 }] }],
  "notes": "Assumed cooked; say 'dry' if it was measured dry.",
  "ai_model": "claude-sonnet-5",
  "ai_raw": { ... }
}
```

`match_description` and `alternatives` are draft-only (confirm-the-match, not
persisted). The client edits the draft (per-food fraction chips, fix grams,
swap the match, mark weighed, delete a food) and
POSTs the foods to `/api/foods`.

## Resolution layer (`internal/ai` + `internal/nutrition`)

The core rule: **the LLM parses and picks; FoodData Central supplies the
numbers; Go does the arithmetic.** The model never emits a macro value for a
grounded food.

**Cascade (record which tier answered, `resolution_tier` 1–4):**

1. **Barcode → Branded** (`off_barcode`, Open Food Facts) — packaged foods.
2. **FDC Foundation, then SR Legacy** — whole/generic foods.
3. **FDC Survey (FNDDS)** — composite/prepared/restaurant dishes ("salad bar",
   "fish tacos"). `usda_search` queries all of `Foundation`, `SR Legacy`,
   `Survey (FNDDS)`, `Branded`.
4. **LLM estimate** — last resort; the one tier where the model gives numbers.

**Division of labor.** The model calls `usda_search`, picks the best entry, and
in `record_meal` returns that food's `source_ref` (fdc_id), `fdc_data_type`,
and a portion in `grams` (resolved from the search result's `foodPortions`
household-measure weights). `finish()` (in `parser.go`) then computes every
amount as `per_100g × grams / 100` from the cached search result, so the stored
numbers are lab-grounded and pass the Atwater check by construction. `micros`
is still attached server-side from the cached full nutrient payload.

**Canonical accretion.** A `canonical_lookup(name)` tool lets the model check
the `canonical_foods` cache *first*; on a hit it records the food directly from
the stored per-100g and skips `usda_search`. Every saved grounded food is
upserted into the cache by normalized name (`service.accreteCanonical`), so
repeat foods never re-resolve and a corrected match propagates forward.

**Confirm-the-match (item 4).** For each usda food the model also lists the
other plausible `alternative_fdc_ids` it saw; the server enriches them into
`alternatives` (name + data_type + per-100g) on the draft. The PWA shows the
matched entry inline with a data-type badge (tiers 1–2 quiet, 3–4 loud), a
one-tap swap that recomputes nutrition from the chosen alternative, and a
weighed-portion toggle. A low-confidence match never blocks the save.

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
  - `canonical_lookup(name)` → checks the `canonical_foods` accretion cache;
    on a hit the food records directly from the cached per-100g, no search.
  - `usda_search(query, page_size)` → top matches from FDC `/v1/foods/search`
    across Foundation, SR Legacy, Survey (FNDDS), and Branded, with per-100g
    core nutrients and `foodPortions` household-measure gram weights
    (`internal/nutrition/fdc.go`).
  - `off_barcode(code)` → Open Food Facts `/api/v2/product/{code}.json` product
    name, brand, serving size, per-100g and per-serving nutriments
    (`internal/nutrition/off.go`).
  - `record_meal(items, notes)` → terminal tool; ends the loop and yields the
    draft. For usda/off foods the model supplies `source_ref` + `fdc_data_type`
    + `grams` + `alternative_fdc_ids` and **omits the macro fields** (Go
    computes them); it gives numbers only for ai/label/web foods. Each item may
    also carry `components` (inclusion phase 2, see **Inclusion score** above)
    — most foods omit it.
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
  `off_barcode`. Photos are always food — never exercise.
- Max ~8 tool-use rounds, then force one of `record_meal`/`log_exercise`
  (`tool_choice: "any"` over both). Model from `AI_MODEL` env (default
  `claude-sonnet-5`).
- **`log_exercise` (phase E4): the same loop understands exercise.** The
  system prompt has Claude classify the input first and call exactly one
  terminal tool — `record_meal` for food, `log_exercise` for a workout
  ("30 min run", "hot yoga at the studio, 60 minutes", "hike after a swim",
  "did my PT for 20 minutes"). Exercise inputs need no USDA/barcode lookups;
  Claude fills an array of sessions directly from the text (each
  `{type, activity?, location?, style?, duration_min?, note?}`, matching the
  phase-1 per-type field rules — "hike then a swim" yields two sessions).
  `ParseResult` carries a `kind` discriminator (`"meal"` default, or
  `"exercise"`) plus an `exercise: [ExerciseSession]` array populated instead
  of `items` when `kind` is `"exercise"`; `ai_raw` still holds the full trace.
  Each session is checked against the same per-type required-field rules the
  save path enforces (`food.ExerciseFieldErrors`, shared with
  `validateExercise`) — an incomplete session isn't dropped, it's flagged in
  `notes` so Jim can complete it in the confirm sheet, since the AI never
  writes to the DB. The `/api/parse` NDJSON handler is otherwise unchanged.
- **Tool results are slim; `micros` is attached server-side.** Lookup results
  sent to the model exclude per-nutrient lists (an FDC food can carry 100+
  nutrient entries — as tokens they made parses slow and expensive, and the
  model would then re-type them into `record_meal`). Instead the parser caches
  each lookup's full payload during the parse and `finish()` attaches it as
  the item's `micros` by matching `source:source_ref` — richer data than a
  model transcription, at zero token cost. `record_meal` has no `micros`
  field.

### Validation (server-side, deterministic)

Before returning a draft, Go validates every food: tier / portion_source /
tier_source / resolution_tier are known enums/ranges; `fraction_pct` in 1..300;
all amounts non-negative; **Atwater check** — `protein_g*4 + carbs_g*4 +
fat_g*9` within ±30% of `calories` (else clamp confidence to `low` and warn in
`notes`). Under the resolution layer, usda/off foods pass Atwater by
construction because Go computed them from FDC; a failure post-resolution
signals a bad composite decomposition or an unreviewed estimate. Totals are
always recomputed in Go/SQL, never trusted from the model.

## PWA (`pwa/index.html`, single file — follow journal/finance conventions)

`S` mutable state + `render()`; one reusable `<dialog>`; bottom tab nav.
Colors/typography: clean, large type, thumb-reachable controls; dark mode via
`prefers-color-scheme`.

- **Today (home).** Top: big calories-remaining ring (over budget adds an
  inner red overage ring), protein-to-go bar, quality score badge, and a macro
  row — calorie-share pie with a legend carrying gram totals, the fiber total,
  and a **saturated-fat** actual/ceiling line (reddened when over). An
  **outlier line** (from `day.anomaly`) appears only on an off day — one
  expandable line naming the 2–3 foods that drove a low score or over-ceiling
  sat fat. Below Training (see below): a **`Food · last 7 days`** card
  (`foodCardHTML()`, phase 3) showing the inclusion score's five components as
  a **meter, not a grid** — the one constraint that decided this design. The
  Training card answers *when* (a Mon–Sun day grid); this card answers *how
  many* (a dot meter with no day-of-week axis at all), so the two cards never
  present adjacent columns that mean different things. Each row's dot count
  **is** that component's weekly target (`fcRowHTML()`; 7/5/5/3/4, so row
  lengths differ by design and are never padded to a common width): filled ●
  for a whole serving in the rolling window, the trailing `expiring_whole`
  filled dots rendered ◐ as a **decay preview** (servings logged on `end-6`
  that age out at the start of tomorrow — pure information, no nagging), open
  ○ for the remainder. A component at or past target shows exactly `target`
  filled dots plus ✓ and never renders overflow — the numeral on the right
  (`fmtServingsX100()`, string-formatted from `servings_x100` with no float
  arithmetic) caps at `target` the same way. The inclusion score itself sits
  quietly in the card header (`.fc-score`, deliberately unstyled next to the
  hero's bold `.score-badge`) and taps to `showInclusionInfo()`, a two-line
  dialog contrasting it with composition. Loads lazily like `S.exercise`
  (`loadInclusion()` → `GET /api/inclusion`, no day param — the rolling window
  always ends at real today, independent of the day being viewed) and
  invalidates on food save/delete/day change so a saved salad moves the dots
  without a reload. Fresh `.fc-*` CSS throughout; `.ti-*` (Training) is
  untouched. Below: each slot shows its **foods flat**, every food tappable to
  edit; the slot header carries ☆ (favorite the meal) and 🗑 (clear the slot).
  Bottom: an always-visible log bar — text, 🎤 mic, 📷 camera, ⭐ favorites,
  ⚖️ weight. Day switcher (‹ today ›) to log to yesterday.
- **Log flow.** Input → spinner → **draft card**: one row per food with name,
  editable grams, calories, macros, per-food fraction chips (¼ ½ ¾ …), and a
  **confirm-the-match line** — data-type badge (tier-colored), matched FDC
  entry name, ⇄ one-tap swap to an alternative (recomputes nutrition from its
  per-100g), ⚖ weighed-portion toggle. No whole-meal fraction — portion is
  per-food. Save → **POST /api/foods** (appends the foods to the slot) → Today
  refreshes. Photos: pick a shot → hint step → client-side canvas downscale to
  ≤1600px JPEG, upload, vision parse; each resulting food keeps the photo.
- **Editing.** Tapping a food opens a single-food sheet (fix grams/fraction/
  tier, swap match, mark weighed, move slot via the slot picker, Delete). A
  wrong match corrected here rewrites the food and accretes to canonical.
- **Inclusion-component chips (phase 2).** Below the match line, `componentRow()`
  — shared by the draft card and the edit-food sheet, both render through
  `draftItemRow()` — shows proposed/confirmed tags as filled chips (serving
  figure computed live from grams × fraction, so editing portion updates it
  with no re-parse) plus a `+` opening the five-way picker. Touching any chip
  marks it `source: "user"`. Settings → **"Tag foods…"** is the phase-2
  backfill screen: candidates untagged-first, the same picker per row, and a
  **"Suggest tags"** button that batches every untagged name into one AI call
  and renders the result as unconfirmed dashed chips Jim accepts or rejects.
- **Favorites.** ☆ a single food from its edit sheet (a **Food** favorite), or
  ☆ a whole slot from the day view (a **Meal** favorite). The ⭐ log-bar button
  opens the favorites sheet split into **Meals** and **Foods** sections; tapping
  one opens it as an editable draft to adjust before adding to a day.
- **Trends.** Week and Month toggles: per-day calorie bars (colored against
  target, score dot inside the bar top, value label above); a macro stack
  chart (grams, protein on the bottom, carbs, fat; protein target as the
  dashed line, protein-gram label above each stack); weight line chart with
  optional goal line; simple inline SVG, no chart library. Month grid follows
  the finance convention of being honest about the in-progress day/month —
  averages count only completed days. Below Weight (Food tab only, phase 4):
  an **inclusion weekly-recap** card (`inclusionWeeksCardHTML()`) — the one
  place calendar weeks apply to food (`GET /api/inclusion/weeks`, fetched
  alongside the range/weights calls in `loadTrends()`), never the rolling
  window Today uses. One block per completed Mon–Sun week (`Week of <date>`,
  `n/5 components`), each component as `count/target` with ✓ on a hit —
  reuses `.fc-row`/`.fc-label`/`.fc-num` from the Today card but drops
  `.fc-dots`, since this reads as a table-ish recap, not a second meter. Week
  toggle shows the single most recent completed week, Month shows the last
  four or five; the in-progress week never appears.
- **Settings.** Calorie target, protein target (entered in grams, stored mg),
  saturated-fat ceiling (grams), goal weight (entered lb, stored grams), weekly
  training targets, a Data card
  (analysis export + full backup), build/version. The **analysis export**
  (`/api/export/analysis`) is the LLM-ready artifact: display units, as-eaten
  values, a per-day rollup plus **`meal_groups`** (the analytical unit — one per
  `(date, slot)`), per-food resolution provenance, and — so an analyst can't be
  misled — **`meta.coverage`** (per-domain first-logged/day counts),
  null-not-zero for unlogged domains, and a top-level **`analysis_warnings`**
  array. The PWA also generates a daily-summary CSV client-side. Distinct from
  the raw full-DB backup at `/api/export`.
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
    type          TEXT NOT NULL CHECK (type IN ('cardio','strength','yoga','meditation','pt')),  -- 'pt' added migration 006
    activity      TEXT,                        -- cardio: Run/Ride/Spin/Hike/Swim/Row/Other (Ride=outdoor, Spin=stationary)
    location      TEXT,                        -- yoga (strength captures location in note)
    style         TEXT,                        -- yoga: Vinyasa/Hot/Other
    duration_min  INT,                         -- cardio/yoga/meditation; NULL for strength + pt
    hr_zones      JSONB,                       -- cardio only: {"1":min,..,"5":min}, migration 008
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
and pt carry no required fields beyond `type` (their `note` holds any detail,
e.g. gym location) and must **not** have `duration_min` set.
`duration_min` is integer minutes — no floats, same as the food domain. **Multiple sessions per day per type are normal** (yoga twice, a hike
after a swim) — saves always append, never upsert/dedupe by (day, type).

`hr_zones` (migration 008) is **cardio-only**, optional time-in-heart-rate-zone
as a JSONB map of zone `"1".."5"` → integer minutes, e.g. `{"1":5,"3":15}`.
It is **independent of `duration_min`** (need not sum to it), integer minutes
(no floats), and empty zones are dropped so an unused panel stores SQL NULL.
The service rejects `hr_zones` on non-cardio types and out-of-range keys. The
shape is Garmin-import-ready: a future import (`input_kind='import'`) maps its
five zone times straight onto this column. Entered in the PWA via a collapsible
panel on the cardio tap/NL-confirm sheets; the `note` free-text field shows on
every exercise type's sheet. Cardio minutes use a single stepper (default 45).

`settings` gained four weekly-target columns, all **Monday–Sunday weeks**:
`cardio_weekly_target` (default 3), `strength_weekly_target` (default 2),
`yoga_weekly_target` (default 2), `meditation_weekly_days` (default 7 — a
days-per-week target, not a session count). Migration 006 (phase E3) added
`pt_weekly_days` (default 7), framed the same way as meditation.

API — same bearer auth, JSON in/out as the rest of `/api`:

```
POST   /api/exercise            {day,type,activity,location,style,duration_min,hr_zones,note,input_kind} → 201 ExerciseSession
GET    /api/exercise?start=&end=                                → [ExerciseSession]  (day range, inclusive; both required)
GET    /api/exercise/{id}                                       → ExerciseSession
PUT    /api/exercise/{id}       same body shape as POST         → ExerciseSession
DELETE /api/exercise/{id}                                       → 204
```

No weekly-rollup endpoint — the home card and trends (exercise phases 2–3)
read raw sessions via the range route and aggregate client-side. The five
targets (cardio/strength/yoga/meditation/pt) are included in
`GET /api/settings` / accepted by `PUT /api/settings`.
`GET /api/export` includes `exercise: [ExerciseSession]`.

### PWA (phase E2): Training card + tap-log

On Today, under the calorie hero, a **Training · this week** card shows one
row per practice — this is the whole reminder-to-move, no nudge sentence.
Every activity (Cardio, Strength, Yoga, Meditation, PT) is a uniform **7-box
Mon–Sun row**: an icon tile, seven day-boxes, then goal circles. A box gets a
dot on any day that activity was logged (binary per day — a second same-day
session doesn't double up); cardio boxes instead show the specific activity
icon for that day (Run/Ride/🚴/Spin=inline spin-bike SVG/Swim/Hike/Row) so the
kind of cardio reads at a glance. Beside the boxes are `max(0, target − daysDone)`
hollow **circles** — the sessions still left toward the weekly goal — and
nothing once met (no badge, no check). Today's column sits behind a pale grey
vertical lane. There is no `n/target` badge and no `last: <Wkday>` subtitle.
Tapping a row (or an empty box) opens a quick-log bottom sheet — chip pickers
with sensible defaults, 2–3 taps, no AI call; saving always appends
(`POST /api/exercise`, `input_kind: 'tap'`, `day` = the currently-viewed day),
never overwrites. Tapping a filled box reopens that session for edit/delete.
The PWA fetches the current week plus an
8-week lookback once per Today load (`GET /api/exercise?start=&end=`) and
recomputes the card from that cache as the viewed day changes. Settings
gained a "Weekly training targets" group for the four columns above.

### PWA (phase E3): Training trends + PT

The Trends tab gained a **Food · Training** segmented control alongside the
existing Week/Month one; "Training" swaps in a **2 wk · 6 wk · 12 wk** range
control that scopes every card below plus the table. All Training data comes
from `GET /api/exercise?start=&end=` for the selected range (Mon-anchored
weeks, oldest to newest, ending at the current in-progress week) and is
aggregated into per-week buckets client-side — same no-rollup-endpoint
pattern as the home card.

- **Active minutes per week** — inline-SVG stacked column chart (cardio,
  yoga, meditation, PT bottom-to-top; strength has no duration so it rides a
  session-count row under the x-axis instead, marked with 🏋️). Each segment
  prints its session count (cardio/yoga) or day count (meditation/PT) when
  tall enough (≥16px); the in-progress current week renders hollow
  (stroke-only) rather than filled, the same honesty convention as the food
  trends. x-axis labels every week at ≤6 bars, or month ticks at 12.
- **Mix cards** — Cardio mix (horizontal bars by activity), Yoga (studio/home
  split bar + style tally), Meditation and PT stat tiles (min/day and
  days/week averages over **completed weeks only**, excluding the current
  partial week). No separate strength card — its numbers live in the
  under-axis row on the minutes chart.
- **Table view** — a `<details>` twin with per-week cardio/strength/yoga/
  meditation/PT/total numbers, the WCAG-clean fallback for anything the chart
  conveys by color or tooltip alone. Current week marked `*`.
- Layout is single-column on phone widths and a wider `<main>` + row/grid mix
  cards on desktop (`main.wide`, ≥720px) — the one place in the PWA that
  isn't fixed to the 480px phone-first shell.

PT (migration 006, `internal/food/types.go` `ExercisePT`) is a fifth exercise
type that behaves exactly like meditation everywhere: `validateExercise`
requires `duration_min > 0` and nothing else, the home card gained a PT habit
row (7-dot Mon–Sun, `daysDone/pt_weekly_days`, 🩼 icon — a placeholder Jim
may swap later) directly under the meditation row, its quick-log sheet is a
Minutes chip picker (`[10 · 15 · 20 · 30 · 45]`, default 15), and Settings
gained a "PT days / week" input beside the meditation one. Color tokens
`--s-cardio`/`--s-strength`/`--s-yoga`/`--s-med`/`--s-pt` (CVD-checked as a
five-color set, `--s-strength` and `--s-pt` step in dark mode) drive the
chart segments and legend; chart text (counts, axis, labels) always wears the
app's text/muted tokens, never a series color.

### PWA (phase E4): NL/voice exercise logging

The one log bar (typed or dictated) understands exercise the same way it
understands food. When `/api/parse` returns `kind: "exercise"`, the PWA shows
a confirm sheet instead of the meal draft dialog — one block per parsed
session using the same chip-picker fields as the phase-2 quick-log sheet
(`EX_FIELDS`), so Jim can fix activity/minutes/etc., or remove a session,
before saving. Save posts once per session to `POST /api/exercise` with
`input_kind: 'text'` or `'voice'` (mirroring how the mic sets `input_kind` on
the food path) and the currently-viewed `day`, then refreshes the Training
card. Meals still render the existing draft dialog; photos are always food.

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
