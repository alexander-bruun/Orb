package store

import (
	"context"
	_ "embed"
)

//go:embed migrate.sql
var migrateSQL string

// Migrate applies the full schema idempotently.
// Safe to call on every startup — all statements use IF NOT EXISTS.
func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, migrateSQL)
	return err
}
