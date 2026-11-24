package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"weddingdb/internal/models"
	"weddingdb/internal/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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

// FindByIDForUpdate reads a guest row and locks it for the current
// transaction. The row lock is Postgres-only; sqlite (tests) has one writer.
func (r *GuestRepo) FindByIDForUpdate(ctx context.Context, id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	var g models.GuestRecord
	q := r.db.WithContext(ctx)
	if r.db.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.Where("id = ? AND wedding_id = ?", id, weddingID).First(&g).Error
	return &g, err
}

func (r *GuestRepo) FindByTable(ctx context.Context, weddingID, tableID uuid.UUID) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	err := r.db.WithContext(ctx).Where("wedding_id = ? AND table_id = ?", weddingID, tableID).Find(&guests).Error
	return guests, err
}

// FindByTableForUpdate reads the guests seated at a table and locks the rows
// for the current transaction, ordered by id so concurrent deletes lock them in
// the same order. The row locks are Postgres-only; sqlite (tests) has one
// writer.
func (r *GuestRepo) FindByTableForUpdate(ctx context.Context, weddingID, tableID uuid.UUID) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	q := r.db.WithContext(ctx)
	if r.db.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.Where("wedding_id = ? AND table_id = ?", weddingID, tableID).Order("id ASC").Find(&guests).Error
	return guests, err
}

// likePattern escapes LIKE metacharacters so a query matches literally.
func likePattern(s string) string {
	escaped := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s)
	return "%" + escaped + "%"
}

func (r *GuestRepo) SearchByWedding(ctx context.Context, weddingID uuid.UUID, query string) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	q := likePattern(query)
	lowerPy := strings.ToLower(models.GenerateNamePinyin(query))
	pinyinQ := likePattern(lowerPy)
	lowerQ := strings.ToLower(query)
	// LEFT counts characters, so the prefix length is a rune count, not bytes.
	err := r.db.WithContext(ctx).Raw(`
		SELECT * FROM guest_records
		WHERE wedding_id = ? AND (name ILIKE ? ESCAPE '\' OR name_pinyin ILIKE ? ESCAPE '\')
		ORDER BY
			CASE
				WHEN LOWER(name) = ? THEN 0
				WHEN LEFT(LOWER(name), ?) = ? THEN 1
				WHEN LEFT(name_pinyin, ?) = ? THEN 2
				ELSE 3
			END, name
		LIMIT 101`,
		weddingID, q, pinyinQ,
		lowerQ, utf8.RuneCountInString(lowerQ), lowerQ, utf8.RuneCountInString(lowerPy), lowerPy).
		Scan(&guests).Error
	return guests, err
}

func (r *GuestRepo) Create(ctx context.Context, g *models.GuestRecord) error {
	g.NamePinyin = models.GenerateNamePinyin(g.Name)
	return r.db.WithContext(ctx).Create(g).Error
}

func (r *GuestRepo) Update(ctx context.Context, g *models.GuestRecord) error {
	g.NamePinyin = models.GenerateNamePinyin(g.Name)
	return r.db.WithContext(ctx).Model(g).Where("id = ? AND wedding_id = ?", g.ID, g.WeddingID).Updates(map[string]interface{}{
		"name":          g.Name,
		"name_pinyin":   g.NamePinyin,
		"email":         g.Email,
		"pax":           g.Pax,
		"table_id":      g.TableID,
		"seat_num":      g.SeatNum,
		"rsvp":          g.RSVP,
		"checked_in_at": g.CheckedInAt,
		"notes":         g.Notes,
		"dietary":       g.Dietary,
		"is_vip":        g.IsVip,
		"angbao_amt":    g.AngbaoAmt,
		"gift_item":     g.GiftItem,
		"field_times":   g.FieldTimes,
		"updated_at":    g.UpdatedAt,
	}).Error
}

func (r *GuestRepo) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.GuestRecord{}).Error
}

func (r *GuestRepo) ListAllByWedding(ctx context.Context, weddingID uuid.UUID) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	err := r.db.WithContext(ctx).Where("wedding_id = ?", weddingID).Order("name ASC").Find(&guests).Error
	return guests, err
}

func (r *GuestRepo) TableOccupancy(ctx context.Context, weddingID uuid.UUID) ([]TableOccupancy, error) {
	type row struct {
		TableID     uuid.UUID
		Pax         int
		HighestSeat int
	}
	rows := make([]row, 0)
	err := r.db.WithContext(ctx).Model(&models.BanquetTable{}).
		Select(`banquet_tables.id as table_id, COALESCE(SUM(guest_records.pax), 0) as pax,
			COALESCE(MAX(CASE WHEN guest_records.seat_num IS NOT NULL
				THEN guest_records.seat_num + guest_records.pax - 1 END), 0) as highest_seat`).
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
	TableID     uuid.UUID `json:"TableID"`
	Pax         int       `json:"Pax"`
	HighestSeat int       `json:"HighestSeat"`
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
