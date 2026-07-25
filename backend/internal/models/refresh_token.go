ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	AdminID   uuid.UUID  `gorm:"type:uuid;index;not null"`
	WeddingID *uuid.UUID `gorm:"type:uuid"`
	Token     string     `gorm:"size:255;uniqueIndex;not null"`
