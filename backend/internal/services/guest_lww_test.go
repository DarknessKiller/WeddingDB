package services

import (
	"encoding/json"
	"testing"
	"time"

	"weddingdb/internal/models"

	"github.com/google/uuid"
)

func TestMergeGuestWrite_NewerWinsPerGroup(t *testing.T) {
	base := time.Now().Add(-time.Hour)
	existing := &models.GuestRecord{Name: "Alice", Notes: "old", Pax: 1, UpdatedAt: base}
	next := *existing
	next.Notes = "queued note"

	won := mergeGuestWrite(existing, &next, base.Add(time.Minute), []string{groupNotes})
	if len(won) != 1 || won[0] != groupNotes {
		t.Fatalf("won = %v, want [notes]", won)
	}
	if existing.Notes != "queued note" {
		t.Fatalf("notes = %q, want queued note", existing.Notes)
	}
	if existing.FieldTimes[groupNotes] != base.Add(time.Minute) {
		t.Fatalf("notes time = %v", existing.FieldTimes[groupNotes])
	}
	if existing.FieldTimes[groupPax] != base {
		t.Fatalf("pax time = %v, want legacy seed %v", existing.FieldTimes[groupPax], base)
	}
}

func TestMergeGuestWrite_OlderLoses(t *testing.T) {
	base := time.Now()
	existing := &models.GuestRecord{Notes: "server", UpdatedAt: base}
	next := *existing
	next.Notes = "stale"

	if won := mergeGuestWrite(existing, &next, base.Add(-time.Minute), []string{groupNotes}); len(won) != 0 {
		t.Fatalf("won = %v, want none", won)
	}
	if existing.Notes != "server" {
		t.Fatalf("notes = %q, want server", existing.Notes)
	}
}

func TestMergeGuestWrite_TieWins(t *testing.T) {
	at := time.Now()
	existing := &models.GuestRecord{Notes: "server", UpdatedAt: at}
	next := *existing
	next.Notes = "tie"

	if won := mergeGuestWrite(existing, &next, at, []string{groupNotes}); len(won) != 1 {
		t.Fatalf("won = %v, want the tie to win", won)
	}
	if existing.Notes != "tie" {
		t.Fatalf("notes = %q, want tie", existing.Notes)
	}
}

func TestMergeGuestWrite_TableAndSeatWinTogether(t *testing.T) {
	base := time.Now()
	oldTable := uuid.New()
	oldSeat := 1
	existing := &models.GuestRecord{TableID: &oldTable, SeatNum: &oldSeat, UpdatedAt: base}
	next := *existing
	newTable := uuid.New()
	newSeat := 4
	next.TableID = &newTable
	next.SeatNum = &newSeat

	won := mergeGuestWrite(existing, &next, base.Add(-time.Minute), []string{groupTableID})
	if len(won) != 0 {
		t.Fatalf("won = %v, want none", won)
	}
	if existing.TableID == nil || *existing.TableID != oldTable || existing.SeatNum == nil || *existing.SeatNum != oldSeat {
		t.Fatalf("stale seat must not half-apply: table=%v seat=%v", existing.TableID, existing.SeatNum)
	}

	won = mergeGuestWrite(existing, &next, base.Add(time.Minute), []string{groupTableID})
	if len(won) != 1 {
		t.Fatalf("won = %v, want [tableId]", won)
	}
	if *existing.TableID != newTable || *existing.SeatNum != newSeat {
		t.Fatalf("table and seat must move together: table=%v seat=%v", existing.TableID, existing.SeatNum)
	}
}

func TestMergeGuestWrite_ClearsSeatWithTable(t *testing.T) {
	base := time.Now()
	tid := uuid.New()
	seat := 2
	existing := &models.GuestRecord{TableID: &tid, SeatNum: &seat, UpdatedAt: base}
	next := *existing
	next.TableID = nil
	next.SeatNum = nil

	mergeGuestWrite(existing, &next, base.Add(time.Minute), []string{groupTableID})
	if existing.TableID != nil || existing.SeatNum != nil {
		t.Fatalf("table=%v seat=%v, want both cleared", existing.TableID, existing.SeatNum)
	}
}

func TestGuestDeleteWins_RowLevel(t *testing.T) {
	base := time.Now()
	existing := &models.GuestRecord{
		UpdatedAt:  base,
		FieldTimes: models.FieldTimes{groupNotes: base.Add(time.Minute)},
	}
	if guestDeleteWins(existing, base.Add(30*time.Second)) {
		t.Fatal("delete older than a column write must lose")
	}
	if !guestDeleteWins(existing, base.Add(time.Minute)) {
		t.Fatal("delete equal to the newest write must win")
	}
}

func TestGuestPatch_CarriesOnlyPresentFields(t *testing.T) {
	var p GuestPatch
	if err := json.Unmarshal([]byte(`{"pax":3}`), &p); err != nil {
		t.Fatal(err)
	}
	dst := &models.GuestRecord{Name: "Alice", Notes: "keep", Pax: 1}
	carried, err := p.applyTo(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(carried) != 1 || carried[0] != groupPax {
		t.Fatalf("carried = %v, want [pax]", carried)
	}
	if dst.Name != "Alice" || dst.Notes != "keep" || dst.Pax != 3 {
		t.Fatalf("absent fields must be preserved: %+v", dst)
	}
}

func TestGuestPatch_NullClearsTableAndSeat(t *testing.T) {
	var p GuestPatch
	if err := json.Unmarshal([]byte(`{"tableId":null,"seatNum":null}`), &p); err != nil {
		t.Fatal(err)
	}
	tid := uuid.New()
	seat := 2
	dst := &models.GuestRecord{TableID: &tid, SeatNum: &seat}
	carried, err := p.applyTo(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(carried) != 1 || carried[0] != groupTableID {
		t.Fatalf("carried = %v, want [tableId]", carried)
	}
	if dst.TableID != nil || dst.SeatNum != nil {
		t.Fatalf("table=%v seat=%v, want both cleared", dst.TableID, dst.SeatNum)
	}
}

func TestGuestPatch_RejectsInvalidFields(t *testing.T) {
	cases := map[string]string{
		"empty name": `{"name":""}`,
		"bad pax":    `{"pax":0}`,
		"bad rsvp":   `{"rsvp":"maybe"}`,
		"bad table":  `{"tableId":"not-a-uuid"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var p GuestPatch
			if err := json.Unmarshal([]byte(body), &p); err != nil {
				t.Fatal(err)
			}
			if _, err := p.applyTo(&models.GuestRecord{}); err == nil {
				t.Fatal("want validation error")
			}
		})
	}
}
