package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"weddingdb/internal/models"
	"weddingdb/internal/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInvalidCursor = errors.New("invalid cursor")

type GuestRepo struct{ db *gorm.DB }

func NewGuestRepo(db *gorm.DB) *GuestRepo {
	return &GuestRepo{db: db}
}

func parseCursorID(s string) (uuid.UUID, error) {
	if id, err := uuid.Parse(s); err == nil {
		return id, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(s)
		if err != nil {
			return uuid.Nil, err
		}
	}
	return uuid.FromBytes(decoded)
}

func (r *GuestRepo) ListByWedding(ctx context.Context, weddingID uuid.UUID, cursor string, limit int) ([]models.GuestRecord, int64, error) {
	guests := make([]models.GuestRecord, 0)
	var total int64
	if err := r.db.WithContext(ctx).Model(&models.GuestRecord{}).Where("wedding_id = ?", weddingID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count guests: %w", err)
	}
	q := r.db.WithContext(ctx).Where("wedding_id = ?", weddingID).Order("id ASC").Limit(limit + 1)
	if cursor != "" {
		cid, err := parseCursorID(cursor)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
		}
		q = q.Where("id > ?", cid)
	}
	err := q.Find(&guests).Error
	return guests, total, err
}

func (r *GuestRepo) FindByID(ctx context.Context, id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	var g models.GuestRecord
	err := r.db.WithContext(ctx).Where("id = ? AND wedding_id = ?", id, weddingID).First(&g).Error
	return &g, err
}

// IDInUse reports whether any wedding has a guest with this ID.
func (r *GuestRepo) IDInUse(ctx context.Context, id uuid.UUID) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&models.GuestRecord{}).Where("id = ?", id).Count(&n).Error
	return n > 0, err
}

func (r *GuestRepo) FindByTable(ctx context.Context, weddingID, tableID uuid.UUID) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	err := r.db.WithContext(ctx).Where("wedding_id = ? AND table_id = ?", weddingID, tableID).Find(&guests).Error
	return guests, err
}

func (r *GuestRepo) Create(ctx context.Context, g *models.GuestRecord) error {
	g.NamePinyin = models.GenerateNamePinyin(g.Name)
	if g.UpdatedAt.IsZero() {
		g.UpdatedAt = time.Now()
	}
	stampFields(g, editableFieldColumns, g.UpdatedAt)
	return r.db.WithContext(ctx).Create(g).Error
}

// likePattern escapes LIKE metacharacters, including the escape character
// itself, so a user query cannot break the pattern.
func likePattern(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return fmt.Sprintf("%%%s%%", s)
}

func (r *GuestRepo) SearchByWedding(ctx context.Context, weddingID uuid.UUID, query string) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	q := likePattern(query)
	pinyinQ := likePattern(strings.ToLower(models.GenerateNamePinyin(query)))
	lowerQ := strings.ToLower(query)
	prefix := fmt.Sprintf("%s%%", strings.ToLower(query))
	err := r.db.WithContext(ctx).Raw(`
		SELECT * FROM guest_records
		WHERE wedding_id = ? AND (name ILIKE ? OR name_pinyin ILIKE ?)
		ORDER BY
			CASE
				WHEN LOWER(name) = ? THEN 0
				WHEN LEFT(LOWER(name), ?) = ? THEN 1
				WHEN LEFT(name_pinyin, ?) = ? THEN 2
				ELSE 3
			END, name
		LIMIT 101`,
		weddingID, q, pinyinQ,
		lowerQ, utf8.RuneCountInString(lowerQ), prefix, utf8.RuneCountInString(lowerQ), strings.ToLower(models.GenerateNamePinyin(query))).
		Scan(&guests).Error
	return guests, err
}

// editableColumns are the fields an edit form owns. checked_in_at is left out:
// only check-in and check-out own it, so a stale edit form cannot undo it.
func editableColumns(g *models.GuestRecord) map[string]any {
	g.NamePinyin = models.GenerateNamePinyin(g.Name)
	return map[string]any{
		"name":        g.Name,
		"name_pinyin": g.NamePinyin,
		"email":       g.Email,
		"pax":         g.Pax,
		"table_id":    g.TableID,
		"seat_num":    g.SeatNum,
		"rsvp":        g.RSVP,
		"notes":       g.Notes,
		"dietary":     g.Dietary,
		"is_vip":      g.IsVip,
		"angbao_amt":  g.AngbaoAmt,
		"gift_item":   g.GiftItem,
	}
}

