-- +goose Up
ALTER TABLE users ADD COLUMN locale TEXT NOT NULL DEFAULT 'zh-CN';
ALTER TABLE users ADD COLUMN theme TEXT NOT NULL DEFAULT 'system'
    CHECK (theme IN ('system', 'light', 'dark'));
