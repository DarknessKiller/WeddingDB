package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"weddingdb/internal/middleware"
	"weddingdb/internal/repository"
	"weddingdb/internal/services"
)

// sseKeepaliveInterval keeps proxies/NAT from killing silent streams (~60s idle)
// and paces the revocation re-check on open streams.
const sseKeepaliveInterval = 20 * time.Second

// sseWriteTimeout bounds one stream write to a client that stopped reading.
const sseWriteTimeout = 10 * time.Second

// SSEHandler provides raw HTTP handlers for SSE streaming.
type SSEHandler struct {
	sseHub      *services.SSEHub
	authService *services.AuthService
	adminRepo   *repository.AdminRepo
	keepalive   time.Duration
}

func NewSSEHandler(sseHub *services.SSEHub, authService *services.AuthService, adminRepo *repository.AdminRepo) *SSEHandler {
	return &SSEHandler{sseHub: sseHub, authService: authService, adminRepo: adminRepo, keepalive: sseKeepaliveInterval}
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

		// Verify wedding scope: non-admin users must have a JWT scoped to the
		// requested wedding and still be assigned to it in the DB.
		if claims.Role != "admin" {
			if claims.WeddingID == nil || *claims.WeddingID != wid {
				http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
				return
			}
			allowed, err := h.adminRepo.HasWeddingAccess(r.Context(), claims.AdminID, wid)
			if err != nil {
				slog.Error("SSE: wedding access check failed", "user", claims.AdminID, "wedding", wid, "err", err)
				http.Error(w, `{"error":"Access check failed"}`, http.StatusInternalServerError)
				return
			}
			if !allowed {
				http.Error(w, `{"error":"Access denied"}`, http.StatusForbidden)
				return
			}
		}

		// Subscribe before writing headers so a rejected connection can still
		// return a normal 503 instead of a half-started stream.
		client, subscribed := h.sseHub.Subscribe(wid, claims.AdminID)
		if !subscribed {
			http.Error(w, `{"error":"Too many open streams"}`, http.StatusServiceUnavailable)
			return
		}
		defer h.sseHub.Unsubscribe(client)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Each write gets a deadline: a phone that left the wifi without closing
		// its socket would otherwise block a write until TCP gives up (15+ min),
		// holding this goroutine and never seeing client.Done.
		rc := http.NewResponseController(w)
		write := func(format string, args ...any) error {
			// ErrNotSupported: a writer without deadlines (test recorder) still streams.
			if err := rc.SetWriteDeadline(time.Now().Add(sseWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return err
			}
			if _, err := fmt.Fprintf(w, format, args...); err != nil {
				return err
			}
			return rc.Flush()
		}

		// Send initial keepalive comment.
		if err := write(": keepalive\n\n"); err != nil {
			slog.Warn("SSE: initial keepalive write failed", "wedding", wid, "err", err)
			return
		}

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
				// An unassign whose revoke failed leaves the token valid; the
				// DB assignment is the truth, so re-check it on every tick.
				if claims.Role != "admin" {
					allowed, err := h.adminRepo.HasWeddingAccess(ctx, claims.AdminID, wid)
					if err != nil || !allowed {
						slog.Info("SSE: closing stream without wedding access", "user", claims.AdminID, "wedding", wid, "err", err)
						return
					}
				}
				if err := write(": ping\n\n"); err != nil {
					return
				}
			case data, ok := <-client.Chan:
				if !ok {
					return
				}
				if err := write("data: %s\n\n", data); err != nil {
					return
				}
			}
		}
	}
}
