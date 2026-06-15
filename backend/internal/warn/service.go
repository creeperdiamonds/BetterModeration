package warn

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Threshold struct {
	ID              string
	WarnCount       int
	ActionType      string
	DurationSeconds *int64
	DecayDays       *int
}

type Punishment struct {
	ID        string
	Type      string
	ExpiresAt *time.Time
}

type WarnResult struct {
	PunishmentID  string
	ActiveWarns   int
	Threshold     *Threshold     // non-nil if a threshold was crossed
	AutoPunish    *Punishment    // non-nil if an auto-punishment was issued
}

type Service struct {
	db          *sqlx.DB
	onBanIssued func(profileID, orgID string)
}

func NewService(db *sqlx.DB) *Service {
	return &Service{db: db}
}

// SetBanHook registers a callback that is called asynchronously after any BAN
// punishment is persisted. Used to pre-compute offline UUIDs without creating
// a circular import between warn and evasion packages.
func (s *Service) SetBanHook(fn func(profileID, orgID string)) {
	s.onBanIssued = fn
}

// Issue writes a WARN punishment, counts active warns, checks thresholds,
// and issues an auto-punishment if one is triggered.
// Returns a WarnResult with everything the caller needs to respond and DM the user.
func (s *Service) Issue(ctx context.Context, orgID, profileID, issuedBy, reason, serverID string) (*WarnResult, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	warnID := uuid.NewString()
	now := time.Now().UTC()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO punishments
			(id, org_id, profile_id, type, reason, issued_by, issued_by_type,
			 issued_at, expires_at, platform, server_id, minecraft_active, public)
		VALUES (?, ?, ?, 'WARN', ?, ?, 'STAFF', ?, NULL, 'DISCORD', ?, 0, 1)`,
		warnID, orgID, profileID, reason, issuedBy, now, serverID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert warn: %w", err)
	}

	// Count active warns — respect decay if configured
	thresholds, err := s.loadThresholds(ctx, tx, orgID)
	if err != nil {
		return nil, fmt.Errorf("load thresholds: %w", err)
	}

	activeWarns, err := s.countActiveWarns(ctx, tx, orgID, profileID, thresholds)
	if err != nil {
		return nil, fmt.Errorf("count warns: %w", err)
	}

	result := &WarnResult{
		PunishmentID: warnID,
		ActiveWarns:  activeWarns,
	}

	// Check if any threshold is crossed
	crossed := s.crossedThreshold(activeWarns, thresholds)
	if crossed != nil {
		result.Threshold = crossed
		auto, err := s.issueAutoPunishment(ctx, tx, orgID, profileID, crossed, now)
		if err != nil {
			return nil, fmt.Errorf("auto punishment: %w", err)
		}
		result.AutoPunish = auto
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return result, nil
}

// CountActive returns the number of active (non-decayed) warns for a profile in an org.
func (s *Service) CountActive(ctx context.Context, orgID, profileID string) (int, error) {
	thresholds, err := s.loadThresholds(ctx, s.db, orgID)
	if err != nil {
		return 0, err
	}
	return s.countActiveWarns(ctx, s.db, orgID, profileID, thresholds)
}

// IssueDirect writes a non-WARN punishment (BAN/MUTE/KICK) to the DB and returns the record.
// The caller is responsible for applying the action on the relevant platform (Discord/Minecraft).
func (s *Service) IssueDirect(ctx context.Context, orgID, profileID, issuedBy, punishType, reason, serverID string, expiresAt *time.Time) (*PunishmentRecord, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("reason is required")
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO punishments
			(id, org_id, profile_id, type, reason, issued_by, issued_by_type,
			 issued_at, expires_at, platform, server_id, minecraft_active, public)
		VALUES (?, ?, ?, ?, ?, ?, 'API', ?, ?, 'SYSTEM', ?, 1, 1)`,
		id, orgID, profileID, punishType, reason, issuedBy, now, expiresAt, serverID,
	)
	if err != nil {
		return nil, fmt.Errorf("insert punishment: %w", err)
	}
	if punishType == "BAN" && s.onBanIssued != nil {
		go s.onBanIssued(profileID, orgID)
	}
	return &PunishmentRecord{
		ID:              id,
		OrgID:           orgID,
		ProfileID:       profileID,
		Type:            punishType,
		Reason:          reason,
		IssuedBy:        &issuedBy,
		IssuedByType:    "API",
		IssuedAt:        now,
		ExpiresAt:       expiresAt,
		MinecraftActive: true,
		Public:          true,
	}, nil
}

