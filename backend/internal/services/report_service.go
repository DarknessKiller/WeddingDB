package services

import (
	"bytes"
	"cmp"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"weddingdb/internal/models"
	"weddingdb/internal/repository"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
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

type angpaoSummary struct {
	TotalAngbao int
	// TotalByTable is keyed by table ID: two tables may share a name.
	TotalByTable    map[uuid.UUID]int
	TotalGuests     int
	CheckedInGuests int
	GiftItems       []string
}

func (s *ReportService) GenerateCSV(ctx context.Context, weddingID uuid.UUID) ([]byte, string, error) {
	wedding, err := s.findWedding(ctx, weddingID)
	if err != nil {
		return nil, "", err
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

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Header
	w.Write([]string{"Guest Name", "Table", "Seat", "Angbao (RM)", "Gift Item", "Checked In"})

	// Data rows
	for _, r := range rows {
		w.Write([]string{csvSafe(r.GuestName), csvSafe(r.TableName), r.SeatStr, r.AngbaoStr, csvSafe(r.GiftItem), r.CheckedIn})
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
	for _, tid := range s.sortedTableKeys(summary.TotalByTable, tableMap) {
		w.Write([]string{csvSafe(tableMap[tid]), fmt.Sprintf("RM %d", summary.TotalByTable[tid]), "", "", "", ""})
	}

	// Gift items
	if len(summary.GiftItems) > 0 {
		w.Write([]string{})
		w.Write([]string{"GIFT ITEMS", "", "", "", "", ""})
		for _, g := range summary.GiftItems {
			w.Write([]string{csvSafe(g), "", "", "", "", ""})
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("angpao-report-%s-%s.csv",
		s.sanitizeFilename(wedding.Name),
		time.Now().Format("2006-01-02"))

	return buf.Bytes(), filename, nil
}

func (s *ReportService) GenerateXLSX(ctx context.Context, weddingID uuid.UUID) ([]byte, string, error) {
	wedding, err := s.findWedding(ctx, weddingID)
	if err != nil {
		return nil, "", err
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
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("close xlsx report", "err", err)
		}
	}()
	// Cell writes only fail on a bad sheet name or coordinate; collect them and
	// fail once before writing instead of checking forty calls one by one.
	var errs []error

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
	errs = append(errs, f.SetSheetName("Sheet1", detailSheet))

	detailHeaders := []string{"Guest Name", "Table", "Seat", "Angbao (RM)", "Gift Item", "Checked In"}
	for i, h := range detailHeaders {
		errs = append(errs, f.SetCellValue(detailSheet, cellName(i+1, 1), h))
	}
	for i := range detailHeaders {
		cell := cellName(i+1, 1)
		errs = append(errs, f.SetCellStyle(detailSheet, cell, cell, headerStyle))
	}

	for ri, r := range rows {
		rowNum := ri + 2
		errs = append(errs, f.SetCellValue(detailSheet, cellName(1, rowNum), r.GuestName))
		errs = append(errs, f.SetCellValue(detailSheet, cellName(2, rowNum), r.TableName))
		if r.SeatNum > 0 {
			errs = append(errs, f.SetCellValue(detailSheet, cellName(3, rowNum), r.SeatNum))
		} else {
			errs = append(errs, f.SetCellValue(detailSheet, cellName(3, rowNum), "-"))
		}
		if r.AngbaoAmt > 0 {
			errs = append(errs, f.SetCellValue(detailSheet, cellName(4, rowNum), r.AngbaoAmt))
			errs = append(errs, f.SetCellStyle(detailSheet, cellName(4, rowNum), cellName(4, rowNum), currencyStyle))
		} else {
			errs = append(errs, f.SetCellValue(detailSheet, cellName(4, rowNum), "-"))
		}
		errs = append(errs, f.SetCellValue(detailSheet, cellName(5, rowNum), r.GiftItem))
		errs = append(errs, f.SetCellValue(detailSheet, cellName(6, rowNum), r.CheckedIn))
	}

	errs = append(errs, f.SetColWidth(detailSheet, "A", "A", 30))
	errs = append(errs, f.SetColWidth(detailSheet, "B", "B", 15))
	errs = append(errs, f.SetColWidth(detailSheet, "C", "C", 8))
	errs = append(errs, f.SetColWidth(detailSheet, "D", "D", 15))
	errs = append(errs, f.SetColWidth(detailSheet, "E", "E", 30))
	errs = append(errs, f.SetColWidth(detailSheet, "F", "F", 22))

	// ── Sheet 2: Summary ──
	summarySheet := "Summary"
	if _, err := f.NewSheet(summarySheet); err != nil {
		return nil, "", fmt.Errorf("summary sheet: %w", err)
	}

	errs = append(errs, f.SetCellValue(summarySheet, "A1", "Angpao Report Summary"))
	errs = append(errs, f.SetCellStyle(summarySheet, "A1", "A1", summaryBold))

	errs = append(errs, f.SetCellValue(summarySheet, "A3", "Total Angbao (RM)"))
	errs = append(errs, f.SetCellValue(summarySheet, "B3", summary.TotalAngbao))
	errs = append(errs, f.SetCellStyle(summarySheet, "A3", "A3", summaryBold))
	errs = append(errs, f.SetCellStyle(summarySheet, "B3", "B3", currencyStyle))

	errs = append(errs, f.SetCellValue(summarySheet, "A4", "Total Guests"))
	errs = append(errs, f.SetCellValue(summarySheet, "B4", summary.TotalGuests))
	errs = append(errs, f.SetCellStyle(summarySheet, "A4", "A4", summaryBold))

	errs = append(errs, f.SetCellValue(summarySheet, "A5", "Checked In"))
	errs = append(errs, f.SetCellValue(summarySheet, "B5", summary.CheckedInGuests))
	errs = append(errs, f.SetCellStyle(summarySheet, "A5", "A5", summaryBold))

	// Per-table totals
	errs = append(errs, f.SetCellValue(summarySheet, "A7", "Table"))
	errs = append(errs, f.SetCellValue(summarySheet, "B7", "Angbao (RM)"))
	errs = append(errs, f.SetCellStyle(summarySheet, "A7", "B7", summaryHeader))

	rowNum := 8
	for _, tid := range s.sortedTableKeys(summary.TotalByTable, tableMap) {
		errs = append(errs, f.SetCellValue(summarySheet, cellName(1, rowNum), tableMap[tid]))
		errs = append(errs, f.SetCellValue(summarySheet, cellName(2, rowNum), summary.TotalByTable[tid]))
		errs = append(errs, f.SetCellStyle(summarySheet, cellName(2, rowNum), cellName(2, rowNum), currencyStyle))
		rowNum++
	}

	// Gift items
	if len(summary.GiftItems) > 0 {
		rowNum++
		errs = append(errs, f.SetCellValue(summarySheet, cellName(1, rowNum), "Gift Items"))
		errs = append(errs, f.SetCellStyle(summarySheet, cellName(1, rowNum), cellName(1, rowNum), summaryHeader))
		rowNum++
		for _, g := range summary.GiftItems {
			errs = append(errs, f.SetCellValue(summarySheet, cellName(1, rowNum), g))
			rowNum++
		}
	}

	errs = append(errs, f.SetColWidth(summarySheet, "A", "A", 30))
	errs = append(errs, f.SetColWidth(summarySheet, "B", "B", 15))

	if err := errors.Join(errs...); err != nil {
		return nil, "", fmt.Errorf("build xlsx: %w", err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, "", err
	}

	filename := fmt.Sprintf("report-%s-%s.xlsx",
		s.sanitizeFilename(wedding.Name),
		time.Now().Local().Format("2006-01-02"))

	return buf.Bytes(), filename, nil
}

// findWedding maps a missing wedding to ErrWeddingNotFound and keeps every
// other error for the caller to log.
func (s *ReportService) findWedding(ctx context.Context, id uuid.UUID) (*models.WeddingEvent, error) {
	wedding, err := s.weddingRepo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWeddingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load wedding %s: %w", id, err)
	}
	return wedding, nil
}

func (s *ReportService) buildRows(guests []models.GuestRecord, tableMap map[uuid.UUID]string) ([]angpaoRow, angpaoSummary) {
	rows := make([]angpaoRow, 0, len(guests))
	summary := angpaoSummary{
		TotalByTable: make(map[uuid.UUID]int),
	}

	for _, g := range guests {
		tableName := ""
		tableID := uuid.Nil // unassigned guests total under the empty name
		if g.TableID != nil {
			tableName = tableMap[*g.TableID]
			tableID = *g.TableID
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
			summary.TotalByTable[tableID] += *g.AngbaoAmt
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

	summary.TotalGuests = len(guests)
	return rows, summary
}

// sortedTableKeys orders table IDs by table name, then ID for equal names.
func (s *ReportService) sortedTableKeys(m map[uuid.UUID]int, names map[uuid.UUID]string) []uuid.UUID {
	return slices.SortedFunc(maps.Keys(m), func(a, b uuid.UUID) int {
		return cmp.Or(cmp.Compare(names[a], names[b]), cmp.Compare(a.String(), b.String()))
	})
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

func cellName(col, row int) string {
	name, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		// Callers pass 1-based slice indexes, which always resolve.
		panic(fmt.Sprintf("cellName(%d, %d): %v", col, row, err))
	}
	return name
}

// csvSafe neutralizes spreadsheet formulas in user-supplied cells. A name or
// gift starting with one of these characters executes when the export is
// opened in Excel or LibreOffice (CSV injection).
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}
