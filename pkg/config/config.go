// Package config provides shared configuration helpers for Orb services.
package config

import "os"

// DefaultDSN is the fallback Postgres connection string used when DATABASE_URL
// is not set. Override it via the DATABASE_URL environment variable in
// production.
const DefaultDSN = "postgres://orb:orb@localhost:5432/orb?sslmode=disable"

// DSN returns the Postgres connection string from the DATABASE_URL environment
// variable, falling back to DefaultDSN when unset.
func DSN() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return DefaultDSN
}

// Env returns the value of the environment variable key, or def if unset.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
