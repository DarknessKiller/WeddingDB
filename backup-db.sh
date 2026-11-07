#!/usr/bin/env bash
set -euo pipefail
umask 077

COMPOSE_CONTAINER=$(docker compose ps -q postgres) || COMPOSE_CONTAINER=
CONTAINER=${COMPOSE_CONTAINER:-${CONTAINER:-}}
: "${CONTAINER:?No postgres container found; set CONTAINER if Compose is unavailable}"
DB_USER=${DB_USER:-weddingdb}
DB_NAME=${DB_NAME:-weddingdb}
BACKUP_DIR=${BACKUP_DIR:-./backups}
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/weddingdb_${TIMESTAMP}.sql.gz"

mkdir -p "$BACKUP_DIR"
TEMP_FILE=$(mktemp "$BACKUP_DIR/.backup.XXXXXX")
trap 'rm -f -- "$TEMP_FILE"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "Backing up $DB_NAME from $CONTAINER..."
docker exec "$CONTAINER" pg_dump -U "$DB_USER" "$DB_NAME" | gzip > "$TEMP_FILE"
gzip -t "$TEMP_FILE"
[[ $(gzip -dc "$TEMP_FILE" | wc -c) -gt 0 ]]
mv -- "$TEMP_FILE" "$BACKUP_FILE"
echo "Saved: $BACKUP_FILE ($(du -h "$BACKUP_FILE" | cut -f1))"
