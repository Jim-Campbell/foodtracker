# Exercise Phase 3 — PWA: Training trends

Read `CLAUDE.md`, `ARCHITECTURE.md` (→ "Exercise"), and the prototype artifact
<https://claude.ai/code/artifact/63d7ec19-4c72-4e71-b15f-5ab2f05db913> — the
"Training trends" section is the **exact target** for this phase (Direction 2,
final). Exercise phases 1–2 are built. This phase adds the Training view to the
existing **Trends** tab. Charts are inline SVG built by string templates — no
libraries, same as the food trends.

Jim reviews trends on a phone but wants them to shine on a laptop: the layout is
single-column on narrow screens and a responsive grid on wide ones.

## Structure

At the top of the Trends tab add a **Food · Training** segmented control. "Food"
shows the existing calorie/protein/weight charts unchanged. "Training" shows the
exercise charts below. A range control **2 wk · 6 wk · 12 wk** sits in the same
filter row and scopes every Training card + the table. All data comes from
`GET /api/exercise?start=&end=` for the selected Mon–Sun range; aggregate
client-side.

## Cards (in order)

1. **Active minutes per week** — a stacked column per week: cardio + yoga +
   meditation **minutes** (strength has no minutes, so it is excluded from the
   bars). 2px surface gap between segments; top segment gets 4px rounded cap.
   - **Inside each segment, print its session count** — a bare number (cardio/
     yoga = sessions that week; meditation = days). Render the label only when
     the segment is tall enough (≥16px); otherwise omit it (the tooltip + table
     still carry it) — never clip.
   - **Strength rides a row beneath the x-axis**: a 🏋️ marker at the left and,
     under each week's column, that week's strength **session count** (`–` for
     zero). This keeps strength visible without inventing a duration.
   - The **in-progress current week is drawn hollow** (outline only, colored
     stroke, count in ink) — the honesty convention from the food trends.
   - x-axis: label every week when ≤6 shown; month ticks (May/Jun/Jul) at 12.
   - Legend for the three stacked series (dot + name); values live in tooltips
     and the table, not on every bar.

2. **Mix cards** (responsive row): 
   - **Cardio mix** — horizontal bars of minutes by activity (Run/Bike/Hike/
     Swim/Row), value printed at each bar end, scoped to the range.
   - **Yoga** — a Studio vs Home split bar + a style tally (Vinyasa/Hot/Other).
   - **Meditation** — a small stat tile: min/day average and days/week average
     over **completed weeks only** in the range (exclude the current partial
     week).
   (There is intentionally **no** separate strength card — its data lives in the
   row under the minutes chart, per Jim's decision.)

3. **Table view** — a `<details>` twin listing per week: cardio sessions/min,
   strength sessions, yoga sessions/min, meditation days/min, total min. The
   current week marked `*`. This is the WCAG-clean equivalent so nothing is
   color- or tooltip-only.

## Color tokens (add to the PWA's `:root`, both themes)

These extend the app palette and are CVD-validated in light and dark:

```
--s-cardio: #c2542a;   /* app terracotta */
--s-strength: #d99a2b;  /* app amber; dark mode step ↓ */
--s-yoga: #3a8f5c;      /* app green */
--s-med: #5271c4;       /* NEW indigo token — the app had no 4th hue */
```
In `@media (prefers-color-scheme: dark)` and the `[data-theme]` overrides, step
strength to `--s-strength: #c08417`. Chart **text** (counts, axis, labels) wears
the app's text/muted tokens, never a series color — identity comes from the
colored segment beside it. Amber sits below 3:1 on white, which is why the
in-bar counts and the table exist (the required relief channel); keep both.

## Interaction

Hover/focus tooltips on every segment, strength count, and split-bar region
(week label + breakdown). Tap targets ≥ the mark; keyboard-focusable
(`tabindex`, focus ring). Respect `prefers-reduced-motion`.

## Out of scope

NL/voice logging (exercise phase 4), Garmin import, any change to the food
trends charts or the calorie/score math. No heatmap (it was prototyped and
cut).

## Acceptance checklist

Server running with a couple months of seeded sessions, tested at phone width
and laptop width:

- Trends → Training shows the stacked minutes chart with session counts inside
  the segments and a strength count row beneath the axis.
- Switching 2 / 6 / 12 wk rescopes every card and the table; at 2 wk the x-axis
  labels each week, at 12 wk it shows month ticks.
- The current (partial) week renders hollow in the minutes chart.
- Cardio mix, yoga split, and meditation tile match the table numbers for the
  selected range; meditation averages exclude the current week.
- A colorblind check (or the table) can recover every value — nothing is
  color-only.
- Dark mode: amber steps down, all four series stay distinct, text stays legible.
