# Phase 6 — Polish, export, README, Render deploy

Read `CLAUDE.md` and `ARCHITECTURE.md` in full first. Phases 1–5 are built and
verified. This phase finishes the loose ends and prepares deployment.

## Tasks

1. `GET /api/export` — streams a single JSON document:
   `{exported_at, settings, weights: [...], meals: [ {…meal, items:[...]}, ... ]}`
   ordered by day. `Content-Disposition: attachment; filename=food-export-YYYYMMDD.json`.
   This is the backup story (Render disks are ephemeral) — keep it dependency-free.
2. Sweep for consistency:
   - Every handler returns JSON errors `{"error": "..."}` with correct status
     codes; no `panic` paths reachable from requests.
   - `updated_at` maintained on meal/settings updates.
   - `/api/health` reports `{ok, photos, ai}` truthfully.
   - `go vet ./...` clean.
3. Smoke script `scripts/smoke.sh`: creates `food_smoke` DB, starts the
   server, exercises health → settings → manual meal → day summary (asserts
   the score-72 worked example from ARCHITECTURE.md) → weight upsert → range
   → export, prints PASS/FAIL per step, tears down (`dropdb`). Must be safe
   to re-run (drop DB if it already exists, per the finance lesson: a
   leftover smoke DB makes the next createdb fail).
4. `README.md`: what the app is (2 paragraphs), the core loop, screenshots
   placeholder, local dev quickstart, env var table, deploy notes, and a
   pointer to ARCHITECTURE.md and docs/diet-framework.md.
5. Deploy prep (do not create cloud resources yourself — produce exact
   instructions in DEPLOY.md for Jim to click through):
   - Render: new PostgreSQL instance; new Web Service from this repo's
     Dockerfile; env vars to set (`DATABASE_URL` from the Render DB internal
     URL, `FOOD_API_KEY` generate with `openssl rand -hex 24`,
     `ANTHROPIC_API_KEY`, `FDC_API_KEY` from https://api.data.gov/signup/,
     the five `R2_*` values — reuse the journal bucket's credentials with a
     new bucket or the `food/` prefix in an existing one).
   - Note: migrations run at startup, so first boot creates the schema.
   - iPhone install steps: open the Render URL in Safari → Share → Add to
     Home Screen → paste API key at first run.
6. Final check that `docker build .` succeeds and the image runs with only
   env vars (no bind mounts).

## Acceptance checklist

- `go build ./... && go test ./... && go vet ./...` pass.
- `scripts/smoke.sh` prints all PASS on a clean machine state and cleans up
  after itself.
- Export of a DB with 2 meals + 1 weight opens as valid JSON containing both,
  with `ai_raw` and `micros` payloads intact.
- README quickstart works when followed literally in a fresh checkout.
