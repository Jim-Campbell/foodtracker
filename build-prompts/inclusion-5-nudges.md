# Inclusion Phase 5 — Nudges (in-app) + tunable targets

Read `CLAUDE.md`, `ARCHITECTURE.md`, and
`build-prompts/inclusion-spec-20260726.md` §7. Inclusion phases 1–4 are built.

## Delivery mechanism — read this before writing code

Spec §7 describes nudges with a cadence and a time window, which implies push.
**This app has no push infrastructure**: `pwa/sw.js` handles caching only, there
is no VAPID keypair, no subscription table, no `push`/`notificationclick`
handler, and no server-side scheduler. Building all of that is a larger project
than the nudges themselves.

So this phase builds nudges as **in-app surfaces** — rendered on the Today
screen, computed client-side from data the app already has. Every selection rule,
suppression rule, and copy rule from spec §7 is implemented exactly; only the
transport differs. Web push is a clean follow-on once the copy and the selection
rules have been lived with for a few weeks, and it reuses all of this phase's
logic (`nudgeFor(window)` returns the same object a push payload would carry).
Do not build push in this phase.

## The honesty constraint (hard requirement — do not compromise it)

**Nudges are driven by the inclusion window only. Never by the composition
score.**

The inclusion score counts only positives, so a logged day containing nothing
beneficial costs exactly what an unlogged day costs — logging honestly can never
lower it. The composition score does *not* have that property: honestly logging a
pizza actively lowers it, so it must never drive a message.

Enforce this **structurally, not by discipline**: the nudge functions take an
`InclusionWindow` as their only argument. They must not receive `S.summary`, a
day score, calories, or macros. A reviewer should be able to confirm the
constraint from the function signature alone. No notification, message, or UI
state may penalize logging a low-quality item — logging honesty is the core
asset of the whole system and outranks any nudge's effectiveness.

## Tier 1 — supply nudge (the high-leverage one)

Acts on the pantry, upstream of every individual meal decision.

- **Surface:** a line directly under the `FOOD · LAST 7 DAYS` card, shown only on
  the configured day (default **Saturday**, settable — spec §10 open decision 2
  says it should precede the actual grocery run, and Jim will tune it).
- **Content:** the components that will miss target if nothing changes.
  Selection: components where `progress_pct < 100`, ranked by relative gap
  `(target - servings) / target` descending, top 3.
  (Spec §7 says "projected to miss over the next 7 days". Since every current
  serving ages out within the coming window, a literal projection is just the
  current gap — implement the gap, and note this in a comment so the shortcut is
  visible rather than silently assumed.)
- **Copy:** `Next 7 days needs: berries, fatty fish.`
- Dismissible for the day (localStorage); reappears next configured day.

## Tier 2 — decision nudge

- **Surface:** the same slot, one line, at most **one per day**.
- **Window:** default 11:00–13:00 device-local — before dinner planning, not at
  22:00 when it is unactionable. Settable.
- **Selection:** among components where `servings < target`, rank by relative gap
  `(target - servings) / target` descending. **Tie-break toward a component with
  a serving aging out within 24h** (`expiring_whole > 0`) — that is the one where
  action today prevents a loss.
- **Suppression:** never fire for a component at target; never more than once per
  day (persist the date + component in localStorage); never fire when the supply
  nudge is showing.

## Copy rules (enforce in code, and test them)

- **Additive framing only.** `Add berries`, `Berries would help this week`,
  `Next 7 days needs: …`.
- **Never** `you missed`, `you failed`, `you're behind`, `streak`, `don't break`,
  or any loss/streak language.
- Keep every string in one `NUDGE_COPY` object at the top of the nudge section so
  the whole voice is auditable at a glance.

## Tunable targets + nudge settings

Add to `settings` (migration `014_inclusion_settings.sql`):

```sql
ALTER TABLE settings
  ADD COLUMN component_targets  JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN supply_nudge_dow   INT   NOT NULL DEFAULT 6,   -- 0=Sun..6=Sat
  ADD COLUMN nudge_start_hour   INT   NOT NULL DEFAULT 11,
  ADD COLUMN nudge_end_hour     INT   NOT NULL DEFAULT 13,
  ADD COLUMN nudges_enabled     BOOLEAN NOT NULL DEFAULT TRUE;
```

`component_targets` overrides the phase-1 Go constants per component id (`{}` =
use the defaults); validate values into 1..21 and ignore unknown ids. Thread it
through `GetSettings`/`UpdateSettings` and into `InclusionWindow` so the dots and
the score both follow the override. **Targets are the only tuning knob** — spec
§3: priority is expressed through thresholds, not weights. Do not add per-component
weights, and keep the equal-weight mean as-is.

Settings UI: a new card, "Inclusion targets & nudges" — five number inputs
(pre-filled with the effective targets), a day picker for the supply nudge, an
hour range for the decision nudge, and a master on/off. Bump the `.buildstamp`.

## Tests

- Selection: given a window with `berries 1/5` and `greens 6/7`, the decision
  nudge picks berries (relative gap 0.8 vs 0.14).
- Tie-break: two components with equal relative gap, one with
  `expiring_whole > 0` → that one wins.
- Suppression: a component at target is never selected; a second call the same
  day returns nothing.
- Copy: assert no string in `NUDGE_COPY` matches
  `/missed|failed|behind|streak|broke/i`.
- Targets: an override of `{"berries": 3}` changes both the dot row length and
  the score.

## Non-goals

- No web push, no service-worker notification handling, no server-side scheduler.
- No nudge may read the composition score, calories, macros, or weight.
- No streaks, no badges, no gamification of any kind.
- Do not add the phase-2 components (EVOO, fermented foods, whole grains) — five
  targets is already a lot to establish at once, and habits build sequentially
  (spec §10 open decision 4).

## Acceptance checklist

- `go build ./... && go test ./...` passes; migration 014 applies to the scratch
  DB; `gofmt -l .` empty; `.buildstamp` bumped.
- On the configured day, the supply line renders with the correct components and
  the exact copy; dismissing hides it for that day only.
- Inside the configured hour window, one decision nudge appears; after
  dismissing or acting, none reappears that day.
- Turning `nudges_enabled` off removes both surfaces entirely.
- Changing a target in Settings changes the dot row length, the numeric, and the
  score together.
- A grep of the nudge code confirms it never references the composition score,
  `s.score`, calories, or macros.
