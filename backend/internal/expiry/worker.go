package expiry

import (
	"context"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jmoiron/sqlx"

	bmsync "creeperdiamonds.xyz/bettermoderation/internal/sync"
)

// expiredPunishment is a row returned by the expiry query.
type expiredPunishment struct {
	ID            string     `db:"id"`
	OrgID         string     `db:"org_id"`
	ProfileID     string     `db:"profile_id"`
	Type          string     `db:"type"`
	Platform      string     `db:"platform"`
	Reason        string     `db:"reason"`
	ExpiresAt     *time.Time `db:"expires_at"`
	MinecraftUUID *string    `db:"minecraft_uuid"` // from profiles join
	DiscordID     *string    `db:"discord_id"`     // from profiles join
	GuildID       *string    `db:"guild_id"`       // from servers join (Discord guild ID)
}

// Worker polls the DB for expired punishments and lifts them on Discord and Minecraft.
type Worker struct {
	db       *sqlx.DB
	discord  *discordgo.Session
	bus      *bmsync.EventBus
	interval time.Duration
}

func NewWorker(db *sqlx.DB, discord *discordgo.Session, bus *bmsync.EventBus, interval time.Duration) *Worker {
	return &Worker{db: db, discord: discord, bus: bus, interval: interval}
}

// Run starts the expiry loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	log.Printf("[expiry] worker started (interval: %s)", w.interval)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Run immediately on start, then on each tick
	w.tick(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Println("[expiry] worker stopped")
			return
		case <-ticker.C:
			w.tick(ctx)
		}
	}
}

func (w *Worker) tick(ctx context.Context) {
	expired, err := w.fetchExpired(ctx)
	if err != nil {
		log.Printf("[expiry] failed to fetch expired punishments: %v", err)
		return
	}
	if len(expired) == 0 {
		return
	}
	log.Printf("[expiry] processing %d expired punishment(s)", len(expired))

	for _, p := range expired {
		w.lift(ctx, p)
	}
}

// fetchExpired returns all punishments whose expires_at has passed and are still active.
// Joins profiles (for discord_id) and servers (for guild_id) so we have everything
// needed to call Discord and Minecraft in one query.
func (w *Worker) fetchExpired(ctx context.Context) ([]expiredPunishment, error) {
	rows, err := w.db.QueryxContext(ctx, `
		SELECT
			p.id,
			p.org_id,
			p.profile_id,
			p.type,
			p.platform,
			p.reason,
			p.expires_at,
			pr.discord_id,
			pr.minecraft_uuid,
			s.platform_id AS guild_id
		FROM punishments p
		JOIN profiles pr ON pr.id = p.profile_id
		LEFT JOIN servers s ON s.org_id = p.org_id AND s.platform = 'DISCORD'
		WHERE
			p.expires_at IS NOT NULL
			AND p.expires_at <= NOW(3)
			AND p.minecraft_active = 1
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []expiredPunishment
	for rows.Next() {
		var p expiredPunishment
		if err := rows.StructScan(&p); err != nil {
			log.Printf("[expiry] scan error: %v", err)
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// lift removes the punishment on Discord (if applicable) and marks it inactive in the DB.
func (w *Worker) lift(ctx context.Context, p expiredPunishment) {
	var liftErr error

	switch p.Platform {
	case "DISCORD":
		liftErr = w.liftDiscord(p)
	case "MINECRAFT":
		liftErr = w.liftMinecraft(ctx, p)
	case "SYSTEM":
		// System punishments are cross-platform — lift both
		liftErr = w.liftDiscord(p)
		if mcErr := w.liftMinecraft(ctx, p); mcErr != nil && liftErr == nil {
			liftErr = mcErr
		}
	}

	if liftErr != nil {
		log.Printf("[expiry] failed to lift punishment %s (%s %s): %v", p.ID, p.Platform, p.Type, liftErr)
		return
	}

	if err := w.markInactive(ctx, p.ID); err != nil {
		log.Printf("[expiry] failed to mark punishment %s inactive: %v", p.ID, err)
		return
	}

	log.Printf("[expiry] lifted %s %s for profile %s", p.Platform, p.Type, p.ProfileID)
	w.notifyPlayer(p)
}

// notifyPlayer sends a Discord DM to the player when their punishment expires.
func (w *Worker) notifyPlayer(p expiredPunishment) {
	if w.discord == nil || p.DiscordID == nil {
		return
	}
	var msg string
	switch p.Type {
	case "BAN":
		msg = "✅ Your ban has been lifted. You may now rejoin the server."
	case "MUTE":
		msg = "✅ Your mute has expired. You may now send messages again."
	default:
		return
	}
	dm, err := w.discord.UserChannelCreate(*p.DiscordID)
	if err != nil {
		return
	}
	w.discord.ChannelMessageSend(dm.ID, msg)
}

func (w *Worker) liftDiscord(p expiredPunishment) error {
	if p.DiscordID == nil || p.GuildID == nil {
		return nil // no Discord account linked or no Discord server linked
	}

	switch p.Type {
	case "BAN":
		if err := w.discord.GuildBanDelete(*p.GuildID, *p.DiscordID); err != nil {
			return err
		}
	case "MUTE":
		// Discord timeouts expire on their own — nothing to do here.
		// Only log it for our records.
	}
	return nil
}

func (w *Worker) liftMinecraft(ctx context.Context, p expiredPunishment) error {
	if w.bus == nil {
		return nil
	}
	evt := bmsync.PunishmentEvent{
		EventType:     "expire",
		PunishmentID:  p.ID,
		OrgID:         p.OrgID,
		ProfileID:     p.ProfileID,
		MinecraftUUID: p.MinecraftUUID,
		Type:          p.Type,
		Reason:        p.Reason,
		ExpiresAt:     p.ExpiresAt,
	}
	payload, err := bmsync.MarshalEvent(evt)
	if err != nil {
		return err
	}
	return w.bus.Publish(ctx, bmsync.ChannelPunishmentRevoke, payload)
}

func (w *Worker) markInactive(ctx context.Context, punishmentID string) error {
	_, err := w.db.ExecContext(ctx, `
		UPDATE punishments
		SET minecraft_active = 0,
		    revoked_at = NOW(3),
		    revoke_reason = 'Expired'
		WHERE id = ?`,
		punishmentID,
	)
	return err
}
