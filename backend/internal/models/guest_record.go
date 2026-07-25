ID          uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	WeddingID   uuid.UUID    `gorm:"type:uuid;index;not null" json:"weddingId"`
	Name        string       `gorm:"size:255;not null" json:"name"`
	Phone       string       `gorm:"size:50" json:"phone"`
	Email       string       `gorm:"size:255" json:"email"`
	Pax         int          `gorm:"not null;default:1" json:"pax"`
	TableID     *uuid.UUID   `gorm:"type:uuid;index" json:"tableId"`
	SeatNum     *int         `json:"seatNum"`
	RSVP        string       `gorm:"size:20;default:no_response" json:"rsvp"`
	CheckedInAt *time.Time   `json:"checkedInAt"`
	Notes       string       `gorm:"type:text" json:"notes"`
	Dietary     StringSlice  `gorm:"type:text" json:"dietary"`
	IsVip       bool         `json:"isVip"`
	AngbaoAmt   *int         `json:"angbaoAmt"`
	GiftItem    *string      `gorm:"size:255" json:"giftItem"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
