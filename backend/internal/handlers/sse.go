package handlers

import (
	"fmt"
	"net/http"
	"strings"
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
// Auth: EventSource cannot send headers, so the JWT is passed via ?token= query param.
func (h *SSEHandler) StreamHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Validate token from query param (EventSource can't set headers).
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, `{"error":"Missing token"}`, http.StatusUnauthorized)
			return
		}
		if _, err := h.authService.ValidateToken(r.Context(), token); err != nil {
			http.Error(w, `{"error":"Invalid token"}`, http.StatusUnauthorized)
			return
		}

		// Extract wedding ID from path: /api/weddings/{wid}/events
		path := r.URL.Path
		parts := strings.Split(path, "/")
		// Expected: ["", "api", "weddings", "{wid}", "events"]
		if len(parts) < 4 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		wid, err := uuid.Parse(parts[3])
		if err != nil {
			http.Error(w, "invalid wedding ID", http.StatusBadRequest)
			return
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

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-client.Done:
				return
			case data, ok := <-client.Chan:
				if !ok {
					return
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}
