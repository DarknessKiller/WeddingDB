#!/usr/bin/env bash
set -euo pipefail

CONTAINER="weddingdb-postgres-1"
DB_USER="weddingdb"
DB_NAME="weddingdb"
BACKUP_DIR="./backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="${BACKUP_DIR}/weddingdb_${TIMESTAMP}.sql.gz"

mkdir -p "$BACKUP_DIR"

echo "Backing up $DB_NAME from $CONTAINER..."
docker exec "$CONTAINER" pg_dump -U "$DB_USER" "$DB_NAME" \
  | sed '/^\\/d' \
  | gzip > "$BACKUP_FILE"
echo "Saved: $BACKUP_FILE ($(du -h "$BACKUP_FILE" | cut -f1))"
