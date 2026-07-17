# Exercise Phase 4 — Natural-language exercise logging

Read `CLAUDE.md`, `ARCHITECTURE.md` (→ "AI meal parsing" and "Exercise"), and
`internal/ai/parser.go` in full first. Exercise phases 1–3 are built. This phase
lets the **one log bar** (typed or dictated) understand exercise the same way it
understands food — "30 min run", "hot yoga at the studio, 60 minutes", "hike
after a swim" — and route the result to `POST /api/exercise` instead of
`/api/meals`.

## Approach — one parse, two terminal tools

The parse already runs an agentic Claude loop that ends by calling `record_meal`
(`internal/ai/parser.go`). Extend that same loop rather than adding a separate
endpoint or a brittle client-side keyword router:

1. **Add a second terminal tool `log_exercise`** alongside `record_meal`. Its
   input is an **array of sessions** (so "hike after a swim" or "yoga twice"
   yields multiple), each: `{type, activity?, location?, style?, duration_min?,
   note?}` matching the phase-1 field rules per type.
2. **Update the system prompt** (`systemPrompt`): the app tracks food *and*
   exercise; classify the user's input and call **exactly one** terminal tool —
   `record_meal` for food, `log_exercise` for workouts. Exercise inputs need no
   USDA/barcode lookups; the model fills sessions directly from the text (map
   casual phrasing to the known chip vocab where obvious — "lifted"/"gym" →
   strength, "ran"/"jog" → cardio Run, "spin"/"cycling" → Bike, "swam" → Swim;
   infer `duration_min` from stated minutes; leave a field null if truly
   unstated and let the user fill it in the confirm sheet). Keep the "no prose
   around tool calls" rule.
3. **`ParseResult` gains a discriminator.** Add `Kind string` (`"meal"` |
   `"exercise"`) and `Exercise []ExerciseSession` (populated when
   `log_exercise` was called; `Items` stays for meals). Default `Kind: "meal"`
   so existing behavior and the food tests are unchanged. Keep the raw model
   output in `AIRaw` (→ `exercise_sessions.ai_raw` on save) as the food path
   keeps `meals.ai_raw`.
4. The `/api/parse` NDJSON handler is otherwise unchanged — it still streams
   progress then one `result`; the result now may be an exercise draft.

## PWA

5. When `/api/parse` returns `kind: "exercise"`, show a **confirm sheet**
   pre-filled with the parsed session(s) using the **same chip-picker UI as the
   phase-2 quick-log sheet** (one section per session; user can fix activity/
   minutes/etc., add or remove a session). Save → `POST /api/exercise` once per
   session with `input_kind: 'text'` (or `'voice'` when the mic produced it) and
   the viewed `day`. Then refresh the Training card. Meals still render the
   existing draft dialog. Voice uses the existing Web Speech flow — no separate
   button.

## Guardrails

- **Never trust AI arithmetic** (existing invariant): `duration_min` is whatever
  the user said or a sensible default; the app does not compute calories from
  exercise and does not let exercise touch the food budget.
- The server still **validates** every parsed session through
  `validateExercise` before returning the draft; drop or flag any session
  missing its required per-type field so the user can complete it, don't save
  silently.
- If `ANTHROPIC_API_KEY` is unset the parse endpoint already 503s — the tap-log
  sheets (phase 2) remain the always-available path.

## Out of scope

Garmin import, HR zones, weight/reps, any change to trends or the score/calorie
math.

## Also update docs

Extend `ARCHITECTURE.md` → "AI meal parsing" to document the `log_exercise`
terminal tool, the `Kind`/`Exercise` fields on `ParseResult`, and the
exercise-draft confirm flow.

## Acceptance checklist

Server running with a real `ANTHROPIC_API_KEY` against a scratch DB:

- Type "30 minute run" → confirm sheet pre-filled cardio Run / 30 → Save →
  Training card shows a cardio session today; `GET /api/exercise` confirms it
  with `input_kind: 'text'`.
- "hot yoga at the studio for an hour" → yoga / Studio / Hot / 60, editable.
- "hike then a swim this morning" → **two** cardio sessions in one confirm sheet.
- "greek yogurt and blueberries" still returns a **meal** draft (kind unchanged)
  and saves to `/api/meals` — the food path is untouched.
- `go build ./... && go test ./...` passes (existing AI parser tests still green;
  add a fixture test that a workout phrase yields `kind:"exercise"` with the
  expected sessions).
- `dropdb food_smoke` at the end.
