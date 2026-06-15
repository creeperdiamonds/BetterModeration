package appeals

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Appeal struct {
	ID            string     `db:"id"             json:"id"`
	PunishmentID  string     `db:"punishment_id"  json:"punishment_id"`
	SubmitterID   string     `db:"submitter_id"   json:"submitter_id"`
	Reason        string     `db:"reason"         json:"reason"`
	Evidence      *string    `db:"evidence"       json:"evidence"`
	Status        string     `db:"status"         json:"status"`
	AssignedTo    *string    `db:"assigned_to"    json:"assigned_to"`
	ReviewerNote  *string    `db:"reviewer_note"  json:"reviewer_note"`
	SubmittedAt   time.Time  `db:"submitted_at"   json:"submitted_at"`
	UpdatedAt     time.Time  `db:"updated_at"     json:"updated_at"`
	ResolvedAt    *time.Time `db:"resolved_at"    json:"resolved_at"`
}

type Service struct {
	db *sqlx.DB
}

func NewService(db *sqlx.DB) *Service {
	return &Service{db: db}
}

const (
	// DeniedCooldownDays is how long a player must wait before re-appealing a denied punishment.
	DeniedCooldownDays = 14
	// MaxOpenAppeals is the maximum number of pending/under-review appeals a submitter may have at once.
	MaxOpenAppeals = 5
)

// Submit creates a new appeal for a punishment.
// Enforces: no duplicate open appeal, denial cooldown, and max-open-appeals cap.
func (s *Service) Submit(ctx context.Context, punishmentID, submitterProfileID, reason string, evidence *string) (*Appeal, error) {
	// Verify the punishment exists and belongs to this submitter
	var exists int
	err := s.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM punishments WHERE id = ? AND profile_id = ?`,
		punishmentID, submitterProfileID,
	).Scan(&exists)
	if err != nil || exists == 0 {
		return nil, fmt.Errorf("punishment not found or does not belong to this profile")
	}

	// Block duplicate: already an open appeal for this punishment
	var openCount int
	err = s.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM appeals WHERE punishment_id = ? AND status IN ('PENDING','UNDER_REVIEW')`,
		punishmentID,
	).Scan(&openCount)
	if err != nil {
		return nil, fmt.Errorf("checking existing appeal: %w", err)
	}
	if openCount > 0 {
		return nil, fmt.Errorf("an appeal for this punishment is already pending review")
	}

	// Enforce denial cooldown: check if the most recent appeal was DENIED within the cooldown window
	var lastDenied sql.NullTime
	err = s.db.QueryRowxContext(ctx,
		`SELECT MAX(resolved_at) FROM appeals WHERE punishment_id = ? AND status = 'DENIED'`,
		punishmentID,
	).Scan(&lastDenied)
	if err != nil {
		return nil, fmt.Errorf("checking denial cooldown: %w", err)
	}
	if lastDenied.Valid {
		cooldownEnd := lastDenied.Time.AddDate(0, 0, DeniedCooldownDays)
		if time.Now().UTC().Before(cooldownEnd) {
			daysLeft := int(time.Until(cooldownEnd).Hours()/24) + 1
			return nil, fmt.Errorf("appeal denied — you may re-appeal in %d day(s)", daysLeft)
		}
	}

	// Cap the number of open appeals per submitter across all punishments
	var totalOpen int
	err = s.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM appeals WHERE submitter_id = ? AND status IN ('PENDING','UNDER_REVIEW')`,
		submitterProfileID,
	).Scan(&totalOpen)
	if err != nil {
		return nil, fmt.Errorf("checking open appeal count: %w", err)
	}
	if totalOpen >= MaxOpenAppeals {
		return nil, fmt.Errorf("you have too many open appeals (%d/%d) — wait for existing ones to be reviewed", totalOpen, MaxOpenAppeals)
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO appeals (id, punishment_id, submitter_id, reason, evidence, status, submitted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'PENDING', ?, ?)`,
		id, punishmentID, submitterProfileID, reason, evidence, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("submit appeal: %w", err)
	}
	return &Appeal{
		ID:           id,
		PunishmentID: punishmentID,
		SubmitterID:  submitterProfileID,
		Reason:       reason,
		Evidence:     evidence,
		Status:       "PENDING",
		SubmittedAt:  now,
		UpdatedAt:    now,
	}, nil
}

// Get returns a single appeal by ID.
func (s *Service) Get(ctx context.Context, appealID string) (*Appeal, error) {
	var a Appeal
	err := s.db.QueryRowxContext(ctx,
		`SELECT id, punishment_id, submitter_id, reason, evidence, status, assigned_to, reviewer_note, submitted_at, updated_at, resolved_at
		 FROM appeals WHERE id = ?`, appealID,
	).StructScan(&a)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("appeal not found")
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// List returns appeals for an org filtered by status (empty string = all).
func (s *Service) List(ctx context.Context, orgID, status string) ([]Appeal, error) {
	query := `
		SELECT a.id, a.punishment_id, a.submitter_id, a.reason, a.evidence, a.status,
		       a.assigned_to, a.reviewer_note, a.submitted_at, a.updated_at, a.resolved_at
		FROM appeals a
		JOIN punishments p ON p.id = a.punishment_id
		WHERE p.org_id = ?`
	args := []any{orgID}
	if status != "" {
		query += ` AND a.status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY a.submitted_at DESC`

	rows, err := s.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Appeal
	for rows.Next() {
		var a Appeal
		if err := rows.StructScan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Review sets the appeal status and optional reviewer note.
// Valid statuses: APPROVED, DENIED, UNDER_REVIEW, ESCALATED.
func (s *Service) Review(ctx context.Context, appealID, reviewerID, status, note string) error {
	now := time.Now().UTC()
	var resolvedAt *time.Time
	if status == "APPROVED" || status == "DENIED" {
		resolvedAt = &now
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE appeals
		SET status = ?, assigned_to = ?, reviewer_note = ?, updated_at = ?, resolved_at = ?
		WHERE id = ?`,
		status, reviewerID, note, now, resolvedAt, appealID,
	)
	if err != nil {
		return fmt.Errorf("review appeal: %w", err)
	}
	return nil
}
