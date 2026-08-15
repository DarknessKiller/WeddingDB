package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/utils"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
	r.db.WithContext(ctx).Model(&models.GuestRecord{}).Where("wedding_id = ?", weddingID).Count(&total)
	q := r.db.WithContext(ctx).Where("wedding_id = ?", weddingID).Order("id ASC").Limit(limit + 1)
	if cursor != "" {
		if cid, err := parseCursorID(cursor); err == nil {
			q = q.Where("id > ?", cid)
		}
	}
	err := q.Find(&guests).Error
	return guests, total, err
}

func (r *GuestRepo) FindByID(ctx context.Context, id, weddingID uuid.UUID) (*models.GuestRecord, error) {
	var g models.GuestRecord
	err := r.db.WithContext(ctx).Where("id = ? AND wedding_id = ?", id, weddingID).First(&g).Error
	return &g, err
}

func (r *GuestRepo) FindByTable(ctx context.Context, weddingID, tableID uuid.UUID) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	err := r.db.WithContext(ctx).Where("wedding_id = ? AND table_id = ?", weddingID, tableID).Find(&guests).Error
	return guests, err
}

func (r *GuestRepo) SearchByWedding(ctx context.Context, weddingID uuid.UUID, query string) ([]models.GuestRecord, error) {
	guests := make([]models.GuestRecord, 0)
	escaped := strings.ReplaceAll(query, "%", "\\%")
	escaped = strings.ReplaceAll(escaped, "_", "\\_")
	q := fmt.Sprintf("%%%s%%", escaped)
	pinyinQ := fmt.Sprintf("%%%s%%", strings.ToLower(models.GenerateNamePinyin(query)))
	lowerQ := strings.ToLower(query)
	err := r.db.WithContext(ctx).Where("wedding_id = ? AND (name ILIKE ? OR name_pinyin ILIKE ? OR phone ILIKE ? OR email ILIKE ?)",
		weddingID, q, pinyinQ, q, q).
		Order(fmt.Sprintf(`
			CASE
				WHEN LOWER(name) = %s THEN 0
				WHEN LOWER(name) LIKE %s THEN 1
				WHEN LOWER(name) LIKE %s THEN 2
				ELSE 3
			END, name`,
			fmt.Sprintf("'%s'", strings.ReplaceAll(lowerQ, "'", "''")),
			fmt.Sprintf("'%s%%'", strings.ReplaceAll(lowerQ, "'", "''")),
			fmt.Sprintf("'%%%s%%'", strings.ReplaceAll(lowerQ, "'", "''")))).
		Find(&guests).Error
	return guests, err
}

func (r *GuestRepo) Create(ctx context.Context, g *models.GuestRecord) error {
	g.NamePinyin = models.GenerateNamePinyin(g.Name)
	return r.db.WithContext(ctx).Create(g).Error
}

func (r *GuestRepo) Update(ctx context.Context, g *models.GuestRecord) error {
	g.NamePinyin = models.GenerateNamePinyin(g.Name)
	return r.db.WithContext(ctx).Model(g).Where("id = ? AND wedding_id = ?", g.ID, g.WeddingID).Updates(map[string]interface{}{
		"name":         g.Name,
		"name_pinyin":  g.NamePinyin,
		"phone":        g.Phone,
		"email":        g.Email,
		"pax":          g.Pax,
		"table_id":     g.TableID,
		"seat_num":     g.SeatNum,
		"rsvp":         g.RSVP,
		"checked_in_at": g.CheckedInAt,
		"notes":        g.Notes,
		"dietary":      g.Dietary,
		"is_vip":       g.IsVip,
		"angbao_amt":   g.AngbaoAmt,
		"gift_item":    g.GiftItem,
	}).Error
}

func (r *GuestRepo) Delete(ctx context.Context, id, weddingID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ? AND wedding_id = ?", id, weddingID).Delete(&models.GuestRecord{}).Error
}

// UnassignByTable clears table_id and seat_num for all guests at a given table.
func (r *GuestRepo) UnassignByTable(ctx context.Context, weddingID, tableID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&models.GuestRecord{}).
		Where("wedding_id = ? AND table_id = ?", weddingID, tableID).
		Updates(map[string]interface{}{"table_id": nil, "seat_num": nil}).Error
}

// ConditionalCheckIn atomically checks in a guest only if not already checked in.
// Returns models.ErrAlreadyCheckedIn if the guest was already checked in.
func (r *GuestRepo) ConditionalCheckIn(ctx context.Context, id, weddingID uuid.UUID, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&models.GuestRecord{}).
		Where("id = ? AND wedding_id = ? AND checked_in_at IS NULL", id, weddingID).
		Update("checked_in_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return models.ErrAlreadyCheckedIn
	}
	return nil
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
