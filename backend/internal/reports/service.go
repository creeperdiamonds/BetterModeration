package reports

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Report struct {
	ID             string     `db:"id"              json:"id"`
	OrgID          string     `db:"org_id"          json:"org_id"`
	ReporterID     string     `db:"reporter_id"     json:"reporter_id"`
	TargetID       string     `db:"target_id"       json:"target_id"`
	ReasonCategory string     `db:"reason_category" json:"reason_category"`
	Description    string     `db:"description"     json:"description"`
	Evidence       *string    `db:"evidence"        json:"evidence"`
	Platform       string     `db:"platform"        json:"platform"`
	ServerID       *string    `db:"server_id"       json:"server_id"`
	Status         string     `db:"status"          json:"status"`
	ClaimedBy      *string    `db:"claimed_by"      json:"claimed_by"`
	ResolutionType *string    `db:"resolution_type" json:"resolution_type"`
	PunishmentID   *string    `db:"punishment_id"   json:"punishment_id"`
	SubmittedAt    time.Time  `db:"submitted_at"    json:"submitted_at"`
	UpdatedAt      time.Time  `db:"updated_at"      json:"updated_at"`
	ResolvedAt     *time.Time `db:"resolved_at"     json:"resolved_at"`
}

type Service struct {
	db *sqlx.DB
}

func NewService(db *sqlx.DB) *Service {
	return &Service{db: db}
}

const (
	// maxReportsPerHour is the maximum number of reports a single profile may submit per hour.
	maxReportsPerHour = 5
	// duplicateReportWindow is how long before the same reporter+target+category is considered a duplicate.
	duplicateReportWindow = 24 * time.Hour
)

// Submit creates a new player report.
// Enforces: no self-report, duplicate prevention, and hourly submission cap.
func (s *Service) Submit(ctx context.Context, orgID, reporterID, targetID, category, description string, evidence *string, platform string, serverID *string) (*Report, error) {
	if reporterID == targetID {
		return nil, fmt.Errorf("cannot report yourself")
	}

	// Prevent duplicate: same reporter+target+category within the dedup window
	cutoff := time.Now().UTC().Add(-duplicateReportWindow)
	var dupCount int
	err := s.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM reports
		 WHERE org_id = ? AND reporter_id = ? AND target_id = ? AND reason_category = ?
		   AND submitted_at >= ?`,
		orgID, reporterID, targetID, category, cutoff,
	).Scan(&dupCount)
	if err != nil {
		return nil, fmt.Errorf("checking duplicate report: %w", err)
	}
	if dupCount > 0 {
		return nil, fmt.Errorf("you have already submitted a %s report against this player recently — please wait 24 hours", category)
	}

	// Rate limit: max reports per hour per reporter in this org
	hourAgo := time.Now().UTC().Add(-time.Hour)
	var recentCount int
	err = s.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM reports WHERE org_id = ? AND reporter_id = ? AND submitted_at >= ?`,
		orgID, reporterID, hourAgo,
	).Scan(&recentCount)
	if err != nil {
		return nil, fmt.Errorf("checking report rate: %w", err)
	}
	if recentCount >= maxReportsPerHour {
		return nil, fmt.Errorf("you are submitting reports too quickly — maximum %d per hour", maxReportsPerHour)
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO reports
			(id, org_id, reporter_id, target_id, reason_category, description, evidence,
			 platform, server_id, status, submitted_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'OPEN', ?, ?)`,
		id, orgID, reporterID, targetID, category, description, evidence, platform, serverID, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("submit report: %w", err)
	}
	return &Report{
		ID:             id,
		OrgID:          orgID,
		ReporterID:     reporterID,
		TargetID:       targetID,
		ReasonCategory: category,
		Description:    description,
		Evidence:       evidence,
		Platform:       platform,
		ServerID:       serverID,
		Status:         "OPEN",
		SubmittedAt:    now,
		UpdatedAt:      now,
	}, nil
}

// Get returns a single report by ID.
func (s *Service) Get(ctx context.Context, reportID string) (*Report, error) {
	var r Report
	err := s.db.QueryRowxContext(ctx, `
		SELECT id, org_id, reporter_id, target_id, reason_category, description, evidence,
		       platform, server_id, status, claimed_by, resolution_type, punishment_id,
		       submitted_at, updated_at, resolved_at
		FROM reports WHERE id = ?`, reportID,
	).StructScan(&r)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("report not found")
	}
	return &r, err
}

// List returns reports for an org, optionally filtered by status.
func (s *Service) List(ctx context.Context, orgID, status string) ([]Report, error) {
	query := `
		SELECT id, org_id, reporter_id, target_id, reason_category, description, evidence,
		       platform, server_id, status, claimed_by, resolution_type, punishment_id,
		       submitted_at, updated_at, resolved_at
		FROM reports WHERE org_id = ?`
	args := []any{orgID}
	if status != "" {
		query += ` AND status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY submitted_at DESC`

	rows, err := s.db.QueryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Report
	for rows.Next() {
		var r Report
		if err := rows.StructScan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Claim assigns a report to a staff member.
func (s *Service) Claim(ctx context.Context, reportID, staffProfileID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE reports SET status = 'CLAIMED', claimed_by = ?, updated_at = NOW(3)
		WHERE id = ? AND status = 'OPEN'`,
		staffProfileID, reportID,
	)
	return err
}

// Resolve closes a report with a resolution type and optional punishment.
func (s *Service) Resolve(ctx context.Context, reportID, resolutionType string, punishmentID *string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE reports
		SET status = 'RESOLVED', resolution_type = ?, punishment_id = ?, updated_at = NOW(3), resolved_at = NOW(3)
		WHERE id = ?`,
		resolutionType, punishmentID, reportID,
	)
	return err
}
