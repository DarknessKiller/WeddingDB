// Package testutil holds helpers shared by tests. It ships with the module
// but is imported only from _test.go files.
package testutil

import "testing"

// NoErr fails the test when err is non-nil.
func NoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
