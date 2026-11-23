// Package testutil holds helpers shared by tests. It ships with the module
// but is imported only from _test.go files.
package testutil

import (
	"os"
	"testing"
)

// NoErr fails the test when err is non-nil.
func NoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// PgDSN returns TEST_DATABASE_URL. Under CI an empty value is a failure, not a
// skip: Postgres tests may not silently skip.
func PgDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL must be set in CI; Postgres tests may not silently skip")
		}
		t.Skip("set TEST_DATABASE_URL to run against Postgres")
	}
	return dsn
}
