# TODO — future features

North star: **healthy aging** — great fitness, a lean body, a sharp mind.
Every feature here should serve one of those three, and the app's founding
rules still hold: logging stays near-zero friction, the AI never writes to the
DB, exercise never credits the calorie budget, no floats in stored data.

Status: brainstorm, roughly ordered within each section. Items marked 🎯 are
the likely next wave after E1–E4 land. Nothing here is committed work.

---

## Post-resolution-layer backlog 🎯 (from the 2026-07-22 health-coach review)

Deferred from the July 2026 build prompt (`food_app_build_prompt_20260722.md`).
Everything here depends on the **FDC resolution layer** being in place first —
the analysis quality of all of it rests on the underlying numbers being right.
Build order within this block: **validation layer → rolling windows → nudges**
(each depends on the one before). The build prompt itself (meal-group export,
missing-vs-zero, FDC resolution cascade, sat-fat target, outlier attribution,
framework default cascade) is the committed near-term work and is **not**
listed here.

### A. Validation layer

Runs on every item, meal group, and day before export. Failures populate
`analysis_warnings` and flag inline with `"validation": ["..."]`. Under FDC
resolution most of these should rarely fire — they exist to catch silent
resolution failures and unreviewed tier-4 LLM estimates.

- [ ] **Macro reconciliation.** `protein_g*4 + carbs_g*4 + fat_g*9` within ±10%
      of stated `calories` (tighter than the current ±30% parse-time Atwater
      check). FDC-sourced data should pass by construction; a post-migration
      failure means a bad composite decomposition or an unreviewed estimate.
- [ ] **Sodium bounds.** Hard ceiling: reject/​warn any single item > 5,000 mg
      and any day > 10,000 mg. Hunt the **1000× multiplier bug** (a think! bar
      reads 210,000 mg for a true ~210 mg; a Five Guys burger 430,000 mg for
      ~430 mg) — likely a g↔mg conversion. Whole-wheat toast reading 2,212 mg
      vs true ~250 mg looks like a *separate* bad-lookup class.
- [ ] **Protein plausibility.** Flag any item where
      `protein_g * 4 > calories * 0.85` — very few whole foods exceed this;
      composite restaurant entries are where the current build fails worst.
- [ ] **Density sanity.** Flag any item outside 0.2–9.0 cal/g.

### B. Rolling windows and trend reporting

Single-day numbers carry too much estimation noise to act on; 7-day means are
usable. Add to the export and a summary view. This is the layer the "outlier
attribution" from the build prompt measures its baseline against.

- [ ] **7- and 14-day rolling means** for calories, protein,
      `protein_pct_calories`, sat fat, and fiber.
- [ ] **Week-over-week deltas.**
- [ ] **Target-attainment rate per rolling window** — days hitting protein,
      days within calorie target, days within sat-fat target.
- [ ] Motivating case: protein rose 133.5 → 174.1 g/day week 1 → week 2, a real
      successful change invisible in the day view. Build the view that would
      have surfaced it live.

### C. Nudges (protect logging fidelity above all else)

Honest logging is the single most valuable property of this dataset; a nudge
that costs one honest entry is a net loss regardless of the advice.

- [ ] **Asymmetric timing.** Positive feedback fires immediately at entry
      (reinforces logging); negative feedback defers to a daily/weekly review
      surface — never a disapproving response to an honest log.
- [ ] **Informational, not evaluative.** State the number ("11.2 cal per gram
      of protein; whey isolate is 4.4"), no frowns/scolding/guilt/streak-breaks.
- [ ] **Substitution, not prohibition.** Always frame as a swap.
- [ ] **Pattern-level, not item-level.** Nudge on rolling-window patterns from
      section B (e.g. 185 cal/day of nuts averaged over two weeks), never on a
      single entry.
- [ ] **Protected foods.** `protected: true` + required `protected_reason` on
      the canonical food table — exempts an item from all negative nudging and
      any discretionary-calorie rollup, settable from the entry screen. Seed
      non-alcoholic beer (*"Alcohol substitute supporting sobriety. Behavioral
      value substantially exceeds its ~58 cal/day cost. Never flag."*).
- [ ] **Positive nudges worth firing:** hitting the protein target, a meal
      group > ~35% protein by calories, fatty fish (omega-3 gap), logging every
      slot in a day, any streak of consecutive logged days.

### D. Smaller items, unscheduled

- [ ] **Star / quick-add audit.** Periodic prompt to review what's starred —
      the two starred items are currently the two least calorie-efficient
      protein sources in the rotation. Friction gradients shape behavior more
      than nudges.
- [ ] **Alcohol as a budget, not a prohibition.** Alcohol is tiered `hard_no`,
      which flags a planned event as a violation. Real rule is one drink/week,
      banked. Optional `allowance` concept: named budget, weekly quota,
      accrues, shows balance, reports draws rather than failures. Low priority.
- [ ] **Visual hierarchy on the day view.** Protein is the stated
      non-negotiable priority, calories the secondary constraint — the screen
      currently says the opposite (big red "+438 over", thin protein bar).
      Make protein the hero number (or a two-part state). Confirm the calorie
      ring's target reflects the rate goal, not a legacy number.

---

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
