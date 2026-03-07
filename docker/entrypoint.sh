#!/bin/sh
set -e

# Start the API in background and then run nginx in foreground.
# The API binds to the port defined by HTTP_PORT env (default 8080).

/opt/api &
API_PID=$!

trap "kill $API_PID; exit 0" INT TERM

exec nginx -g 'daemon off;'
