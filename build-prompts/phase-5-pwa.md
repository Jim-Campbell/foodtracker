# Phase 5 — PWA: Today, log flow, Trends, Settings

Read `CLAUDE.md` and `ARCHITECTURE.md` in full first (especially → "PWA").
Phases 1–4 are built; the entire API works via curl. This phase replaces the
placeholder `pwa/index.html` with the real app. This is the largest phase —
work screen by screen and keep the server running to test as you go.

**Design bar:** this app lives on a phone and gets used 4–6 times a day. Big
type, generous tap targets (≥44 px), one-thumb reachable controls, minimal
chrome, fast perceived response (optimistic spinners, no layout jank). Logging
a meal must take under 10 seconds of user attention.

## Reference implementations

- `~/projects/finance/pwa/index.html` — single-file conventions: `S` state
  object, `render()`, one `<dialog id="dlg">` with `openDialog(html)`,
  bottom tab nav, fetch wrapper with bearer key, dark-mode CSS variables.
- `~/projects/journal/pwa/index.html` — `webkitSpeechRecognition` mic flow
  (lines ~1169+), API-key first-run prompt, manifest + service worker.

## Structure

Single `pwa/index.html`. Tabs: **Today · Trends · Settings**. API key in
`localStorage` (`food_key`) with a first-run prompt. All lb↔g and g↔mg
conversions happen at the display edge only — state holds API units.

## Tasks

### Today (home, default tab)

1. Header: day switcher `‹ Mon Jul 7 ›` (defaults to device-local today; can
   go back to log yesterday; no future days).
2. Hero: **calories-remaining ring** (SVG donut: eaten vs target, turns amber
   within 10% of target, red over), large center number = remaining (negative
   shows as "+123 over"). Under it a **protein bar** (eaten g / target g,
   fills green at target) and the **quality score badge** (0–100 chip,
   colored: ≥80 green, 60–79 amber, <60 red; hidden when null).
3. Meal list grouped by slot (breakfast/lunch/dinner/snack, unslotted last):
   each row = description or first item names, calories, small score-tier-free
   design (no tier words in the UI — score only per invariants), thumbnail if
   photo. Tap → edit dialog: change slot/day, per-item rows with fraction
   chips (¼ ½ ¾ All), editable grams (scale that item's nutrition linearly
   client-side), delete item, delete meal, save via `PUT /api/meals/{id}`.
4. **Log bar pinned to the bottom** (above tab nav): text input with
   placeholder "What did you eat?", 🎤 mic, 📷 camera, ⚖️ weight.
   - Text/mic → `POST /api/parse` → draft dialog.
   - Mic: Web Speech API exactly like journal (interim results into the
     input; if `SpeechRecognition` unavailable, hide the button — iOS
     keyboard dictation still works into the input).
   - Camera: `<input type=file accept=image/* capture=environment>` →
     client-side canvas downscale (longest edge ≤1600 px, JPEG q0.8) →
     `POST /api/photos` → `POST /api/analyze-photo` with any text in the
     input as `hint` → draft dialog. Hide when health says `photos: false`.
   - Weight: mini dialog, number input in **lb** (one decimal), stored via
     `POST /api/weights` as grams (`Math.round(lb * 453.592)`), shows last
     weigh-in for reference.
5. **Draft dialog** (the make-or-break screen): spinner while parsing; then
   items list — name, quantity, grams, calories, protein — each with fraction
   chips and ✕; a whole-meal fraction row that sets all items; `notes` from
   the parse shown as a dismissible hint line; slot picker (smart default by
   time of day: <10:30 breakfast, <15:00 lunch, <21:00 dinner, else snack);
   footer shows draft totals (as-eaten, live-updating) and a big **Save**.
   Save → `POST /api/meals` (include description, input_kind, photo fields,
   ai_model, ai_raw) → close, refresh Today. Escape/cancel discards — drafts
   are never persisted.

### Trends

6. Week/Month segmented control.
   - **Calorie bars**: one bar per day (`GET /api/range`), height = calories,
     horizontal target line, bar colored by over/under; a small score dot on
     each bar's top (color scale as the badge). Tap a bar → jump Today tab to
     that day.
   - **Protein line** overlaid or as a second mini-chart (grams vs target line).
   - **Weight chart**: line of weigh-ins over the visible range
     (`GET /api/weights`), optional goal-weight dashed line, min/max padding
     so small changes are visible. Show 7-day moving average as a smoother
     second line when ≥7 points.
   - Averages shown above the charts count **completed days only** (exclude
     today) — an in-progress day would dilute them.
   - All charts are inline SVG built by string templates. No libraries.

### Settings

7. Calorie target (kcal), protein target (g, stored mg), goal weight (lb,
   stored g) → `PUT /api/settings`. Export button → authenticated fetch of
   `/api/export` (phase 6 endpoint — point at it now, fine if 404 until then)
   downloaded as a file. API-key reset. App version string.

### PWA shell

8. `manifest.json` (name "Food", standalone, portrait, theme colors, 192/512
   icons — generate simple SVG-derived PNGs), apple-touch-icon,
   `sw.js` cache-first for `/` + manifest + icons, network-only for `/api/`;
   bump a cache version constant on deploy like journal does.

## Out of scope

Export endpoint implementation (phase 6), README, deploy.

## Acceptance checklist

Run the server with real `ANTHROPIC_API_KEY`/`FDC_API_KEY` against a scratch
DB and test in a phone-width viewport (≤430 px):

- First run prompts for the API key, then loads Today.
- Type "greek yogurt with blueberries and walnuts" → draft in <10 s → Save →
  ring, protein bar, and score update; the meal is listed under the right slot.
- Tap the meal → set the yogurt to ½ → totals drop accordingly (verify one
  number by hand).
- Photo flow works end to end with a nutrition-label photo and hint
  "I had half of this" → items arrive with fraction 50 pre-selected.
- Log a weight in lb; Trends shows it plotted; `GET /api/weights` shows grams.
- Log meals on two different days; Week view bars match `GET /api/range`;
  averages exclude today.
- Day switcher: log to yesterday; the entry files under yesterday's date.
- Dark mode looks intentional (toggle via OS setting).
- Lighthouse-level basics: installable manifest, service worker registers,
  app loads offline shell (API calls fail gracefully with a retry message).
