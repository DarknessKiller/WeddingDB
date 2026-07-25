return map[string]any{"guests": guests, "total": total}, nil
}

func (h *GuestHandler) Get(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	guest, err := h.guestService.Get(id, wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Guest not found"}
	}
	return guest, nil
}

func (h *GuestHandler) Create(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if body.Name == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if body.Pax < 1 {
		return nil, fuego.BadRequestError{Title: "Pax must be at least 1"}
	}
	validRSVP := map[string]bool{"confirmed": true, "pending": true, "declined": true, "no_response": true}
	if body.RSVP != "" && !validRSVP[body.RSVP] {
		return nil, fuego.BadRequestError{Title: "Invalid RSVP status"}
	}
	wid := DecodeWID(c)
	guest := &models.GuestRecord{
		WeddingID: wid,
		Name:      body.Name,
		Phone:     body.Phone,
		Email:     body.Email,
		Pax:       body.Pax,
		RSVP:      body.RSVP,
		IsVip:     body.IsVip,
		Notes:     body.Notes,
		Dietary:   body.Dietary,
	}
	if err := h.guestService.Create(guest); err != nil {
		return nil, err
	}
	if body.TableID != nil && *body.TableID != "" {
		if tid, err := uuid.Parse(*body.TableID); err == nil {
			seatNum := 1
			if body.SeatNum != nil {
				seatNum = *body.SeatNum
			}
			if err := h.guestService.AssignSeat(guest.ID, wid, tid, seatNum); err == nil {
				guest.TableID = &tid
				guest.SeatNum = &seatNum
			}
		}
	}
	return guest, nil
}

func (h *GuestHandler) Update(c fuego.ContextWithBody[GuestCreateRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if body.Name == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if body.Pax < 1 {
		return nil, fuego.BadRequestError{Title: "Pax must be at least 1"}
	}
	validRSVP := map[string]bool{"confirmed": true, "pending": true, "declined": true, "no_response": true}
	if body.RSVP != "" && !validRSVP[body.RSVP] {
		return nil, fuego.BadRequestError{Title: "Invalid RSVP status"}
	}
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	guest, err := h.guestService.Get(id, wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Guest not found"}
	}
	guest.Name = body.Name
	guest.Phone = body.Phone
	guest.Email = body.Email
	guest.Pax = body.Pax
	guest.RSVP = body.RSVP
	guest.IsVip = body.IsVip
	guest.Notes = body.Notes
	guest.Dietary = body.Dietary
	guest.AngbaoAmt = body.AngbaoAmt
	guest.GiftItem = body.GiftItem
	if body.TableID != nil && *body.TableID != "" {
		if tid, err := uuid.Parse(*body.TableID); err == nil {
			seatNum := 1
			if body.SeatNum != nil {
				seatNum = *body.SeatNum
			}
			if err := h.guestService.AssignSeat(guest.ID, wid, tid, seatNum); err == nil {
				guest.TableID = &tid
				guest.SeatNum = &seatNum
			}
		}
	} else if body.TableID != nil && *body.TableID == "" {
		// Explicitly clearing seat assignment
		guest.TableID = nil
		guest.SeatNum = nil
	}
	// If body.TableID is nil (omitted), preserve existing seat
	if err := h.guestService.Update(guest); err != nil {
		return nil, err
	}
	return guest, nil
}

func (h *GuestHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	if err := h.guestService.Delete(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}

type CheckInRequest struct {
	AngbaoAmt *int    `json:"angbaoAmt"`
	GiftItem  *string `json:"giftItem"`
}

func (h *GuestHandler) CheckIn(c fuego.ContextWithBody[CheckInRequest]) (any, error) {
	body, _ := c.Body()
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	if body.AngbaoAmt != nil || body.GiftItem != nil {
		guest, err := h.guestService.Get(id, wid)
		if err != nil {
			return nil, fuego.NotFoundError{Title: "Guest not found"}
		}
		if body.AngbaoAmt != nil {
			guest.AngbaoAmt = body.AngbaoAmt
		}
		if body.GiftItem != nil {
			guest.GiftItem = body.GiftItem
		}
		if err := h.guestService.Update(guest); err != nil {
			return nil, err
		}
	}
	if err := h.guestService.CheckIn(id, wid); err != nil {
		return nil, err
	}
	guest, err := h.guestService.Get(id, wid)
	if err != nil {
		return nil, err
	}
	return guest, nil
}

func (h *GuestHandler) CheckOut(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	id := DecodeID(c.PathParam("id"))
	if err := h.guestService.CheckOut(id, wid); err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *GuestHandler) Search(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	query := c.QueryParam("q")
	guests, err := h.guestService.Search(wid, query)
	if err != nil {
		return nil, err
	}
	return guests, nil
}

func (h *GuestHandler) Occupancy(c fuego.ContextWithBody[any]) (any, error) {
	wid := DecodeWID(c)
	occ, err := h.guestService.Occupancy(wid)
	if err != nil {
		return nil, err
	}
	if occ == nil {
		occ = []repository.TableOccupancy{}
	}
	return occ, nil
}

type BulkImportRequest struct {
	Guests []GuestCreateRequest `json:"guests"`
}

func (h *GuestHandler) BulkImport(c fuego.ContextWithBody[BulkImportRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	if len(body.Guests) > 1000 {
		return nil, fuego.BadRequestError{Title: "Maximum 1000 guests per import"}
	}
	wid := DecodeWID(c)
	var guests []models.GuestRecord
	for _, g := range body.Guests {
		gr := models.GuestRecord{
			WeddingID: wid,
			Name:      g.Name,
			Phone:     g.Phone,
			Email:     g.Email,
			Pax:       g.Pax,
			RSVP:      g.RSVP,
			IsVip:     g.IsVip,
			Notes:     g.Notes,
			Dietary:   g.Dietary,
		}
		if g.TableID != nil {
			tid := DecodeID(*g.TableID)
			gr.TableID = &tid
		}
		if g.SeatNum != nil {
			gr.SeatNum = g.SeatNum
		}
		guests = append(guests, gr)
	}
	count, err := h.guestService.BulkCreate(guests)
	if err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to import guests"}
	}
	return map[string]any{"imported": count}, nil
}

type AssignSeatRequest struct {
	TableID string `json:"tableId"`
	SeatNum int    `json:"seatNum"`
}

func (h *GuestHandler) AssignSeat(c fuego.ContextWithBody[AssignSeatRequest]) (any, error) {
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	wid := DecodeWID(c)
	guestID := DecodeID(c.PathParam("id"))
	tableID := DecodeID(body.TableID)
	if err := h.guestService.AssignSeat(guestID, wid, tableID, body.SeatNum); err != nil {
		return nil, fuego.BadRequestError{Title: err.Error()}
	}
	return nil, nil
}
