// Package logsender is a Sender that writes notifications to the log. It
// stands in for a real provider (SES, SendGrid, Twilio, ...): swapping it is
// a one-line change in the module's wiring.
package logsender

import (
	"context"
	"log/slog"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/app"
)

// Sender implements app.Sender.
type Sender struct {
	logger *slog.Logger
}

// New returns a Sender logging to logger.
func New(logger *slog.Logger) *Sender {
	return &Sender{logger: logger}
}

// Send implements app.Sender.
func (s *Sender) Send(ctx context.Context, n app.Notification) error {
	s.logger.InfoContext(ctx, "notification sent",
		"notification_id", n.ID, "event_id", n.EventID, "recipient", n.Recipient, "subject", n.Subject)
	return nil
}
