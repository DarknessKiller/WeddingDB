#!/usr/bin/env bash
set -euo pipefail
umask 077

if [[ $# -lt 2 || $# -gt 3 || ( $# -eq 3 && $3 != --force ) ]]; then
    echo "Usage: $0 restore.sql.gz new_database [--force]" >&2
    exit 1
fi
BACKUP_FILE=$1
DB_NAME=$2
# Restrict names so neither SQL literals nor connection strings can be injected.
if [[ ! $DB_NAME =~ ^[a-zA-Z_][a-zA-Z0-9_]{0,62}$ || $DB_NAME == postgres || $DB_NAME == template[01] ]]; then
    echo "Use a database name of 1-63 letters, digits or underscores, starting with a letter or underscore; system databases are forbidden." >&2
    exit 1
fi
gzip -t -- "$BACKUP_FILE"
[[ $(gzip -dc -- "$BACKUP_FILE" | wc -c) -gt 0 ]]
COMPOSE_CONTAINER=$(docker compose ps -q postgres) || COMPOSE_CONTAINER=
CONTAINER=${COMPOSE_CONTAINER:-${CONTAINER:-}}
: "${CONTAINER:?No postgres container found; set CONTAINER if Compose is unavailable}"
DB_USER=${DB_USER:-weddingdb}
EXISTS=$(docker exec "$CONTAINER" psql -X -U "$DB_USER" -d postgres -v ON_ERROR_STOP=1 -Atc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'")
if [[ $EXISTS == 1 ]]; then
    if [[ ${3:-} != --force ]]; then
        echo "Database $DB_NAME exists; refusing restore. --force drops and recreates it." >&2
        exit 1
    fi
    docker exec "$CONTAINER" dropdb -U "$DB_USER" -- "$DB_NAME"
fi
docker exec "$CONTAINER" createdb -U "$DB_USER" -E UTF8 -T template0 -- "$DB_NAME"
gzip -dc -- "$BACKUP_FILE" | docker exec -i "$CONTAINER" psql -X -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 --single-transaction
docker exec "$CONTAINER" psql -X -U "$DB_USER" -d "$DB_NAME" -v ON_ERROR_STOP=1 -c "
SELECT 'wedding_events' AS table_name, count(*) AS rows FROM wedding_events
UNION ALL SELECT 'guest_records', count(*) FROM guest_records
UNION ALL SELECT 'banquet_tables', count(*) FROM banquet_tables
UNION ALL SELECT 'admin_users', count(*) FROM admin_users;"
