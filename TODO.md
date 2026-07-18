# TODO — future features

North star: **healthy aging** — great fitness, a lean body, a sharp mind.
Every feature here should serve one of those three, and the app's founding
rules still hold: logging stays near-zero friction, the AI never writes to the
DB, exercise never credits the calorie budget, no floats in stored data.

Status: brainstorm, roughly ordered within each section. Items marked 🎯 are
the likely next wave after E1–E4 land. Nothing here is committed work.

## Garmin import 🎯

The watch already records cardio and yoga; stop double-logging them.

- [ ] **FIT/TCX file import (v1).** `POST /api/import/garmin` accepting FIT
      files (from Garmin Connect's export or emailed from the watch app).
      Parse activity type, start time, duration → draft `exercise_sessions`
      rows (`input_kind: 'import'`) shown for confirm, never auto-saved —
      same draft-then-save contract as the AI parsers.
- [ ] **Dedupe against manual logs.** An import that overlaps a same-day
      manual session of the same type within ±30 min replaces/merges it
      (keep the manual note, take the device duration). Never double-count.
- [ ] **Type mapping table.** Garmin `running/cycling/lap_swimming/yoga/...` →
      our types + activities (Run/Bike/Swim/…); unknown types land as
      cardio Other for the confirm sheet to fix.
- [ ] **Garmin Connect API (v2).** OAuth-based background sync once the
      manual path proves the model. Requires Garmin developer-program
      approval — apply early, build later.
- [ ] **Richer per-session data (v2+).** HR zones, avg/max HR, distance,
      elevation — extend `exercise_sessions` with a `metrics JSONB` column
      (same philosophy as `meal_items.micros`: keep the full payload, analyze
      later).

## Fitness (strength, capacity, structure)

- [ ] **Strength session detail.** Exercise library + per-session sets ×
      reps × weight (integer lbs→grams? store grams, display lbs — same
      no-floats rule). Start with a flat "exercises" table + a picker seeded
      from Jim's actual routine; progressive-overload chart per lift.
- [ ] **PT program checklist.** PT today is did-it + minutes; add an optional
      named-exercise checklist (the prescribed routine) so a session can
      record which movements got done. Completion % feeds the habit row.
- [ ] **Cardio zones (from Garmin metrics).** Time-in-zone per week; a
      "Zone 2 minutes" weekly target alongside the session target — the
      aging-relevant base-building metric.
- [ ] **VO2max & resting-HR trend.** Garmin computes both; chart them on
      Trends → Training as long-horizon lines (quarter/year). These are the
      two best single numbers for "am I aging well aerobically."
- [ ] **Recovery awareness.** Surface Garmin HRV/body-battery on the Today
      card as a small hint chip ("recovery low — easy day?") — informational
      only, never gating.

## Lean body (nutrition & composition)

- [ ] **Protein per kg guidance.** Target is already grams/day; show g/kg of
      current body weight (aging target ≈ 1.2–1.6 g/kg) so the target adapts
      as weight drops.
- [ ] **Micronutrient gap report.** `meal_items.micros` already stores full
      nutrient payloads — actually use them: weekly fiber, potassium,
      calcium, magnesium, omega-3 vs reference intakes; flag chronic gaps.
- [ ] **Sodium & added-sugar watchlist.** Already stored per item; add weekly
      trend + a quiet over-threshold marker on day summaries.
- [ ] **Alcohol tracking.** Tier data mostly captures it (hard_no), but an
      explicit drinks/week count is the honest metric; parse "two beers" into
      a counted field, chart weekly units.
- [ ] **Body composition.** Waist measurement quick-entry (monthly prompt);
      smart-scale import (body-fat %, lean mass) if the scale exports;
      lean-mass trend beside the weight chart — the number that matters for
      sarcopenia, not just scale weight.
- [ ] **Plateau & trend intelligence.** 7/28-day moving averages with a
      "true trend" line (weight noise vs signal); plateau detection that
      suggests a review rather than a harsher target.

## Sharp mind

- [ ] **Sleep import (Garmin).** Duration, consistency, and a bedtime-drift
      chart. Sleep is the highest-leverage cognitive input the watch already
      measures; correlate with next-day eating quality (late-night snack
      detection is already possible from `eaten_at` vs `day`).
- [ ] **Meditation depth.** Streak history and time-of-day pattern; nothing
      heavier — the practice is the feature.
- [ ] **Caffeine cutoff awareness.** Coffee/tea are already logged as items;
      flag caffeine after a configurable hour and correlate with sleep score
      once Garmin sleep lands.
- [ ] **Cognitive-health markers (log-only).** Optional yearly/quarterly
      journal fields: blood pressure, lipids, A1C, and an annual "how's the
      mind" note. The app is the one place all the health data already lives.

## Insights & motivation (the connective tissue)

- [ ] **Weekly review, written by Claude.** Sunday-evening generated summary:
      food quality vs targets, training week vs targets, weight trend, one
      specific suggestion for next week. Draft-only email/screen — the same
      "AI proposes, Jim disposes" contract. This is the feature that turns
      four data streams into one story.
- [ ] **Correlation explorer.** Simple two-series overlay picker (e.g., sleep
      vs quality score, training minutes vs weight trend) over 3–12 months —
      the payoff for keeping every raw payload.
- [ ] **Milestones & annual review.** Streak records, total active hours,
      weight-journey chart, "a year of training" wall — the journal-keeper's
      reward, generated each January.
- [ ] **PWA push reminders (opt-in, gentle).** A single evening nudge only if
      nothing is logged for the day — never per-meal nagging; the home-screen
      dots stay the primary reminder.

## Infrastructure that unlocks the above

- [ ] **`metrics JSONB` on exercise_sessions** (see Garmin) — do this at
      import-v1 time so device data lands rich from day one.
- [ ] **New tables:** `sleep_nights`, `measurements` (waist, body-fat,
      BP, labs — one generic typed table), `imports` (file hash, source,
      applied rows) for idempotent re-imports.
- [ ] **Export keeps pace.** Every new table joins `/api/export` the same
      release it ships — the backup story must never lag the data.
- [ ] **Fixture-based import tests.** Real FIT files from Jim's watch as test
      fixtures, same pattern as the nutrition-client fixtures.

## Explicitly not doing

- Calorie credits for exercise (settled — separation is an invariant).
- Social/sharing features, gamification beyond streaks/milestones.
- Medical advice. The app logs and charts; interpretation stays with Jim
  and his doctor.
