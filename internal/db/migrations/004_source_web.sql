-- 'web' source: nutrition found via the model's web search (chain-restaurant
-- published numbers, etc). source_ref holds the URL.
ALTER TABLE meal_items DROP CONSTRAINT meal_items_source_check;
ALTER TABLE meal_items ADD CONSTRAINT meal_items_source_check
    CHECK (source IN ('usda','off','ai','label','manual','web'));
