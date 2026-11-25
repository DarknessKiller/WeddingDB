package services

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapSearchQuery(t *testing.T) {
	long := strings.Repeat("婚礼", 200) // 400 runes, 1200 bytes
	got := capSearchQuery(long)
	if n := utf8.RuneCountInString(got); n != maxSearchQueryRunes {
		t.Fatalf("capped length = %d runes, want %d", n, maxSearchQueryRunes)
	}
	if !strings.HasPrefix(long, got) {
		t.Fatal("capped query must be a prefix of the input")
	}
	if got := capSearchQuery("Alice"); got != "Alice" {
		t.Fatalf("short query changed: %q", got)
	}
}
