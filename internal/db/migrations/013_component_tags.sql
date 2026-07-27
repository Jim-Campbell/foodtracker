-- Inclusion score tag layer (inclusion phase 1). Maps a logged food to the
-- dietary-framework components it counts toward (leafy greens, berries,
-- legumes, fatty fish, cruciferous) so the rolling-7-day inclusion score can
-- be computed alongside — never merged with — the existing calorie-weighted
-- composition score.
--
-- Two deliberate departures from the original spec's (fdc_id, component_id)
-- design: keyed on normalized_name (food.NormalizeFoodName) rather than
-- fdc_id, since a large share of logged foods (off/label/web/ai) never
-- resolve to an FDC entry, and canonical_foods already proves this key covers
-- every food; fdc_id is kept as a nullable secondary key so an FDC-resolved
-- food still matches if its display name drifts. Lookup order: fdc_id exact
-- hit -> normalized_name hit -> untagged.
--
-- grams_per_serving (not servings_per_ref): every logged portion in this app
-- is integer grams, so storing the reference in grams keeps the serving math
-- integer and lets a composite dish carry its own reference ("lentil soup"
-- can be legumes @ 400 g/serving while "cooked lentils" is legumes @ 90).
--
-- Multi-tag is expected: kale is both leafy_greens and cruciferous.

CREATE TABLE food_component_tags (
    id                BIGSERIAL PRIMARY KEY,
    normalized_name   TEXT   NOT NULL,       -- food.NormalizeFoodName(item name)
    fdc_id            BIGINT,                -- when the food resolved to an FDC entry
    component_id      TEXT   NOT NULL CHECK (component_id IN
                        ('leafy_greens','berries','legumes','fatty_fish','cruciferous')),
    grams_per_serving INT    NOT NULL CHECK (grams_per_serving BETWEEN 1 AND 2000),
    tag_source        TEXT   NOT NULL DEFAULT 'ai' CHECK (tag_source IN ('ai','user')),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX food_component_tags_name_idx
    ON food_component_tags (normalized_name, component_id);
CREATE INDEX food_component_tags_fdc_idx
    ON food_component_tags (fdc_id) WHERE fdc_id IS NOT NULL;
