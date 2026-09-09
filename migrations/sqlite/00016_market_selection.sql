-- +goose Up
ALTER TABLE market_items ADD COLUMN selection_json TEXT;
