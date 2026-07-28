#!/bin/sh
set -eu

database_started=false
api_pid=
web_pid=

cleanup() {
  status=$?
  trap - EXIT INT TERM HUP

  if [ -n "$api_pid" ]; then
    kill "$api_pid" 2>/dev/null || true
  fi
  if [ -n "$web_pid" ]; then
    kill "$web_pid" 2>/dev/null || true
  fi

  if [ -n "$api_pid" ]; then
    wait "$api_pid" 2>/dev/null || true
  fi
  if [ -n "$web_pid" ]; then
    wait "$web_pid" 2>/dev/null || true
  fi

  if [ "$database_started" = true ]; then
    make dev-db-down
  fi

  exit "$status"
}

handle_signal() {
  exit 130
}

trap cleanup EXIT
trap handle_signal INT TERM HUP

if [ -z "$(docker compose -f compose.yaml -f compose.dev.yaml ps --status running --quiet postgres)" ]; then
  database_started=true
fi

make dev-db-up

if [ ! -d web/node_modules ]; then
  make dev-web-install
fi

printf '%s\n' \
  "Starting Pulse API at http://localhost:8080" \
  "Starting Vite at http://localhost:5173" \
  "Press Ctrl+C to stop."

make dev-api &
api_pid=$!
make dev-web &
web_pid=$!

status=0
while :; do
  if ! kill -0 "$api_pid" 2>/dev/null; then
    wait "$api_pid" || status=$?
    break
  fi
  if ! kill -0 "$web_pid" 2>/dev/null; then
    wait "$web_pid" || status=$?
    break
  fi
  sleep 1
done

exit "$status"
