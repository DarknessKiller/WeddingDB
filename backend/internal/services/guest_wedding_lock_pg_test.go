package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestCheckIn_WaitsForWeddingLock proves every guest-write path takes the
// wedding row lock first (CODING_STANDARDS.md lock order): a transaction that
// holds the wedding row blocks the write until it commits, and the write then
// applies. Requires TEST_DATABASE_URL pointing at a *_test database.
func TestCheckIn_WaitsForWeddingLock(t *testing.T) {
	dsn := pgDSN(t)
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
		db.Exec("DROP TABLE IF EXISTS guest_records, banquet_tables, wedding_events CASCADE")
	}
	drop()
	t.Cleanup(drop)
	if err := db.AutoMigrate(&models.WeddingEvent{}, &models.BanquetTable{}, &models.GuestRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	ctx := t.Context()
	svc := NewGuestService(repository.NewGuestRepo(db), nil, db)
	base := time.Now().Add(-10 * time.Minute).UTC()
	opAt := base.Add(time.Minute)

	type write struct {
		name string
		// seedCheckIn pre-checks the guest in, so the check-out paths have
		// something to clear.
		seedCheckIn bool
		op          func(ctx context.Context, wid, gid uuid.UUID) error
		blocked     func(t *testing.T, gid uuid.UUID)
		want        func(t *testing.T, gid uuid.UUID)
	}
	readsGuest := func(t *testing.T, gid uuid.UUID, fn func(*models.GuestRecord)) {
		t.Helper()
		var g models.GuestRecord
		if err := db.Where("id = ?", gid).First(&g).Error; err != nil {
			t.Fatalf("read guest: %v", err)
		}
		fn(&g)
	}
	cases := []write{
		{
			name: "checkin",
			op: func(ctx context.Context, wid, gid uuid.UUID) error {
				_, err := svc.CheckIn(ctx, gid, wid, "late")
				return err
			},
			blocked: func(t *testing.T, gid uuid.UUID) {
				readsGuest(t, gid, func(g *models.GuestRecord) {
					if g.CheckedInAt != nil {
						t.Fatal("check-in applied while the wedding row was locked")
					}
				})
			},
			want: func(t *testing.T, gid uuid.UUID) {
				readsGuest(t, gid, func(g *models.GuestRecord) {
					if g.CheckedInAt == nil || g.RSVP != "confirmed" {
						t.Fatalf("guest = checkedInAt %v rsvp %q, want the check-in applied", g.CheckedInAt, g.RSVP)
					}
				})
			},
		},
		{
			name:        "checkout",
			seedCheckIn: true,
			op: func(ctx context.Context, wid, gid uuid.UUID) error {
				return svc.CheckOut(ctx, gid, wid)
			},
			blocked: func(t *testing.T, gid uuid.UUID) {
				readsGuest(t, gid, func(g *models.GuestRecord) {
					if g.CheckedInAt == nil {
						t.Fatal("check-out applied while the wedding row was locked")
					}
				})
			},
			want: func(t *testing.T, gid uuid.UUID) {
				readsGuest(t, gid, func(g *models.GuestRecord) {
					if g.CheckedInAt != nil {
						t.Fatalf("guest still checked in at %v, want the check-out applied", g.CheckedInAt)
					}
				})
			},
		},
		{
			name: "sync delete",
			op: func(ctx context.Context, wid, gid uuid.UUID) error {
				_, err := svc.Sync(ctx, wid, []SyncMutation{{
					Op: SyncOpDelete, GuestID: gid.String(), ClientUpdatedAt: opAt,
				}})
				return err
			},
			blocked: func(t *testing.T, gid uuid.UUID) {
				var count int64
				testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("id = ?", gid).Count(&count).Error)
				if count != 1 {
					t.Fatal("sync delete applied while the wedding row was locked")
				}
			},
			want: func(t *testing.T, gid uuid.UUID) {
				var count int64
				testutil.NoErr(t, db.Model(&models.GuestRecord{}).Where("id = ?", gid).Count(&count).Error)
				if count != 0 {
					t.Fatal("guest still present, want the sync delete applied")
				}
			},
		},
		{
			name: "sync checkin",
			op: func(ctx context.Context, wid, gid uuid.UUID) error {
				_, err := svc.Sync(ctx, wid, []SyncMutation{{
					Op: SyncOpCheckIn, GuestID: gid.String(), ClientUpdatedAt: opAt,
				}})
				return err
			},
			blocked: func(t *testing.T, gid uuid.UUID) {
				readsGuest(t, gid, func(g *models.GuestRecord) {
					if g.CheckedInAt != nil {
						t.Fatal("sync check-in applied while the wedding row was locked")
					}
				})
			},
			want: func(t *testing.T, gid uuid.UUID) {
				readsGuest(t, gid, func(g *models.GuestRecord) {
					if g.CheckedInAt == nil {
						t.Fatal("guest not checked in, want the sync check-in applied")
					}
				})
			},
		},
		{
			name:        "sync checkout",
			seedCheckIn: true,
			op: func(ctx context.Context, wid, gid uuid.UUID) error {
				_, err := svc.Sync(ctx, wid, []SyncMutation{{
					Op: SyncOpCheckOut, GuestID: gid.String(), ClientUpdatedAt: opAt,
				}})
				return err
			},
			blocked: func(t *testing.T, gid uuid.UUID) {
				readsGuest(t, gid, func(g *models.GuestRecord) {
					if g.CheckedInAt == nil {
						t.Fatal("sync check-out applied while the wedding row was locked")
					}
				})
			},
			want: func(t *testing.T, gid uuid.UUID) {
				readsGuest(t, gid, func(g *models.GuestRecord) {
					if g.CheckedInAt != nil {
						t.Fatalf("guest still checked in at %v, want the sync check-out applied", g.CheckedInAt)
					}
				})
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wid, gid := uuid.New(), uuid.New()
			testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "Lock Order"}).Error)
			guest := models.GuestRecord{
				ID: gid, WeddingID: wid, Name: "Guest", Pax: 1, RSVP: "pending",
				CreatedAt: base, UpdatedAt: base,
			}
			if c.seedCheckIn {
				guest.CheckedInAt = &base
				guest.RSVP = "confirmed"
				guest.FieldTimes = models.FieldTimes{groupCheckedInAt: base, groupRSVP: base}
			}
			testutil.NoErr(t, db.Create(&guest).Error)

			// Hold the wedding row lock. Every guest write must wait for it,
			// so none may apply until this transaction commits.
			lockTx := db.Begin()
			testutil.NoErr(t, lockTx.Error)
			// Rollback is a no-op once the transaction commits; it releases the
			// lock when an assertion fails before the commit.
			t.Cleanup(func() { lockTx.Rollback() })
			var lockedWedding models.WeddingEvent
			testutil.NoErr(t, lockTx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id").Where("id = ?", wid).First(&lockedWedding).Error)

			done := make(chan error, 1)
			go func() { done <- c.op(ctx, wid, gid) }()

			select {
			case err := <-done:
				t.Fatalf("write finished before the wedding row lock was released: %v", err)
			case <-time.After(300 * time.Millisecond):
			}
			c.blocked(t, gid)

			testutil.NoErr(t, lockTx.Commit().Error)
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("write failed after the lock was released: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("timed out waiting for the write to finish")
			}
			c.want(t, gid)
		})
	}
}
