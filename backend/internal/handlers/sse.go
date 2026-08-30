package handlers

import (
	"fmt"
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
// Auth: EventSource cannot send headers, so the JWT is passed via ?token= query param.
func (h *SSEHandler) StreamHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate token from query param (EventSource can't set headers).
		token := r.URL.Query().Get("token")
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
		fmt.Fprintf(w, ": keepalive\n\n")
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
