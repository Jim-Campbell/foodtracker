# Food App Build Prompt — Inclusion Score

*July 26, 2026. Companion to `food_app_build_prompt_20260722.md`. Source of truth for tiers and rationale: `Jim_Diet_Framework.md` v6.*

---

## 1. Problem statement

The existing meal/day score is **calorie-weighted**. It answers: *of the calories I consumed, what tier were they?* That is a **composition** score, and it should be kept unchanged — it is the only construct that catches the failure mode where a large share of calories arrive as Soft No / Hard No. Unweighted averaging would let one bite of salmon offset a 900-calorie pizza.

The flaw is structural, not a calibration issue: a calorie-weighted average is **incapable** of registering beneficial inclusion. Kale at ~7 cal/cup can never move the average regardless of tier weights. The reductio is that a day of only olive oil and avocado scores near-perfect.

Worked example from the log on 2026-07-26: breakfast of sprouted rolled oats + raw peach + Orgain scored **93**. All three are correctly-tiered Hard Yes items. The day contained zero leafy greens, zero berries, zero legumes, zero fatty fish. The 93 is not wrong — it is answering a different question.

**Build a second, independent score. Do not merge them.**

Precedent: every validated dietary pattern score (MIND, Mediterranean/PREDIMED, DASH, HEI) is built on servings-per-period, not calorie-weighted averages, precisely because the highest-value foods are calorie-trivial. This is a methodological argument, not an outcome-authority one — note that the 2023 MIND RCT was null on cognition over 3 years.

---

## 2. Components

Five components at launch. Targets are per rolling 7-day window.

| id | label | emoji | target | source |
|---|---|---|---|---|
| `leafy_greens` | Leafy greens | 🥬 | 7 | Framework line 26; cognition / APOE e4 |
| `berries` | Berries | 🫐 | 5 | Framework line 34 flags as *priority — polyphenols, cognitive evidence*; MIND baseline is 2, raised for e4 |
| `legumes` | Legumes | 🫘 | 5 | Framework line 45 + v6 "Legumes — beyond protein" note (FHILL, 20g/day) |
| `fatty_fish` | Fatty fish | 🐟 | 3 | Framework line 42, *priority for omega-3 index*; index last measured April 2024 and overdue; Lp(a) 563 |
| `cruciferous` | Cruciferous | 🥦 | 4 | Framework line 27, listed separately from leafy greens |

**Phase 2 — do not build now:** EVOO as primary fat (daily binary), fermented foods (framework lines 31, 81), whole grains (line 56).

**Deliberately excluded: nuts/seeds.** Framework line 126 records nuts and nut butter averaging 185 cal/day while delivering ~3g protein/day over a two-week measured window. A nut target would sit permanently satisfied and contribute only noise. Do not add it.

### Serving definitions

| component | 1 serving |
|---|---|
| `leafy_greens` | 1 cup raw **or** ½ cup cooked |
| `berries` | ½ cup |
| `legumes` | ½ cup cooked |
| `fatty_fish` | 3–4 oz (85–115 g) |
| `cruciferous` | 1 cup raw **or** ½ cup cooked |

Servings are computed from logged quantity, fractional allowed, then summed.

**Per-meal cap: 2 servings per component per meal.** Rationale: the construct is *exposure frequency over time*, not volume. A single enormous salad should not satisfy the weekly greens target. This mirrors how MIND and DASH count frequency rather than mass. Flagged as revisitable — see §8.

---

## 3. Scoring

For each component `c` in the window:

```
progress[c] = min(count[c] / target[c], 1.0)
inclusion_score = mean(progress[c] for all c) * 100
```

**The 1.0 cap is non-negotiable.** Without it, six servings of berries paper over zero fatty fish — which is the composition score's failure mode inverted. Breadth of coverage is the entire point of this score.

