package services

import (
	"encoding/json"
	"fmt"

	"weddingdb/internal/models"
)

// GuestPatch is one guest write. Only the fields present in the JSON body are
// carried: an absent field keeps its stored value, while an explicit null clears
// a nullable field. Online updates and offline sync mutations share this type so
// they follow the same rule.
type GuestPatch struct {
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Pax       int      `json:"pax"`
	RSVP      string   `json:"rsvp"`
	IsVip     bool     `json:"isVip"`
	Notes     string   `json:"notes"`
	Dietary   []string `json:"dietary"`
	TableID   *string  `json:"tableId"`
	SeatNum   *int     `json:"seatNum"`
	AngbaoAmt *int     `json:"angbaoAmt"`
	GiftItem  *string  `json:"giftItem"`

	set map[string]bool
}

// SyncPayload is the guest patch shape used by offline sync mutations.
type SyncPayload = GuestPatch

func (p *GuestPatch) UnmarshalJSON(data []byte) error {
	type plain GuestPatch
	var v plain
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*p = GuestPatch(v)
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return err
	}
	p.set = make(map[string]bool, len(keys))
	for k := range keys {
		p.set[k] = true
	}
	return nil
}

// carries reports whether the JSON body contained key. A patch built in Go
// rather than decoded carries every group, matching the old full-payload
// callers.
func (p *GuestPatch) carries(key string) bool {
	if p == nil {
		return false
	}
	return p.set == nil || p.set[key]
}

// applyTo copies the groups this patch carries onto dst and returns their keys.
// It validates field formats; cross-row rules (seat overlap, guest cap) are
// checked on the merged row.
func (p *GuestPatch) applyTo(dst *models.GuestRecord) ([]string, error) {
	carried := make([]string, 0, len(guestGroups()))
	if p.carries("name") {
		if p.Name == "" {
			return nil, fmt.Errorf("%w: name is required", ErrGuestPatchInvalid)
		}
		dst.Name = p.Name
		carried = append(carried, groupName)
	}
	if p.carries("email") {
		dst.Email = p.Email
		carried = append(carried, groupEmail)
	}
	if p.carries("pax") {
		if p.Pax < 1 {
			return nil, fmt.Errorf("%w: pax must be at least 1", ErrGuestPatchInvalid)
		}
		dst.Pax = p.Pax
		carried = append(carried, groupPax)
	}
	if p.carries("rsvp") {
		if !validRSVP(p.RSVP) {
			return nil, fmt.Errorf("%w: invalid RSVP status %q", ErrGuestPatchInvalid, p.RSVP)
		}
		dst.RSVP = p.RSVP
		carried = append(carried, groupRSVP)
	}
	if p.carries("isVip") {
		dst.IsVip = p.IsVip
		carried = append(carried, groupIsVip)
	}
	if p.carries("notes") {
		dst.Notes = p.Notes
		carried = append(carried, groupNotes)
	}
	if p.carries("dietary") {
		dst.Dietary = p.Dietary
		carried = append(carried, groupDietary)
	}
	if p.carries("angbaoAmt") {
		dst.AngbaoAmt = p.AngbaoAmt
		carried = append(carried, groupAngbaoAmt)
	}
	if p.carries("giftItem") {
		dst.GiftItem = p.GiftItem
		carried = append(carried, groupGiftItem)
	}
	if p.carries("tableId") {
		switch {
		case p.TableID == nil || *p.TableID == "":
			dst.TableID = nil
			dst.SeatNum = nil
		default:
			tid, err := parseSyncID(*p.TableID)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid tableId %q", ErrGuestPatchInvalid, *p.TableID)
			}
			dst.TableID = &tid
			dst.SeatNum = p.SeatNum
		}
		carried = append(carried, groupTableID)
	}
	return carried, nil
}

func validRSVP(rsvp string) bool {
	switch rsvp {
	case "confirmed", "pending", "declined", "no_response":
		return true
	}
	return false
}
