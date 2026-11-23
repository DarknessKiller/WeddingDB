package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type searchSQLLogger struct {
	logger.Interface
	sql string
}

func (l *searchSQLLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	l.sql, _ = fc()
}

func TestSearchByWeddingMatchesNamesOnly(t *testing.T) {
	capture := &searchSQLLogger{Interface: logger.Default}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		DryRun: true,
		Logger: capture,
	})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}

	repo := NewGuestRepo(db)
	for _, query := range []string{"张", "13800138000", "guest@example.com"} {
		t.Run(query, func(t *testing.T) {
			capture.sql = ""
			if _, err := repo.SearchByWedding(context.Background(), uuid.New(), query); err != nil && !errors.Is(err, gorm.ErrDryRunModeUnsupported) {
				t.Fatalf("search: %v", err)
			}
			if !strings.Contains(capture.sql, "name ILIKE") || !strings.Contains(capture.sql, "name_pinyin ILIKE") {
				t.Errorf("SQL must match name and pinyin: %q", capture.sql)
			}
			if n := strings.Count(capture.sql, `ESCAPE '\'`); n != 2 {
				t.Errorf("both ILIKE clauses must pin the escape, got %d: %q", n, capture.sql)
			}
			lowerSQL := strings.ToLower(capture.sql)
			if strings.Contains(lowerSQL, "phone") || strings.Contains(lowerSQL, "email") {
				t.Errorf("SQL must not match contact fields: %q", capture.sql)
			}
		})
	}
}

func TestLikePattern_EscapesMetacharacters(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"%", `%\%%`},
		{"_", `%\_%`},
		{`\`, `%\\%`},
		{"Alice", "%Alice%"},
	} {
		if got := likePattern(tc.in); got != tc.want {
			t.Fatalf("likePattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
