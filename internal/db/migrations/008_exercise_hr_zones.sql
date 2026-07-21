-- Per-cardio-session time-in-heart-rate-zone, stored as a JSONB object keyed
-- by zone number ("1".."5") to integer minutes, e.g. {"1":5,"2":20,"3":15}.
-- Zones are optional and independent of duration_min (they need not sum to it).
-- JSONB (rather than five columns) leaves room to attach richer Garmin metrics
-- later without another migration; a future Garmin import maps its five zone
-- times straight onto this shape with input_kind='import'.
ALTER TABLE exercise_sessions ADD COLUMN hr_zones JSONB;
