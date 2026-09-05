-- +goose Up
ALTER TABLE users ADD COLUMN accent TEXT NOT NULL DEFAULT 'emerald'
    CHECK (accent IN ('emerald', 'blue', 'violet', 'amber', 'rose'));
