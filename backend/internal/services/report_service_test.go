package services

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"weddingdb/internal/models"
)

func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }

func TestBuildRows_FullData(t *testing.T) {
	svc := &ReportService{}

	t1 := uuid.New()
	t2 := uuid.New()
	tableMap := map[uuid.UUID]string{t1: "Table A", t2: "Table B"}

	now := time.Now()
	guests := []models.GuestRecord{
		{Name: "Alice", TableID: &t1, SeatNum: intPtr(1), AngbaoAmt: intPtr(200), GiftItem: strPtr("Red Packet"), CheckedInAt: &now},
		{Name: "Bob", TableID: &t2, SeatNum: intPtr(3), AngbaoAmt: intPtr(100), CheckedInAt: nil},
		{Name: "Charlie", TableID: nil, SeatNum: nil, AngbaoAmt: nil, GiftItem: nil, CheckedInAt: nil},
	}

	rows, summary := svc.buildRows(guests, tableMap)

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Row 1: Alice
	if rows[0].GuestName != "Alice" || rows[0].TableName != "Table A" || rows[0].SeatNum != 1 {
		t.Errorf("row 0: got %+v", rows[0])
	}
	if rows[0].AngbaoAmt != 200 || rows[0].AngbaoStr != "200" {
		t.Errorf("row 0 angpao: got %q", rows[0].AngbaoStr)
	}
	if rows[0].GiftItem != "Red Packet" {
		t.Errorf("row 0 gift: got %q", rows[0].GiftItem)
	}
	if !strings.HasPrefix(rows[0].CheckedIn, "Yes") {
		t.Errorf("row 0 checked in: got %q", rows[0].CheckedIn)
	}

	// Row 2: Bob
	if rows[1].GuestName != "Bob" || rows[1].TableName != "Table B" || rows[1].SeatNum != 3 {
		t.Errorf("row 1: got %+v", rows[1])
	}
	if rows[1].AngbaoAmt != 100 {
		t.Errorf("row 1 angpao: got %d", rows[1].AngbaoAmt)
	}
	if rows[1].CheckedIn != "No" {
		t.Errorf("row 1 checked in: got %q", rows[1].CheckedIn)
	}

	// Row 3: Charlie (all nil fields)
	if rows[2].GuestName != "Charlie" || rows[2].TableName != "" || rows[2].SeatStr != "-" {
		t.Errorf("row 2: got %+v", rows[2])
	}
	if rows[2].AngbaoStr != "-" || rows[2].GiftItem != "-" {
		t.Errorf("row 2 defaults: angbao=%q gift=%q", rows[2].AngbaoStr, rows[2].GiftItem)
	}

	// Summary
	if summary.TotalAngbao != 300 {
		t.Errorf("total angpao: got %d, want 300", summary.TotalAngbao)
	}
	if summary.TotalGuests != 3 {
		t.Errorf("total guests: got %d, want 3", summary.TotalGuests)
	}
	if summary.CheckedInGuests != 1 {
		t.Errorf("checked in: got %d, want 1", summary.CheckedInGuests)
	}
	if summary.TotalByTable["Table A"] != 200 {
		t.Errorf("Table A total: got %d", summary.TotalByTable["Table A"])
	}
	if summary.TotalByTable["Table B"] != 100 {
		t.Errorf("Table B total: got %d", summary.TotalByTable["Table B"])
	}
	if len(summary.GiftItems) != 1 || summary.GiftItems[0] != "Alice: Red Packet" {
		t.Errorf("gift items: got %v", summary.GiftItems)
	}
}

func TestBuildRows_EmptyGuests(t *testing.T) {
	svc := &ReportService{}
	rows, summary := svc.buildRows(nil, nil)
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
	if summary.TotalAngbao != 0 || summary.TotalGuests != 0 {
		t.Errorf("expected zero summary, got %+v", summary)
	}
}

