-- +goose Up
ALTER TABLE item_categories
ADD COLUMN icon_key TEXT NOT NULL DEFAULT 'package';

-- +goose Down
SELECT 1;
