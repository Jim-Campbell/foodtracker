# Inclusion Phase 4 — Trends: weekly inclusion retrospective

Read `CLAUDE.md`, `ARCHITECTURE.md` (→ "PWA", "Inclusion score"), and
`build-prompts/inclusion-spec-20260726.md` §8. Inclusion phases 1–3 are built;
`GET /api/inclusion/weeks?start=&end=` already returns completed Mon–Sun weeks.

Small phase. `pwa/index.html` only (plus an ARCHITECTURE.md line). **Bump the
`.buildstamp`.**

## What goes where

A new card at the **bottom of the Food tab in Trends** (`renderTrends`, the
`S.trendsTab === 'food'` path) — below the Weight card. Not on Today.

**Calendar weeks (Mon–Sun) apply to food in this one place and nowhere else.**
Today's card is rolling and stays rolling; this card is retrospective, and
retrospective evaluation is exactly what calendar periods are good for. Do not
let the two logics leak into each other — no rolling window in this card, no
calendar week in the Today card.

```
Week of Jul 20    3/5 components

🥬 Leafy greens   6/7
🫐 Berries        2/5
🫘 Legumes        5/5  ✓
🐟 Fatty fish     3/3  ✓
🥦 Cruciferous    4/4  ✓
```

Rules:

- **Completed weeks only.** The in-progress week is excluded entirely — it is not
  a retrospective yet. (The Training trends chart draws the current week hollow;
  here, omit it. Different construct: there is nothing to evaluate mid-window.)
- Newest week first. Scope to the existing Food range control (`S.trendsRange`,
  Week/Month) — `week` shows the single most recent completed week, `month`
  shows the last four or five.
- Header per week: `Week of <Mon d>` and `n/5 components` where `n` counts
  components that reached target.
- Per component: `count/target`, with `✓` on the ones that hit. No dots here —
  the dot meter is the Today card's shape; this is a table-ish recap and should
  read as one. Reuse the numeric formatting helper from phase 3 (integer
  hundredths → string).
- De-emphasize met rows the same way phase 3 does, so the eye lands on the
  misses.
- Empty state (no completed weeks with any tagged food yet): one muted line, no
  empty scaffolding.

## Data

`loadTrends()` already fetches the food range; add the weeks fetch alongside it
(`Promise.all`) into `S.inclusionWeeks`, and null it in `setTrendsRange` the same
way `S.trendRows` is nulled. Do not fetch on every `render()`.

## Non-goals

- No inclusion chart, sparkline, or trend line. Five numbers per week is the
  whole design; a chart of a five-component breadth score over four weeks is
  noise.
- Do not touch the calorie, macro, or weight charts, or the Training tab.
- Do not add the inclusion score to the Food tab's `avg score` tile — that tile
  is the composition score and stays that.

## Acceptance checklist

- Trends → Food, at both Week and Month, shows the retrospective card at the
  bottom with correct per-component counts matching `GET /api/inclusion/weeks`.
- The current, in-progress week never appears.
- Switching Week/Month rescopes it; switching to the Training tab and back does
  not refetch or flicker.
- Today's card is unchanged and still rolling.
- Phone and laptop widths both clean; dark mode legible; `.buildstamp` bumped.
- `go build ./... && go test ./...` still passes.
