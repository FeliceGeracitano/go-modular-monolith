-- +goose Up
CREATE TABLE outbox.messages (
    seq          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id           uuid        NOT NULL UNIQUE,
    name         text        NOT NULL,
    payload      jsonb       NOT NULL,
    occurred_at  timestamptz NOT NULL,
    available_at timestamptz NOT NULL,
    attempts     integer     NOT NULL DEFAULT 0,
    last_error   text,
    processed_at timestamptz
);

-- The relay only ever scans messages that still need delivering.
CREATE INDEX messages_pending_idx ON outbox.messages (available_at, seq) WHERE processed_at IS NULL;

-- +goose Down
DROP TABLE outbox.messages;
