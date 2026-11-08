-- +goose Up
CREATE TABLE sessions (
    id          UUID PRIMARY KEY,
    table_id    INT NULL CHECK (table_id BETWEEN 1 AND 999),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE carts (
    session_id  UUID PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE cart_items (
    session_id  UUID NOT NULL REFERENCES carts(session_id) ON DELETE CASCADE,
    item_id     BIGINT NOT NULL,
    qty         INT NOT NULL CHECK (qty BETWEEN 1 AND 20),
    added_from  TEXT NOT NULL CHECK (added_from IN ('menu','rec_feed','rec_cart')),
    added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, item_id)
);
CREATE TABLE events (
    id          BIGSERIAL PRIMARY KEY,
    session_id  UUID NOT NULL,
    table_id    INT NULL,
    name        TEXT NOT NULL,
    payload     JSONB NOT NULL DEFAULT '{}',
    ts          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_name_ts ON events (name, ts);
CREATE INDEX idx_events_session ON events (session_id);

-- +goose Down
DROP TABLE events;
DROP TABLE cart_items;
DROP TABLE carts;
DROP TABLE sessions;
