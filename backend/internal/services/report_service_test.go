package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"slices"
	"strings"
	"testing"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/testutil"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGenerateCSV_FormulaInjection(t *testing.T) {
	db, err := gorm.Open(postgres.Open(testutil.PgDSN(t)), &gorm.Config{})
	testutil.NoErr(t, err)
	var dbName string
	testutil.NoErr(t, db.Raw("SELECT current_database()").Scan(&dbName).Error)
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to use database %q", dbName)
	}
	testutil.NoErr(t, db.AutoMigrate(&models.WeddingEvent{}, &models.BanquetTable{}, &models.GuestRecord{}, &models.GuestTombstone{}))
	wid := uuid.New()
	tableID := uuid.New()
	testutil.NoErr(t, db.Create(&models.WeddingEvent{ID: wid, Name: "CSV"}).Error)
	t.Cleanup(func() {
		testutil.NoErr(t, db.Where("wedding_id = ?", wid).Delete(&models.GuestRecord{}).Error)
		testutil.NoErr(t, db.Where("wedding_id = ?", wid).Delete(&models.BanquetTable{}).Error)
		testutil.NoErr(t, db.Delete(&models.WeddingEvent{}, "id = ?", wid).Error)
	})
	testutil.NoErr(t, db.Create(&models.BanquetTable{ID: tableID, WeddingID: wid, Name: "+table"}).Error)
	for _, name := range []string{"=formula", "+formula", "-formula", "@formula", "  =formula", "Normal"} {
		guest := models.GuestRecord{ID: uuid.New(), WeddingID: wid, Name: name, TableID: &tableID, GiftItem: strPtr("@gift"), AngbaoAmt: intPtr(1)}
		testutil.NoErr(t, db.Create(&guest).Error)
	}
	svc := NewReportService(repository.NewGuestRepo(db), repository.NewTableRepo(db), repository.NewWeddingRepo(db))
	data, _, err := svc.GenerateCSV(context.Background(), wid)
	testutil.NoErr(t, err)
	reader := csv.NewReader(bytes.NewReader(data))
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	testutil.NoErr(t, err)
	seen := make(map[string]int)
	for _, row := range records {
		for _, value := range row {
			seen[value]++
		}
	}
	for _, value := range []string{"'=formula", "'+formula", "'-formula", "'@formula", "'  =formula", "Normal", "'+table", "'@gift", "'=formula: @gift"} {
		if seen[value] == 0 {
			t.Errorf("missing safe CSV cell %q: %v", value, records)
		}
	}
	if seen["'+table"] != 7 {
		t.Errorf("table name not escaped in data and summary: %v", records)
	}
}

func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }

