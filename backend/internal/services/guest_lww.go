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
// time, so ties count as newer. A group with no stored time (a row that
// predates field_times) is seeded with updated_at before the merge.
func mergeGuestWrite(dst, src *models.GuestRecord, at time.Time, carried []string) []string {
	if dst.FieldTimes == nil {
		dst.FieldTimes = make(models.FieldTimes, len(guestGroups()))
	}
	// Seed every missing group, not only on a nil map: the updated_at bump
	// below must never become a group's fallback time.
	for _, key := range guestGroups() {
		if _, ok := dst.FieldTimes[key]; !ok {
			dst.FieldTimes[key] = dst.UpdatedAt
		}
	}
	won := make([]string, 0, len(carried))
	advanced := false
	for _, key := range carried {
		stored := fieldTime(dst, key)
		if at.Before(stored) {
			continue
		}
		// A tie is a replay of the stored write (invariant 5): it must not bump updated_at.
		advanced = advanced || stored.Before(at)
		copyGuestGroup(dst, src, key)
		dst.FieldTimes[key] = at
		won = append(won, key)
	}
	// updated_at is the client's staleness watermark, so it must move forward on
	// every applied write: an older offline edit winning one group would
	// otherwise leave it unchanged and let a stale response tie with this one.
	// LWW reads only field_times once they exist, so the bump never decides a merge.
	if advanced {
		dst.UpdatedAt = latest(dst.UpdatedAt, at, time.Now())
	}
	return won
}

func latest(ts ...time.Time) time.Time {
	var out time.Time
	for _, t := range ts {
		if t.After(out) {
			out = t
		}
	}
	return out
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
// column write on the row. Ties count as newer. Once field_times exist they are
// the write times; updated_at is then server time and would beat any offline delete.
func guestDeleteWins(g *models.GuestRecord, at time.Time) bool {
	newest := g.UpdatedAt
	if len(g.FieldTimes) > 0 {
		newest = time.Time{}
		for _, t := range g.FieldTimes {
			newest = latest(newest, t)
		}
	}
	return !at.Before(newest)
}
