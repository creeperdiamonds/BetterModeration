package evasion

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	bmsync "creeperdiamonds.xyz/bettermoderation/internal/sync"
)

// Service orchestrates all offline-mode evasion detection signals and produces
// a join decision (ALLOW / FLAG / DENY) for every player login.
type Service struct {
	db  *sqlx.DB
	bus *bmsync.EventBus
}

func NewService(db *sqlx.DB, bus *bmsync.EventBus) *Service {
	return &Service{db: db, bus: bus}
}

// NewConnectRequest is a convenience constructor for the HTTP handler.
func (s *Service) NewConnectRequest(uuid, username, ip, orgID, serverID string, offline bool) ConnectRequest {
	return ConnectRequest{
		UUID:        uuid,
		Username:    username,
		IP:          ip,
		OrgID:       orgID,
		ServerID:    serverID,
		OfflineMode: offline,
	}
}

// ConnectRequest holds all information about a player attempting to join.
type ConnectRequest struct {
	UUID        string // as sent by the Minecraft client
	Username    string
	IP          string
	OrgID       string
	ServerID    string
	OfflineMode bool
}

// ConnectResult is returned to the HTTP handler and forwarded to the plugin.
type ConnectResult struct {
	Action         string   // "ALLOW" | "FLAG" | "DENY"
	KickMessage    string   // only set when Action == "DENY"
	SuspicionScore int
	Flags          []string
	ProfileID      string
}

// Connect is the main entry point called on every player join.
// It resolves or creates the player's profile, tracks their IP and username,
// logs the join event, assembles all evasion signals, scores them, and returns
// the recommended action. Side effects (IP enrichment, Redis event) are async.
func (s *Service) Connect(ctx context.Context, req ConnectRequest) (*ConnectResult, error) {
	// 1. Resolve profile by UUID (creates one if new player)
	profileID, err := s.resolveOrCreateProfile(ctx, req.UUID)
	if err != nil {
		return nil, fmt.Errorf("resolve profile: %w", err)
	}

	// 2. Track IP and username (best-effort)
	if req.IP != "" && !IsPrivateIP(req.IP) {
		if err := s.trackIP(ctx, profileID, req.IP); err != nil {
			log.Printf("[evasion] trackIP: %v", err)
		}
	}
	if req.Username != "" {
		if err := s.TrackUsername(ctx, profileID, req.Username); err != nil {
			log.Printf("[evasion] TrackUsername: %v", err)
		}
	}

	// 3. Log the join event
	if err := s.LogJoin(ctx, profileID, req.OrgID, req.ServerID, req.Username, req.IP, req.OfflineMode); err != nil {
		log.Printf("[evasion] LogJoin: %v", err)
	}

	// 4. Kick off IP enrichment asynchronously — never blocks the join path
	if req.IP != "" && !IsPrivateIP(req.IP) {
		go GetOrFetchMeta(context.Background(), s.db, req.IP)
	}

	// 5. Assemble signals
	sig, err := s.assembleSignals(ctx, req, profileID)
	if err != nil {
		log.Printf("[evasion] assembleSignals error (failing open): %v", err)
		return &ConnectResult{Action: ActionAllow, ProfileID: profileID}, nil
	}

	// 6. Score
	scored := Score(sig)

	cr := &ConnectResult{
		Action:         scored.Action,
		SuspicionScore: scored.Score,
		Flags:          scored.Flags,
		ProfileID:      profileID,
	}

	if scored.Action == ActionDeny {
		cr.KickMessage = buildKickMessage(scored.Flags)
	}

	// 7. Publish Redis event on FLAG or DENY so the Discord bot can alert staff
	if scored.Action == ActionFlag || scored.Action == ActionDeny {
		go s.publishFlaggedEvent(req, profileID, scored)
	}

	return cr, nil
}

// TrackUsername upserts a username into player_usernames.
func (s *Service) TrackUsername(ctx context.Context, profileID, username string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO player_usernames (profile_id, username, first_seen, last_seen)
		VALUES (?, ?, NOW(3), NOW(3))
		ON DUPLICATE KEY UPDATE last_seen = NOW(3)`,
		profileID, username,
	)
	return err
}

// LogJoin appends a row to join_events.
func (s *Service) LogJoin(ctx context.Context, profileID, orgID, serverID, username, ip string, offline bool) error {
	offlineInt := 0
	if offline {
		offlineInt = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO join_events (profile_id, org_id, server_id, username, ip_address, offline_mode, joined_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(3))`,
		profileID, orgID, serverID, username, ip, offlineInt,
	)
	return err
}

