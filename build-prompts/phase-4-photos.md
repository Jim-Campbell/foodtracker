# Phase 4 — Photos: R2 upload, vision analysis (label / package / barcode / plate)

Read `CLAUDE.md` and `ARCHITECTURE.md` in full first. Phases 1–3 are built.
This phase delivers `POST /api/photos` (upload) and `POST /api/analyze-photo`
(vision parse), reusing the phase-3 parse loop.

## Reference implementation

- `~/projects/journal/internal/storage/r2.go` — copy near-verbatim into
  `internal/storage/r2.go` (same env var names: `R2_ACCOUNT_ID`,
  `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET`, `R2_PUBLIC_URL`).
  Also copy how journal's `main.go` constructs it and how its photo-upload
  handler reads multipart form data.

## Tasks

1. Wire the R2 client into `cmd/server/main.go`. If any `R2_*` var is unset,
   the two photo endpoints return 503 "photos not configured" and
   `/api/health` reports `"photos": false`. Add the aws-sdk-go-v2 modules the
   journal client needs.
2. `POST /api/photos` — multipart `image` field, content-type must be
   image/jpeg|png|webp|heic, max 10 MB. Object key
   `food/2026/07/<uuid>.<ext>`. Returns `{key, url}`.
3. `POST /api/analyze-photo {key, hint, day}`:
   - Fetch the object from R2 (add a `Get` method to the storage client if
     journal's lacks one), base64 it into an image content block.
   - First user message = image block + text block containing the hint (may
     be empty) and the day. Run the same parse loop as `/api/parse`.
   - Extend the system prompt with photo behavior:
     - **Nutrition label visible** → transcribe the panel exactly (serving
       size, servings per container, per-serving values); `source: "label"`;
       compute full-portion values from the servings the user indicates (hint
       "I ate the whole box" × servings per container; no hint → one serving,
       noted in `notes`).
     - **Barcode with legible digits** → call `off_barcode` with the digits;
       on a hit, `source: "off"`, `source_ref` = barcode. On a miss, fall
       back to package text + `usda_search`.
     - **Package front only** → identify the product, `usda_search` Branded
       or estimate; `confidence: "medium"` at best.
     - **Plate of food** → identify each component, estimate portion weight
       from visual cues (plate ≈ 27 cm reference), `usda_search` each;
       `confidence` honest per item.
     - A hint like "I had half of this" → `fraction_pct: 50` on the relevant
       items, never pre-scaled values.
   - Response is a normal ParseResult; the client will include `photo_key` /
     `photo_url` / `input_kind` when it saves the meal.
4. Validation identical to `/api/parse` (ValidateItem + Atwater on every item).
5. Tests: handler tests with a fake storage client and fake AI transport —
   upload happy path, oversize rejection, bad content type, 503 when R2
   unconfigured, analyze passes image bytes into the first message.
6. Manual live checks (document results, not Go tests): a photo of a
   nutrition label, a barcode, and a plate — via `curl -F image=@...` then
   analyze with hint "I had half of this" and confirm `fraction_pct: 50`.

## Out of scope

PWA camera UI (phase 5), client-side downscaling (phase 5), thumbnails.

## Acceptance checklist

- `go build ./... && go test ./...` passes.
- Upload → analyze → `POST /api/meals` (with `photo_key`, `photo_url`,
  `input_kind: "photo"`) round-trips; `GET /api/meals?day=` returns the
  photo URL.
- Label photo parse quotes the label's numbers (spot-check one macro).
- Barcode photo parse shows `source: "off"` with the barcode in `source_ref`.
- With R2 vars unset: both endpoints 503, health `"photos": false`, server
  still boots.
