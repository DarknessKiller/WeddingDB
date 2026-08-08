package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"weddingdb/internal/services"
)

type ReportHandler struct {
	reportService *services.ReportService
}

func NewReportHandler(reportService *services.ReportService) *ReportHandler {
	return &ReportHandler{reportService: reportService}
}

// ExportAngpao handles GET /api/weddings/{wid}/reports/angpao?format=csv|xlsx
// Returns a file download with Content-Disposition header.
func (h *ReportHandler) ExportAngpao(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	wid, err := DecodeWID(rawPathParams{r: r})
	if err != nil {
		http.Error(w, "Invalid wedding ID", http.StatusBadRequest)
		return
	}

	format := r.URL.Query().Get("format")
	if format != "csv" && format != "xlsx" {
		format = "csv"
	}

	var data []byte
	var filename string

	switch format {
	case "xlsx":
		data, filename, err = h.reportService.GenerateXLSX(ctx, wid)
	default:
		data, filename, err = h.reportService.GenerateCSV(ctx, wid)
	}

	if err != nil {
		http.Error(w, "Failed to generate report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	// Sanitize filename: strip control chars, use RFC 5987 encoding for non-ASCII
	safeFilename := sanitizeContentDisposition(filename)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeFilename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// rawPathParams implements the PathParam interface for raw net/http handlers.
type rawPathParams struct {
	r *http.Request
}

func (p rawPathParams) PathParam(name string) string {
	return p.r.PathValue(name)
}

// sanitizeContentDisposition strips control characters from filenames
// to prevent HTTP header injection via Content-Disposition.
func sanitizeContentDisposition(name string) string {
	// Strip all control characters (CR, LF, NUL, etc.)
	cleaned := make([]rune, 0, len(name))
	for _, r := range name {
		if r >= 32 && r != 127 {
			cleaned = append(cleaned, r)
		}
	}
	name = string(cleaned)
	// Replace path separators
	name = strings.ReplaceAll(name, `/`, `-`)
	name = strings.ReplaceAll(name, `\`, `-`)
	name = strings.TrimSpace(name)
	if len(name) > 50 {
		name = name[:50]
	}
	if name == "" {
		name = "report"
	}
	return name
}
