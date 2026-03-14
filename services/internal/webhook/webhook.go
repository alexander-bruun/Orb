// Package webhook provides a dispatcher for sending events to registered webhook endpoints.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexander-bruun/orb/services/internal/store"
)

// Event type constants for all supported webhook events.
const (
	EventTrackPlayed      = "track.played"
	EventUserCreated      = "user.created"
	EventUserActivated    = "user.activated"
	EventUserDeactivated  = "user.deactivated"
	EventUserAdminGranted = "user.admin_granted"
	EventUserAdminRevoked = "user.admin_revoked"
	EventUserDeleted      = "user.deleted"
	EventIngestCompleted  = "ingest.completed"
	EventTest             = "webhook.test"
)

// AllEvents is the canonical list of all supported event types.
var AllEvents = []string{
	EventTrackPlayed,
	EventUserCreated,
	EventUserActivated,
	EventUserDeactivated,
	EventUserAdminGranted,
	EventUserAdminRevoked,
	EventUserDeleted,
	EventIngestCompleted,
}

// Dispatcher sends webhook events to registered endpoints.
type Dispatcher struct {
	db     *store.Store
	client *http.Client
}

// New creates a new Dispatcher.
func New(db *store.Store) *Dispatcher {
	return &Dispatcher{
		db:     db,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// eventPayload is the JSON body sent to webhook endpoints.
type eventPayload struct {
	Event     string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// Dispatch fires an event to all matching webhooks asynchronously.
// It returns immediately; delivery happens in a background goroutine.
func (d *Dispatcher) Dispatch(ctx context.Context, event string, data any) {
	go d.dispatch(context.Background(), event, data)
}

// DispatchTo delivers an event directly to a single webhook, bypassing the
// enabled/event-filter lookup. Used for the test endpoint.
func (d *Dispatcher) DispatchTo(ctx context.Context, h store.Webhook, event string, data any) {
	go d.deliver(context.Background(), h, event, data)
}

func (d *Dispatcher) dispatch(ctx context.Context, event string, data any) {
	hooks, err := d.db.ListWebhooksForEvent(ctx, event)
	if err != nil {
		slog.Warn("webhook: failed to list hooks", "event", event, "err", err)
		return
	}
	for _, h := range hooks {
		d.deliver(ctx, h, event, data)
	}
}

func (d *Dispatcher) deliver(ctx context.Context, h store.Webhook, event string, data any) {
	p := eventPayload{Event: event, Timestamp: time.Now().UTC(), Data: data}
	body, err := json.Marshal(p)
	if err != nil {
		slog.Warn("webhook: marshal failed", "id", h.ID, "err", err)
		return
	}
	ts := fmt.Sprintf("%d", time.Now().Unix())
	sig := sign(h.Secret, body)

	var statusCode *int
	var errMsg *string

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<(attempt-1)) * time.Second)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
		if err != nil {
			s := err.Error()
			errMsg = &s
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Orb-Event", event)
		req.Header.Set("X-Orb-Timestamp", ts)
		req.Header.Set("X-Orb-Signature", "sha256="+sig)

		resp, err := d.client.Do(req)
		if err != nil {
			s := err.Error()
			errMsg = &s
			if attempt < 2 {
				continue
			}
			break
		}
		resp.Body.Close()
		code := resp.StatusCode
		statusCode = &code
		errMsg = nil
		if resp.StatusCode < 500 {
			break // success or 4xx — don't retry
		}
	}

	_ = d.db.CreateWebhookDelivery(ctx, store.CreateWebhookDeliveryParams{
		WebhookID:  h.ID,
		Event:      event,
		Payload:    body,
		StatusCode: statusCode,
		Error:      errMsg,
	})
	slog.Info("webhook delivered", "id", h.ID, "event", event, "status", statusCode)
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
