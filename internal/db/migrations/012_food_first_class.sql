-- Food-first-class model. Previously a `meals` row was one logging ENTRY with
-- N items, and the day view grouped entries under slot headers. Now a `meals`
-- row is the (day, slot) CONTAINER — one Breakfast/Lunch/Dinner/Snack per day —
-- and `meal_items` is the first-class Food, carrying its own photo, provenance,
-- and logging metadata. Logging appends foods to the day's slot container.
--
-- This migration: (1) moves per-entry metadata down onto each food, (2) merges
-- all entries sharing a (day, slot) into one container, (3) enforces one
-- container per (day, slot), (4) drops the now-per-food columns from meals.
-- Nutrition data is untouched — foods keep their as-logged numbers.

-- (1) Per-food metadata columns, seeded from the parent meal.
ALTER TABLE meal_items
    ADD COLUMN eaten_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN input_kind TEXT NOT NULL DEFAULT 'manual'
        CHECK (input_kind IN ('text','voice','photo','barcode','manual')),
    ADD COLUMN photo_key  TEXT,
    ADD COLUMN photo_url  TEXT,
    ADD COLUMN ai_model   TEXT,
    ADD COLUMN ai_raw     JSONB;

UPDATE meal_items mi SET
    eaten_at   = m.eaten_at,
    input_kind = m.input_kind,
    photo_key  = m.photo_key,
    photo_url  = m.photo_url,
    ai_model   = m.ai_model,
    ai_raw     = m.ai_raw
FROM meals m WHERE mi.meal_id = m.id;

-- (2) Re-parent every food onto the lowest-id meal for its (day, slot), then
-- delete the emptied duplicate containers. IS NOT DISTINCT FROM equates NULL
-- slots so unslotted entries merge too.
UPDATE meal_items mi SET meal_id = keep.id
FROM meals m,
     LATERAL (
         SELECT MIN(m2.id) AS id FROM meals m2
         WHERE m2.day = m.day AND m2.slot IS NOT DISTINCT FROM m.slot
     ) keep
WHERE mi.meal_id = m.id AND m.id <> keep.id;

DELETE FROM meals m
WHERE m.id <> (
    SELECT MIN(m2.id) FROM meals m2
    WHERE m2.day = m.day AND m2.slot IS NOT DISTINCT FROM m.slot
);

-- (3) One container per (day, slot); COALESCE folds NULL slots into one bucket.
CREATE UNIQUE INDEX meals_day_slot_idx ON meals (day, COALESCE(slot, ''));

-- (4) These now live on each food.
ALTER TABLE meals
    DROP COLUMN description,
    DROP COLUMN eaten_at,
    DROP COLUMN input_kind,
    DROP COLUMN photo_key,
    DROP COLUMN photo_url,
    DROP COLUMN ai_model,
    DROP COLUMN ai_raw;

-- Favorites gain a kind so the UI can split Foods (single item) from Meals
-- (collections). Backfill by item count.
ALTER TABLE favorites
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'meal' CHECK (kind IN ('food','meal'));

UPDATE favorites SET kind = 'food'
WHERE jsonb_array_length(items) = 1;