// GetPunishments returns all punishments for a profile.
func (s *Service) GetPunishments(ctx context.Context, profileID string) ([]PunishmentRecord, error) {
	rows, err := s.db.QueryxContext(ctx, `
		SELECT id, org_id, profile_id, type, reason, issued_by, issued_by_type,
		       issued_at, expires_at, revoked_at, revoke_reason, minecraft_active, public
		FROM punishments
		WHERE profile_id = ?
		ORDER BY issued_at DESC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PunishmentRecord
	for rows.Next() {
		var p PunishmentRecord
		if err := rows.StructScan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPunishment returns a single punishment by ID.
func (s *Service) GetPunishment(ctx context.Context, id string) (*PunishmentRecord, error) {
	var p PunishmentRecord
	err := s.db.QueryRowxContext(ctx, `
		SELECT id, org_id, profile_id, type, reason, issued_by, issued_by_type,
		       issued_at, expires_at, revoked_at, revoke_reason, minecraft_active, public
		FROM punishments WHERE id = ?`, id).StructScan(&p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Revoke marks a punishment as revoked.
func (s *Service) Revoke(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE punishments SET minecraft_active = 0, revoked_at = NOW(3), revoke_reason = 'Manually revoked'
		WHERE id = ?`, id)
	return err
}

// PunishmentRecord is the full DB row returned by API queries.
type PunishmentRecord struct {
	ID             string     `db:"id"              json:"id"`
	OrgID          string     `db:"org_id"          json:"org_id"`
	ProfileID      string     `db:"profile_id"      json:"profile_id"`
	Type           string     `db:"type"            json:"type"`
	Reason         string     `db:"reason"          json:"reason"`
	IssuedBy       *string    `db:"issued_by"       json:"issued_by"`
	IssuedByType   string     `db:"issued_by_type"  json:"issued_by_type"`
	IssuedAt       time.Time  `db:"issued_at"       json:"issued_at"`
	ExpiresAt      *time.Time `db:"expires_at"      json:"expires_at"`
	RevokedAt      *time.Time `db:"revoked_at"      json:"revoked_at"`
	RevokeReason   *string    `db:"revoke_reason"   json:"revoke_reason"`
	MinecraftActive bool      `db:"minecraft_active" json:"minecraft_active"`
	Public         bool       `db:"public"          json:"public"`
}

// --- internal ---

type querier interface {
	QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error)
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

func (s *Service) loadThresholds(ctx context.Context, q querier, orgID string) ([]Threshold, error) {
	rows, err := q.QueryxContext(ctx, `
		SELECT id, warn_count, action_type, duration_seconds, decay_days
		FROM warning_thresholds
		WHERE org_id = ?
		ORDER BY warn_count ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Threshold
	for rows.Next() {
		var t Threshold
		if err := rows.Scan(&t.ID, &t.WarnCount, &t.ActionType, &t.DurationSeconds, &t.DecayDays); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) countActiveWarns(ctx context.Context, q querier, orgID, profileID string, thresholds []Threshold) (int, error) {
	// Use the shortest decay window across all thresholds so we're conservative.
	// If any threshold has no decay (nil), warns never decay → use that.
	var decayCutoff *time.Time
	for _, t := range thresholds {
		if t.DecayDays == nil {
			decayCutoff = nil
			break
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -*t.DecayDays)
		if decayCutoff == nil || cutoff.Before(*decayCutoff) {
			decayCutoff = &cutoff
		}
	}

	query := `
		SELECT COUNT(*) FROM punishments
		WHERE org_id = ? AND profile_id = ? AND type = 'WARN' AND minecraft_active = 0`
	args := []interface{}{orgID, profileID}

	if decayCutoff != nil {
		query += ` AND issued_at >= ?`
		args = append(args, *decayCutoff)
	}

	rows, err := q.QueryxContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var count int
	if rows.Next() {
		rows.Scan(&count)
	}
	return count, rows.Err()
}

func (s *Service) crossedThreshold(activeWarns int, thresholds []Threshold) *Threshold {
	// Find the highest threshold that the current warn count exactly hits.
	var match *Threshold
	for i := range thresholds {
		if activeWarns == thresholds[i].WarnCount {
			match = &thresholds[i]
		}
	}
	return match
}

func (s *Service) issueAutoPunishment(ctx context.Context, tx *sqlx.Tx, orgID, profileID string, t *Threshold, now time.Time) (*Punishment, error) {
	id := uuid.NewString()

	var expiresAt *time.Time
	if t.DurationSeconds != nil {
		exp := now.Add(time.Duration(*t.DurationSeconds) * time.Second)
		expiresAt = &exp
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO punishments
			(id, org_id, profile_id, type, reason, issued_by, issued_by_type,
			 issued_at, expires_at, platform, server_id, minecraft_active, public)
		VALUES (?, ?, ?, ?, 'Automatic: warning threshold reached', NULL, 'SYSTEM', ?, ?, 'SYSTEM', NULL, 1, 1)`,
		id, orgID, profileID, t.ActionType, now, expiresAt,
	)
	if err != nil {
		return nil, err
	}

	if t.ActionType == "BAN" && s.onBanIssued != nil {
		go s.onBanIssued(profileID, orgID)
	}

	return &Punishment{
		ID:        id,
		Type:      t.ActionType,
		ExpiresAt: expiresAt,
	}, nil
}
