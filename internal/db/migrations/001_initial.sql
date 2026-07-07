CREATE TABLE meals (
    id            BIGSERIAL PRIMARY KEY,
    day           DATE NOT NULL,
    eaten_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    slot          TEXT CHECK (slot IN ('breakfast','lunch','dinner','snack')),
    description   TEXT NOT NULL DEFAULT '',
    input_kind    TEXT NOT NULL DEFAULT 'text'
                  CHECK (input_kind IN ('text','voice','photo','barcode','manual')),
    photo_key     TEXT,
    photo_url     TEXT,
    ai_model      TEXT,
    ai_raw        JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX meals_day_idx ON meals(day);

CREATE TABLE meal_items (
    id            BIGSERIAL PRIMARY KEY,
    meal_id       BIGINT NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
    position      INT NOT NULL DEFAULT 0,
    name          TEXT NOT NULL,
    brand         TEXT,
    quantity      TEXT NOT NULL DEFAULT '',
    grams         INT,
    fraction_pct  INT NOT NULL DEFAULT 100 CHECK (fraction_pct BETWEEN 1 AND 100),
    calories      INT NOT NULL DEFAULT 0,
    protein_mg    BIGINT NOT NULL DEFAULT 0,
    carbs_mg      BIGINT NOT NULL DEFAULT 0,
    fat_mg        BIGINT NOT NULL DEFAULT 0,
    fiber_mg      BIGINT NOT NULL DEFAULT 0,
    sat_fat_mg    BIGINT NOT NULL DEFAULT 0,
    sugar_mg      BIGINT NOT NULL DEFAULT 0,
    sodium_mg     BIGINT NOT NULL DEFAULT 0,
    micros        JSONB,
    tier          TEXT NOT NULL DEFAULT 'neutral'
                  CHECK (tier IN ('hard_yes','soft_yes','neutral','soft_no','hard_no')),
    tier_reason   TEXT NOT NULL DEFAULT '',
    source        TEXT NOT NULL DEFAULT 'ai'
                  CHECK (source IN ('usda','off','ai','label','manual')),
    source_ref    TEXT,
    confidence    TEXT NOT NULL DEFAULT 'medium'
                  CHECK (confidence IN ('high','medium','low'))
);
CREATE INDEX meal_items_meal_idx ON meal_items(meal_id);

CREATE TABLE weights (
    id         BIGSERIAL PRIMARY KEY,
    day        DATE NOT NULL UNIQUE,
    weight_g   INT NOT NULL,
    note       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE settings (
    id                 INT PRIMARY KEY CHECK (id = 1),
    calorie_target     INT NOT NULL DEFAULT 1800,
    protein_target_mg  BIGINT NOT NULL DEFAULT 165000,
    weight_target_g    INT,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO settings (id) VALUES (1) ON CONFLICT DO NOTHING;
