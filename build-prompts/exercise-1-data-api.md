# Exercise Phase 1 — Data + API: sessions, targets, summaries

Read `CLAUDE.md` and `ARCHITECTURE.md` in full first. The food app (phases 1–6)
is already built and deployed. This phase adds **exercise tracking** at the
data + API layer only — no PWA, no AI. Everything is testable with `curl`.

Design decisions are settled (see the design spike in
`memory/exercise-tracking-expansion.md` and the prototype artifact
<https://claude.ai/code/artifact/63d7ec19-4c72-4e71-b15f-5ab2f05db913>). Do not
re-litigate them; this is execution work.

## Domain rules that must hold

- **Exercise is completely separate from calories.** Nothing in this phase
  touches `DaySummary` calorie/score math or the meals tables. A session never
  credits or debits the food budget.
- **No floats** (same invariant as food): `duration_min` is integer minutes.
  Later we may add HR zones / weight / reps — leave room, don't add them now.
- **`day` is a user-chosen DATE** (device-local today by default), independent
  of `performed_at`, exactly like `meals.day`.
- **Multiple sessions per day per type are normal and expected** (yoga twice; a
  hike after a swim). Never upsert or dedupe by (day, type) — every save
  appends a row.
- Fields captured now, by type:
  - **cardio** → `activity` (Run/Bike/Hike/Swim/Row/Other) + `duration_min`
  - **strength** → `location` (Crunch/Home/24 Hour/Rec Center); no duration yet
  - **yoga** → `location` (Studio/Home) + `style` (Vinyasa/Hot/Other) + `duration_min`
  - **meditation** → `duration_min`
  `activity`/`location`/`style` are free-text `TEXT` (not DB enums) so "Other"
  and future values need no migration; the **service** validates the expected
  field is present per type.

## Reference implementations (mirror these, don't invent)

- `internal/db/migrations/00N_*.sql` — migration file style; the runner in
  `internal/db/migrate.go` applies `*.sql` in filename order.
- `internal/food/types.go`, `store.go`, `service.go` — type/Store/service split
  and the `validDate`, `validateMeal` patterns.
- `internal/db/store.go` — pgx query + transaction idioms.
- `internal/api/food.go` — `Routes(r)`, JSON decode, `writeJSON`/`writeError`,
  `h.fail` error mapping.

## Tasks

1. **Migration** `internal/db/migrations/005_exercise.sql`:
   ```sql
   CREATE TABLE exercise_sessions (
       id            BIGSERIAL PRIMARY KEY,
       day           DATE NOT NULL,
       performed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
       type          TEXT NOT NULL CHECK (type IN ('cardio','strength','yoga','meditation')),
       activity      TEXT,                       -- cardio: Run/Bike/Hike/Swim/Row/Other
       location      TEXT,                       -- strength + yoga
       style         TEXT,                       -- yoga
       duration_min  INT,                        -- cardio/yoga/meditation; NULL for strength
       note          TEXT NOT NULL DEFAULT '',
       input_kind    TEXT NOT NULL DEFAULT 'tap'
                     CHECK (input_kind IN ('tap','text','voice','import')),
       ai_raw        JSONB,                      -- raw AI output when NL-parsed (exercise phase 4)
       created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
       updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
   );
   CREATE INDEX exercise_sessions_day_idx ON exercise_sessions(day);

   ALTER TABLE settings
       ADD COLUMN cardio_weekly_target     INT NOT NULL DEFAULT 3,
       ADD COLUMN strength_weekly_target   INT NOT NULL DEFAULT 2,
       ADD COLUMN yoga_weekly_target       INT NOT NULL DEFAULT 2,
       ADD COLUMN meditation_weekly_days   INT NOT NULL DEFAULT 7;
   ```
   Weeks are **Monday–Sunday**; meditation's target is days-per-week (default 7,
   i.e. daily). Migration must be idempotent-safe under the existing advisory-lock
   runner (a plain `CREATE TABLE` / `ADD COLUMN` is fine — it runs once, recorded
   in `schema_migrations`).

