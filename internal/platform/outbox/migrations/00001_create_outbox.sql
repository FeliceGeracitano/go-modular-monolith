-- +goose Up
CREATE TABLE outbox.messages (
    seq          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    id           uuid        NOT NULL UNIQUE,
    name         text        NOT NULL,
    payload      jsonb       NOT NULL,
    -- Transport metadata, such as the trace context of the publisher.
    headers      jsonb       NOT NULL DEFAULT '{}',
    occurred_at  timestamptz NOT NULL,
    available_at timestamptz NOT NULL,
    attempts     integer     NOT NULL DEFAULT 0,
    last_error   text,
    processed_at timestamptz
);

-- The relay only ever scans messages that still need delivering.
CREATE INDEX messages_pending_idx ON outbox.messages (available_at, seq) WHERE processed_at IS NULL;

-- Publishing is open to every module: each one appends events from inside
-- its own transaction, under its own database role. Reading and updating
-- the queue stays with the outbox's role.
GRANT USAGE ON SCHEMA outbox TO PUBLIC;
GRANT INSERT ON outbox.messages TO PUBLIC;

-- +goose Down
DROP TABLE outbox.messages;
