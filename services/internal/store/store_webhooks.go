package store

import (
	"context"
	"database/sql"
)

// ── Webhooks ──────────────────────────────────────────────────────────────

// CreateWebhook inserts a new webhook endpoint.
func (s *Store) CreateWebhook(ctx context.Context, p CreateWebhookParams) (Webhook, error) {
	var w Webhook
	err := s.pool.QueryRow(ctx, `
		INSERT INTO webhooks (id, url, secret, events, description)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, url, secret, events, enabled, description, created_at, updated_at
	`, p.ID, p.URL, p.Secret, p.Events, p.Description).
		Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

// ListWebhooks returns all webhooks ordered by creation time descending.
func (s *Store) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, url, secret, events, enabled, description, created_at, updated_at
		FROM webhooks ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]Webhook, 0)
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, w)
	}
	return results, rows.Err()
}

// GetWebhook returns a single webhook by ID.
func (s *Store) GetWebhook(ctx context.Context, id string) (Webhook, error) {
	var w Webhook
	err := s.pool.QueryRow(ctx, `
		SELECT id, url, secret, events, enabled, description, created_at, updated_at
		FROM webhooks WHERE id = $1
	`, id).Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

// UpdateWebhook updates an existing webhook.
func (s *Store) UpdateWebhook(ctx context.Context, p UpdateWebhookParams) (Webhook, error) {
	var w Webhook
	err := s.pool.QueryRow(ctx, `
		UPDATE webhooks SET url = $2, secret = $3, events = $4, enabled = $5, description = $6, updated_at = now()
		WHERE id = $1
		RETURNING id, url, secret, events, enabled, description, created_at, updated_at
	`, p.ID, p.URL, p.Secret, p.Events, p.Enabled, p.Description).
		Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt)
	return w, err
}

// DeleteWebhook removes a webhook and its delivery records.
func (s *Store) DeleteWebhook(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	return err
}

// ListWebhooksForEvent returns all enabled webhooks subscribed to a given event.
func (s *Store) ListWebhooksForEvent(ctx context.Context, event string) ([]Webhook, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, url, secret, events, enabled, description, created_at, updated_at
		FROM webhooks
		WHERE enabled = TRUE AND $1 = ANY(events)
	`, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]Webhook, 0)
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.URL, &w.Secret, &w.Events, &w.Enabled, &w.Description, &w.CreatedAt, &w.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, w)
	}
	return results, rows.Err()
}

// CreateWebhookDelivery records a webhook delivery attempt.
func (s *Store) CreateWebhookDelivery(ctx context.Context, p CreateWebhookDeliveryParams) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO webhook_deliveries (webhook_id, event, payload, status_code, error)
		VALUES ($1, $2, $3, $4, $5)
	`, p.WebhookID, p.Event, p.Payload, p.StatusCode, p.Error)
	return err
}

func (s *Store) ListWebhookDeliveries(ctx context.Context, webhookID string, limit int) ([]WebhookDelivery, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, webhook_id, event, payload, status_code, error, delivered_at
		FROM webhook_deliveries WHERE webhook_id = $1
		ORDER BY delivered_at DESC LIMIT $2
	`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]WebhookDelivery, 0)
	for rows.Next() {
		var d WebhookDelivery
		var statusCode sql.NullInt32
		var errMsg sql.NullString
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &d.Payload, &statusCode, &errMsg, &d.DeliveredAt); err != nil {
			return nil, err
		}
		if statusCode.Valid {
			v := int(statusCode.Int32)
			d.StatusCode = &v
		}
		if errMsg.Valid {
			d.Error = &errMsg.String
		}
		results = append(results, d)
	}
	return results, rows.Err()
}