// editableFieldColumns are the columns an online edit owns. An online write
// stamps all of them at the write time; offline sync stamps only the columns
// its payload carries, so an unrelated newer write cannot block a field.
var editableFieldColumns = []string{
	"name", "email", "pax", "table_id", "seat_num", "rsvp", "notes",
	"dietary", "is_vip", "angbao_amt", "gift_item",
}

func stampFields(g *models.GuestRecord, columns []string, at time.Time) {
	if g.FieldUpdatedAt == nil {
		g.FieldUpdatedAt = make(map[string]time.Time, len(columns))
	}
	for _, column := range columns {
		g.FieldUpdatedAt[column] = at
	}
}

// FieldVersion is the time of the newest write to column. Rows created before
// per-field versions existed fall back to the row version.
func FieldVersion(g *models.GuestRecord, column string) time.Time {
	if t, ok := g.FieldUpdatedAt[column]; ok {
		return t
	}
	return g.UpdatedAt
}

// NewestWrite returns the newest write time recorded on the row.
func NewestWrite(g *models.GuestRecord) time.Time {
	newest := g.UpdatedAt
	for _, t := range g.FieldUpdatedAt {
		if t.After(newest) {
			newest = t
		}
	}
	return newest
}

func stampsValue(stamps map[string]time.Time) (string, error) {
	b, err := json.Marshal(stamps)
	if err != nil {
		return "", fmt.Errorf("marshal field versions: %w", err)
	}
	return string(b), nil
}

// Update writes the editable columns of a guest.
func (r *GuestRepo) Update(ctx context.Context, g *models.GuestRecord) error {
	stampFields(g, editableFieldColumns, time.Now())
	columns := editableColumns(g)
	stamps, err := stampsValue(g.FieldUpdatedAt)
	if err != nil {
		return err
	}
	columns["field_updated_at"] = stamps
	return r.db.WithContext(ctx).Model(g).Where("id = ? AND wedding_id = ?", g.ID, g.WeddingID).
		Updates(columns).Error
}

// UpdateIfUnchanged writes the editable columns only while the row still
// matches the version the caller loaded, so a stale edit form cannot revert a
// ErrGuestChanged reports that the guest changed after the caller loaded it.
var ErrGuestChanged = errors.New("guest changed since it was loaded")

// concurrent check-in, angbao entry or seat move. Returns ErrGuestChanged when
// it lost that race.
func (r *GuestRepo) UpdateIfUnchanged(ctx context.Context, g *models.GuestRecord, expectedUpdatedAt time.Time) error {
	stampFields(g, editableFieldColumns, time.Now())
	columns := editableColumns(g)
	stamps, err := stampsValue(g.FieldUpdatedAt)
	if err != nil {
		return err
	}
	columns["field_updated_at"] = stamps
	res := r.db.WithContext(ctx).Model(g).
		Where("id = ? AND wedding_id = ? AND updated_at = ?", g.ID, g.WeddingID, expectedUpdatedAt).
		Updates(columns)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrGuestChanged
	}
	return nil
}

// UpdateFields writes only the given columns of one guest, so a concurrent
// check-in or edit of other columns survives. Returns gorm.ErrRecordNotFound
// when no guest matches.
func (r *GuestRepo) UpdateFields(ctx context.Context, id, weddingID uuid.UUID, fields map[string]any) error {
	return r.updateFieldsAt(ctx, id, weddingID, fields, time.Now(), nil)
}

// SyncUpdateFields writes only the given columns and merges their per-field
// version stamps, so a mutation that carries one field leaves the time of
// every other field untouched.
func (r *GuestRepo) SyncUpdateFields(ctx context.Context, id, weddingID uuid.UUID, fields map[string]any, at, rowUpdatedAt time.Time) error {
	return r.updateFieldsAt(ctx, id, weddingID, fields, at, &rowUpdatedAt)
}