2. **Types** (`internal/food/types.go`): add
   ```go
   type ExerciseSession struct {
       ID          int64           `json:"id"`
       Day         string          `json:"day"`           // YYYY-MM-DD
       PerformedAt time.Time       `json:"performed_at"`
       Type        string          `json:"type"`
       Activity    *string         `json:"activity,omitempty"`
       Location    *string         `json:"location,omitempty"`
       Style       *string         `json:"style,omitempty"`
       DurationMin *int            `json:"duration_min,omitempty"`
       Note        string          `json:"note"`
       InputKind   string          `json:"input_kind"`
       AIRaw       json.RawMessage `json:"ai_raw,omitempty"`
       CreatedAt   time.Time       `json:"created_at"`
       UpdatedAt   time.Time       `json:"updated_at"`
   }
   ```
   Add type/input-kind constants + `validExerciseTypes` / `validExerciseInputKinds`
   maps next to the existing food ones. Extend `Settings` with
   `CardioWeeklyTarget`, `StrengthWeeklyTarget`, `YogaWeeklyTarget`,
   `MeditationWeeklyDays` (all `int`, snake_case JSON). Add
   `Exercise []ExerciseSession` to `ExportDoc` (ordered by day).

3. **Store** (`internal/food/store.go` interface + `internal/db/store.go` impl):
   - `CreateExercise(ctx, *ExerciseSession) error`
   - `GetExercise(ctx, id) (*ExerciseSession, error)`
   - `UpdateExercise(ctx, *ExerciseSession) error`
   - `DeleteExercise(ctx, id) error`
   - `ListExerciseRange(ctx, start, end string) ([]ExerciseSession, error)` —
     one query, `WHERE day BETWEEN $1 AND $2 ORDER BY day, performed_at`.
   - `ListAllExercise(ctx) ([]ExerciseSession, error)` — for export.
   Extend the settings get/put queries to read/write the four new columns.

4. **Service** (`internal/food/service.go`): `validateExercise(*ExerciseSession)`
   — `validDate(day)`; `type` in enum; `input_kind` in enum; **per-type field
   checks**: cardio requires non-empty `activity` and `duration_min > 0`;
   yoga requires `location`, `style`, `duration_min > 0`; meditation requires
   `duration_min > 0`; strength requires `location` and must have
   `duration_min == nil`. Non-negative duration always. Thin
   `CreateExercise/GetExercise/UpdateExercise/DeleteExercise/ListExerciseRange`
   wrappers that validate then delegate. Extend `UpdateSettings` to persist the
   four targets (validate ≥ 0). Add exercise to `Export`.

5. **API** (new `internal/api/exercise.go`, wired from `Handler.Routes`):
   ```
   POST   /api/exercise            {day,type,activity,location,style,duration_min,note,input_kind} → 201 ExerciseSession
   GET    /api/exercise?start=&end=                                → [ExerciseSession]  (day range, inclusive)
   GET    /api/exercise/{id}                                       → ExerciseSession
   PUT    /api/exercise/{id}       same body shape as POST         → ExerciseSession
   DELETE /api/exercise/{id}                                       → 204
   ```
   `start`/`end` required on the list route (400 if missing), `YYYY-MM-DD`.
   The home card and trends both read raw sessions via the range route and
   aggregate client-side — do **not** build weekly-rollup endpoints. Include the
   four targets in `GET /api/settings` and accept them in `PUT /api/settings`
   (already covered by task 4 if you extend the existing handler).

6. **Tests** (`internal/food/service_test.go` additions, fake store):
   - Valid cardio/yoga/strength/meditation sessions pass; a cardio without
     `activity`, a yoga without `style`, a strength **with** `duration_min`, and
     a meditation with `duration_min: 0` each fail validation.
   - Two sessions same day+type both persist (append, no dedupe).
   - `ListExerciseRange` is inclusive of both ends and excludes outside days.
   - Settings round-trips the four targets; a negative target is rejected.

## Out of scope

No PWA, no AI/NL parsing (exercise phase 4), no Garmin import, no calorie
interaction, no HR zones / weight / reps.

## Also update docs

Add an "Exercise" section to `ARCHITECTURE.md` (schema of `exercise_sessions`,
the four settings targets, the five endpoints, the separation-from-calories
invariant, Mon–Sun weeks) so it stays the source of truth. One paragraph in
`CLAUDE.md` → domain invariants noting exercise is calorie-independent and
duration is integer minutes.

## Acceptance checklist

- `go build ./... && go test ./...` passes.
- Against a scratch DB (`FOOD_API_KEY=x`), with `curl`:
  - `POST /api/exercise` a cardio (Run, 32) and a second cardio same day → two
    rows returned by `GET /api/exercise?start=&end=` for that day.
  - `POST` a strength with `duration_min` set → 400; without it → 201.
  - `PUT /api/settings` with `cardio_weekly_target: 4` → `GET /api/settings`
    reflects it.
  - `GET /api/export` includes the exercise sessions.
- `dropdb food_smoke` at the end.
