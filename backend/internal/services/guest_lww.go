package services

import (
	"time"

	"weddingdb/internal/models"
)

// Guest column groups. TableID and SeatNum share one group: a seat only means
// something with its table, so one device's table move never pairs with another
// device's seat.
const (
	groupName        = "name"
	groupEmail       = "email"
	groupPax         = "pax"
	groupTableID     = "tableId"
	groupRSVP        = "rsvp"
	groupCheckedInAt = "checkedInAt"
	groupNotes       = "notes"
	groupDietary     = "dietary"
	groupIsVip       = "isVip"
	groupAngbaoAmt   = "angbaoAmt"
	groupGiftItem    = "giftItem"
)

func guestGroups() []string {
	return []string{
		groupName,
		groupEmail,
		groupPax,
		groupTableID,
		groupRSVP,
		groupCheckedInAt,
		groupNotes,
		groupDietary,
		groupIsVip,
		groupAngbaoAmt,
		groupGiftItem,
	}
}

// mergeGuestWrite merges the carried groups of src into dst and returns the
// groups that won. dst must be the locked row; src supplies values for carried
// groups only. A write wins a group when its time is not older than the stored
// time, so ties count as newer. A row that predates field_times seeds every
// group with its updated_at before the first merge.
func mergeGuestWrite(dst, src *models.GuestRecord, at time.Time, carried []string) []string {
	if dst.FieldTimes == nil {
		dst.FieldTimes = make(models.FieldTimes, len(guestGroups()))
		for _, key := range guestGroups() {
			dst.FieldTimes[key] = dst.UpdatedAt
		}
	}
	won := make([]string, 0, len(carried))
	for _, key := range carried {
		if at.Before(fieldTime(dst, key)) {
			continue
		}
		copyGuestGroup(dst, src, key)
		dst.FieldTimes[key] = at
		won = append(won, key)
	}
	if at.After(dst.UpdatedAt) {
		dst.UpdatedAt = at
	}
	return won
}

// fieldTime returns the stored write time of a group, falling back to the row
// time for rows that have never stored one.
func fieldTime(g *models.GuestRecord, group string) time.Time {
	if t, ok := g.FieldTimes[group]; ok {
		return t
	}
	return g.UpdatedAt
}

func copyGuestGroup(dst, src *models.GuestRecord, group string) {
	switch group {
	case groupName:
		dst.Name = src.Name
		dst.NamePinyin = models.GenerateNamePinyin(src.Name)
	case groupEmail:
		dst.Email = src.Email
	case groupPax:
		dst.Pax = src.Pax
	case groupTableID:
		dst.TableID = src.TableID
		dst.SeatNum = src.SeatNum
		if dst.TableID == nil {
			dst.SeatNum = nil // a seat without a table is not a valid assignment
		}
	case groupRSVP:
		dst.RSVP = src.RSVP
	case groupCheckedInAt:
		dst.CheckedInAt = src.CheckedInAt
	case groupNotes:
		dst.Notes = src.Notes
	case groupDietary:
		dst.Dietary = src.Dietary
	case groupIsVip:
		dst.IsVip = src.IsVip
	case groupAngbaoAmt:
		dst.AngbaoAmt = src.AngbaoAmt
	case groupGiftItem:
		dst.GiftItem = src.GiftItem
	}
}

// guestDeleteWins reports whether a row-level delete at at is newer than every
// column write on the row. Ties count as newer.
func guestDeleteWins(g *models.GuestRecord, at time.Time) bool {
	newest := g.UpdatedAt
	for _, t := range g.FieldTimes {
		if t.After(newest) {
			newest = t
		}
	}
	return !at.Before(newest)
}
