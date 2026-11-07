package services

import (
	"os"
	"testing"
)

// pgDSN returns TEST_DATABASE_URL. Under CI an empty value is a failure, not a
// skip: these are the only proofs of the seat/concurrency fixes.
func pgDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI; Postgres concurrency tests may not silently skip")
		}
		t.Skip("set TEST_DATABASE_URL to run against Postgres")
	}
	return dsn
}
