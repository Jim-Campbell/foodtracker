# Exercise Phase 2 — PWA: home Training card + quick-log + settings

Read `CLAUDE.md`, `ARCHITECTURE.md` (→ "Exercise"), and study the prototype
artifact before writing code:
<https://claude.ai/code/artifact/63d7ec19-4c72-4e71-b15f-5ab2f05db913>. Exercise
phase 1 (data + API) is built. This phase adds exercise **logging and the
home-screen reminder** to the single-file PWA (`pwa/index.html`). No trends
(exercise phase 3), no natural-language parsing (exercise phase 4) yet.

The artifact is a faithful design mock of the target look, units, and
interactions — match its behavior, but write real code against the app's
existing `S`-state / `render()` / `openDialog()` conventions rather than copying
the mock's standalone scaffolding.

## The reminder, in one sentence

Under the calorie hero on **Today**, a **Training · this week** card shows one
row per practice with progress toward a weekly target — this is the whole
"subtle reminder to move" and it must be visible without scrolling past the
hero.

## Tasks

1. **State + fetch.** On loading Today, fetch the current Mon–Sun week's
   sessions **plus a lookback** (fetch `GET /api/exercise?start=&end=` with
   `start` = Monday of 8 weeks ago, `end` = today) and cache on `S`. Weeks are
   Monday-anchored; add a `weekStart(dateStr)` helper. Invalidate this cache on
   any exercise save/delete and when the day changes.

2. **Training card** (under the hero, above the meal list). Four rows:
   - **Cardio / Strength / Yoga** — target rows. Left: icon (🏃/🏋️/🧘) + name +
     a subtitle (see task 3). Middle: dots — one filled green per session this
     week up to the target, a dashed-outline "today" dot as the next unfilled
     dot when nothing is logged for that type today, extra filled dots if the
     week exceeds target. Right: `n/target`, with a ✓ and green treatment when
     met.
   - **Meditation** — a 7-dot daily row (one dot per weekday Mon–Sun, filled for
     days with any meditation session, dashed on today if none yet) and a
     `daysDone/7` count.
   Counts are **sessions**, not days, for cardio/strength/yoga; meditation
   counts **days**. Multiple sessions the same day each add a dot.

3. **Subtitle with cross-week "last:".** Each target row's subtitle:
   - if there is ≥1 session of that type **today** → `today` (or `today ×N` for
     N>1 the same day);
   - else if there is one earlier **this week** → `last: <Wkday>`;
   - else fall back to the most recent session **before this week** →
     `last: <Wkday> · last wk` (or `· N wks ago` if older). This fallback is
     **not Monday-only** — it applies any day the current week has no session of
     that type yet. If there is no prior session at all → `none yet`.

4. **Quick-log sheet** — tapping any row opens a bottom sheet with chip pickers,
   sensible defaults pre-selected, saving in 2–3 taps with no AI call:
   - cardio: Activity [Run · Bike · Hike · Swim · Row · Other] (default Run) +
     Minutes [20 · 30 · 45 · 60 · 90] (default 30)
   - strength: Where [Crunch · Home · 24 Hour · Rec Center] (default Crunch)
   - yoga: Where [Studio · Home] + Style [Vinyasa · Hot · Other] + Length
     [30 · 45 · 60 · 75 · 90] (default Studio / Vinyasa / 60)
   - meditation: Minutes [5 · 10 · 15 · 20 · 30] (default 10)
   Save → `POST /api/exercise` with `input_kind: 'tap'` and `day` = the
   currently-viewed day → toast, refresh the card. **Every save appends** — the
   sheet never overwrites an earlier session, and its subhead should say so when
   one already exists that day ("adds another session"). Allow a free-text
   Minutes/other entry too (the chips are shortcuts, not the only options).

5. **Editing.** Tapping a filled dot (or a "history" affordance you add) opens
   that session in the same sheet for edit (`PUT`) or delete (`DELETE`). Keep it
   simple; the primary flow is logging.

6. **Log bar.** Change the bottom log-bar placeholder from "What did you eat?"
   to **"What did you eat — or do?"** (natural-language exercise parsing lands in
   exercise phase 4; the placeholder just signals intent now).

7. **Settings.** Add a "Weekly training targets" group: number inputs for
   cardio / strength / yoga sessions per week and meditation days per week,
   saved via the extended `PUT /api/settings`. Default display from
   `GET /api/settings`.

## Look & tokens

Match the app's existing card/dark-mode system. The card is quiet: filled dots
use `--green`, met rows tint the icon chip with a muted green, the dashed
"today" dot uses `--primary`. No nudge/suggestion sentence — the dots and
subtitles carry the reminder (this was an explicit design decision).

## Out of scope

Trends charts (exercise phase 3), NL/voice exercise parsing (exercise phase 4),
Garmin import. Don't touch the calorie ring, protein bar, score, or meal flow.

## Acceptance checklist

Server running against a scratch DB, phone-width viewport:

- Today shows the Training card under the hero without scrolling.
- Tap Cardio → Run/30 → Save → toast; the card shows 1 dot and `1/3`, subtitle
  `today`. Tap Cardio again → Bike/45 → a 2nd dot, subtitle `today ×2`, no
  overwrite.
- With no yoga this week but one last week, the Yoga subtitle reads
  `last: <day> · last wk`; confirm it still says that on a non-Monday.
- Meditation logged today fills today's dot and increments `days/7`.
- Meeting a target flips the row to `n/n ✓` with green treatment.
- Change the cardio weekly target in Settings to 4 → the card now shows `/4`.
- Reload → all logged sessions persist (they're in the DB, not just `S`).
- Dark mode looks intentional.
