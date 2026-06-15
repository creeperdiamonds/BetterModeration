package profile

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Profile struct {
	ID            string     `db:"id"`
	DiscordID     *string    `db:"discord_id"`
	MinecraftUUID *string    `db:"minecraft_uuid"`
	LinkedAt      *time.Time `db:"linked_at"`
	CreatedAt     time.Time  `db:"created_at"`
}

type OrgContext struct {
	OrgID    string
	ServerID string
}

type Service struct {
	db *sqlx.DB
}

func NewService(db *sqlx.DB) *Service {
	return &Service{db: db}
}

// DB exposes the underlying database connection for use by callers that need
// to write related records (e.g. the audit log handler writing punishments).
func (s *Service) DB() *sqlx.DB { return s.db }

// ResolveByDiscord returns the profile for a Discord user ID, creating one if it doesn't exist.
func (s *Service) ResolveByDiscord(ctx context.Context, discordID string) (*Profile, error) {
	var p Profile
	err := s.db.QueryRowxContext(ctx,
		`SELECT id, discord_id, minecraft_uuid, linked_at, created_at FROM profiles WHERE discord_id = ?`,
		discordID,
	).StructScan(&p)
	if err == nil {
		return &p, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("query profile: %w", err)
	}
	return s.create(ctx, &discordID, nil)
}

// ResolveByMinecraft returns the profile for a Minecraft UUID, creating one if it doesn't exist.
func (s *Service) ResolveByMinecraft(ctx context.Context, minecraftUUID string) (*Profile, error) {
	var p Profile
	err := s.db.QueryRowxContext(ctx,
		`SELECT id, discord_id, minecraft_uuid, linked_at, created_at FROM profiles WHERE minecraft_uuid = ?`,
		minecraftUUID,
	).StructScan(&p)
	if err == nil {
		return &p, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("query profile: %w", err)
	}
	return s.create(ctx, nil, &minecraftUUID)
}

// OrgForGuild returns the org and server IDs for a Discord guild.
func (s *Service) OrgForGuild(ctx context.Context, guildID string) (*OrgContext, error) {
	var out OrgContext
	err := s.db.QueryRowxContext(ctx,
		`SELECT org_id, id AS server_id FROM servers WHERE platform = 'DISCORD' AND platform_id = ?`,
		guildID,
	).Scan(&out.OrgID, &out.ServerID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("guild %s not linked", guildID)
	}
	if err != nil {
		return nil, fmt.Errorf("org lookup: %w", err)
	}
	return &out, nil
}

// TrackIP records a player's IP address for alt detection.
func (s *Service) TrackIP(ctx context.Context, profileID, ip string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO profile_ips (profile_id, ip_address, first_seen, last_seen)
		VALUES (?, ?, NOW(3), NOW(3))
		ON DUPLICATE KEY UPDATE last_seen = NOW(3)`,
		profileID, ip,
	)
	return err
}

// IsIPBanned returns true if the given IP is linked to an active BAN in the given org.
// When true, it also returns the ban reason and expiry (nil = permanent).
func (s *Service) IsIPBanned(ctx context.Context, ip, orgID string) (bool, string, *time.Time, error) {
	var reason string
	var expiresAt *time.Time
	err := s.db.QueryRowxContext(ctx, `
		SELECT p.reason, p.expires_at
		FROM profile_ips pi
		JOIN punishments p ON p.profile_id = pi.profile_id
		WHERE pi.ip_address = ?
		  AND p.org_id = ?
		  AND p.type = 'BAN'
		  AND p.minecraft_active = 1
		  AND (p.expires_at IS NULL OR p.expires_at > NOW(3))
		LIMIT 1`,
		ip, orgID,
	).Scan(&reason, &expiresAt)
	if err == sql.ErrNoRows {
		return false, "", nil, nil
	}
	if err != nil {
		return false, "", nil, err
	}
	return true, reason, expiresAt, nil
}

// FindAlts returns profile IDs that share an IP with the given profile.
func (s *Service) FindAlts(ctx context.Context, profileID string) ([]string, error) {
	rows, err := s.db.QueryxContext(ctx, `
		SELECT DISTINCT pi2.profile_id
		FROM profile_ips pi1
		JOIN profile_ips pi2 ON pi2.ip_address = pi1.ip_address
		WHERE pi1.profile_id = ? AND pi2.profile_id != ?`,
		profileID, profileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetByID returns a profile by its internal UUID.
func (s *Service) GetByID(ctx context.Context, id string) (*Profile, error) {
	var p Profile
	err := s.db.QueryRowxContext(ctx,
		`SELECT id, discord_id, minecraft_uuid, linked_at, created_at FROM profiles WHERE id = ?`,
		id,
	).StructScan(&p)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return &p, nil
}

// UpdateServerStatus sets the online flag for a Minecraft server.
func (s *Service) UpdateServerStatus(ctx context.Context, serverID string, online bool) error {
	onlineInt := 0
	if online {
		onlineInt = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE servers SET online = ? WHERE id = ?`,
		onlineInt, serverID,
	)
	return err
}

func (s *Service) create(ctx context.Context, discordID, minecraftUUID *string) (*Profile, error) {
	p := &Profile{
		ID:            uuid.NewString(),
		DiscordID:     discordID,
		MinecraftUUID: minecraftUUID,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO profiles (id, discord_id, minecraft_uuid) VALUES (?, ?, ?)`,
		p.ID, discordID, minecraftUUID,
	)
	if err != nil {
		return nil, fmt.Errorf("create profile: %w", err)
	}
	return p, nil
}