// ComputeAndStoreBannedOfflineUUIDs computes offline UUIDs for every known username
// of the given banned profile and inserts them into banned_offline_uuids.
// Uses INSERT IGNORE — safe to call multiple times.
func (s *Service) ComputeAndStoreBannedOfflineUUIDs(ctx context.Context, profileID, orgID string) error {
	rows, err := s.db.QueryxContext(ctx, `
		SELECT username FROM player_usernames WHERE profile_id = ?`, profileID)
	if err != nil {
		return fmt.Errorf("query usernames: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var username string
		if err := rows.Scan(&username); err != nil {
			continue
		}
		offlineUUID := OfflinePlayerUUID(username)
		_, err := s.db.ExecContext(ctx, `
			INSERT IGNORE INTO banned_offline_uuids (offline_uuid, profile_id, org_id, username, computed_at)
			VALUES (?, ?, ?, ?, NOW(3))`,
			offlineUUID, profileID, orgID, username,
		)
		if err != nil {
			log.Printf("[evasion] insert banned_offline_uuids (%s): %v", username, err)
		}
	}
	return rows.Err()
}

// ── private helpers ────────────────────────────────────────────────────────

func (s *Service) trackIP(ctx context.Context, profileID, ip string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO profile_ips (profile_id, ip_address, first_seen, last_seen)
		VALUES (?, ?, NOW(3), NOW(3))
		ON DUPLICATE KEY UPDATE last_seen = NOW(3)`,
		profileID, ip,
	)
	return err
}

func (s *Service) resolveOrCreateProfile(ctx context.Context, minecraftUUID string) (string, error) {
	var profileID string
	err := s.db.QueryRowxContext(ctx,
		`SELECT id FROM profiles WHERE minecraft_uuid = ?`, minecraftUUID,
	).Scan(&profileID)
	if err == nil {
		return profileID, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	newID := uuid.NewString()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO profiles (id, minecraft_uuid, created_at) VALUES (?, ?, NOW(3))`,
		newID, minecraftUUID,
	)
	if err != nil {
		return "", fmt.Errorf("create profile: %w", err)
	}
	return newID, nil
}

func (s *Service) assembleSignals(ctx context.Context, req ConnectRequest, profileID string) (Signals, error) {
	var sig Signals

	// VPN / hosting / ASN signals from the enrichment cache
	if meta, _ := GetOrFetchMeta(ctx, s.db, req.IP); meta != nil {
		sig.IsVPN = meta.IsVPN
		sig.IsProxy = meta.IsProxy
		sig.IsHosting = meta.IsHosting
		if meta.ASN != "" {
			sig.ASNMatchesBanned = s.asnMatchesBanned(ctx, meta.ASN, req.OrgID)
		}
	}

	// Direct IP ban
	sig.IPBanned = s.ipBanned(ctx, req.IP, req.OrgID)

	// Offline UUID pre-computation match
	sig.OfflineUUIDMatch = s.offlineUUIDBanned(ctx, req.UUID, req.OrgID)

	// Username signals (in-process Levenshtein against banned username set)
	bannedUsernames, _ := s.getBannedUsernames(ctx, req.OrgID)
	if len(bannedUsernames) > 0 && req.Username != "" {
		sig.UsernameExact, sig.UsernameLevenshtein, sig.UsernamePrefix =
			s.usernameSignals(ctx, req.Username, profileID, req.OrgID, bannedUsernames)
	}

	// /24 subnet match (IPv4 only)
	sig.SubnetMatch = s.subnetMatchesBanned(ctx, req.IP, req.OrgID)

	// Brand new account — count includes the row we just inserted, so threshold is ≤ 1
	var joinCount int
	s.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM join_events WHERE profile_id = ? AND org_id = ?`,
		profileID, req.OrgID).Scan(&joinCount)
	sig.BrandNewAccount = joinCount <= 1

	// Time correlation: BAN issued against an IP-sharing profile in the last 10 min
	if req.IP != "" && !IsPrivateIP(req.IP) {
		var recentBan int
		s.db.QueryRowxContext(ctx, `
			SELECT COUNT(*)
			FROM punishments p
			JOIN profile_ips pi ON pi.profile_id = p.profile_id
			WHERE pi.ip_address = ?
			  AND p.org_id = ?
			  AND p.type = 'BAN'
			  AND p.issued_at >= NOW(3) - INTERVAL 10 MINUTE`,
			req.IP, req.OrgID).Scan(&recentBan)
		sig.TimeCorrelation = recentBan > 0
	}

	return sig, nil
}

func (s *Service) ipBanned(ctx context.Context, ip, orgID string) bool {
	if ip == "" || IsPrivateIP(ip) {
		return false
	}
	var count int
	s.db.QueryRowxContext(ctx, `
		SELECT COUNT(*)
		FROM profile_ips pi
		JOIN punishments p ON p.profile_id = pi.profile_id
		WHERE pi.ip_address = ?
		  AND p.org_id = ?
		  AND p.type = 'BAN'
		  AND p.minecraft_active = 1
		  AND (p.expires_at IS NULL OR p.expires_at > NOW(3))`,
		ip, orgID).Scan(&count)
	return count > 0
}

func (s *Service) offlineUUIDBanned(ctx context.Context, offlineUUID, orgID string) bool {
	var count int
	s.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM banned_offline_uuids WHERE offline_uuid = ? AND org_id = ?`,
		offlineUUID, orgID).Scan(&count)
	return count > 0
}

