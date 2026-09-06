package repository

import (
	"context"
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
			if _, err := repo.SearchByWedding(context.Background(), uuid.New(), query); err != nil {
				t.Fatalf("search: %v", err)
			}
			if !strings.Contains(capture.sql, "name ILIKE") || !strings.Contains(capture.sql, "name_pinyin ILIKE") {
				t.Errorf("SQL must match name and pinyin: %q", capture.sql)
			}
			lowerSQL := strings.ToLower(capture.sql)
			if strings.Contains(lowerSQL, "phone") || strings.Contains(lowerSQL, "email") {
				t.Errorf("SQL must not match contact fields: %q", capture.sql)
			}
		})
	}
}
