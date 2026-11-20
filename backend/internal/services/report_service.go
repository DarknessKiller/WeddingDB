package services

import (
	"bytes"
	"cmp"
	"context"
	"encoding/csv"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

type ReportService struct {
	guestRepo   *repository.GuestRepo
	tableRepo   *repository.TableRepo
	weddingRepo *repository.WeddingRepo
}

func NewReportService(
	guestRepo *repository.GuestRepo,
	tableRepo *repository.TableRepo,
	weddingRepo *repository.WeddingRepo,
) *ReportService {
	return &ReportService{
		guestRepo:   guestRepo,
		tableRepo:   tableRepo,
		weddingRepo: weddingRepo,
	}
}

type angpaoRow struct {
	GuestName string
	TableName string
	SeatNum   int
	SeatStr   string
	AngbaoAmt int
	AngbaoStr string
	GiftItem  string
	CheckedIn string
}

type tableTotal struct {
	Name   string
	Amount int
}

type angpaoSummary struct {
	TotalAngbao     int
	TotalByTable    []tableTotal // sorted by name; keyed by table ID so same-named tables stay apart
	TotalGuests     int
	CheckedInGuests int
	GiftItems       []string
}

func (s *ReportService) GenerateCSV(ctx context.Context, weddingID uuid.UUID) ([]byte, string, error) {
	wedding, err := s.weddingRepo.FindByID(ctx, weddingID)
	if err != nil {
		return nil, "", fmt.Errorf("wedding not found")
	}

	guests, err := s.guestRepo.ListAllByWedding(ctx, weddingID)
	if err != nil {
		return nil, "", err
	}

	tables, err := s.tableRepo.ListByWedding(ctx, weddingID)
	if err != nil {
		return nil, "", err
	}

	tableMap := make(map[uuid.UUID]string)
	for _, t := range tables {
		tableMap[t.ID] = t.Name
	}

	rows, summary := s.buildRows(guests, tableMap)
	data, err := s.writeCSV(rows, summary)
	if err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("angpao-report-%s-%s.csv",
		s.sanitizeFilename(wedding.Name),
		time.Now().Format("2006-01-02"))

	return data, filename, nil
}

// writeCSV renders the report rows and summary as CSV. User-entered text goes
// through csvText so the file is safe to open in a spreadsheet app.
func (s *ReportService) writeCSV(rows []angpaoRow, summary angpaoSummary) ([]byte, error) {
	var buf bytes.Buffer
	// UTF-8 BOM so Excel decodes Chinese names instead of guessing a codepage.
	buf.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(&buf)

	// Header
	w.Write([]string{"Guest Name", "Table", "Seat", "Angbao (RM)", "Gift Item", "Checked In"})

	// Data rows
	for _, r := range rows {
		w.Write([]string{csvText(r.GuestName), csvText(r.TableName), r.SeatStr, r.AngbaoStr, csvText(r.GiftItem), r.CheckedIn})
	}

	// Blank row separator
	w.Write([]string{})

	// Summary
	w.Write([]string{"SUMMARY", "", "", "", "", ""})
	w.Write([]string{"Total Angbao", fmt.Sprintf("RM %d", summary.TotalAngbao), "", "", "", ""})
	w.Write([]string{"Total Guests", strconv.Itoa(summary.TotalGuests), "", "", "", ""})
	w.Write([]string{"Checked In", strconv.Itoa(summary.CheckedInGuests), "", "", "", ""})
	w.Write([]string{})

	// Per-table totals
	w.Write([]string{"TABLE TOTALS", "Angbao (RM)", "", "", "", ""})
	for _, t := range summary.TotalByTable {
		w.Write([]string{csvText(t.Name), fmt.Sprintf("RM %d", t.Amount), "", "", "", ""})
	}

	// Gift items
	if len(summary.GiftItems) > 0 {
		w.Write([]string{})
		w.Write([]string{"GIFT ITEMS", "", "", "", "", ""})
		for _, g := range summary.GiftItems {
			w.Write([]string{csvText(g), "", "", "", "", ""})
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *ReportService) GenerateXLSX(ctx context.Context, weddingID uuid.UUID) ([]byte, string, error) {
	wedding, err := s.weddingRepo.FindByID(ctx, weddingID)
	if err != nil {
		return nil, "", fmt.Errorf("wedding not found")
	}

	guests, err := s.guestRepo.ListAllByWedding(ctx, weddingID)
	if err != nil {
		return nil, "", err
	}

	tables, err := s.tableRepo.ListByWedding(ctx, weddingID)
	if err != nil {
		return nil, "", err
	}

	tableMap := make(map[uuid.UUID]string)
	for _, t := range tables {
		tableMap[t.ID] = t.Name
	}

	rows, summary := s.buildRows(guests, tableMap)

	f := excelize.NewFile()
	defer f.Close()

	// Styles
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#FEE2E2"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return nil, "", fmt.Errorf("header style: %w", err)
	}
	summaryBold, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
	})
	if err != nil {
		return nil, "", fmt.Errorf("summary style: %w", err)
	}
	customRM := `"RM "#,##0`
	currencyStyle, err := f.NewStyle(&excelize.Style{
		CustomNumFmt: &customRM,
	})
	if err != nil {
		return nil, "", fmt.Errorf("currency style: %w", err)
	}
	summaryHeader, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#FEE2E2"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return nil, "", fmt.Errorf("summary header style: %w", err)
	}

	// ── Sheet 1: Guest Details ──
	detailSheet := "Guest Details"
	f.SetSheetName("Sheet1", detailSheet)

	detailHeaders := []string{"Guest Name", "Table", "Seat", "Angbao (RM)", "Gift Item", "Checked In"}
	for i, h := range detailHeaders {
		f.SetCellValue(detailSheet, cellName(i+1, 1), h)
	}
	for i := range detailHeaders {
		cell := cellName(i+1, 1)
		f.SetCellStyle(detailSheet, cell, cell, headerStyle)
	}

	for ri, r := range rows {
		rowNum := ri + 2
		f.SetCellValue(detailSheet, cellName(1, rowNum), r.GuestName)
		f.SetCellValue(detailSheet, cellName(2, rowNum), r.TableName)
		if r.SeatNum > 0 {
			f.SetCellValue(detailSheet, cellName(3, rowNum), r.SeatNum)
		} else {
			f.SetCellValue(detailSheet, cellName(3, rowNum), "-")
		}
		if r.AngbaoAmt > 0 {
			f.SetCellValue(detailSheet, cellName(4, rowNum), r.AngbaoAmt)
			f.SetCellStyle(detailSheet, cellName(4, rowNum), cellName(4, rowNum), currencyStyle)
		} else {
			f.SetCellValue(detailSheet, cellName(4, rowNum), "-")
		}
		f.SetCellValue(detailSheet, cellName(5, rowNum), r.GiftItem)
		f.SetCellValue(detailSheet, cellName(6, rowNum), r.CheckedIn)
	}

	f.SetColWidth(detailSheet, "A", "A", 30)
	f.SetColWidth(detailSheet, "B", "B", 15)
	f.SetColWidth(detailSheet, "C", "C", 8)
	f.SetColWidth(detailSheet, "D", "D", 15)
	f.SetColWidth(detailSheet, "E", "E", 30)
	f.SetColWidth(detailSheet, "F", "F", 22)

	// ── Sheet 2: Summary ──
	summarySheet := "Summary"
	f.NewSheet(summarySheet)

	f.SetCellValue(summarySheet, "A1", "Angpao Report Summary")
	f.SetCellStyle(summarySheet, "A1", "A1", summaryBold)

	f.SetCellValue(summarySheet, "A3", "Total Angbao (RM)")
	f.SetCellValue(summarySheet, "B3", summary.TotalAngbao)
	f.SetCellStyle(summarySheet, "A3", "A3", summaryBold)
	f.SetCellStyle(summarySheet, "B3", "B3", currencyStyle)

	f.SetCellValue(summarySheet, "A4", "Total Guests")
	f.SetCellValue(summarySheet, "B4", summary.TotalGuests)
	f.SetCellStyle(summarySheet, "A4", "A4", summaryBold)

	f.SetCellValue(summarySheet, "A5", "Checked In")
	f.SetCellValue(summarySheet, "B5", summary.CheckedInGuests)
	f.SetCellStyle(summarySheet, "A5", "A5", summaryBold)

	// Per-table totals
	f.SetCellValue(summarySheet, "A7", "Table")
	f.SetCellValue(summarySheet, "B7", "Angbao (RM)")
	f.SetCellStyle(summarySheet, "A7", "B7", summaryHeader)

	rowNum := 8
	for _, t := range summary.TotalByTable {
		f.SetCellValue(summarySheet, cellName(1, rowNum), t.Name)
		f.SetCellValue(summarySheet, cellName(2, rowNum), t.Amount)
		f.SetCellStyle(summarySheet, cellName(2, rowNum), cellName(2, rowNum), currencyStyle)
		rowNum++
	}

	// Gift items
	if len(summary.GiftItems) > 0 {
		rowNum++
		f.SetCellValue(summarySheet, cellName(1, rowNum), "Gift Items")
		f.SetCellStyle(summarySheet, cellName(1, rowNum), cellName(1, rowNum), summaryHeader)
		rowNum++
		for _, g := range summary.GiftItems {
			f.SetCellValue(summarySheet, cellName(1, rowNum), g)
			rowNum++
		}
	}

	f.SetColWidth(summarySheet, "A", "A", 30)
	f.SetColWidth(summarySheet, "B", "B", 15)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("report-%s-%s.xlsx",
		s.sanitizeFilename(wedding.Name),
		time.Now().Local().Format("2006-01-02"))

	return buf.Bytes(), filename, nil
}