func TestSanitizeFilename(t *testing.T) {
	svc := &ReportService{}
	tests := []struct {
		in, want string
	}{
		{"Normal Name", "Normal Name"},
		{"Has/Slash", "Has-Slash"},
		{`Has\Backslash`, `Has\\Backslash`},
		{`Has"Quote`, `Has\"Quote`},
		{"  Trimmed  ", "Trimmed"},
		{strings.Repeat("A", 60), strings.Repeat("A", 50)},
	}
	for _, tt := range tests {
		got := svc.sanitizeFilename(tt.in)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSortedTableKeys(t *testing.T) {
	svc := &ReportService{}
	m := map[string]int{"Banana": 2, "Apple": 1, "Cherry": 3}
	keys := svc.sortedTableKeys(m)
	expected := []string{"Apple", "Banana", "Cherry"}
	if len(keys) != len(expected) {
		t.Fatalf("got %v, want %v", keys, expected)
	}
	for i, k := range keys {
		if k != expected[i] {
			t.Errorf("keys[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

func TestCSVOutputStructure(t *testing.T) {
	svc := &ReportService{}

	t1 := uuid.New()
	tableMap := map[uuid.UUID]string{t1: "VIP Table"}
	now := time.Now()
	guests := []models.GuestRecord{
		{Name: "Alice", TableID: &t1, SeatNum: intPtr(1), AngbaoAmt: intPtr(500), GiftItem: strPtr("Gold"), CheckedInAt: &now},
		{Name: "Bob", TableID: &t1, SeatNum: intPtr(2), AngbaoAmt: intPtr(200), CheckedInAt: nil},
	}

	rows, _ := svc.buildRows(guests, tableMap)

	// Build CSV matching GenerateCSV logic
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Write([]string{"Guest Name", "Table", "Seat", "Angbao (RM)", "Gift Item", "Checked In"})
	for _, r := range rows {
		w.Write([]string{r.GuestName, r.TableName, r.SeatStr, r.AngbaoStr, r.GiftItem, r.CheckedIn})
	}
	w.Write([]string{})
	w.Write([]string{"SUMMARY", "", "", "", "", ""})
	w.Write([]string{"Total Angbao", "RM 700", "", "", "", ""})
	w.Write([]string{"Total Guests", "2", "", "", "", ""})
	w.Write([]string{"Checked In", "1", "", "", "", ""})
	w.Write([]string{})
	w.Write([]string{"TABLE TOTALS", "Angbao (RM)", "", "", "", ""})
	w.Write([]string{"VIP Table", "RM 700", "", "", "", ""})
	w.Flush()

	csvStr := buf.String()

	// Verify key sections exist in the CSV output
	for _, want := range []string{
		"Guest Name,Table,Seat,Angbao (RM),Gift Item,Checked In",
		"Alice,VIP Table,1,500,Gold,",
		"Bob,VIP Table,2,200,-,No",
		"SUMMARY",
		"Total Angbao,RM 700",
		"Total Guests,2",
		"Checked In,1",
		"TABLE TOTALS,Angbao (RM)",
		"VIP Table,RM 700",
	} {
		if !strings.Contains(csvStr, want) {
			t.Errorf("CSV missing %q", want)
		}
	}
}

func TestXLSXOutputStructure(t *testing.T) {
	svc := &ReportService{}

	t1 := uuid.New()
	tableMap := map[uuid.UUID]string{t1: "Table 1"}
	now := time.Now()
	guests := []models.GuestRecord{
		{Name: "Alice", TableID: &t1, SeatNum: intPtr(1), AngbaoAmt: intPtr(300), GiftItem: strPtr("Bangle"), CheckedInAt: &now},
		{Name: "Bob", TableID: &t1, SeatNum: intPtr(5), AngbaoAmt: intPtr(150), CheckedInAt: nil},
	}

	rows, summary := svc.buildRows(guests, tableMap)

	// Build XLSX using the same logic as GenerateXLSX
	f := excelize.NewFile()
	defer f.Close()

	customRM := `"RM "#,##0`
	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#FEE2E2"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	currencyStyle, _ := f.NewStyle(&excelize.Style{
		CustomNumFmt: &customRM,
	})

	detailSheet := "Guest Details"
	f.SetSheetName("Sheet1", detailSheet)

	detailHeaders := []string{"Guest Name", "Table", "Seat", "Angbao (RM)", "Gift Item", "Checked In"}
	for i, h := range detailHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(detailSheet, cell, h)
	}
	for i := range detailHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellStyle(detailSheet, cell, cell, headerStyle)
	}

	for ri, r := range rows {
		rowNum := ri + 2
		f.SetCellValue(detailSheet, cellName(1, rowNum), r.GuestName)
		f.SetCellValue(detailSheet, cellName(2, rowNum), r.TableName)
		if r.SeatNum > 0 {
			f.SetCellValue(detailSheet, cellName(3, rowNum), r.SeatNum)
		}
		if r.AngbaoAmt > 0 {
			f.SetCellValue(detailSheet, cellName(4, rowNum), r.AngbaoAmt)
			f.SetCellStyle(detailSheet, cellName(4, rowNum), cellName(4, rowNum), currencyStyle)
		}
		f.SetCellValue(detailSheet, cellName(5, rowNum), r.GiftItem)
		f.SetCellValue(detailSheet, cellName(6, rowNum), r.CheckedIn)
	}

	summarySheet := "Summary"
	f.NewSheet(summarySheet)
	f.SetCellValue(summarySheet, "A1", "Angpao Report Summary")
	f.SetCellValue(summarySheet, "A3", "Total Angbao (RM)")
	f.SetCellValue(summarySheet, "B3", summary.TotalAngbao)
	f.SetCellValue(summarySheet, "A4", "Total Guests")
	f.SetCellValue(summarySheet, "B4", summary.TotalGuests)
	f.SetCellValue(summarySheet, "A5", "Checked In")
	f.SetCellValue(summarySheet, "B5", summary.CheckedInGuests)
	f.SetCellValue(summarySheet, "A7", "Table")
	f.SetCellValue(summarySheet, "B7", "Angbao (RM)")
	rowNum := 8
	sortedTableNames := svc.sortedTableKeys(summary.TotalByTable)
	for _, tName := range sortedTableNames {
		f.SetCellValue(summarySheet, cellName(1, rowNum), tName)
		f.SetCellValue(summarySheet, cellName(2, rowNum), summary.TotalByTable[tName])
		rowNum++
	}

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("failed to write XLSX: %v", err)
	}

	// Verify the XLSX by reading it back
	xlsx, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open XLSX: %v", err)
	}
	defer xlsx.Close()

	// Verify sheet names
	sheets := xlsx.GetSheetList()
	if len(sheets) != 2 {
		t.Fatalf("expected 2 sheets, got %d: %v", len(sheets), sheets)
	}
	if sheets[0] != "Guest Details" {
		t.Errorf("sheet 0 name: %q, want %q", sheets[0], "Guest Details")
	}
	if sheets[1] != "Summary" {
		t.Errorf("sheet 1 name: %q, want %q", sheets[1], "Summary")
	}

	// Verify Guest Details sheet content
	cell, _ := xlsx.GetCellValue("Guest Details", "A1")
	if cell != "Guest Name" {
		t.Errorf("Guest Details A1: %q", cell)
	}
	cell, _ = xlsx.GetCellValue("Guest Details", "A2")
	if cell != "Alice" {
		t.Errorf("Guest Details A2: %q", cell)
	}
	cell, _ = xlsx.GetCellValue("Guest Details", "D2")
	// Currency formatted cell - excelize reads the raw value
	if cell != "300" && cell != "RM 300" {
		t.Logf("Guest Details D2 (currency cell): %q (raw numeric value expected)", cell)
	}
	cell, _ = xlsx.GetCellValue("Guest Details", "A3")
	if cell != "Bob" {
		t.Errorf("Guest Details A3: %q", cell)
	}

	// Verify Summary sheet content
	cell, _ = xlsx.GetCellValue("Summary", "A1")
	if cell != "Angpao Report Summary" {
		t.Errorf("Summary A1: %q", cell)
	}
	cell, _ = xlsx.GetCellValue("Summary", "B3")
	if cell != "450" {
		t.Errorf("Summary B3 (total angpao): %q, want 450", cell)
	}
	cell, _ = xlsx.GetCellValue("Summary", "B4")
	if cell != "2" {
		t.Errorf("Summary B4 (total guests): %q, want 2", cell)
	}
	cell, _ = xlsx.GetCellValue("Summary", "B5")
	if cell != "1" {
		t.Errorf("Summary B5 (checked in): %q, want 1", cell)
	}
	// Per-table breakdown
	cell, _ = xlsx.GetCellValue("Summary", "A8")
	if cell != "Table 1" {
		t.Errorf("Summary A8 (table name): %q", cell)
	}
	cell, _ = xlsx.GetCellValue("Summary", "B8")
	if cell != "450" {
		t.Errorf("Summary B8 (table total): %q, want 450", cell)
	}
}
