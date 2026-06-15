package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	bmsync "creeperdiamonds.xyz/bettermoderation/internal/sync"
)

// sseStream handles GET /v1/events/stream.
// Minecraft plugins connect here on startup to receive real-time punishment events
// without needing a direct Redis connection. The server_id and org_id come from
// the API key middleware context.
func (ro *Router) sseStream(w http.ResponseWriter, r *http.Request) {
	serverID := serverIDFromCtx(r.Context())
	orgID := orgIDFromCtx(r.Context())
	if serverID == "" || orgID == "" {
		writeError(w, http.StatusUnauthorized, "missing server context")
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Send a heartbeat immediately so the client knows it's connected
	fmt.Fprintf(w, ": connected server=%s\n\n", serverID)
	flusher.Flush()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Subscribe to both issue and revoke channels; filter by orgID
	issueCh := make(chan string, 32)
	revokeCh := make(chan string, 32)

	go ro.bus.Subscribe(ctx, bmsync.ChannelPunishmentIssue, func(payload string) {
		evt, err := bmsync.UnmarshalEvent(payload)
		if err != nil || evt.OrgID != orgID {
			return
		}
		select {
		case issueCh <- payload:
		default:
		}
	})
	go ro.bus.Subscribe(ctx, bmsync.ChannelPunishmentRevoke, func(payload string) {
		evt, err := bmsync.UnmarshalEvent(payload)
		if err != nil || evt.OrgID != orgID {
			return
		}
		select {
		case revokeCh <- payload:
		default:
		}
	})

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	log.Printf("[sse] server %s connected (org %s)", serverID, orgID)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[sse] server %s disconnected", serverID)
			return

		case payload := <-issueCh:
			fmt.Fprintf(w, "event: punishment.issue\ndata: %s\n\n", payload)
			flusher.Flush()

		case payload := <-revokeCh:
			fmt.Fprintf(w, "event: punishment.revoke\ndata: %s\n\n", payload)
			flusher.Flush()

		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
