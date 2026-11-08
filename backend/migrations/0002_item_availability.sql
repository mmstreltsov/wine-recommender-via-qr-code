-- +goose Up
CREATE TABLE item_availability (
    item_id      BIGINT PRIMARY KEY,
    is_available BOOLEAN NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE item_availability;
