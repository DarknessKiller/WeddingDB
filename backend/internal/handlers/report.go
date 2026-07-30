package handlers

import (
	"fmt"
	"net/http"
	"strconv"

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
		data, filename, err = h.reportService.GenerateXLSX(wid)
	default:
		data, filename, err = h.reportService.GenerateCSV(wid)
	}

	if err != nil {
		http.Error(w, "Failed to generate report", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
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
