package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"creeperdiamonds.xyz/bettermoderation/internal/appeals"
	"creeperdiamonds.xyz/bettermoderation/internal/audit"
	"creeperdiamonds.xyz/bettermoderation/internal/auth"
	"creeperdiamonds.xyz/bettermoderation/internal/automod"
	"creeperdiamonds.xyz/bettermoderation/internal/linking"
	"creeperdiamonds.xyz/bettermoderation/internal/permission"
	"creeperdiamonds.xyz/bettermoderation/internal/profile"
	"creeperdiamonds.xyz/bettermoderation/internal/evasion"
	"creeperdiamonds.xyz/bettermoderation/internal/reports"
	bmsync "creeperdiamonds.xyz/bettermoderation/internal/sync"
	"creeperdiamonds.xyz/bettermoderation/internal/warn"
	"creeperdiamonds.xyz/bettermoderation/internal/webhook"
)

type Router struct {
	profiles *profile.Service
	warns    *warn.Service
	linking  *linking.Service
	appeals  *appeals.Service
	reports  *reports.Service
	webhooks *webhook.Dispatcher
	bus      *bmsync.EventBus
	oauth    *auth.Manager
	automod  *automod.Engine
	perms    *permission.Loader
	evasion  *evasion.Service
	rdb      *redis.Client
	db       *sqlx.DB
}

