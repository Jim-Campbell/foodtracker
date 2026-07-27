-- Tunable inclusion targets + nudge settings (inclusion phase 5). Targets are
-- the only tuning knob (spec §3: priority is expressed through thresholds,
-- not weights) -- component_targets overrides food.ComponentCatalog's default
-- per-component target; {} means "use the defaults". The four nudge columns
-- configure the in-app nudge surfaces (build-prompts/inclusion-5-nudges.md);
-- there is no push infrastructure in this app, so these only gate client-side
-- rendering.
ALTER TABLE settings
  ADD COLUMN component_targets  JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN supply_nudge_dow   INT   NOT NULL DEFAULT 6,   -- 0=Sun..6=Sat
  ADD COLUMN nudge_start_hour   INT   NOT NULL DEFAULT 11,
  ADD COLUMN nudge_end_hour     INT   NOT NULL DEFAULT 13,
  ADD COLUMN nudges_enabled     BOOLEAN NOT NULL DEFAULT TRUE;
