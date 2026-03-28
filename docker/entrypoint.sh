#!/bin/sh
set -e

SECRETS_DIR=${SECRETS_DIR:-/run/secrets}

# Build DATABASE_URL from the persisted password file if not already set.
if [ -z "$DATABASE_URL" ] && [ -f "${SECRETS_DIR}/db_password" ]; then
  DB_PASS=$(cat "${SECRETS_DIR}/db_password")
  export DATABASE_URL="postgres://orb:${DB_PASS}@${DB_HOST:-postgres}:5432/orb?sslmode=disable"
fi

# Load JWT secret from file if not already set.
if [ -z "$JWT_SECRET" ] && [ -f "${SECRETS_DIR}/jwt_secret" ]; then
  export JWT_SECRET=$(cat "${SECRETS_DIR}/jwt_secret")
fi

# Start the API in background and then run nginx in foreground.
# The API binds to the port defined by HTTP_PORT env (default 8080).

/opt/api &
API_PID=$!

trap "kill $API_PID; exit 0" INT TERM

exec nginx -g 'daemon off;'
