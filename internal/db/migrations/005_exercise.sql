CREATE TABLE exercise_sessions (
    id            BIGSERIAL PRIMARY KEY,
    day           DATE NOT NULL,
    performed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    type          TEXT NOT NULL CHECK (type IN ('cardio','strength','yoga','meditation')),
    activity      TEXT,                       -- cardio: Run/Bike/Hike/Swim/Row/Other
    location      TEXT,                       -- strength + yoga
    style         TEXT,                       -- yoga
    duration_min  INT,                        -- cardio/yoga/meditation; NULL for strength
    note          TEXT NOT NULL DEFAULT '',
    input_kind    TEXT NOT NULL DEFAULT 'tap'
                  CHECK (input_kind IN ('tap','text','voice','import')),
    ai_raw        JSONB,                      -- raw AI output when NL-parsed (exercise phase 4)
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX exercise_sessions_day_idx ON exercise_sessions(day);

ALTER TABLE settings
    ADD COLUMN cardio_weekly_target     INT NOT NULL DEFAULT 3,
    ADD COLUMN strength_weekly_target   INT NOT NULL DEFAULT 2,
    ADD COLUMN yoga_weekly_target       INT NOT NULL DEFAULT 2,
    ADD COLUMN meditation_weekly_days   INT NOT NULL DEFAULT 7;
