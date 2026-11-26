// Package postgres owns the connection pool, transaction propagation and
// schema migrations shared by every module.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is the subset of pgx shared by a pool and a transaction.
// Repositories talk to the database exclusively through it.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DB wraps a pgx connection pool.
type DB struct {
	pool *pgxpool.Pool
	cfg  *pgxpool.Config
}

// Open connects to the database as the login role in url and verifies the
// connection. The returned handle is the administrative one: it runs
// migrations and health checks. Business modules never receive it; they get
// a restricted handle from Scoped.
func Open(ctx context.Context, url string, maxConns int32) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse url: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.ConnConfig.Tracer = queryTracer{}

	pool, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &DB{pool: pool, cfg: cfg}, nil
}

// RoleFor returns the name of the database role that owns access to a
// schema. Migrate creates it; Scoped connects as it.
func RoleFor(schema string) string { return schema + "_rw" }

// Scoped returns a handle with its own connection pool whose sessions run
// as the given schema's role, so they can read and write that schema and
// nothing else. A module that reaches into another module's tables gets
// "permission denied" from the database instead of a code review comment.
//
// Separate pools also isolate modules from each other under load: one
// module exhausting its connections cannot starve the rest.
//
// The pool connects lazily, so Scoped succeeds before the role exists (that
// is, before the first migration). The caller must Close the handle.
//
//nolint:contextcheck // takes no context: creating a pool opens no connection, so there is nothing to cancel
func (db *DB) Scoped(schema string) (*DB, error) {
	if !schemaName.MatchString(schema) {
		return nil, fmt.Errorf("postgres: invalid schema name %q", schema)
	}
	cfg := db.cfg.Copy()
	setRole := "SET ROLE " + RoleFor(schema)
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if _, err := conn.Exec(ctx, setRole); err != nil {
			return fmt.Errorf("postgres: %s (has the schema been migrated?): %w", setRole, err)
		}
		return nil
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: pool for %s: %w", schema, err)
	}
	return &DB{pool: pool, cfg: cfg}, nil
}

// Close releases every connection.
func (db *DB) Close() { db.pool.Close() }

// Ping checks that the database is reachable.
func (db *DB) Ping(ctx context.Context) error { return db.pool.Ping(ctx) }

// Pool exposes the underlying pool for infrastructure code (migrations,
// tests). Module code should use Conn instead.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

type txKey struct{}

// Conn returns the transaction bound to ctx by WithinTx, or the pool when
// there is none. Repositories call it on every operation, which lets the
// application layer decide transaction boundaries without repositories
// knowing about them.
func (db *DB) Conn(ctx context.Context) Querier {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return db.pool
}

// InTx reports whether ctx carries a transaction started by WithinTx.
func InTx(ctx context.Context) bool {
	_, ok := ctx.Value(txKey{}).(pgx.Tx)
	return ok
}

// TxFrom returns the transaction carried by ctx, if any. It lets
// infrastructure that must take part in the caller's transaction (the
// outbox) do so without holding a database handle of its own.
func TxFrom(ctx context.Context) (Querier, bool) {
	tx, ok := ctx.Value(txKey{}).(pgx.Tx)
	return tx, ok
}

// WithinTx runs fn inside a database transaction. The transaction travels in
// the context passed to fn; it commits when fn returns nil and rolls back
// otherwise (including on panic). Nested calls join the outer transaction.
func (db *DB) WithinTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if InTx(ctx) {
		return fn(ctx)
	}

	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			// Roll back even when ctx was cancelled, so the connection
			// returns to the pool clean.
			if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("postgres: rollback: %w", rbErr))
			}
			return
		}
		if cmErr := tx.Commit(ctx); cmErr != nil {
			err = fmt.Errorf("postgres: commit: %w", cmErr)
		}
	}()

	return fn(context.WithValue(ctx, txKey{}, tx))
}

// IsUniqueViolation reports whether err is a unique constraint violation.
// When constraint is not empty it must match the violated constraint's name.
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return constraint == "" || pgErr.ConstraintName == constraint
}
