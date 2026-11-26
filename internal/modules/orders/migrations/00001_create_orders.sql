-- +goose Up
CREATE TABLE orders.orders (
    id           uuid        PRIMARY KEY,
    -- A reference by ID to another module's data: deliberately NOT a foreign
    -- key. Modules never share tables, joins or constraints.
    customer_id  uuid        NOT NULL,
    status       text        NOT NULL,
    currency     char(3)     NOT NULL,
    total_amount bigint      NOT NULL,
    placed_at    timestamptz NOT NULL
);

CREATE INDEX orders_customer_id_idx ON orders.orders (customer_id);

CREATE TABLE orders.order_lines (
    order_id   uuid    NOT NULL REFERENCES orders.orders (id) ON DELETE CASCADE,
    line_no    integer NOT NULL,
    sku        text    NOT NULL,
    quantity   bigint  NOT NULL CHECK (quantity > 0),
    unit_price bigint  NOT NULL CHECK (unit_price >= 0),
    PRIMARY KEY (order_id, line_no)
);

-- +goose Down
DROP TABLE orders.order_lines;
DROP TABLE orders.orders;
