package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"weddingdb/internal/middleware"
	"weddingdb/internal/services"
)

// sseKeepaliveInterval keeps proxies/NAT from killing silent streams (~60s idle)
// and paces the revocation re-check on open streams.
const sseKeepaliveInterval = 20 * time.Second

// SSEHandler provides raw HTTP handlers for SSE streaming.
type SSEHandler struct {
	sseHub      *services.SSEHub
	authService *services.AuthService
	keepalive   time.Duration
}

func NewSSEHandler(sseHub *services.SSEHub, authService *services.AuthService) *SSEHandler {
	return &SSEHandler{sseHub: sseHub, authService: authService, keepalive: sseKeepaliveInterval}
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
		if claims.ForcePasswordChange {
			http.Error(w, `{"error":"Password change required"}`, http.StatusForbidden)
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
		if claims.Role != "admin" {
			if claims.WeddingID == nil || *claims.WeddingID != wid {
				http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
				return
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		// Subscribe to wedding events.
		client := h.sseHub.Subscribe(wid)
		defer h.sseHub.Unsubscribe(client)

		// Send initial keepalive comment.
		if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
			slog.Warn("SSE: initial keepalive write failed", "wedding", wid, "err", err)
		}
		flusher.Flush()

		// Periodic keepalive: silent streams get killed by proxies/NAT at ~60s idle.
		keepalive := time.NewTicker(h.keepalive)
		defer keepalive.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-client.Done:
				return
			case <-keepalive.C:
				// Logout, password change or reset, user deletion and wedding
				// unassignment all revoke the token; end the stream when they do.
				// Also end at token expiry: the stream outlives the access token.
				if claims.ExpiresAt == nil || time.Now().After(claims.ExpiresAt.Time) {
					slog.Info("SSE: closing expired stream", "user", claims.AdminID, "wedding", wid)
					return
				}
				if err := h.authService.CheckRevoked(ctx, claims); err != nil {
					slog.Info("SSE: closing revoked stream", "user", claims.AdminID, "wedding", wid, "err", err)
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
