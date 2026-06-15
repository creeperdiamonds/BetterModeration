package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"creeperdiamonds.xyz/bettermoderation/internal/automod"
)

// ── AutoMod Rules ─────────────────────────────────────────────────────────────

func (ro *Router) handleListAutomodRules(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id required")
		return
	}
	rows, err := ro.db.QueryxContext(r.Context(), `
		SELECT id, org_id, name, enabled, test_mode, trigger_type, trigger_value,
		       action_type, action_duration_seconds, platform, priority
		FROM automod_rules WHERE org_id = ? ORDER BY priority ASC`, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var rules []automod.Rule
	for rows.Next() {
		var rule automod.Rule
		if err := rows.StructScan(&rule); err != nil {
			continue
		}
		rules = append(rules, rule)
	}
	if rules == nil {
		rules = []automod.Rule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

func (ro *Router) handleCreateAutomodRule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrgID          string  `json:"org_id"`
		Name           string  `json:"name"`
		Enabled        bool    `json:"enabled"`
		TestMode       bool    `json:"test_mode"`
		TriggerType    string  `json:"trigger_type"`
		TriggerValue   *string `json:"trigger_value"`
		ActionType     string  `json:"action_type"`
		ActionDuration *int64  `json:"action_duration_seconds"`
		Platform       string  `json:"platform"`
		Priority       int     `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		body.OrgID == "" || body.TriggerType == "" || body.ActionType == "" {
		writeError(w, http.StatusBadRequest, "org_id, trigger_type, and action_type required")
		return
	}
	if body.Platform == "" {
		body.Platform = "ALL"
	}
	if err := validateLen("name", body.Name, 100); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id := uuid.NewString()
	_, err := ro.db.ExecContext(r.Context(), `
		INSERT INTO automod_rules
			(id, org_id, name, enabled, test_mode, trigger_type, trigger_value,
			 action_type, action_duration_seconds, platform, priority, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(3))`,
		id, body.OrgID, body.Name, body.Enabled, body.TestMode, body.TriggerType, body.TriggerValue,
		body.ActionType, body.ActionDuration, body.Platform, body.Priority,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ro.automod.Invalidate(r.Context(), body.OrgID)
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (ro *Router) handleUpdateAutomodRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name           *string `json:"name"`
		Enabled        *bool   `json:"enabled"`
		TestMode       *bool   `json:"test_mode"`
		TriggerType    *string `json:"trigger_type"`
		TriggerValue   *string `json:"trigger_value"`
		ActionType     *string `json:"action_type"`
		ActionDuration *int64  `json:"action_duration_seconds"`
		Platform       *string `json:"platform"`
		Priority       *int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Fetch current row to get org_id for cache invalidation
	var orgID string
	if err := ro.db.QueryRowxContext(r.Context(), `SELECT org_id FROM automod_rules WHERE id = ?`, id).Scan(&orgID); err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}

	_, err := ro.db.ExecContext(r.Context(), `
		UPDATE automod_rules SET
			name                  = COALESCE(?, name),
			enabled               = COALESCE(?, enabled),
			test_mode             = COALESCE(?, test_mode),
			trigger_type          = COALESCE(?, trigger_type),
			trigger_value         = COALESCE(?, trigger_value),
			action_type           = COALESCE(?, action_type),
			action_duration_seconds = COALESCE(?, action_duration_seconds),
			platform              = COALESCE(?, platform),
			priority              = COALESCE(?, priority),
			updated_at            = NOW(3)
		WHERE id = ?`,
		body.Name, body.Enabled, body.TestMode, body.TriggerType, body.TriggerValue,
		body.ActionType, body.ActionDuration, body.Platform, body.Priority, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ro.automod.Invalidate(r.Context(), orgID)
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) handleDeleteAutomodRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var orgID string
	if err := ro.db.QueryRowxContext(r.Context(), `SELECT org_id FROM automod_rules WHERE id = ?`, id).Scan(&orgID); err != nil {
		writeError(w, http.StatusNotFound, "rule not found")
		return
	}

	ro.db.ExecContext(r.Context(), `DELETE FROM automod_rules WHERE id = ?`, id)
	ro.automod.Invalidate(r.Context(), orgID)
	w.WriteHeader(http.StatusNoContent)
}

// ── Alt detection endpoint ─────────────────────────────────────────────────────

type altInfo struct {
	ProfileID     string    `json:"profile_id"`
	MinecraftUUID *string   `json:"minecraft_uuid"`
	DiscordID     *string   `json:"discord_id"`
	HasActiveBan  bool      `json:"has_active_ban"`
	BanReason     *string   `json:"ban_reason"`
	BanExpiresAt  *time.Time `json:"ban_expires_at"`
}

func (ro *Router) handleMinecraftAlts(w http.ResponseWriter, r *http.Request) {
	minecraftUUID := r.PathValue("uuid")

	prof, err := ro.profiles.ResolveByMinecraft(r.Context(), minecraftUUID)
	if err != nil {
		writeJSON(w, http.StatusOK, []altInfo{})
		return
	}

	altIDs, err := ro.profiles.FindAlts(r.Context(), prof.ID)
	if err != nil || len(altIDs) == 0 {
		writeJSON(w, http.StatusOK, []altInfo{})
		return
	}

	// For each alt, check whether they have an active ban
	var alts []altInfo
	for _, altProfileID := range altIDs {
		var info altInfo
		info.ProfileID = altProfileID

		// Fetch basic profile info
		ro.db.QueryRowxContext(r.Context(), `SELECT minecraft_uuid, discord_id FROM profiles WHERE id = ?`, altProfileID).
			Scan(&info.MinecraftUUID, &info.DiscordID)

		// Check for active ban
		var banReason *string
		var banExpires *time.Time
		err := ro.db.QueryRowxContext(r.Context(), `
			SELECT reason, expires_at FROM punishments
			WHERE profile_id = ? AND type = 'BAN' AND minecraft_active = 1
			  AND (expires_at IS NULL OR expires_at > NOW(3))
			ORDER BY issued_at DESC LIMIT 1`, altProfileID,
		).Scan(&banReason, &banExpires)

		if err == nil {
			info.HasActiveBan = true
			info.BanReason = banReason
			info.BanExpiresAt = banExpires
		}

		alts = append(alts, info)
	}

	writeJSON(w, http.StatusOK, alts)
}

// ── Permission cache invalidation ──────────────────────────────────────────────

func (ro *Router) handleInvalidatePerms(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GuildID string `json:"guild_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GuildID == "" {
		writeError(w, http.StatusBadRequest, "guild_id required")
		return
	}
	if err := ro.perms.Invalidate(r.Context(), body.GuildID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "invalidated"})
}