func (s *Service) asnMatchesBanned(ctx context.Context, asn, orgID string) bool {
	var count int
	s.db.QueryRowxContext(ctx, `
		SELECT COUNT(*)
		FROM ip_metadata im
		JOIN profile_ips pi ON pi.ip_address = im.ip_address
		JOIN punishments p  ON p.profile_id  = pi.profile_id
		WHERE im.asn = ?
		  AND p.org_id = ?
		  AND p.type = 'BAN'
		  AND p.minecraft_active = 1
		  AND (p.expires_at IS NULL OR p.expires_at > NOW(3))`,
		asn, orgID).Scan(&count)
	return count > 0
}

func (s *Service) subnetMatchesBanned(ctx context.Context, ip, orgID string) bool {
	if ip == "" || IsPrivateIP(ip) || !strings.Contains(ip, ".") {
		return false
	}
	var count int
	s.db.QueryRowxContext(ctx, `
		SELECT COUNT(*)
		FROM profile_ips pi
		JOIN punishments p ON p.profile_id = pi.profile_id
		WHERE (INET_ATON(pi.ip_address) & 0xFFFFFF00) = (INET_ATON(?) & 0xFFFFFF00)
		  AND p.org_id = ?
		  AND p.type = 'BAN'
		  AND p.minecraft_active = 1
		  AND (p.expires_at IS NULL OR p.expires_at > NOW(3))`,
		ip, orgID).Scan(&count)
	return count > 0
}

func (s *Service) getBannedUsernames(ctx context.Context, orgID string) ([]string, error) {
	rows, err := s.db.QueryxContext(ctx, `
		SELECT DISTINCT pu.username
		FROM player_usernames pu
		JOIN punishments p ON p.profile_id = pu.profile_id
		WHERE p.org_id = ?
		  AND p.type = 'BAN'
		  AND p.minecraft_active = 1
		  AND (p.expires_at IS NULL OR p.expires_at > NOW(3))`,
		orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

func (s *Service) usernameSignals(ctx context.Context, username, profileID, orgID string, banned []string) (exact, lev, prefix bool) {
	lowerIn := strings.ToLower(username)
	runes := []rune(username)
	prefixLen := 6
	if len(runes) < prefixLen {
		prefixLen = len(runes)
	}
	inPrefix := strings.ToLower(string(runes[:prefixLen]))

	// Brand new check for Levenshtein guard
	var joinCount int
	s.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM join_events WHERE profile_id = ? AND org_id = ?`,
		profileID, orgID).Scan(&joinCount)
	isNew := joinCount <= 1

	for _, b := range banned {
		lowerB := strings.ToLower(b)

		if lowerIn == lowerB {
			exact = true
			continue
		}

		rb := []rune(b)
		if len(rb) >= prefixLen && strings.ToLower(string(rb[:prefixLen])) == inPrefix {
			prefix = true
		}

		if isNew && !lev && Levenshtein(lowerIn, lowerB) <= 2 {
			lev = true
		}
	}
	return
}

func (s *Service) publishFlaggedEvent(req ConnectRequest, profileID string, result ScoreResult) {
	evt := bmsync.JoinFlaggedEvent{
		ProfileID:      profileID,
		MinecraftUUID:  req.UUID,
		Username:       req.Username,
		IP:             req.IP,
		OrgID:          req.OrgID,
		ServerID:       req.ServerID,
		SuspicionScore: result.Score,
		Flags:          result.Flags,
		Action:         result.Action,
		JoinedAt:       time.Now().UTC(),
	}
	b, err := json.Marshal(evt)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.bus.Publish(ctx, bmsync.ChannelJoinFlagged, string(b)); err != nil {
		log.Printf("[evasion] failed to publish flagged join event: %v", err)
	}
}

func buildKickMessage(flags []string) string {
	for _, f := range flags {
		switch f {
		case FlagIPBanned, FlagOfflineUUIDMatch, FlagUsernameExactBanned:
			return "§cYou are banned from this server.\n§7Appeal at: §bhttps://bettermoderation.dev/appeal"
		}
	}
	return "§cYou have been denied access to this server.\n§7Suspicious activity detected.\n§7If you believe this is a mistake, contact staff."
}
