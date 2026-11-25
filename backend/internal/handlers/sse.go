package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"weddingdb/internal/middleware"
	"weddingdb/internal/services"
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
			middleware.WriteAuthError(w, err)
			return
		}
		// Raw mux route skips AuthMiddleware, so enforce its forced-change gate here.
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

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		// Subscribe to wedding events.
		client, err := h.sseHub.Subscribe(r.Context(), wid, claims.AdminID)
		if err != nil {
			if errors.Is(err, services.ErrSSECapacity) || errors.Is(err, services.ErrSSEUserCapacity) {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
			} else {
				http.Error(w, "service temporarily unavailable", http.StatusServiceUnavailable)
			}
			return
		}
		defer h.sseHub.Unsubscribe(client)
		controller := http.NewResponseController(w)
		if err := controller.SetWriteDeadline(client.Deadline()); err != nil {
			http.Error(w, "streaming not supported", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

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
			if err := controller.SetWriteDeadline(client.Deadline()); err != nil || !time.Now().Before(client.Deadline()) {
				return
			}
			select {
			case <-client.Done:
				return
			default:
			}
			expiry := time.NewTimer(time.Until(client.Deadline()))
			select {
			case <-ctx.Done():
				expiry.Stop()
				return
			case <-client.Done:
				expiry.Stop()
				return
			case <-expiry.C:
				continue
			case <-keepalive.C:
				expiry.Stop()
				if !time.Now().Before(client.Deadline()) {
					return
				}
				// Expired, revoked or logged-out tokens end the stream; an outage
				// on the recheck does not, matching the 503 "keep session" contract.
				if _, err := h.authService.ValidateToken(ctx, token); err != nil {
					if errors.Is(err, services.ErrTokenRejected) {
						return
					}
					slog.Warn("SSE: token recheck unavailable", "wedding", wid, "err", err)
				}
				if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case data, ok := <-client.Chan:
				expiry.Stop()
				if !time.Now().Before(client.Deadline()) {
					return
				}
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
