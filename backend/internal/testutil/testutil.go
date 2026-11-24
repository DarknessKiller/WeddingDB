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

// RedisAddr returns TEST_REDIS_ADDR. Under CI an empty value is a failure, not
// a skip: the distributed SSE cap has no other proof.
func RedisAddr(t *testing.T) string {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_REDIS_ADDR must be set in CI; SSE Redis tests may not silently skip")
		}
		t.Skip("set TEST_REDIS_ADDR to run real Redis integration test")
	}
	return addr
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
