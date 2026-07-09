-- Favorites are reusable meal templates: a named snapshot of items that can
-- be re-logged with one tap. Items are a JSONB snapshot (same shape as
-- meal_items rows), not references, so editing or deleting the original meal
-- never mutates a favorite.
CREATE TABLE favorites (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    items      JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
