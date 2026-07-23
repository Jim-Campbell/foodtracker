-- Saturated fat as a tracked target (build-prompt item 5). It's the one metric
-- in the app that bears on cardiovascular risk rather than body composition,
-- and the one Jim is furthest from target on (~27 g/day logged vs a target
-- near 14 g — roughly 6% of a 2,050 kcal day). Ceiling-style, like the calorie
-- target. Default 14 g = 14000 mg; editable in Settings.

ALTER TABLE settings
    ADD COLUMN sat_fat_target_mg BIGINT NOT NULL DEFAULT 14000;
