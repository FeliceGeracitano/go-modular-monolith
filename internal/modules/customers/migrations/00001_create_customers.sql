-- +goose Up
CREATE TABLE customers.customers (
    id         uuid        PRIMARY KEY,
    name       text        NOT NULL,
    email      text        NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT customers_email_key UNIQUE (email)
);

-- +goose Down
DROP TABLE customers.customers;
