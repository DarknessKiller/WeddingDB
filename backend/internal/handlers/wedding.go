package handlers

import (
	"cmp"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"weddingdb/internal/models"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"

	"github.com/go-fuego/fuego"
	"github.com/google/uuid"
)

type WeddingHandler struct {
	weddingService *services.WeddingService
	adminRepo      *repository.AdminRepo
}

func NewWeddingHandler(weddingService *services.WeddingService, adminRepo *repository.AdminRepo) *WeddingHandler {
	return &WeddingHandler{weddingService: weddingService, adminRepo: adminRepo}
}

type WeddingRequest struct {
	Name string `json:"name"`
	Date string `json:"date"`
}

func (h *WeddingHandler) List(c fuego.ContextNoBody) (any, error) {
	ctx := c.Context()
	role := RoleFromContext(ctx)
	if role == "admin" {
		return h.weddingService.List(ctx)
	}
	// user: only return their own wedding from JWT
	wid := WeddingIDFromContext(ctx)
	if wid == nil {
		return []models.WeddingEvent{}, nil
	}
	w, err := h.weddingService.Get(ctx, *wid)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	return []models.WeddingEvent{*w}, nil
}

func (h *WeddingHandler) Get(c fuego.ContextNoBody) (any, error) {
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	ctx := c.Context()
	if err := requireWeddingAccess(ctx, h.adminRepo, id); err != nil {
		return nil, err
	}
	return h.weddingService.Get(ctx, id)
}

func (h *WeddingHandler) Create(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	d, err := time.Parse("2006-01-02", body.Date)
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid date format, use YYYY-MM-DD"}
	}
	w := &models.WeddingEvent{Name: strings.TrimSpace(body.Name), Date: d}
	if w.Name == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if err := maxLen("Name", w.Name, 255); err != nil {
		return nil, err
	}
	if err := h.weddingService.Create(ctx, w); err != nil {
		return nil, err
	}
	// Link the creating admin to this wedding
	adminID := AdminIDFromContext(c.Context())
	if adminID != uuid.Nil {
		if err := h.adminRepo.AddUserWedding(ctx, adminID, w.ID); err != nil {
			return nil, err
		}
	}
	c.SetStatus(201)
	return w, nil
}

