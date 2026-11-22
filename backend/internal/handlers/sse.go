package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"weddingdb/internal/middleware"
	"weddingdb/internal/services"

	"github.com/google/uuid"
)

// SSEHandler provides raw HTTP handlers for SSE streaming.
type SSEHandler struct {
	sseHub      *services.SSEHub
	authService *services.AuthService
}

func NewSSEHandler(sseHub *services.SSEHub, authService *services.AuthService) *SSEHandler {
	return &SSEHandler{sseHub: sseHub, authService: authService}
}

// StreamHandler returns an http.HandlerFunc for SSE streaming.
// Registered directly on the mux to bypass Fuego's response buffering.
// Auth: clients must send the JWT in an Authorization: Bearer header.
func (h *SSEHandler) StreamHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Read token from the standard Authorization: Bearer header.
		token := middleware.ExtractBearer(r)
		if token == "" {
			http.Error(w, `{"error":"Missing token"}`, http.StatusUnauthorized)
			return
		}
		claims, err := h.authService.ValidateToken(r.Context(), token)
		if err != nil {
			http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
			return
		}

		// Extract wedding ID from path: /api/weddings/{wid}/events
		widStr := r.PathValue("wid")
		wid, err := middleware.DecodeWIDString(widStr)
		if err != nil {
			http.Error(w, "invalid wedding ID", http.StatusBadRequest)
			return
		}

		// Verify wedding scope: non-admin users must have a JWT scoped to the requested wedding.
		if !canStream(claims, wid) {
			http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// The server's WriteTimeout applies to the whole response and would end
		// every stream at 60s. Clear it for this response only before writing.
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
			slog.Warn("SSE: clearing write deadline failed", "wedding", wid, "err", err)
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		// Subscribe to wedding events.
		client, err := h.sseHub.Subscribe(wid, claims.AdminID)
		if err != nil {
			http.Error(w, `{"error":"Too many event streams"}`, http.StatusServiceUnavailable)
			return
		}
		defer h.sseHub.Unsubscribe(client)

		// Send initial keepalive comment.
		if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
			slog.Warn("SSE: initial keepalive write failed", "wedding", wid, "err", err)
		}
		flusher.Flush()

		// Periodic keepalive: silent streams get killed by proxies/NAT at ~60s idle.
		keepalive := time.NewTicker(20 * time.Second)
		defer keepalive.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-client.Done:
				return
			case <-keepalive.C:
				// Re-check on every tick: a revoked or expired token, or a lost
				// wedding scope, ends the stream instead of leaking events.
				claims, err := h.authService.ValidateToken(ctx, token)
				if err != nil || !canStream(claims, wid) {
					return
				}
				if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case data, ok := <-client.Chan:
				if !ok {
					return
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// canStream reports whether claims may read wid's events: admins read every
// wedding, other users only the one their token is scoped to. A token pending
// a forced password change reads nothing.
func canStream(claims *services.AccessClaims, wid uuid.UUID) bool {
	if claims.ForcePasswordChange {
		return false
	}
	return claims.Role == "admin" || (claims.WeddingID != nil && *claims.WeddingID == wid)
}
