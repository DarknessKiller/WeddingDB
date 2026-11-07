package repository

import (
	"context"
	"errors"
	"testing"

	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestListByWedding_CountErrorPropagates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	testutil.NoErr(t, err)
	testutil.NoErr(t, db.Exec(`CREATE TABLE guest_records (id TEXT PRIMARY KEY, wedding_id TEXT)`).Error)
	boom := errors.New("count failed")
	// Fail only the Count query (its destination is *int64); Find still succeeds.
	testutil.NoErr(t, db.Callback().Query().Before("gorm:query").Register("fail_count", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*int64); ok {
			tx.AddError(boom) //nolint:errcheck // AddError echoes the error just added
		}
	}))
	if _, _, err := NewGuestRepo(db).ListByWedding(context.Background(), uuid.New(), "", 10); !errors.Is(err, boom) {
		t.Fatalf("got %v, want count error", err)
	}
}
