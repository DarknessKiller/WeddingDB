package handlers

import (
	"errors"
	"log/slog"
	"mime"
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

	if errors.Is(err, services.ErrWeddingNotFound) {
		http.Error(w, "Wedding not found", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "generate report failed", "wid", wid, "format", format, "err", err)
		http.Error(w, "Failed to generate report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	// Sanitize filename: strip control chars, use RFC 5987 encoding for non-ASCII
	safeFilename := sanitizeContentDisposition(filename)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": safeFilename}))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		slog.Warn("report body write failed", "err", err)
	}
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
	if name == "" {
		name = "report"
	}
	return name
}
