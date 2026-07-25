#!/bin/sh
set -eu

chrome="${CHROME_BIN:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
profile="$(mktemp -d /tmp/pulse-e2e-chrome.XXXXXX)"
log_file="$profile/chrome.log"

"$chrome" \
  --headless=new \
  --remote-debugging-port=9223 \
  --user-data-dir="$profile" \
  --no-first-run \
  --no-default-browser-check \
  "${PULSE_E2E_URL:-http://localhost:8080/}" >"$log_file" 2>&1 &
chrome_pid=$!
trap 'kill "$chrome_pid" 2>/dev/null || true' EXIT

attempt=0
until curl --silent --fail http://127.0.0.1:9223/json/list >/dev/null; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 40 ]; then
    echo "Chrome DevTools did not start; log: $log_file" >&2
    exit 1
  fi
  sleep 0.25
done

node scripts/e2e-rss.mjs
