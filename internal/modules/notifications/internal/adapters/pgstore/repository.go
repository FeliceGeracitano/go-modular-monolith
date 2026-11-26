// Package pgstore is the PostgreSQL adapter for the notifications module.
// It only ever touches tables in the "notifications" schema.
package pgstore

import (
	"context"
	"fmt"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/app"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
)

// Repository implements app.Repository.
type Repository struct {
	db *postgres.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db}
}

// Insert implements app.Repository.
func (r *Repository) Insert(ctx context.Context, n app.Notification) (bool, error) {
	tag, err := r.db.Conn(ctx).Exec(ctx, `
		INSERT INTO notifications.notifications (id, event_id, customer_id, recipient, subject, body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (event_id) DO NOTHING`,
		n.ID, n.EventID, n.CustomerID, n.Recipient, n.Subject, n.Body, n.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("notifications: insert: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
