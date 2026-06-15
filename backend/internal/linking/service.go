package linking

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Service struct {
	db *sqlx.DB
}

func NewService(db *sqlx.DB) *Service {
	return &Service{db: db}
}

// GenerateServerCode creates a one-time linking code for a Discord guild.
// The admin invites the bot, calls this, gets the code, runs /bmsetup <code> in-game.
func (s *Service) GenerateServerCode(ctx context.Context, guildID, ownerDiscordID string) (string, error) {
	// Ensure org exists for this guild, create one if not
	orgID, err := s.ensureOrg(ctx, guildID, ownerDiscordID)
	if err != nil {
		return "", fmt.Errorf("ensure org: %w", err)
	}

	code, err := randomCode(12)
	if err != nil {
		return "", err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO link_codes (code, type, org_id, profile_id, platform_id, expires_at, used)
		VALUES (?, 'SERVER_LINK', ?, NULL, ?, DATE_ADD(NOW(3), INTERVAL 15 MINUTE), 0)`,
		code, orgID, guildID,
	)
	if err != nil {
		return "", fmt.Errorf("insert link code: %w", err)
	}

	return code, nil
}

// RedeemServerCode is called when a Minecraft server operator runs /bmsetup <code>.
// Links the Minecraft server to the org and registers it in the servers table.
func (s *Service) RedeemServerCode(ctx context.Context, code, minecraftServerID, serverName, platform string) (string, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var row struct {
		OrgID      string    `db:"org_id"`
		PlatformID string    `db:"platform_id"`
		ExpiresAt  time.Time `db:"expires_at"`
		Used       bool      `db:"used"`
	}
	err = tx.QueryRowxContext(ctx,
		`SELECT org_id, platform_id, expires_at, used FROM link_codes WHERE code = ? AND type = 'SERVER_LINK'`,
		code,
	).StructScan(&row)
	if err != nil {
		return "", fmt.Errorf("code not found")
	}
	if row.Used {
		return "", fmt.Errorf("code already used")
	}
	if time.Now().After(row.ExpiresAt) {
		return "", fmt.Errorf("code expired")
	}

	// Generate an API key for this Minecraft server to authenticate with the backend
	apiKey, err := randomCode(32)
	if err != nil {
		return "", err
	}

	serverID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO servers (id, org_id, name, platform, platform_id, api_key_hash, online, linked_at)
		VALUES (?, ?, ?, ?, ?, SHA2(?, 256), 0, NOW(3))`,
		serverID, row.OrgID, serverName, platform, minecraftServerID, apiKey,
	)
	if err != nil {
		return "", fmt.Errorf("insert server: %w", err)
	}

	_, err = tx.ExecContext(ctx, `UPDATE link_codes SET used = 1 WHERE code = ?`, code)
	if err != nil {
		return "", fmt.Errorf("mark code used: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	// Return the raw API key — only shown once, server stores it in config
	return apiKey, nil
}

// GeneratePlayerCode creates a code for a player to link their Minecraft account to their Discord.
func (s *Service) GeneratePlayerCode(ctx context.Context, profileID, minecraftUUID string) (string, error) {
	code, err := randomCode(8)
	if err != nil {
		return "", err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO link_codes (code, type, org_id, profile_id, platform_id, expires_at, used)
		VALUES (?, 'PLAYER_LINK', NULL, ?, ?, DATE_ADD(NOW(3), INTERVAL 10 MINUTE), 0)
		ON DUPLICATE KEY UPDATE code = VALUES(code), expires_at = VALUES(expires_at), used = 0`,
		code, profileID, minecraftUUID,
	)
	if err != nil {
		return "", fmt.Errorf("insert player link code: %w", err)
	}

	return code, nil
}

// RedeemPlayerCode links a Minecraft UUID to an existing profile (called from website after Discord OAuth2).
func (s *Service) RedeemPlayerCode(ctx context.Context, code, discordID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var row struct {
		ProfileID  string    `db:"profile_id"`
		PlatformID string    `db:"platform_id"` // minecraft UUID
		ExpiresAt  time.Time `db:"expires_at"`
		Used       bool      `db:"used"`
	}
	err = tx.QueryRowxContext(ctx,
		`SELECT profile_id, platform_id, expires_at, used FROM link_codes WHERE code = ? AND type = 'PLAYER_LINK'`,
		code,
	).StructScan(&row)
	if err != nil {
		return fmt.Errorf("code not found")
	}
	if row.Used {
		return fmt.Errorf("code already used")
	}
	if time.Now().After(row.ExpiresAt) {
		return fmt.Errorf("code expired")
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE profiles SET discord_id = ?, linked_at = NOW(3) WHERE id = ?`,
		discordID, row.ProfileID,
	)
	if err != nil {
		return fmt.Errorf("link profile: %w", err)
	}

	_, err = tx.ExecContext(ctx, `UPDATE link_codes SET used = 1 WHERE code = ?`, code)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Service) ensureOrg(ctx context.Context, guildID, ownerDiscordID string) (string, error) {
	var orgID string
	err := s.db.QueryRowxContext(ctx,
		`SELECT o.id FROM organizations o
		 JOIN servers sv ON sv.org_id = o.id
		 WHERE sv.platform = 'DISCORD' AND sv.platform_id = ?`, guildID,
	).Scan(&orgID)
	if err == nil {
		return orgID, nil
	}

	// Create new org + Discord server entry
	orgID = uuid.NewString()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO organizations (id, name, owner_discord_id) VALUES (?, ?, ?)`,
		orgID, "New Server", ownerDiscordID,
	)
	if err != nil {
		return "", err
	}

	discordServerID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO servers (id, org_id, name, platform, platform_id, api_key_hash, online, linked_at)
		VALUES (?, ?, 'Discord', 'DISCORD', ?, '', 1, NOW(3))`,
		discordServerID, orgID, guildID,
	)
	if err != nil {
		return "", err
	}

	return orgID, tx.Commit()
}

func randomCode(n int) (string, error) {
	b := make([]byte, n/2+1)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:n], nil
}
