# Inclusion Phase 2 — Tag capture: parser, draft card, backfill

Read `CLAUDE.md`, `ARCHITECTURE.md` (→ "AI meal parsing", "Resolution layer",
and the new "Inclusion score" section), and
`build-prompts/inclusion-spec-20260726.md` §5. Inclusion phase 1 is built: the
`food_component_tags` table, the catalog, the math, and the API all exist.

This phase **fills the tag table**. It is the substantive implementation work of
the whole feature — the score is only as good as the tags behind it. The
principle (spec §5) is: **tag once at the moment a food is first resolved,
persist, reuse on every subsequent encounter.** The table fills from actual
eating, not from an upfront taxonomy exercise.

## 1. The parser proposes tags

`internal/ai/tools.go` — add an optional `components` array to each item in
`recordMealSchema`:

```json
"components": {
  "type": "array",
  "description": "Diet-framework inclusion components this food contributes to, or omitted/empty when it contributes to none. Most foods contribute to none.",
  "items": {
    "type": "object",
    "properties": {
      "component_id": { "type": "string", "enum": ["leafy_greens","berries","legumes","fatty_fish","cruciferous"] },
      "grams_per_serving": { "type": "integer", "description": "Grams of THIS food that make one serving of that component. For a whole food use the standard serving weight; for a composite dish use the grams of the dish that carry one serving (e.g. 400 g of lentil soup = 1 legume serving)." }
    },
    "required": ["component_id", "grams_per_serving"]
  }
}
```

`internal/ai/parser.go` — carry `components` onto the draft `MealItem` (a new
`Components []ItemComponent` field on `MealItem`, draft-only until save, added to
the `cleanItem` strip list in the PWA the same way `alternatives` is). Validate
server-side: unknown `component_id` dropped, `grams_per_serving` clamped to
1..2000, duplicates collapsed. The model never writes to the DB — this is a
draft proposal like everything else.

### System-prompt rules (`(p *Parser) systemPrompt`)

Encode the spec's decisions verbatim in intent; these are the cases that decide
whether the score means anything:

- **Kale → both `leafy_greens` and `cruciferous`.** Double-counting is
  deliberate — the framework lists the two categories separately with distinct
  rationales.
- **Peach, apple, banana, melon, whole fruit generally → no component.** Whole
  fruit is a distinct framework line from berries. (This is a real miss from the
  2026-07-26 log: a raw peach must not resolve to `berries`.)
- **Starchy vegetables** (potato, sweet potato, winter squash, corn) → **no
  component**; framework line 89 treats them separately.
- **Nuts, seeds, nut butters, olive oil, avocado → no component.** There is no
  such component and there will not be one.
- Lean fish (cod, halibut, tilapia, sole) is **not** `fatty_fish`. Fatty fish is
  salmon, sardines, mackerel, herring, anchovies.
- Tag only the component-bearing part of a composite dish, via
  `grams_per_serving` on the whole dish.
- Default serving weights when nothing better is known: greens 30 g raw /
  85 g cooked, berries 75 g, legumes 90 g cooked, fatty fish 100 g,
  cruciferous 85 g. Prefer the FDC portion data when the search result carries a
  household measure.
- When in doubt, **omit** — a wrong tag is worse than a missing one, because a
  missing one is visible in the card and a wrong one silently inflates the score.

### Reuse via canonical lookup

`canonicalLookupTool` (`execCanonicalLookup`) already returns a cached food's
per-100g, tier, and portion. Extend its result payload with that food's existing
component tags and tell the model in the tool description to reuse them verbatim
on a hit. This is the "tag once, reuse forever" mechanic, and it keeps the model
out of the decision on foods Jim eats every week.

## 2. Persist on save

In `internal/food/service.go`'s save path, alongside `accreteCanonical`: for each
saved food carrying components, upsert into `food_component_tags` keyed on
`NormalizeFoodName(item.Name)` (plus `fdc_id` when present), `tag_source: 'ai'`.

**A `tag_source: 'user'` row is never overwritten by an AI proposal.** Jim's
correction is the durable one. Failures are logged and swallowed exactly like
canonical accretion — tagging must never fail a save.

## 3. Draft card UI (`pwa/index.html`)

Bump the `.buildstamp` (`20260726.N` → next N; new date → `.1`).

Below the existing `matchRow` in `draftItemRow`, add a **component row**:

- Proposed components render as filled chips: `🥬 Leafy greens · 1.5 svg` — the
  serving figure computed live from the current `grams` × `fraction_pct` and the
  chip's `grams_per_serving`, so editing grams updates it immediately. Format the
  number from integer hundredths (`x100`); never do float math on stored values.
- A small `+` opens the full five-chip picker; tapping a chip toggles it on/off
  with the component's default grams-per-serving.
- When the model proposed nothing and the food is one of the obvious misses,
  **show nothing but the `+`** — no empty row, no nagging.
- Never blocks the save, exactly like the match row.
- Tapping a chip's serving figure opens a tiny grams-per-serving input (this is
  how a composite dish gets corrected once, forever).

Chips carry a `user` source when Jim touches them, so the save writes
`tag_source: 'user'`.

Also add the same chip row to the **edit-food sheet** so a mis-tagged food
already in the log can be fixed without re-logging.

## 4. Backfill (needed — the card is empty on day one otherwise)

The window is 7 days and the tag table starts empty, so without this the Today
card in phase 3 reads all zeros for a week. Build a **Settings → "Tag foods"**
screen:

- Lists distinct food names from the last 30 days of `meal_items` plus every
  `canonical_foods` row, **untagged first**, with times-logged as the sort key so
  the highest-leverage foods are at the top.
- Each row: name, its current chips, tap to toggle components — same picker
  component as the draft card.
- A **"Suggest tags"** button posts a batch of untagged names to a new
  `POST /api/component-tags/suggest`, which runs **one** Claude call and returns
  proposals. Proposals render as *unconfirmed* chips that Jim accepts or
  rejects; **nothing is written until he accepts** — the AI-never-writes
  invariant holds here too.
- Show a running count: `31 of 68 foods tagged`.

Keep the endpoint's prompt short and reuse the same component rules as the
parser system prompt (factor them into one shared string constant rather than
maintaining two copies).

## Non-goals

- Do not change the composition score, tiering, or nutrition resolution.
- Do not auto-write AI tag suggestions without confirmation.
- Do not add components beyond the five.
- No Today-card or Trends UI in this phase (phases 3–4).

## Acceptance checklist

- `go build ./... && go test ./...` passes; `gofmt -l .` empty; build stamp
  bumped.
- Parser tests (`internal/ai/parser_test.go` style, recorded fixtures) cover: a
  kale item producing both tags; a peach producing none; an unknown
  `component_id` being dropped rather than 500-ing.
- Logging "spinach salad, 2 cups" through the real flow shows a
  `🥬 Leafy greens · 2 svg` chip in the draft, saves, and appears in
  `GET /api/component-tags`.
- Logging the same food again the next day reuses the tag with no re-decision
  (visible as a `canonical_lookup` hit in the parse log).
- Toggling a chip off in the draft and saving writes `tag_source: 'user'`, and a
  later AI proposal for that food does **not** overwrite it.
- Settings → Tag foods lists real logged names, "Suggest tags" returns
  proposals, and nothing persists until accepted.
- `GET /api/inclusion` now returns non-zero counts against real logged history.
