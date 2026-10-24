package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
)

// TestGuestCap_PostgresConcurrent proves the wedding row lock stops parallel
// creates from crossing the cap. Requires a real Postgres:
//
//	TEST_DATABASE_URL=postgresql://postgres@127.0.0.1:55432/weddingdb_test?sslmode=disable \
//	  go test ./internal/services -run TestGuestCap_PostgresConcurrent -v
func TestGuestCap_PostgresConcurrent(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run against Postgres")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("postgres open: %v", err)
	}
	// The test drops tables; never let it point at a real database.
	var dbName string
	if err := db.Raw("SELECT current_database()").Scan(&dbName).Error; err != nil {
		t.Fatalf("read database name: %v", err)
	}
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to drop tables in %q; point TEST_DATABASE_URL at a *_test database", dbName)
	}
	drop := func() {
		db.Exec("DROP TABLE IF EXISTS guest_records, wedding_events CASCADE")
	}
	drop()
	t.Cleanup(drop)
	if err := db.AutoMigrate(&models.WeddingEvent{}, &models.GuestRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	svc := NewGuestService(repository.NewGuestRepo(db), repository.NewTableRepo(db), nil, nil, db)
	ctx := context.Background()
	wid := uuid.New()
	if err := db.Create(&models.WeddingEvent{ID: wid, Name: "Cap Test"}).Error; err != nil {
		t.Fatalf("seed wedding: %v", err)
	}
	seed := make([]models.GuestRecord, 990)
	for i := range seed {
		seed[i] = models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: fmt.Sprintf("Seed %d", i), Pax: 1}
	}
	if err := db.CreateInBatches(seed, 200).Error; err != nil {
		t.Fatalf("seed guests: %v", err)
	}

	const attempts = 20
	var ok, capped atomic.Int32
	errs := make(chan error, attempts)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			err := svc.Create(ctx, &models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: fmt.Sprintf("Race %d", i), Pax: 1})
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, ErrGuestCapExceeded):
				capped.Add(1)
			default:
				errs <- err
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("unexpected create error: %v", err)
	}

	var total int
	if err := db.Model(&models.GuestRecord{}).Where("wedding_id = ?", wid).
		Select("COALESCE(SUM(pax), 0)").Row().Scan(&total); err != nil {
		t.Fatalf("count guests: %v", err)
	}
	if ok.Load() != 10 || capped.Load() != 10 || total != MaxGuestsPerWedding {
		t.Fatalf("ok=%d capped=%d total=%d, want 10/10/%d", ok.Load(), capped.Load(), total, MaxGuestsPerWedding)
	}
}
