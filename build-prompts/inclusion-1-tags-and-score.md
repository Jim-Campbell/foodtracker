# Inclusion Phase 1 — Component tags + inclusion score (data + API, no UI)

Read `CLAUDE.md`, `ARCHITECTURE.md`, `docs/diet-framework.md`, and
`build-prompts/inclusion-spec-20260726.md` (the design source of truth — §1–§4
define the construct, §5 the tag layer). This phase builds the **data layer and
the math**. No PWA changes at all; everything here is curl-testable.

## What this is

The existing day score (`internal/food/score.go` → `DayScore`) is
calorie-weighted: *of the calories I ate, what tier were they?* That is a
**composition** score and it stays exactly as-is. This phase adds a **second,
independent** score — **inclusion**: *did I actually eat the handful of foods
with the strongest evidence behind them?* Calorie-trivial foods (kale, berries)
can never move a calorie-weighted mean, which is a structural flaw, not a
calibration one.

**The two scores are never merged, never averaged, never combined into one
number.** Do not modify `DayScore`, `TierValue`, `DaySummary.Score`, or anything
under the "Quality score" heading in ARCHITECTURE.md.

## Components (fixed at five)

| id | label | emoji | weekly target | 1 serving | default g/serving |
|---|---|---|---|---|---|
| `leafy_greens` | Leafy greens | 🥬 | 7 | 1 cup raw / ½ cup cooked | 30 |
| `berries` | Berries | 🫐 | 5 | ½ cup | 75 |
| `legumes` | Legumes | 🫘 | 5 | ½ cup cooked | 90 |
| `fatty_fish` | Fatty fish | 🐟 | 3 | 3–4 oz | 100 |
| `cruciferous` | Cruciferous | 🥦 | 4 | 1 cup raw / ½ cup cooked | 85 |

Define these once in Go (`internal/food/inclusion.go`) as an ordered slice —
id, label, emoji, target, serving text, default grams-per-serving — and export
it. It is the single source of truth for every later phase and for the API.

**Do not add a nuts/seeds component** (spec §2 — measured at 185 cal/day for ~3g
protein/day; the target would sit permanently satisfied and contribute only
noise). Phase-2 components (EVOO, fermented, whole grains) are **not** built
now; leave no placeholders for them beyond the fact that the slice is a slice.

Targets live in Go constants for now. A settings override lands in inclusion
phase 5 — don't build it here.

## Migration `internal/db/migrations/013_component_tags.sql`

```sql
CREATE TABLE food_component_tags (
    id                BIGSERIAL PRIMARY KEY,
    normalized_name   TEXT   NOT NULL,       -- food.NormalizeFoodName(item name)
    fdc_id            BIGINT,                -- when the food resolved to an FDC entry
    component_id      TEXT   NOT NULL CHECK (component_id IN
                        ('leafy_greens','berries','legumes','fatty_fish','cruciferous')),
    grams_per_serving INT    NOT NULL CHECK (grams_per_serving BETWEEN 1 AND 2000),
    tag_source        TEXT   NOT NULL DEFAULT 'ai' CHECK (tag_source IN ('ai','user')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX food_component_tags_name_idx
    ON food_component_tags (normalized_name, component_id);
CREATE INDEX food_component_tags_fdc_idx
    ON food_component_tags (fdc_id) WHERE fdc_id IS NOT NULL;
```

### Two deliberate departures from spec §5 — implement these, not the literal spec

1. **Keyed on `normalized_name`, not `fdc_id`.** The spec's table is
   `(fdc_id, component_id)`. In this codebase a large share of logged foods have
   no `fdc_id` at all (`off` barcodes, `label` photos, `web`, `ai` estimates),
   and the app already has an accretion key that covers every food:
   `canonical_foods.normalized_name` (see `internal/food/canonical.go`). Keying
   on the name makes the tag layer work for every food; `fdc_id` is kept as a
   nullable secondary key so an FDC-resolved food still matches when the display
   name drifts. **Lookup order: `fdc_id` exact hit → `normalized_name` hit →
   untagged.**
