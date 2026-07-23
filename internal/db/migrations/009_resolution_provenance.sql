-- Resolution provenance (build-prompt item 3): record how each item's numbers
-- were resolved, so lab-analyzed FDC data is distinguishable at a glance from
-- an LLM estimate, and the diet-framework default cascade is distinguishable
-- from an explicit table hit.
--
-- All columns are nullable and unconstrained-by-default: existing (pre-item-3)
-- rows keep NULL, which honestly reads as "unknown provenance" rather than a
-- fabricated value. New parses populate them.

ALTER TABLE meal_items
    ADD COLUMN fdc_id          BIGINT,   -- FDC FoodData Central id when source='usda'
    ADD COLUMN fdc_data_type   TEXT,     -- 'Branded' | 'Foundation' | 'SR Legacy' | 'Survey (FNDDS)'
    ADD COLUMN resolution_tier INT       -- 1=barcode/branded/label, 2=Foundation/SR, 3=Survey/web, 4=LLM estimate
        CHECK (resolution_tier IS NULL OR resolution_tier BETWEEN 1 AND 4),
    ADD COLUMN portion_source  TEXT      -- how the portion weight was arrived at
        CHECK (portion_source IS NULL OR portion_source IN ('weighed','package_unit','estimated')),
    ADD COLUMN tier_source     TEXT      -- did the quality tier come from the framework table or its default cascade
        CHECK (tier_source IS NULL OR tier_source IN ('table','cascade'));
