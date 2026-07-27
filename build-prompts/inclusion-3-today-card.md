# Inclusion Phase 3 — Today card: `FOOD · LAST 7 DAYS`

Read `CLAUDE.md`, `ARCHITECTURE.md` (→ "PWA", "Inclusion score"), and
`build-prompts/inclusion-spec-20260726.md` §6 — that section is the **exact
target** for this phase. Inclusion phases 1–2 are built: `GET /api/inclusion`
returns real counts and the tag table is populated.

This phase is `pwa/index.html` only, plus an ARCHITECTURE.md note. **Bump the
`.buildstamp`** (`20260726.N` → next N; new date → `.1`).

## The one constraint that decides the design

The Training card uses an **M–T–W–T–F–S–S day grid** (`trainingCardHTML`,
`trainRowHTML`, the `.ti-*` classes). The food card must **not**. If both cards
show day columns in adjacent cards, the columns mean different things one above
the other, which is the single genuinely bad outcome here.

A grid answers *when*. A meter answers *how many*. Different question, different
shape. So:

- **No day-of-week columns. No date axis of any kind. No calendar-week logic.**
- The dot vocabulary — filled = done, open circle = remaining — is deliberately
  the **same** as the Training card's `left` column (`.ti-circle`), so there is
  nothing new to learn. Reuse the visual language, not the layout.

## Placement and shape

In `renderToday`, insert the card **directly below** `trainingCardHTML()` and
above the meal slots. Header: `FOOD · LAST 7 DAYS` (rendered with the same `h3`
treatment the other cards use — `Food · last 7 days` matching the app's existing
sentence-case headers is fine; match `Training · this week`).

```
FOOD · LAST 7 DAYS

🥬  Leafy greens    ● ● ● ● ● ○ ○     5/7
🫐  Berries         ● ◐ ○ ○ ○         2/5
🫘  Legumes         ● ● ● ○ ○         3/5
🐟  Fatty fish      ● ○ ○             1/3
🥦  Cruciferous     ● ● ● ●           4/4 ✓
```

Rules, all of them load-bearing:

- **Dot positions per row = that component's `target`.** Row lengths differ by
  design; the length itself carries information. Do not pad rows to a common
  width, do not right-align them into a column grid.
- **Filled `●`** — a whole serving inside the window (`whole` from the API).
- **Faded `◐`** — a serving that ages out within 24h (`expiring_whole`; it was
  logged on `end-6`). Render the *last* `expiring_whole` of the filled dots
  faded. This is the **decay preview**: a count can fall tomorrow with no action
  taken, and the display itself is where that shows up — no notification, no
  nagging, no honesty risk. It is only information about a window sliding.
- **Open `○`** — remaining to target.
- `count > target`: render exactly `target` filled dots plus `✓`. **Never render
  overflow.** The cap is the point of the score.
- Completed rows are de-emphasized (muted label, as the met training rows are).
- The numeric on the right is `servings/target`. Format from integer hundredths:
  show a whole number when the remainder is zero, otherwise one decimal
  (`2.5/5`). Do the formatting in string-land — no float arithmetic on stored
  values (CLAUDE.md invariant).

Use fresh `.fc-*` classes; do not reuse or restyle `.ti-*` (the Training card
must be untouched — its markup, spacing, and behavior stay exactly as they are).

## The score itself

Show the inclusion score as a small number on the right of the card header
(`62`), styled quieter than the hero's composition badge — it is a second
signal, not a competing headline. Tapping it opens an info dialog in the style of
`showScoreInfo()` explaining the two scores in two sentences:

> **Composition** (the badge up top) asks: *of the calories I ate, what tier
> were they?* **Inclusion** asks: *did I eat the foods with the strongest
> evidence behind them?* A calorie-weighted average can't register a cup of kale,
> so this counts servings instead.

**Do not touch `scoreBadge`, `scoreColor`, `showScoreInfo`, the hero card, the
ring, or `macroRow`.** The two scores are never merged and never shown as one
number.

## Data loading

Add `inclusion: null` to `S` and load it exactly like `S.exercise` — lazily from
`renderToday` when null, via `api('/inclusion')`, then `render()`. Invalidate it
(`S.inclusion = null`) wherever `invalidateExercise()`-style invalidation
happens after a food save, delete, or day change — a saved salad must move the
dots without a reload. Loading state: the card's own `Loading…` line, same as
the Training card, not a whole-page spinner.

Empty state (no tags at all yet, everything zero): render the rows with all open
circles and a single muted line — `Tag foods as you log them to fill this in`
linking to Settings → Tag foods. Never a scolding empty state.

## Also

- Tapping a component row opens the quick log bar prefilled with nothing — or,
  simpler and preferred for this phase, does nothing at all. **Do not** invent a
  logging flow from this card; it is a display.
- Keyboard/focus: rows are not interactive except the score, so nothing else
  needs a focus ring.
- Check both themes and both widths (phone ~390px and laptop). Dot rows must not
  introduce horizontal scroll at 320px — the app has a live bug history here
  (see commit `ea99011`), so verify at that width specifically.

## Acceptance checklist

Server running against the smoke DB with a week of tagged history:

- The food card sits below Training, has no day columns, no dates, and no
  weekday letters anywhere.
- Row lengths differ per target (7, 5, 5, 3, 4 dots).
- A component over target shows `target` dots plus `✓` and nothing more.
- A serving logged exactly 6 days ago renders as the faded `◐`; the same serving
  7 days ago does not appear at all.
- The score in the header matches `GET /api/inclusion`'s `score` exactly.
- The hero composition badge, ring, protein bar, macro row, and the whole
  Training card are pixel-identical to before this phase.
- No horizontal scroll at 320px; dark mode legible; `.buildstamp` bumped.
- ARCHITECTURE.md's PWA section documents the card and states the
  grid-vs-meter constraint so it survives the next redesign.
