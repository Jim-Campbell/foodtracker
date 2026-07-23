-- Canonical food table (build-prompt item 3): built by accretion. Once a food
-- resolves through the cascade, its resolved per-100g nutrition + quality tier
-- are written here keyed on a normalized name, so subsequent logs of the same
-- food reuse it instead of re-resolving. Corrections overwrite the entry and
-- propagate to future logs only — meal_items keep their own as-logged snapshot,
-- so history is never rewritten.
--
-- protected/protected_reason are groundwork for the backlog "protected foods"
-- feature (exempt an item from negative nudging); the columns land now so the
-- table doesn't need re-migrating later.

CREATE TABLE canonical_foods (
    id                  BIGSERIAL PRIMARY KEY,
    normalized_name     TEXT NOT NULL UNIQUE,   -- lower-cased, whitespace-collapsed key
    display_name        TEXT NOT NULL,          -- the name as most recently logged
    source              TEXT NOT NULL,          -- usda | off | label | web | ai | manual
    source_ref          TEXT,                   -- fdc id, barcode, or url
    fdc_id              BIGINT,
    fdc_data_type       TEXT,
    resolution_tier     INT,
    -- per-100g nutrition, the reusable basis; as-logged = these * grams / 100
    cal_per_100g        INT    NOT NULL DEFAULT 0,
    protein_mg_per_100g BIGINT NOT NULL DEFAULT 0,
    carbs_mg_per_100g   BIGINT NOT NULL DEFAULT 0,
    fat_mg_per_100g     BIGINT NOT NULL DEFAULT 0,
    fiber_mg_per_100g   BIGINT NOT NULL DEFAULT 0,
    sat_fat_mg_per_100g BIGINT NOT NULL DEFAULT 0,
    sugar_mg_per_100g   BIGINT NOT NULL DEFAULT 0,
    sodium_mg_per_100g  BIGINT NOT NULL DEFAULT 0,
    default_grams       INT,                    -- a typical portion to pre-fill
    tier                TEXT NOT NULL DEFAULT 'neutral',
    tier_reason         TEXT NOT NULL DEFAULT '',
    tier_source         TEXT,
    protected           BOOLEAN NOT NULL DEFAULT FALSE,
    protected_reason    TEXT NOT NULL DEFAULT '',
    times_logged        INT NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
