package testutil

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPgDSN_ReturnsConfiguredURL(t *testing.T) {
	t.Setenv("TEST_DATABASE_URL", "postgres://example/test")
	t.Setenv("CI", "")
	if got := PgDSN(t); got != "postgres://example/test" {
		t.Fatalf("PgDSN = %q, want the configured URL", got)
	}
}

// TestPgDSN_FailsInCIWithoutURL re-runs this test in a child process with an
// empty DSN, because t.Fatal ends the test binary.
func TestPgDSN_FailsInCIWithoutURL(t *testing.T) {
	runCIHelper(t, "PGDSN_CI_HELPER", func() {
		t.Setenv("CI", "1")
		t.Setenv("TEST_DATABASE_URL", "")
		PgDSN(t)
	}, "TEST_DATABASE_URL must be set in CI")
}

func TestRedisAddr_ReturnsConfiguredAddr(t *testing.T) {
	t.Setenv("TEST_REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("CI", "")
	if got := RedisAddr(t); got != "127.0.0.1:6379" {
		t.Fatalf("RedisAddr = %q, want the configured address", got)
	}
}

func TestRedisAddr_FailsInCIWithoutAddr(t *testing.T) {
	runCIHelper(t, "REDISADDR_CI_HELPER", func() {
		t.Setenv("CI", "1")
		t.Setenv("TEST_REDIS_ADDR", "")
		RedisAddr(t)
	}, "TEST_REDIS_ADDR must be set in CI")
}

// runCIHelper runs body in a child test process and requires that it fails with
// want, proving the helper does not silently skip under CI.
func runCIHelper(t *testing.T, marker string, body func(), want string) {
	t.Helper()
	if os.Getenv(marker) == "1" {
		body()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run="+t.Name())
	cmd.Env = append(os.Environ(), marker+"=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helper without its setting in CI exited 0, want failure; output %s", out)
	}
	if !strings.Contains(string(out), want) {
		t.Fatalf("helper output %q does not contain %q", out, want)
	}
}