func NewRouter(
	profiles *profile.Service,
	warns *warn.Service,
	linkingSvc *linking.Service,
	appealsSvc *appeals.Service,
	reportsSvc *reports.Service,
	webhookDisp *webhook.Dispatcher,
	bus *bmsync.EventBus,
	oauthMgr *auth.Manager,
	automodEngine *automod.Engine,
	permLoader *permission.Loader,
	evasionSvc *evasion.Service,
	rdb *redis.Client,
	db *sqlx.DB,
) http.Handler {
	r := &Router{
		profiles: profiles,
		warns:    warns,
		linking:  linkingSvc,
		appeals:  appealsSvc,
		reports:  reportsSvc,
		webhooks: webhookDisp,
		bus:      bus,
		oauth:    oauthMgr,
		automod:  automodEngine,
		perms:    permLoader,
		evasion:  evasionSvc,
		rdb:      rdb,
		db:       db,
	}

	serverAuth := RequireServerKey(db)

	// Rate limiters for public-facing endpoints
	publicRL := RateLimit(rdb, 60, time.Minute, KeyByIP)           // 60 req/min per IP (general browsing)
	submitRL := RateLimit(rdb, 10, time.Minute, KeyByIP)           // 10 submissions/min per IP
	lookupRL := RateLimit(rdb, 30, time.Minute, KeyByIP)           // 30 lookups/min per IP
	authRL := RateLimit(rdb, 10, 15*time.Minute, KeyByIP)          // 10 auth attempts per 15 min
	pluginRL := RateLimit(rdb, 300, time.Minute, KeyByServerID)    // 300 req/min per server key

	mux := http.NewServeMux()

	handle := func(pattern string, h http.Handler, middlewares ...func(http.Handler) http.Handler) {
		for i := len(middlewares) - 1; i >= 0; i-- {
			h = middlewares[i](h)
		}
		mux.Handle(pattern, h)
	}

	// Health (no rate limit)
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /readyz", handleReadyz)

	// Discord OAuth2
	handle("GET /auth/discord", http.HandlerFunc(r.handleDiscordLogin), authRL)
	handle("GET /auth/discord/callback", http.HandlerFunc(r.handleDiscordCallback), authRL)
	mux.HandleFunc("POST /auth/logout", r.handleLogout)
	handle("GET /auth/me", http.HandlerFunc(r.handleMe), publicRL)

	// Linking (server redeem is auth-exempt — it's how a server obtains its key)
	handle("POST /v1/link/server/generate", http.HandlerFunc(r.handleGenerateServerCode), submitRL)
	handle("POST /v1/link/server/redeem", http.HandlerFunc(r.handleRedeemServerCode), submitRL)
	handle("POST /v1/link/player/generate", http.HandlerFunc(r.handleGeneratePlayerCode), submitRL)
	handle("POST /v1/link/player/redeem", http.HandlerFunc(r.handleRedeemPlayerCode), submitRL)

	// Profiles (public read)
	handle("GET /v1/profiles/{id}", http.HandlerFunc(r.handleGetProfile), lookupRL)
	handle("GET /v1/profiles/{id}/punishments", http.HandlerFunc(r.handleGetProfilePunishments), lookupRL)
	handle("GET /v1/profiles/{id}/alts", http.HandlerFunc(r.handleGetProfileAlts), lookupRL)

	// Punishments
	mux.HandleFunc("POST /v1/punishments", r.handleIssuePunishment)
	mux.HandleFunc("DELETE /v1/punishments/{id}", r.handleRevokePunishment)
	mux.HandleFunc("GET /v1/punishments/{id}", r.handleGetPunishment)

	// Minecraft plugin endpoints (require server API key + per-server rate limit)
	handle("GET /v1/minecraft/{uuid}/punishments", http.HandlerFunc(r.handleMinecraftPunishments), serverAuth, pluginRL)
	handle("GET /v1/minecraft/{uuid}/alts", http.HandlerFunc(r.handleMinecraftAlts), serverAuth, pluginRL)
	handle("POST /v1/sessions/disconnect", http.HandlerFunc(r.handleSessionDisconnect), serverAuth, pluginRL)
	handle("PUT /v1/servers/{id}/status", http.HandlerFunc(r.handleUpdateServerStatus), serverAuth, pluginRL)
	handle("POST /v1/players/track-ip", http.HandlerFunc(r.handleTrackIP), serverAuth, pluginRL)
	handle("GET /v1/events/stream", http.HandlerFunc(r.sseStream), serverAuth)
	handle("POST /v1/sessions/connect", http.HandlerFunc(r.handleSessionConnect), serverAuth, pluginRL)
	handle("POST /v1/admin/backfill-offline-uuids", http.HandlerFunc(r.handleBackfillOfflineUUIDs), serverAuth)

	// Appeals
	handle("POST /v1/appeals", http.HandlerFunc(r.handleSubmitAppeal), submitRL)
	handle("GET /v1/appeals", http.HandlerFunc(r.handleListAppeals), publicRL)
	handle("GET /v1/appeals/{id}", http.HandlerFunc(r.handleGetAppeal), publicRL)
	mux.HandleFunc("PUT /v1/appeals/{id}", r.handleReviewAppeal)

	// Reports
	handle("POST /v1/reports", http.HandlerFunc(r.handleSubmitReport), submitRL)
	handle("GET /v1/reports", http.HandlerFunc(r.handleListReports), publicRL)
	handle("GET /v1/reports/{id}", http.HandlerFunc(r.handleGetReport), publicRL)
	mux.HandleFunc("PUT /v1/reports/{id}/claim", r.handleClaimReport)
	mux.HandleFunc("PUT /v1/reports/{id}/resolve", r.handleResolveReport)

	// Webhooks
	mux.HandleFunc("POST /v1/webhooks", r.handleCreateWebhook)
	mux.HandleFunc("GET /v1/webhooks", r.handleListWebhooks)
	mux.HandleFunc("DELETE /v1/webhooks/{id}", r.handleDeleteWebhook)
	mux.HandleFunc("POST /v1/webhooks/{id}/test", r.handleTestWebhook)

	// AutoMod rules
	handle("GET /v1/automod/rules", http.HandlerFunc(r.handleListAutomodRules), publicRL)
	mux.HandleFunc("POST /v1/automod/rules", r.handleCreateAutomodRule)
	mux.HandleFunc("PUT /v1/automod/rules/{id}", r.handleUpdateAutomodRule)
	mux.HandleFunc("DELETE /v1/automod/rules/{id}", r.handleDeleteAutomodRule)

	// Permission cache management
	mux.HandleFunc("POST /v1/permissions/invalidate", r.handleInvalidatePerms)

	return mux
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func handleHealthz(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }
func handleReadyz(w http.ResponseWriter, r *http.Request)  { w.Write([]byte("ready")) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func randomSecret() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ── Linking ───────────────────────────────────────────────────────────────────

func (ro *Router) handleGenerateServerCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		GuildID        string `json:"guild_id"`
		OwnerDiscordID string `json:"owner_discord_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GuildID == "" || body.OwnerDiscordID == "" {
		writeError(w, http.StatusBadRequest, "guild_id and owner_discord_id required")
		return
	}
	code, err := ro.linking.GenerateServerCode(r.Context(), body.GuildID, body.OwnerDiscordID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": code})
}

func (ro *Router) handleRedeemServerCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code     string `json:"code"`
		ServerID string `json:"server_id"`
		Name     string `json:"name"`
		Platform string `json:"platform"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" || body.ServerID == "" {
		writeError(w, http.StatusBadRequest, "code, server_id, name, and platform required")
		return
	}
	apiKey, err := ro.linking.RedeemServerCode(r.Context(), body.Code, body.ServerID, body.Name, body.Platform)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"api_key": apiKey})
}

