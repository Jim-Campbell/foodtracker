# Phase 1 — Scaffold: server skeleton, database, auth, Dockerfile

Read `CLAUDE.md` and `ARCHITECTURE.md` in full before writing any code. This
phase produces a running Go server with the complete database schema, bearer
auth, and deployment plumbing — no business logic yet.

## Reference implementations

Copy structure and idioms from the sibling apps rather than inventing:

- `~/projects/finance/cmd/server/main.go` — env config, dependency injection,
  graceful shutdown
- `~/projects/finance/internal/db/` — pgx pool setup and the
  migrations-at-startup runner (embed `migrations/*.sql`, apply in filename
  order, record applied names in a `schema_migrations` table)
- `~/projects/finance/internal/api/` — chi router setup, bearer-auth
  middleware, request-logging middleware, static PWA serving
- `~/projects/finance/Dockerfile` — multi-stage build

## Tasks

1. `go mod init github.com/jimgcampbell/food` (Go 1.25). Dependencies:
   `github.com/go-chi/chi/v5`, `github.com/jackc/pgx/v5`. Nothing else yet.
2. `cmd/server/main.go`: read env per ARCHITECTURE.md → Environment
   (`DATABASE_URL` and `FOOD_API_KEY` required — fail fast with a clear
   message; `PORT` default 8082; the AI/FDC/R2 vars are read but only logged
   as configured/not-configured in this phase). Wire pgx pool → migrations →
   router → `http.Server` with graceful shutdown.
3. `internal/db/`: pgx pool constructor + migrations runner +
   `migrations/001_initial.sql` containing **exactly** the schema in
   ARCHITECTURE.md → "Database schema (migration 001)", including the
   `INSERT INTO settings (id) VALUES (1)` seed (make it idempotent:
   `ON CONFLICT DO NOTHING`).
4. `internal/api/`: chi router with logging middleware; bearer-auth middleware
   on everything under `/api` except `GET /api/health`; health handler
   returning `{"ok":true,"photos":false,"ai":false}` (booleans reflect whether
   the R2 / Anthropic env vars are set); serve `pwa/` at `/` (placeholder
   `pwa/index.html` that just says "food app" is fine).
5. `Dockerfile` (multi-stage `golang:1.25-alpine` builder with
   `GOTOOLCHAIN=local` → `alpine` runtime, copy `pwa/` into the image),
   `.gitignore` (binaries, `.env`).
6. `.env.example` listing every env var from ARCHITECTURE.md with comments.

## Out of scope

No domain types, no handlers beyond health, no AI, no R2, no real PWA.

## Acceptance checklist

- `go build ./... && go test ./...` passes (no tests yet is fine).
- `createdb food_smoke && DATABASE_URL=postgres://localhost:5432/food_smoke?sslmode=disable FOOD_API_KEY=x go run ./cmd/server`
  starts, applies migration 001, and logs the port.
- `curl localhost:8082/api/health` → 200 with the JSON above, **without** a key.
- `curl localhost:8082/api/anything-else` → 401 without
  `Authorization: Bearer x`, 404 with it.
- `psql food_smoke -c '\d meal_items'` shows the schema; `settings` has row id=1.
- Restarting the server does not re-apply or fail on migration 001.
- `docker build .` succeeds.
- `dropdb food_smoke` at the end.
