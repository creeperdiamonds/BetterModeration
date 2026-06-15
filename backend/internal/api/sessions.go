package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (ro *Router) handleSessionConnect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UUID        string `json:"uuid"`
		Username    string `json:"username"`
		IP          string `json:"ip"`
		OfflineMode bool   `json:"offline_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		strings.TrimSpace(body.UUID) == "" || strings.TrimSpace(body.Username) == "" {
		writeError(w, http.StatusBadRequest, "uuid and username required")
		return
	}

	req := ro.evasion.NewConnectRequest(
		body.UUID, body.Username, body.IP,
		orgIDFromCtx(r.Context()), serverIDFromCtx(r.Context()),
		body.OfflineMode,
	)

	result, err := ro.evasion.Connect(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"action":          result.Action,
		"kick_message":    result.KickMessage,
		"suspicion_score": result.SuspicionScore,
		"flags":           result.Flags,
		"profile_id":      result.ProfileID,
	})
}

func (ro *Router) handleBackfillOfflineUUIDs(w http.ResponseWriter, r *http.Request) {
	count, err := ro.evasion.BackfillBannedOfflineUUIDs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows_written": count})
}
