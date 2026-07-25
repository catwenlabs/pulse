#!/bin/sh
set -eu

backup_dir="${PULSE_BACKUP_DIR:-./backups}"
retention_days="${PULSE_BACKUP_RETENTION_DAYS:-14}"

case "$backup_dir" in
  ""|"/") echo "unsafe backup directory" >&2; exit 1 ;;
esac

mkdir -p "$backup_dir"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
target="$backup_dir/pulse-$timestamp.sql.gz"
temporary="$backup_dir/.pulse-$timestamp.sql"
trap 'rm -f "$temporary"' EXIT

docker compose exec -T postgres pg_dump \
  --username=pulse \
  --dbname=pulse \
  --clean \
  --if-exists \
  --no-owner > "$temporary"

gzip -9 -c "$temporary" > "$target"
gzip -t "$target"
find "$backup_dir" -type f -name 'pulse-*.sql.gz' -mtime "+$retention_days" -delete
printf '%s\n' "$target"
