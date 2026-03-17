-- +goose Up
CREATE TABLE skills (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    category   TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE profile_skills (
    profile_id UUID NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    skill_id   UUID NOT NULL REFERENCES skills (id) ON DELETE CASCADE,
    PRIMARY KEY (profile_id, skill_id)
);

-- +goose Down
DROP TABLE IF EXISTS profile_skills;
DROP TABLE IF EXISTS skills;