2. **`grams_per_serving` instead of `servings_per_ref`.** Servings must be
   derived from a logged portion, and every portion in this app is integer
   grams. Storing grams-per-serving makes the reference explicit, keeps the math
   integer, and handles composite dishes naturally — "lentil soup" can carry
   `legumes @ 400 g/serving` while "cooked lentils" carries `legumes @ 90`.

Multi-tag is expected and allowed: kale is `leafy_greens` **and** `cruciferous`
(spec §5 — the framework lists them separately with distinct rationales).

## Go — `internal/food/inclusion.go`

**No floats anywhere** (CLAUDE.md invariant). Servings are integer hundredths
throughout: `servings_x100`, where 250 = 2.5 servings. Multiply before dividing.

```go
// per logged food, per matching tag:
gramsEaten   := int64(*it.Grams) * int64(it.FractionPct) / 100
servingsX100 := gramsEaten * 100 / int64(tag.GramsPerServing)
```

- A food with `Grams == nil` or `<= 0` **cannot contribute** — skip it and count
  it into an `UnmeasuredFoods` field on the result so the miss is visible rather
  than silent.
- **Per-meal cap: 200 (2.00 servings) per component per meal**, where "meal" is
  the `(day, slot)` container (migration 012's model — one Breakfast/Lunch/
  Dinner/Snack per day). Cap the *sum* for a component within one container, not
  each food. Rationale (spec §2): the construct is exposure frequency over time,
  not volume — one enormous salad must not satisfy the weekly greens target.
  Put the cap in a named constant `MaxServingsX100PerMeal = 200`; it is spec §10
  open decision 1 and must be one line to change.
- Window: **last 7 calendar days inclusive of `end`** — `end-6 .. end`. Rolling,
  never calendar weeks (spec §4). Food has unlimited slots and fully recoverable
  misses, so there is nothing to write off and no reset.
- Per component: `progressPct = min(countX100 / target, 100)`. The **1.0 cap is
  non-negotiable** — without it six servings of berries paper over zero fatty
  fish, which is the composition score's failure mode inverted.
- `InclusionScore = sum(progressPct) / len(components)`, integer.
- Also compute, per component: `Whole = countX100 / 100` (whole servings, for
  the dot row), `Met = countX100 >= target*100`, and `ExpiringWhole` — the whole
  servings logged on **`end-6`**, i.e. the ones that age out at the start of
  tomorrow. Phase 3 renders those faded; the math belongs here.

Suggested shape (adjust names to taste, keep them stable — later phases build on
them):

```go
type ComponentDef struct { ID, Label, Emoji, ServingText string; Target, DefaultGramsPerServing int }
type ComponentProgress struct {
    ID, Label, Emoji string
    Target        int   `json:"target"`
    ServingsX100  int64 `json:"servings_x100"`
    Whole         int   `json:"whole"`
    ExpiringWhole int   `json:"expiring_whole"`
    ProgressPct   int   `json:"progress_pct"`
    Met           bool  `json:"met"`
}
type InclusionWindow struct {
    Start, End      string
    Score           int
    Components      []ComponentProgress
    UnmeasuredFoods int
}
```

Write the counter as a **pure function** over `([]Meal, map[tagKey][]ComponentTag,
window bounds)` so it is testable with no DB. The service method just loads and
calls it.

## Store + service

- `internal/food/store.go` (the `Store` interface) gains:
  ```go
  ListComponentTags(ctx context.Context) ([]ComponentTag, error)
  SetComponentTags(ctx context.Context, normalizedName string, fdcID *int64, tags []ComponentTag, source string) error // replaces the whole set for that name
  ```
  The table is small (one row per food per component) — load it all and match in
  Go rather than doing SQL joins. Implement in `internal/db/store.go` following
  the existing canonical-foods methods; mirror the fake in
  `internal/food/fake_store_test.go`.
