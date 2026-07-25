package models

import (
	"time"

	"github.com/google/uuid"
)

type WeddingEvent struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