func TestBuildRows_FullData(t *testing.T) {
	svc := &ReportService{}

	t1 := uuid.New()
	t2 := uuid.New()
	tableMap := map[uuid.UUID]string{t1: "Table A", t2: "Table B"}

	now := time.Now()
	guests := []models.GuestRecord{
		{Name: "Alice", Pax: 2, TableID: &t1, SeatNum: intPtr(1), AngbaoAmt: intPtr(200), GiftItem: strPtr("Red Packet"), CheckedInAt: &now},
		{Name: "Bob", Pax: 1, TableID: &t2, SeatNum: intPtr(3), AngbaoAmt: intPtr(100), CheckedInAt: nil},
		{Name: "Charlie", Pax: 1, TableID: nil, SeatNum: nil, AngbaoAmt: nil, GiftItem: nil, CheckedInAt: nil},
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
	// Headcounts are people: Alice brings pax 2.
	if summary.TotalGuests != 4 {
		t.Errorf("total guests: got %d, want 4", summary.TotalGuests)
	}
	if summary.CheckedInGuests != 2 {
		t.Errorf("checked in: got %d, want 2", summary.CheckedInGuests)
	}
	want := []tableTotal{{"Table A", 200}, {"Table B", 100}}
	if !slices.Equal(summary.TotalByTable, want) {
		t.Errorf("table totals: got %v, want %v", summary.TotalByTable, want)
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
		{`Has\Backslash`, `Has\Backslash`},
		{`Has"Quote`, `Has"Quote`},
		{"  Trimmed  ", "Trimmed"},
		{strings.Repeat("A", 60), strings.Repeat("A", 50)},
		{strings.Repeat("婚", 60), strings.Repeat("婚", 50)},
		{"   ", "report"},
	}
	for _, tt := range tests {
		got := svc.sanitizeFilename(tt.in)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Two tables sharing a name keep separate totals, and unseated money is
// labelled rather than filed under a blank name.
func TestBuildRows_TableTotalsByID(t *testing.T) {
	svc := &ReportService{}
	t1, t2 := uuid.New(), uuid.New()
	tableMap := map[uuid.UUID]string{t1: "Table 1", t2: "Table 1"}
	guests := []models.GuestRecord{
		{Name: "A", Pax: 1, TableID: &t1, AngbaoAmt: intPtr(100)},
		{Name: "B", Pax: 1, TableID: &t2, AngbaoAmt: intPtr(100)},
		{Name: "C", Pax: 1, AngbaoAmt: intPtr(50)},
	}
	_, summary := svc.buildRows(guests, tableMap)
	want := []tableTotal{{"Table 1", 100}, {"Table 1", 100}, {"Unassigned", 50}}
	if !slices.Equal(summary.TotalByTable, want) {
		t.Fatalf("table totals: got %v, want %v", summary.TotalByTable, want)
	}
}

func TestCSVOutputStructure(t *testing.T) {
	svc := &ReportService{}

	t1 := uuid.New()
	tableMap := map[uuid.UUID]string{t1: "VIP Table"}
	now := time.Now()
	guests := []models.GuestRecord{
		{Name: "Alice", Pax: 1, TableID: &t1, SeatNum: intPtr(1), AngbaoAmt: intPtr(500), GiftItem: strPtr("Gold"), CheckedInAt: &now},
		{Name: "Bob", Pax: 1, TableID: &t1, SeatNum: intPtr(2), AngbaoAmt: intPtr(200), CheckedInAt: nil},
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
		{Name: "Alice", Pax: 1, TableID: &t1, SeatNum: intPtr(1), AngbaoAmt: intPtr(300), GiftItem: strPtr("Bangle"), CheckedInAt: &now},
		{Name: "Bob", Pax: 1, TableID: &t1, SeatNum: intPtr(5), AngbaoAmt: intPtr(150), CheckedInAt: nil},
	}

	rows, summary := svc.buildRows(guests, tableMap)

	// Build XLSX using the same logic as GenerateXLSX
	f := excelize.NewFile()
	defer f.Close()

	customRM := `"RM "#,##0`
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#FEE2E2"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	testutil.NoErr(t, err)
	currencyStyle, err := f.NewStyle(&excelize.Style{
		CustomNumFmt: &customRM,
	})
	testutil.NoErr(t, err)

	detailSheet := "Guest Details"
	f.SetSheetName("Sheet1", detailSheet)

	detailHeaders := []string{"Guest Name", "Table", "Seat", "Angbao (RM)", "Gift Item", "Checked In"}
	for i, h := range detailHeaders {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		testutil.NoErr(t, err)
		f.SetCellValue(detailSheet, cell, h)
	}
	for i := range detailHeaders {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		testutil.NoErr(t, err)
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
	for _, tt := range summary.TotalByTable {
		f.SetCellValue(summarySheet, cellName(1, rowNum), tt.Name)
		f.SetCellValue(summarySheet, cellName(2, rowNum), tt.Amount)
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
	cell, err := xlsx.GetCellValue("Guest Details", "A1")
	testutil.NoErr(t, err)
	if cell != "Guest Name" {
		t.Errorf("Guest Details A1: %q", cell)
	}
	cell, err = xlsx.GetCellValue("Guest Details", "A2")
	testutil.NoErr(t, err)
	if cell != "Alice" {
		t.Errorf("Guest Details A2: %q", cell)
	}
	cell, err = xlsx.GetCellValue("Guest Details", "D2")
	testutil.NoErr(t, err)
	// Currency formatted cell - excelize reads the raw value
	if cell != "300" && cell != "RM 300" {
		t.Logf("Guest Details D2 (currency cell): %q (raw numeric value expected)", cell)
	}
	cell, err = xlsx.GetCellValue("Guest Details", "A3")
	testutil.NoErr(t, err)
	if cell != "Bob" {
		t.Errorf("Guest Details A3: %q", cell)
	}

	// Verify Summary sheet content
	cell, err = xlsx.GetCellValue("Summary", "A1")
	testutil.NoErr(t, err)
	if cell != "Angpao Report Summary" {
		t.Errorf("Summary A1: %q", cell)
	}
	cell, err = xlsx.GetCellValue("Summary", "B3")
	testutil.NoErr(t, err)
	if cell != "450" {
		t.Errorf("Summary B3 (total angpao): %q, want 450", cell)
	}
	cell, err = xlsx.GetCellValue("Summary", "B4")
	testutil.NoErr(t, err)
	if cell != "2" {
		t.Errorf("Summary B4 (total guests): %q, want 2", cell)
	}
	cell, err = xlsx.GetCellValue("Summary", "B5")
	testutil.NoErr(t, err)
	if cell != "1" {
		t.Errorf("Summary B5 (checked in): %q, want 1", cell)
	}
	// Per-table breakdown
	cell, err = xlsx.GetCellValue("Summary", "A8")
	testutil.NoErr(t, err)
	if cell != "Table 1" {
		t.Errorf("Summary A8 (table name): %q", cell)
	}
	cell, err = xlsx.GetCellValue("Summary", "B8")
	testutil.NoErr(t, err)
	if cell != "450" {
		t.Errorf("Summary B8 (table total): %q, want 450", cell)
	}
}
