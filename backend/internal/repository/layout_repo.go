package repository

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"weddingdb/internal/models"
)

type LayoutRepo struct{ db *gorm.DB }

func NewLayoutRepo(db *gorm.DB) *LayoutRepo { return &LayoutRepo{db: db} }

func (r *LayoutRepo) ElementsByWedding(weddingID uuid.UUID) ([]models.HallElement, error) {
	els := make([]models.HallElement, 0)
	err := r.db.Where("wedding_id = ?", weddingID).Order("z_index").Find(&els).Error
	return els, err
}

func (r *LayoutRepo) SaveLayout(
	weddingID uuid.UUID,
	hallWidth, hallHeight int,
	tablePos map[uuid.UUID][3]float64,
	toCreate, toUpdate []models.HallElement,
	toDelete []uuid.UUID,
) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if hallWidth > 0 && hallHeight > 0 {
			if err := tx.Model(&models.WeddingEvent{}).Where("id = ?", weddingID).
				Updates(map[string]any{"hall_width": hallWidth, "hall_height": hallHeight}).Error; err != nil {
				return err
			}
		}
		for id, pos := range tablePos {
			res := tx.Model(&models.BanquetTable{}).Where("id = ? AND wedding_id = ?", id, weddingID).
				Updates(map[string]any{"x": pos[0], "y": pos[1], "degree": pos[2]})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return gorm.ErrRecordNotFound
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
}
