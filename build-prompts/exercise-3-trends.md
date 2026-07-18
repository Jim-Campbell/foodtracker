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

## Also: add Physical Therapy (PT) — a fifth type, mirroring meditation

PT is a new tracked type that behaves **exactly like meditation**: a daily-habit
row (did it + minutes), no activity/location/style. Because E1 (data) and E2
(home card) were already built before PT was requested, this phase carries the
full PT slice — data layer, home card retrofit, and trends — as one additive
change. The rule of thumb: **wherever the code special-cases `meditation`, add a
parallel `pt` case.** PT is calorie-independent like everything else here.

### Data layer (E1 is already built — extend it)

1. **Migration** `internal/db/migrations/006_exercise_pt.sql`:
   ```sql
   ALTER TABLE exercise_sessions DROP CONSTRAINT exercise_sessions_type_check;
   ALTER TABLE exercise_sessions ADD CONSTRAINT exercise_sessions_type_check
       CHECK (type IN ('cardio','strength','yoga','meditation','pt'));
   ALTER TABLE settings ADD COLUMN pt_weekly_days INT NOT NULL DEFAULT 7;
   ```
   (`exercise_sessions_type_check` is the auto-generated name for E1's inline
   CHECK — confirm with `psql -c '\d exercise_sessions'` before writing it.)
   PT's weekly target defaults to **7 (daily)**, the same framing as meditation;
   Jim tunes it in Settings.
2. **Go:** add `ExercisePT = "pt"` beside `ExerciseMeditation` in
   `internal/food/types.go` and to the `validExerciseTypes` map; add
   `PTWeeklyDays int \`json:"pt_weekly_days"\`` to `Settings`. In
   `internal/food/service.go`, `validateExercise` gets a `case ExercisePT:`
   identical to the `ExerciseMeditation` case (requires `duration_min > 0`; no
   activity/location/style); extend the `UpdateSettings` non-negative check to
   include `PTWeeklyDays`. Thread `pt_weekly_days` through the settings
   read/write SQL in `internal/db/store.go`. Mirror the meditation validation
   test with a `pt` case.

### Home card (retrofit the E2 Training card)

3. Add a **PT habit row** to the Today Training card, identical in structure to
   the meditation row: a 7-dot Mon–Sun daily row (filled per day with any PT
   session, dashed on today if none), `daysDone/target` where target =
   `pt_weekly_days`, met/green treatment when `daysDone >= target`, and the same
   cross-week **"last:"** subtitle fallback. Place it directly below the
   meditation row (the two habit rows sit together, after cardio/strength/yoga).
   - Quick-log sheet: **Minutes** chips `[10 · 15 · 20 · 30 · 45]` (default 15 —
     PT sessions run a bit longer than a sit), free-text allowed, `input_kind:
     'tap'`, appends like every other type.
   - Icon: use 🩼 as a **placeholder** — leave it easy to swap; Jim will pick the
     final glyph.
   - If the E2 meditation row hard-coded `/7`, parameterize both rows by their
     setting (`meditation_weekly_days`, `pt_weekly_days`) so a changed target
     shows correctly.
4. **Settings:** add a "PT days per week" input beside the meditation-days input,
   saved via `PUT /api/settings`.

### Trends (this phase's core — PT joins the charts)

5. **Active minutes per week:** PT becomes a **fourth stacked series** (cardio,
   yoga, meditation, PT — strength still rides the count row below the axis, not
   the stack). The in-segment count for PT is **days** (like meditation). Add PT
   to the legend.
6. **Mix cards:** add a **PT tile** mirroring the meditation tile — min/day
   average and days/week average over completed weeks in the range.
7. **Table view:** add a PT column (days / min), same shape as the meditation
   column.
8. **Color token:** add `--s-pt: #b5548f` (light) / `#c56ba0` (dark) to `:root`
   and both theme overrides. This is CVD-validated as a set with the existing
   four (worst adjacent ΔE 17.5 light / 12.4 dark, all ≥3:1 on their surfaces).
   PT text still wears the app's text tokens, never the plum — identity comes
   from the segment/dot beside it.

## Out of scope

NL/voice logging (exercise phase 4 — PT is handled there), Garmin import, any
change to the food trends charts or the calorie/score math. No heatmap (it was
prototyped and cut).

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
- Dark mode: amber steps down, all five series (incl. PT plum) stay distinct,
  text stays legible.
- **PT:** `go build ./... && go test ./...` passes after migration 006 and the
  Go additions. A PT session logs from the home card's new habit row (days/7),
  its "last:" fallback works, `pt_weekly_days` is editable in Settings, and PT
  appears both as a stacked-minutes segment and a mix tile with matching table
  numbers.
