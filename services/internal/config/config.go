// Package config provides shared configuration helpers for Orb services.
package config

import "os"

// DSN returns the Postgres connection string from the DATABASE_URL environment
// variable. In Docker, the entrypoint populates this from /run/secrets/db_password.
// Returns an empty string if unset, which will cause the database driver to
// return a clear error rather than attempting a connection with stale defaults.
func DSN() string {
	return os.Getenv("DATABASE_URL")
}

// Env returns the value of the environment variable key, or def if unset.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