func (r *GuestRepo) updateFieldsAt(ctx context.Context, id, weddingID uuid.UUID, fields map[string]any, at time.Time, rowUpdatedAt *time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var g models.GuestRecord
		q := tx.WithContext(ctx)
		if tx.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Select("id", "field_updated_at", "updated_at").
			Where("id = ? AND wedding_id = ?", id, weddingID).First(&g).Error; err != nil {
			return err
		}
		// Snapshot the pre-write row version for fields that predate per-field
		// versions, or the updated_at this write sets would hide their real time.
		for _, column := range editableFieldColumns {
			if _, ok := g.FieldUpdatedAt[column]; !ok {
				stampFields(&g, []string{column}, g.UpdatedAt)
			}
		}
		columns := make([]string, 0, len(fields))
		for column := range fields {
			switch column {
			case "updated_at", "field_updated_at":
				continue
			}
			columns = append(columns, column)
		}
		stampFields(&g, columns, at)
		stamps, err := stampsValue(g.FieldUpdatedAt)
		if err != nil {
			return err
		}
		fields["field_updated_at"] = stamps
		if rowUpdatedAt != nil {
			fields["updated_at"] = *rowUpdatedAt
		}
		res := tx.Model(&models.GuestRecord{}).
			Where("id = ? AND wedding_id = ?", id, weddingID).Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *GuestRepo) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.GuestRecord{}).Error
}

// UnassignByTable clears table_id and seat_num for all guests at a given
// table. It goes through UpdateFields so each guest's per-field versions are
// stamped; a plain bulk UPDATE would leave an older queued offline move looking
// like the newest write for those columns.
func (r *GuestRepo) UnassignByTable(ctx context.Context, weddingID, tableID uuid.UUID) error {
	var ids []uuid.UUID
	if err := r.db.WithContext(ctx).Model(&models.GuestRecord{}).
		Where("wedding_id = ? AND table_id = ?", weddingID, tableID).
		Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := r.UpdateFields(ctx, id, weddingID, map[string]any{"table_id": nil, "seat_num": nil}); err != nil {
			return err
		}
	}
	return nil
}

// FindByIDForUpdate loads a guest under a row lock so the caller's read and
// write see the same row. The lock is Postgres-only; sqlite (tests) has one
// writer.
func (r *GuestRepo) FindByIDForUpdate(ctx context.Context, id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	q := r.db.WithContext(ctx)
	if r.db.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var g models.GuestRecord
	err := q.Where("id = ? AND wedding_id = ?", id, weddingID).First(&g).Error
	return &g, err
}

func (r *GuestRepo) ListAllByWedding(ctx context.Context, weddingID uuid.UUID) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	err := r.db.WithContext(ctx).Where("wedding_id = ?", weddingID).Order("name ASC").Find(&guests).Error
	return guests, err
}

func (r *GuestRepo) TableOccupancy(ctx context.Context, weddingID uuid.UUID) ([]TableOccupancy, error) {
	type row struct {
		TableID uuid.UUID
		Pax     int
	}
	rows := make([]row, 0)
	err := r.db.WithContext(ctx).Model(&models.BanquetTable{}).
		Select("banquet_tables.id as table_id, COALESCE(SUM(guest_records.pax), 0) as pax").
		Joins("LEFT JOIN guest_records ON guest_records.table_id = banquet_tables.id AND guest_records.wedding_id = ?", weddingID).
		Where("banquet_tables.wedding_id = ?", weddingID).
		Group("banquet_tables.id").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]TableOccupancy, 0, len(rows))
	for _, row := range rows {
		result = append(result, TableOccupancy(row))
	}
	return result, nil
}

type TableOccupancy struct {
	TableID uuid.UUID `json:"TableID"`
	Pax     int       `json:"Pax"`
}

func (t TableOccupancy) MarshalJSON() ([]byte, error) {
	type Alias TableOccupancy
	return json.Marshal(&struct {
		TableID string `json:"TableID"`
		Alias
	}{
		TableID: utils.EncodeUUID(t.TableID),
		Alias:   (Alias)(t),
	})
}

// SyncCreate creates a guest with explicit CreatedAt/UpdatedAt for offline
// sync, stamping every field with the creation time.
func (r *GuestRepo) SyncCreate(ctx context.Context, g *models.GuestRecord) error {
	g.NamePinyin = models.GenerateNamePinyin(g.Name)
	if g.UpdatedAt.IsZero() {
		g.UpdatedAt = time.Now()
	}
	stampFields(g, editableFieldColumns, g.UpdatedAt)
	return r.db.WithContext(ctx).Create(g).Error
}