func (h *WeddingHandler) Update(c fuego.ContextWithBody[WeddingRequest]) (any, error) {
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	ctx := c.Context()
	if err := requireWeddingAccess(ctx, h.adminRepo, id); err != nil {
		return nil, err
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	w, err := h.weddingService.Get(ctx, id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return nil, fuego.BadRequestError{Title: "Name is required"}
	}
	if err := maxLen("Name", name, 255); err != nil {
		return nil, err
	}
	w.Name = name
	if body.Date != "" {
		d, err := time.Parse("2006-01-02", body.Date)
		if err != nil {
			return nil, fuego.BadRequestError{Title: "Invalid date format, use YYYY-MM-DD"}
		}
		w.Date = d
	}
	if err := h.weddingService.UpdateBasic(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// KioskSettingsRequest uses pointers so an omitted field stays unchanged.
type KioskSettingsRequest struct {
	Name                  *string `json:"name"` // nil = unchanged (dirty-only from frontend)
	VenueName             *string `json:"venueName"`
	VenueAddress          *string `json:"venueAddress"`
	KioskDescription      *string `json:"kioskDescription"`
	KioskLogoUrl          *string `json:"kioskLogoUrl"`
	KioskBackgroundUrl    *string `json:"kioskBackgroundUrl"`
	KioskBackgroundBlur   *int    `json:"kioskBackgroundBlur"`
	KioskBackgroundSize   *string `json:"kioskBackgroundSize"`
	KioskBackgroundPosX   *string `json:"kioskBackgroundPosX"`
	KioskBackgroundPosY   *string `json:"kioskBackgroundPosY"`
	KioskLogoSize         *string `json:"kioskLogoSize"`
	KioskLogoPosX         *string `json:"kioskLogoPosX"`
	KioskLogoPosY         *string `json:"kioskLogoPosY"`
	ShowSeatNumbers       *bool   `json:"showSeatNumbers"`
	KioskIdleResetSeconds *int    `json:"kioskIdleResetSeconds"`
}

func (h *WeddingHandler) UpdateKioskSettings(c fuego.ContextWithBody[KioskSettingsRequest]) (any, error) {
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	ctx := c.Context()
	if err := requireWeddingAccess(ctx, h.adminRepo, id); err != nil {
		return nil, err
	}
	body, err := c.Body()
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid request"}
	}
	w, err := h.weddingService.Get(ctx, id)
	if err != nil {
		return nil, fuego.NotFoundError{Title: "Wedding not found"}
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" {
			return nil, fuego.BadRequestError{Title: "Name is required"}
		}
		w.Name = name
	}
	if body.VenueName != nil {
		w.VenueName = strings.TrimSpace(*body.VenueName)
	}
	if body.VenueAddress != nil {
		w.VenueAddress = strings.TrimSpace(*body.VenueAddress)
	}
	if body.KioskDescription != nil {
		w.KioskDescription = strings.TrimSpace(*body.KioskDescription)
	}
	if err := cmp.Or(maxLen("Name", w.Name, 255), maxLen("Venue name", w.VenueName, 255), maxLen("Venue address", w.VenueAddress, 500), maxLen("Kiosk description", w.KioskDescription, 2000)); err != nil {
		return nil, err
	}
	if body.KioskLogoUrl != nil {
		w.KioskLogoUrl = sanitizeURL(*body.KioskLogoUrl)
	}
	if body.KioskBackgroundUrl != nil {
		bg, ok := backgroundURL(*body.KioskBackgroundUrl)
		if !ok {
			return nil, fuego.BadRequestError{Title: "kioskBackgroundUrl must be an uploaded image or an http(s) URL"}
		}
		w.KioskBackgroundUrl = bg
	}
	if body.KioskBackgroundBlur != nil {
		w.KioskBackgroundBlur = max(0, min(20, *body.KioskBackgroundBlur))
	}
	if body.KioskBackgroundSize != nil {
		size := strings.TrimSpace(*body.KioskBackgroundSize)
		if !validBackgroundSizes[size] {
			return nil, fuego.BadRequestError{Title: "kioskBackgroundSize must be cover, contain, or auto"}
		}
		w.KioskBackgroundSize = size
	}
	if body.KioskLogoSize != nil {
		size := strings.TrimSpace(*body.KioskLogoSize)
		if !validBackgroundSizes[size] {
			return nil, fuego.BadRequestError{Title: "kioskLogoSize must be cover, contain, or auto"}
		}
		w.KioskLogoSize = size
	}
	if err := applyPosition(&w.KioskBackgroundPosX, body.KioskBackgroundPosX, validPositionsX, "kioskBackgroundPosX", "left, center, right"); err != nil {
		return nil, err
	}
	if err := applyPosition(&w.KioskBackgroundPosY, body.KioskBackgroundPosY, validPositionsY, "kioskBackgroundPosY", "top, center, bottom"); err != nil {
		return nil, err
	}
	if err := applyPosition(&w.KioskLogoPosX, body.KioskLogoPosX, validPositionsX, "kioskLogoPosX", "left, center, right"); err != nil {
		return nil, err
	}
	if err := applyPosition(&w.KioskLogoPosY, body.KioskLogoPosY, validPositionsY, "kioskLogoPosY", "top, center, bottom"); err != nil {
		return nil, err
	}
	if body.ShowSeatNumbers != nil {
		w.ShowSeatNumbers = *body.ShowSeatNumbers
	}
	if body.KioskIdleResetSeconds != nil {
		w.KioskIdleResetSeconds = max(0, min(3600, *body.KioskIdleResetSeconds))
	}
	if err := h.weddingService.UpdateKiosk(ctx, w, body.Name != nil); err != nil {
		return nil, fuego.InternalServerError{Title: "Failed to update kiosk settings"}
	}
	return w, nil
}

func (h *WeddingHandler) Delete(c fuego.ContextWithBody[any]) (any, error) {
	ctx := c.Context()
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	id, err := DecodeID(c.PathParam("id"))
	if err != nil {
		return nil, fuego.BadRequestError{Title: "Invalid ID"}
	}
	if err := h.weddingService.Delete(ctx, id); err != nil {
		return nil, err
	}
	c.SetStatus(204)
	return nil, nil
}

// uploadName matches file names the upload handler writes; no leading dot,
// so the upload index itself is never a valid target.
var uploadName = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

// uploadPath returns the /uploads/<file> path s points at, or "" for empty
// input. It accepts the absolute form the settings page sends (origin + the
// upload URL) and drops the origin, so the kiosk's CSS url() only ever loads
// our own uploads. ok is false for anything else.
func uploadPath(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", true
	}
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") {
		return "", false
	}
	name, ok := strings.CutPrefix(u.Path, "/uploads/")
	if !ok || !uploadName.MatchString(name) {
		return "", false
	}
	return "/uploads/" + name, true
}

// backgroundURL accepts our own uploads (normalised by uploadPath) or an
// external http(s) image URL. Characters that could end a CSS url("...") or
// open a new declaration are refused, since the kiosk renders it in a style.
func backgroundURL(s string) (string, bool) {
	if p, ok := uploadPath(s); ok {
		return p, true
	}
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil || u.User != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		strings.ContainsAny(s, "\"'\\()<>; \t\r\n") {
		return "", false
	}
	return s, true
}

// sanitizeURL strips any value that isn't a safe URL prefix or a relative path.
// Prevents CSS injection via url() in style attributes.
func sanitizeURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "//") {
		return ""
	}
	allowed := strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./")
	if !allowed {
		return ""
	}
	// Block values containing CSS-breaking characters
	if strings.ContainsAny(s, ")\"';}{") {
		return ""
	}
	return s
}

var (
	validBackgroundSizes = map[string]bool{"cover": true, "contain": true, "auto": true}
	validPositionsX      = map[string]bool{"left": true, "center": true, "right": true}
	validPositionsY      = map[string]bool{"top": true, "center": true, "bottom": true}
)

// normalizePosition accepts a keyword or a percentage in [0, 100] and returns
// its canonical form. Anything else is rejected rather than silently coerced.
func normalizePosition(val string, keywords map[string]bool) (string, bool) {
	if keywords[val] {
		return val, true
	}
	if !strings.HasSuffix(val, "%") {
		return "", false
	}
	n, err := strconv.ParseFloat(strings.TrimSuffix(val, "%"), 64)
	if err != nil || !(n >= 0 && n <= 100) {
		return "", false
	}
	out := strconv.FormatFloat(n, 'f', -1, 64) + "%"
	if len(out) > 10 { // kiosk*PosX/Y columns are varchar(10)
		return "", false
	}
	return out, true
}

func applyPosition(dst *string, src *string, keywords map[string]bool, field, allowed string) error {
	if src == nil {
		return nil
	}
	val, ok := normalizePosition(strings.TrimSpace(*src), keywords)
	if !ok {
		return fuego.BadRequestError{Title: field + " must be " + allowed + ", or a percentage from 0% to 100%"}
	}
	*dst = val
	return nil
}