func (ro *Router) handleGeneratePlayerCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProfileID     string `json:"profile_id"`
		MinecraftUUID string `json:"minecraft_uuid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" || body.MinecraftUUID == "" {
		writeError(w, http.StatusBadRequest, "profile_id and minecraft_uuid required")
		return
	}
	code, err := ro.linking.GeneratePlayerCode(r.Context(), body.ProfileID, body.MinecraftUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"code": code})
}

func (ro *Router) handleRedeemPlayerCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code      string `json:"code"`
		DiscordID string `json:"discord_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Code == "" || body.DiscordID == "" {
		writeError(w, http.StatusBadRequest, "code and discord_id required")
		return
	}
	if err := ro.linking.RedeemPlayerCode(r.Context(), body.Code, body.DiscordID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "linked"})
}

// ── Profiles ──────────────────────────────────────────────────────────────────

func (ro *Router) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := ro.profiles.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "profile not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (ro *Router) handleGetProfilePunishments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	punishments, err := ro.warns.GetPunishments(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, punishments)
}

func (ro *Router) handleGetProfileAlts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	alts, err := ro.profiles.FindAlts(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"alts": alts})
}

// ── Punishments ───────────────────────────────────────────────────────────────

func (ro *Router) handleIssuePunishment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrgID     string  `json:"org_id"`
		ProfileID string  `json:"profile_id"`
		IssuedBy  string  `json:"issued_by"`
		Type      string  `json:"type"`
		Reason    string  `json:"reason"`
		ServerID  string  `json:"server_id"`
		ExpiresAt *string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.OrgID == "" || body.ProfileID == "" || body.Type == "" || body.Reason == "" {
		writeError(w, http.StatusBadRequest, "org_id, profile_id, type, and reason required")
		return
	}
	if err := validatePunishmentInput(body.Reason); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if body.Type == "WARN" {
		result, err := ro.warns.Issue(r.Context(), body.OrgID, body.ProfileID, body.IssuedBy, body.Reason, body.ServerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		audit.Log(r.Context(), ro.db, body.OrgID, body.IssuedBy, "STAFF", "WARN_ISSUED", body.ProfileID, result.PunishmentID,
			map[string]any{"reason": body.Reason}, r.RemoteAddr, "API")
		ro.webhooks.Dispatch(r.Context(), body.OrgID, "punishment.issued", map[string]any{
			"punishment_id": result.PunishmentID,
			"profile_id":    body.ProfileID,
			"type":          "WARN",
			"reason":        body.Reason,
		})
		writeJSON(w, http.StatusCreated, result)
		return
	}
	var expiresAt *time.Time
	if body.ExpiresAt != nil && *body.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "expires_at must be RFC3339")
			return
		}
		expiresAt = &t
	}
	record, err := ro.warns.IssueDirect(r.Context(), body.OrgID, body.ProfileID, body.IssuedBy, body.Type, body.Reason, body.ServerID, expiresAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	audit.Log(r.Context(), ro.db, body.OrgID, body.IssuedBy, "STAFF", body.Type+"_ISSUED", body.ProfileID, record.ID,
		map[string]any{"reason": body.Reason}, r.RemoteAddr, "API")
	ro.webhooks.Dispatch(r.Context(), body.OrgID, "punishment.issued", map[string]any{
		"punishment_id": record.ID,
		"profile_id":    body.ProfileID,
		"type":          body.Type,
		"reason":        body.Reason,
	})
	writeJSON(w, http.StatusCreated, record)
}

