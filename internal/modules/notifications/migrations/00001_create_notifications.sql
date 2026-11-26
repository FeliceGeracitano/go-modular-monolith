-- +goose Up
CREATE TABLE notifications.notifications (
    id          uuid        PRIMARY KEY,
    -- One notification per integration event: the unique constraint is what
    -- makes at-least-once delivery safe.
    event_id    uuid        NOT NULL,
    customer_id uuid        NOT NULL,
    recipient   text        NOT NULL,
    subject     text        NOT NULL,
    body        text        NOT NULL,
    created_at  timestamptz NOT NULL,
    CONSTRAINT notifications_event_id_key UNIQUE (event_id)
);

-- +goose Down
DROP TABLE notifications.notifications;