- `internal/food/service.go` gains `InclusionWindow(ctx, end string)` (defaults
  `end` to today when empty, validates the date like every other method) and
  `InclusionWeeks(ctx, start, end string)` returning **completed Mon–Sun calendar
  weeks** for the Trends retrospective (spec §8 — calendar weeks apply to food in
  exactly one place, and this is it). Reuse `ListMealsRange`.
- Add the tag table to `Export` (`ExportDoc` gains `ComponentTags`) so a backup
  is still a full backup.

## API (`internal/api/food.go`, in `Routes`)

```
GET /api/components                  → the ComponentDef catalog (ids, labels, emoji, targets, serving text)
GET /api/inclusion?end=YYYY-MM-DD    → InclusionWindow (end defaults to today)
GET /api/inclusion/weeks?start=&end= → []{week_start, components_hit, components:[...]}, completed weeks only, newest first
GET /api/component-tags              → all tags, newest first
PUT /api/component-tags              → {normalized_name, fdc_id?, tags:[{component_id, grams_per_serving}]} replaces that food's set, tag_source 'user'
DELETE /api/component-tags/{name}    → clears a food's tags
```

Validate `component_id` against the catalog and `grams_per_serving` into
1..2000; reject unknown ids with 400. Follow the existing handler idioms
(`writeJSON`, `writeError`, `h.fail`).

## Tests (`internal/food/inclusion_test.go`)

Cover, at minimum:

- Straight count: 3 logged cups of greens at 30 g/serving → `ServingsX100 == 300`.
- `fraction_pct` scaling: 200 g of a 100 g/serving food at `fraction_pct: 50`
  → 100 (1.00 serving).
- **Per-meal cap:** 400 g of greens at 30 g/serving in one slot → capped at 200,
  not 1333. Two slots each over the cap → 400 total.
- **Progress cap:** 10 servings of berries against a target of 5 →
  `ProgressPct == 100`, and the score still reflects zeros elsewhere (breadth is
  the point).
- Window edges: a serving logged on `end-6` counts and is reported in
  `ExpiringWhole`; the same serving on `end-7` does **not** count at all.
- Untagged foods and `Grams == nil` foods contribute nothing;
  `UnmeasuredFoods` counts the latter.
- Multi-tag: one kale food credits both `leafy_greens` and `cruciferous`.
- All-zero day → score 0 (not an error, not nil) — an empty window is a real
  answer here, unlike the composition score.
- A regression test asserting the composition `DayScore` for a fixture is
  unchanged by anything in this phase.

## Out of scope for this phase

Tag capture in the parse flow (phase 2), any PWA change (phases 3–4), nudges
(phase 5), settings overrides (phase 5), backfilling existing history (phase 2).
Do not touch `pwa/index.html` in this phase at all.

## Acceptance checklist

- `go build ./... && go test ./...` passes; `gofmt -l .` is empty.
- Migration 013 applies cleanly to a scratch DB
  (`createdb food_smoke`, run the server, `dropdb food_smoke` after).
- With a couple of hand-inserted tags and a few logged foods:
  `curl -H "Authorization: Bearer $FOOD_API_KEY" localhost:8082/api/inclusion`
  returns five components in catalog order with sane counts, an integer score,
  and `expiring_whole` populated for a food logged 6 days ago.
- `PUT /api/component-tags` then `GET` round-trips; a second PUT for the same
  name replaces rather than duplicates (unique index holds).
- `GET /api/inclusion/weeks` returns only completed Mon–Sun weeks.
- `GET /api/export` includes `component_tags`.
- ARCHITECTURE.md has a new **"Inclusion score"** section, placed after
  "Quality score", stating plainly that the two scores are independent and that
  the calorie-weighted one is unchanged.