Equal weights across components. Priority is expressed through **target thresholds**, not weights (fatty fish is at 3/wk rather than MIND's 1/wk because of the overdue omega-3 index). One tuning knob, not two.

---

## 4. Rolling window mechanics

**Window = the last 7 calendar days inclusive of today** (`today - 6` through `today`).

A serving logged on day `D` is in-window for days `D` through `D + 6`. It **ages out** at the start of day `D + 7`.

Rationale for rolling rather than calendar week: food has unlimited slots and fully recoverable misses. There is no cap on opportunities to eat greens and no recovery constraint, so nothing is ever unrecoverable and there is nothing to write off. A weekly reset would introduce a false boundary and create both Sunday-cram and write-off-Friday dynamics.

**This is deliberately different from training.** Training keeps calendar weeks — see §6.

---

## 5. Tag layer

The scoring depends on resolving logged foods to components. This is the substantive implementation work.

### Data model

```
food_component_tags(
  fdc_id            -- FDC food identifier
  component_id      -- one of the five component ids
  servings_per_ref  -- servings contributed per reference quantity
)
```

Composite key on `(fdc_id, component_id)`. Multi-tag is **allowed and expected**.

### Population

Piggyback on the **entry-time match confirmation** flow already built (see `food_app_build_prompt_20260722.md`). When a food is resolved to an FDC entry for the first time, prompt for component tags at the same moment. Tag once, persist, reuse on every subsequent encounter. Incremental — the tag table fills from actual eating, not from an upfront taxonomy exercise.

### Encoded cases

- **Kale → `leafy_greens` AND `cruciferous`.** Double-counting is allowed. The framework lists the two categories separately with distinct rationales, so a food that genuinely belongs to both should count for both. Revisitable — see §8.
- **Peach → no component.** Whole fruit is a distinct framework line from berries. This is a real miss from the 2026-07-26 log and must not resolve to `berries`.
- **Starchy vegetables** (potato, sweet potato, winter squash, corn) → no component. Framework line 89 treats these separately.

---

## 6. UI — Today view

### Constraint

The training card uses a **M–T–W–T–F–S–S day grid**. The food card must **not**. If both cards show day columns, the columns mean different things in adjacent cards, which is the one genuinely bad outcome.

A grid answers *when*. A meter answers *how many*. Different question, different shape — the paradigm collision dissolves once the visual forms differ.

### Food card

Placed below the `TRAINING · THIS WEEK` card. Header: **`FOOD · LAST 7 DAYS`**.

```
FOOD · LAST 7 DAYS

🥬  Leafy greens    ● ● ● ● ● ○ ○     5/7
🫐  Berries         ● ◐ ○ ○ ○         2/5
🫘  Legumes         ● ● ● ○ ○         3/5
🐟  Fatty fish      ● ○ ○             1/3
🥦  Cruciferous     ● ● ● ●           4/4 ✓
```

**Rules:**

- No day-of-week columns. No date axis of any kind.
- Dot positions per row = `target[c]`. Row lengths differ by design; the length itself carries information.
- **Filled `●`** — a logged serving inside the window.
- **Faded `◐`** — a serving that ages out within 24h (logged on `today - 6`). See decay preview below.
- **Open `○`** — remaining to target.
- `count > target`: render `target` filled dots plus `✓`. **Do not render overflow.** The cap is the point.
- Completed rows are de-emphasized.

The visual vocabulary — filled dots done, open circles remaining — is identical to the `left` column already in the training card. Nothing new to learn.

### Decay preview

Because servings age out, a count can fall tomorrow with no action taken. Rendering the about-to-expire dot in a faded shade surfaces this in the display itself.

This is a nudge with no notification, no nagging, and no honesty risk — it is only information about a window sliding.

### Training card

**Unchanged.** Calendar weeks, day grid, `left` column, Monday reset — all as-is.

Rationale for the asymmetry: training has finite slots and unrecoverable misses. Sessions are constrained by recovery, and a missed Wednesday cannot be doubled onto Sunday. The reset is the feature — the period closes, gets evaluated, and re-commits. "2 cardio left" on a Sunday is not an actionable prompt; it is a closing retrospective, and that is the intended read.

Exercise is a **plan you execute**. Food is an **exposure you accumulate**. Different constructs, different time models, deliberately different UI.

---

## 7. Nudges

### Honesty constraint (hard requirement)

**Nudges are driven by the inclusion score only. Never by the composition score.**

The inclusion score counts only positives, so a logged day containing nothing beneficial costs exactly what an unlogged day costs. Logging honestly can never lower it. The composition score does not have this property — honestly logging a pizza actively lowers it — so it must never drive a notification.

No notification, message, or UI state may penalize logging a low-quality item. Logging honesty is the core asset of the whole system and takes precedence over any nudge's effectiveness.

### Tier 1 — supply nudge

- **Cadence:** weekly, on a user-configured day/time. Intended to land before the grocery run.
- **Content:** components projected to miss target over the next 7 days.
- **Copy:** `"Next 7 days needs: berries, fatty fish."`
- This is the high-leverage nudge. It acts on the pantry, upstream of every individual meal decision.

### Tier 2 — decision nudge

- **Cadence:** maximum one per day. Default window 11:00–13:00 local — before dinner planning, not at 22:00 when it is unactionable.
- **Selection:** among components where `count[c] < target[c]`, rank by relative gap `(target[c] - count[c]) / target[c]` descending. Tie-break toward any component with a serving aging out within 24h.
- **Suppression:** never fire on a component at target. Never fire more than once per day.

### Copy rules

- Additive framing only. `"Add X"`, `"X would help this week"`.
- Never `"you missed"`, `"you failed"`, `"you're behind"`, or streak-loss language.

---

## 8. Trends — weekly retrospective

A separate item in **Trends**, not on Today.

Calendar weeks (Mon–Sun), matching how retrospective evaluation actually works. Per completed week: components hit vs. total, and per-component count vs. target.

```
Week of Jul 20    3/5 components

🥬 Leafy greens   6/7
🫐 Berries        2/5
🫘 Legumes        5/5  ✓
🐟 Fatty fish     3/3  ✓
🥦 Cruciferous    4/4  ✓
```

**This is the only place calendar weeks apply to food.** It provides the reflective closure that calendar periods are good for, without contaminating the Today card's rolling logic.

---

## 9. Non-goals

- Do **not** merge inclusion and composition into a single number. A pizza-plus-kale day would score respectably and both signals would be lost.
- Do **not** apply calendar-week logic to the food card on Today.
- Do **not** add day columns to the food card.
- Do **not** add nuts/seeds as a component.
- Do **not** modify the existing composition score.
- Do **not** render overflow past target.

---

## 10. Open decisions

1. **Per-meal serving cap of 2** (§2) — recommended, not yet confirmed. Alternative is uncapped, which permits one large salad to satisfy the weekly greens target.
2. **Supply nudge day/time** (§7) — needs to be set to whatever precedes the actual grocery run.
3. **Kale double-counting** (§5) — spec'd as allowed. Revisit if the greens and cruciferous rows start moving in lockstep, which would indicate the two components are not measuring distinct things in practice.
4. **Phase 2 components** (§2) — EVOO, fermented foods, whole grains. Deferred deliberately; five targets is already a lot to establish at once, and habits build sequentially.
