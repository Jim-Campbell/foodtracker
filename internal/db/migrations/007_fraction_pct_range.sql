-- Widen the eaten-portion range so a single item can be logged at up to 3x
-- the full portion (fraction_pct 300), matching the PWA quantity stepper.
ALTER TABLE meal_items DROP CONSTRAINT meal_items_fraction_pct_check;
ALTER TABLE meal_items ADD CONSTRAINT meal_items_fraction_pct_check
    CHECK (fraction_pct BETWEEN 1 AND 300);
