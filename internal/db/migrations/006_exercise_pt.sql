ALTER TABLE exercise_sessions DROP CONSTRAINT exercise_sessions_type_check;
ALTER TABLE exercise_sessions ADD CONSTRAINT exercise_sessions_type_check
    CHECK (type IN ('cardio','strength','yoga','meditation','pt'));
ALTER TABLE settings ADD COLUMN pt_weekly_days INT NOT NULL DEFAULT 7;