func (ro *Router) handleRevokePunishment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := ro.warns.GetPunishment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "punishment not found")
		return
	}
	if err := ro.warns.Revoke(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	audit.Log(r.Context(), ro.db, p.OrgID, "", "STAFF", p.Type+"_REVOKED", p.ProfileID, id,
		nil, r.RemoteAddr, "API")
	ro.webhooks.Dispatch(r.Context(), p.OrgID, "punishment.revoked", map[string]any{
		"punishment_id": id,
		"profile_id":    p.ProfileID,
		"type":          p.Type,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) handleGetPunishment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := ro.warns.GetPunishment(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "punishment not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ── Minecraft-facing ──────────────────────────────────────────────────────────

func (ro *Router) handleMinecraftPunishments(w http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")
	ip := r.URL.Query().Get("ip")

	prof, err := ro.profiles.ResolveByMinecraft(r.Context(), uuid)
	if err != nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}

	// Track the player's IP asynchronously (best-effort).
	if ip != "" {
		go ro.profiles.TrackIP(context.Background(), prof.ID, ip)
	}

	punishments, err := ro.warns.GetPunishments(r.Context(), prof.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var active []warn.PunishmentRecord
	hasBan := false
	for _, p := range punishments {
		if p.MinecraftActive {
			active = append(active, p)
			if p.Type == "BAN" {
				hasBan = true
			}
		}
	}

	// If the player's own profile isn't already banned, check whether their IP
	// is associated with an active ban in this org (catches ban evasion via new accounts).
	if !hasBan && ip != "" {
		orgID := orgIDFromCtx(r.Context())
		if orgID != "" {
			if banned, reason, expiresAt, _ := ro.profiles.IsIPBanned(r.Context(), ip, orgID); banned {
				active = append(active, warn.PunishmentRecord{
					ID:              "ip-ban",
					OrgID:           orgID,
					ProfileID:       prof.ID,
					Type:            "BAN",
					Reason:          reason,
					IssuedByType:    "SYSTEM",
					IssuedAt:        time.Now().UTC(),
					ExpiresAt:       expiresAt,
					MinecraftActive: true,
					Public:          false,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, active)
}

func (ro *Router) handleSessionDisconnect(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) handleUpdateServerStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Online bool `json:"online"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "online field required")
		return
	}
	if err := ro.profiles.UpdateServerStatus(r.Context(), id, body.Online); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) handleTrackIP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProfileID string `json:"profile_id"`
		IP        string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" || body.IP == "" {
		writeError(w, http.StatusBadRequest, "profile_id and ip required")
		return
	}
	if err := ro.profiles.TrackIP(r.Context(), body.ProfileID, body.IP); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Appeals ───────────────────────────────────────────────────────────────────

func (ro *Router) handleSubmitAppeal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PunishmentID string  `json:"punishment_id"`
		ProfileID    string  `json:"profile_id"`
		Reason       string  `json:"reason"`
		Evidence     *string `json:"evidence"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PunishmentID == "" || body.ProfileID == "" || body.Reason == "" {
		writeError(w, http.StatusBadRequest, "punishment_id, profile_id, and reason required")
		return
	}
	ev := ""
	if body.Evidence != nil {
		ev = *body.Evidence
	}
	if err := validateAppealInput(body.Reason, ev); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	appeal, err := ro.appeals.Submit(r.Context(), body.PunishmentID, body.ProfileID, body.Reason, body.Evidence)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, appeal)
}

func (ro *Router) handleListAppeals(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	status := r.URL.Query().Get("status")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id query param required")
		return
	}
	list, err := ro.appeals.List(r.Context(), orgID, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (ro *Router) handleGetAppeal(w http.ResponseWriter, r *http.Request) {
	a, err := ro.appeals.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (ro *Router) handleReviewAppeal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		ReviewerID string `json:"reviewer_id"`
		Status     string `json:"status"`
		Note       string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status == "" {
		writeError(w, http.StatusBadRequest, "reviewer_id, status required")
		return
	}
	appeal, err := ro.appeals.Get(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "appeal not found")
		return
	}
	if err := ro.appeals.Review(r.Context(), id, body.ReviewerID, body.Status, body.Note); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Fetch org_id via the punishment linked to this appeal
	var orgID string
	ro.db.QueryRowContext(r.Context(), `SELECT org_id FROM punishments WHERE id = ?`, appeal.PunishmentID).Scan(&orgID)
	audit.Log(r.Context(), ro.db, orgID, body.ReviewerID, "STAFF", "APPEAL_"+body.Status,
		appeal.SubmitterID, id, map[string]any{"note": body.Note}, r.RemoteAddr, "API")
	ro.webhooks.Dispatch(r.Context(), orgID, "appeal.updated", map[string]any{
		"appeal_id":      id,
		"punishment_id":  appeal.PunishmentID,
		"status":         body.Status,
		"reviewer_id":    body.ReviewerID,
	})
	w.WriteHeader(http.StatusNoContent)
}

// ── Reports ───────────────────────────────────────────────────────────────────

func (ro *Router) handleSubmitReport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrgID       string  `json:"org_id"`
		ReporterID  string  `json:"reporter_id"`
		TargetID    string  `json:"target_id"`
		Category    string  `json:"category"`
		Description string  `json:"description"`
		Evidence    *string `json:"evidence"`
		Platform    string  `json:"platform"`
		ServerID    *string `json:"server_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.OrgID == "" || body.ReporterID == "" || body.TargetID == "" {
		writeError(w, http.StatusBadRequest, "org_id, reporter_id, target_id, category, description required")
		return
	}
	ev := ""
	if body.Evidence != nil {
		ev = *body.Evidence
	}
	if err := validateReportInput(body.Description, ev); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rep, err := ro.reports.Submit(r.Context(), body.OrgID, body.ReporterID, body.TargetID, body.Category, body.Description, body.Evidence, body.Platform, body.ServerID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rep)
}

func (ro *Router) handleListReports(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	status := r.URL.Query().Get("status")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id query param required")
		return
	}
	list, err := ro.reports.List(r.Context(), orgID, status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (ro *Router) handleGetReport(w http.ResponseWriter, r *http.Request) {
	rep, err := ro.reports.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (ro *Router) handleClaimReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		StaffProfileID string `json:"staff_profile_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.StaffProfileID == "" {
		writeError(w, http.StatusBadRequest, "staff_profile_id required")
		return
	}
	if err := ro.reports.Claim(r.Context(), id, body.StaffProfileID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) handleResolveReport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		ResolutionType string  `json:"resolution_type"`
		PunishmentID   *string `json:"punishment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ResolutionType == "" {
		writeError(w, http.StatusBadRequest, "resolution_type required")
		return
	}
	if err := ro.reports.Resolve(r.Context(), id, body.ResolutionType, body.PunishmentID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Webhooks ──────────────────────────────────────────────────────────────────

func (ro *Router) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrgID  string   `json:"org_id"`
		Name   string   `json:"name"`
		URL    string   `json:"url"`
		Events []string `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.OrgID == "" || body.URL == "" {
		writeError(w, http.StatusBadRequest, "org_id, url, and events required")
		return
	}
	eventsJSON, _ := json.Marshal(body.Events)
	secret := randomSecret()
	_, err := ro.db.ExecContext(r.Context(), `
		INSERT INTO webhook_subscriptions (id, org_id, name, url, secret, events, enabled, created_at)
		VALUES (UUID(), ?, ?, ?, ?, ?, 1, NOW(3))`,
		body.OrgID, body.Name, body.URL, secret, string(eventsJSON),
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"status": "created",
		"secret": secret,
		"note":   "Store this secret — it will not be shown again.",
	})
}

func (ro *Router) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		writeError(w, http.StatusBadRequest, "org_id required")
		return
	}
	rows, err := ro.db.QueryxContext(r.Context(), `
		SELECT id, org_id, name, url, events, enabled, created_at
		FROM webhook_subscriptions WHERE org_id = ?`, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err == nil {
			out = append(out, row)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (ro *Router) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	ro.db.ExecContext(r.Context(), `DELETE FROM webhook_subscriptions WHERE id = ?`, r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func (ro *Router) handleTestWebhook(w http.ResponseWriter, r *http.Request) {
	var body struct{ OrgID string `json:"org_id"` }
	json.NewDecoder(r.Body).Decode(&body)
	ro.webhooks.Dispatch(r.Context(), body.OrgID, "test", webhook.TestPayload(body.OrgID))
	writeJSON(w, http.StatusOK, map[string]string{"status": "dispatched"})
}

// ── Auth (Discord OAuth2) ─────────────────────────────────────────────────────

func (ro *Router) handleDiscordLogin(w http.ResponseWriter, r *http.Request) {
	redirectURL, state, err := ro.oauth.AuthCodeURL()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate OAuth URL")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (ro *Router) handleDiscordCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, "invalid state parameter")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing code")
		return
	}
	token, _, err := ro.oauth.Exchange(r.Context(), code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "OAuth exchange failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "bm_session",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (ro *Router) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "bm_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (ro *Router) handleMe(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("bm_session")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "not logged in")
		return
	}
	sess := ro.oauth.Verify(cookie.Value)
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"discord_id": sess.DiscordID,
		"username":   sess.Username,
		"issued_at":  sess.IssuedAt,
	})
}
