package repository

import (
	"context"
	"errors"

	"weddingdb/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrLayoutVersionConflict is returned when a layout save names a version
// other than the stored one: someone else saved since the editor loaded.
var ErrLayoutVersionConflict = errors.New("layout was changed by someone else")

type LayoutRepo struct{ db *gorm.DB }

func NewLayoutRepo(db *gorm.DB) *LayoutRepo { return &LayoutRepo{db: db} }

func (r *LayoutRepo) ElementsByWedding(ctx context.Context, weddingID uuid.UUID) ([]models.HallElement, error) {
	els := make([]models.HallElement, 0)
	err := r.db.WithContext(ctx).Where("wedding_id = ?", weddingID).Order("z_index").Find(&els).Error
	return els, err
}

// SaveLayout writes the layout and returns the new layout version. A non-nil
// version must equal the stored one or ErrLayoutVersionConflict is returned;
// nil skips the check (older clients).
func (r *LayoutRepo) SaveLayout(
	ctx context.Context,
	weddingID uuid.UUID,
	version *int,
	hallWidth, hallHeight int,
	tables []models.TableUpdate,
	toCreate, toUpdate []models.HallElement,
	toDelete []uuid.UUID,
) (int, error) {
	var newVersion int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Wedding row lock first, like every guest write, so saves serialize and
		// the version check sees the last committed save. sqlite has one writer.
		q := tx
		if tx.Name() == "postgres" {
			q = tx.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var w models.WeddingEvent
		if err := q.Select("id", "layout_version").Where("id = ?", weddingID).First(&w).Error; err != nil {
			return err
		}
		if version != nil && *version != w.LayoutVersion {
			return ErrLayoutVersionConflict
		}
		newVersion = w.LayoutVersion + 1
		fields := map[string]any{"layout_version": newVersion}
		if hallWidth > 0 && hallHeight > 0 {
			fields["hall_width"] = hallWidth
			fields["hall_height"] = hallHeight
		}
		if err := tx.Model(&models.WeddingEvent{}).Where("id = ?", weddingID).Updates(fields).Error; err != nil {
			return err
		}
		for _, t := range tables {
			res := tx.Model(&models.BanquetTable{}).Where("id = ? AND wedding_id = ?", t.ID, weddingID).
				Updates(map[string]any{
					"x": t.X, "y": t.Y, "degree": t.Degree,
					"capacity": t.Capacity, "is_vip": t.IsVip,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
			if err := checkCapacityCoversSeats(tx, weddingID, t.ID, t.Capacity); err != nil {
				return err
			}
		}
		for i := range toCreate {
			toCreate[i].WeddingID = weddingID
			if err := tx.Create(&toCreate[i]).Error; err != nil {
				return err
			}
		}
		for i := range toUpdate {
			res := tx.Model(&models.HallElement{}).Where("id = ? AND wedding_id = ?", toUpdate[i].ID, weddingID).
				Updates(map[string]any{
					"type": toUpdate[i].Type, "x": toUpdate[i].X, "y": toUpdate[i].Y,
					"degree": toUpdate[i].Degree, "width": toUpdate[i].Width, "height": toUpdate[i].Height,
					"name": toUpdate[i].Name, "color": toUpdate[i].Color,
					"text_color": toUpdate[i].TextColor, "stroke_color": toUpdate[i].StrokeColor,
					"opacity": toUpdate[i].Opacity, "z_index": toUpdate[i].ZIndex,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
			}
		}
		if len(toDelete) > 0 {
			if err := tx.Where("id IN ? AND wedding_id = ?", toDelete, weddingID).
				Delete(&models.HallElement{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return newVersion, nil
}
