-- Favorites are unique by name (case-insensitive): saving a favorite with an
-- existing name replaces its items instead of adding a duplicate row. Clean
-- up duplicates from before this rule (keep the oldest row per name).
DELETE FROM favorites f
USING favorites k
WHERE lower(f.name) = lower(k.name) AND f.id > k.id;

CREATE UNIQUE INDEX favorites_name_key ON favorites (lower(name));
