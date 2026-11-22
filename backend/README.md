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

The migration job reads the same environment as the server (`DATABASE_URL`, or
the `DB_*` variables) and exits nonzero on failure. It is safe to rerun.