func (s *ReportService) buildRows(guests []models.GuestRecord, tableMap map[uuid.UUID]string) ([]angpaoRow, angpaoSummary) {
	rows := make([]angpaoRow, 0, len(guests))
	summary := angpaoSummary{}
	byTable := make(map[uuid.UUID]int) // uuid.Nil = unassigned

	for _, g := range guests {
		tableName := ""
		tableID := uuid.Nil
		if g.TableID != nil {
			tableID = *g.TableID
			tableName = tableMap[tableID]
		}

		seatNum := 0
		seatStr := "-"
		if g.SeatNum != nil {
			seatNum = *g.SeatNum
			seatStr = strconv.Itoa(*g.SeatNum)
		}

		angbaoAmt := 0
		angbaoStr := "-"
		if g.AngbaoAmt != nil {
			angbaoAmt = *g.AngbaoAmt
			angbaoStr = strconv.Itoa(*g.AngbaoAmt)
			summary.TotalAngbao += *g.AngbaoAmt
			byTable[tableID] += *g.AngbaoAmt
		}

		giftStr := "-"
		if g.GiftItem != nil && *g.GiftItem != "" {
			giftStr = *g.GiftItem
			summary.GiftItems = append(summary.GiftItems, fmt.Sprintf("%s: %s", g.Name, giftStr))
		}

		checkedInStr := "No"
		if g.CheckedInAt != nil {
			checkedInStr = "Yes (" + g.CheckedInAt.Local().Format("2006-01-02 15:04") + ")"
			summary.CheckedInGuests++
		}

		rows = append(rows, angpaoRow{
			GuestName: g.Name,
			TableName: tableName,
			SeatNum:   seatNum,
			SeatStr:   seatStr,
			AngbaoAmt: angbaoAmt,
			AngbaoStr: angbaoStr,
			GiftItem:  giftStr,
			CheckedIn: checkedInStr,
		})
	}

	for id, amt := range byTable {
		name := tableMap[id]
		if id == uuid.Nil {
			name = "Unassigned"
		}
		summary.TotalByTable = append(summary.TotalByTable, tableTotal{Name: name, Amount: amt})
	}
	slices.SortFunc(summary.TotalByTable, func(a, b tableTotal) int { return cmp.Compare(a.Name, b.Name) })

	summary.TotalGuests = len(guests)
	return rows, summary
}

func (s *ReportService) sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, `\`, `\\`)
	name = strings.ReplaceAll(name, `"`, `\"`)
	name = strings.ReplaceAll(name, `/`, `-`)
	name = strings.ReplaceAll(name, `\n`, `-`)
	name = strings.TrimSpace(name)
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

// csvText neutralises user-entered text for CSV export. Spreadsheet apps run
// a cell that starts with = + - @ tab or CR as a formula, so such values get a
// leading apostrophe, which the apps hide and treat as "this is text".
func csvText(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func cellName(col, row int) string {
	name, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		// Callers pass 1-based slice indexes, which always resolve.
		panic(fmt.Sprintf("cellName(%d, %d): %v", col, row, err))
	}
	return name
}
