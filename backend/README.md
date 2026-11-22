# Backend

## Migrations

Run the migration job before the server on every deploy. The server no longer
applies schema changes or backfills at boot.

```sh
go run ./cmd/migrate
./server
```

With an image that ships both binaries:

```sh
./migrate
./server
```

`docker-compose.yml` runs this as the one-shot `migrate` service, and `app` waits
for it to complete.

The migration job reads the same environment as the server (`DATABASE_URL`, or
the `DB_*` variables) and exits nonzero on failure. It is safe to rerun.
