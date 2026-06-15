package bot

import (
	"context"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/google/uuid"

	bmsync "creeperdiamonds.xyz/bettermoderation/internal/sync"
	"creeperdiamonds.xyz/bettermoderation/internal/warn"
)

// recordNativePunishment writes a punishment issued via Discord's native UI to our DB
// and publishes a sync event so Minecraft plugins can mirror it.
func (b *Bot) recordNativePunishment(ctx context.Context, e *discordgo.GuildAuditLogEntryCreate, punishType string) {
	org, err := b.Profiles.OrgForGuild(ctx, e.GuildID)
	if err != nil {
		log.Printf("[audit] org not found for guild %s: %v", e.GuildID, err)
		return
	}

	targetProfile, err := b.Profiles.ResolveByDiscord(ctx, e.TargetID)
	if err != nil {
		log.Printf("[audit] failed to resolve profile for discord user %s: %v", e.TargetID, err)
		return
	}

	reason := "No reason provided"
	if e.Reason != nil && *e.Reason != "" {
		reason = *e.Reason
	}

	punishmentID := uuid.NewString()
	now := time.Now().UTC()

	issuedBy := e.UserID
	_, err = b.Profiles.DB().ExecContext(ctx, `
		INSERT INTO punishments
			(id, org_id, profile_id, type, reason, issued_by, issued_by_type,
			 issued_at, expires_at, platform, server_id, minecraft_active, public)
		VALUES (?, ?, ?, ?, ?, ?, 'STAFF', ?, NULL, 'DISCORD', ?, 1, 1)`,
		punishmentID, org.OrgID, targetProfile.ID, punishType, reason, issuedBy, now, org.ServerID,
	)
	if err != nil {
		log.Printf("[audit] failed to record punishment: %v", err)
		return
	}

	if b.Bus == nil {
		return
	}

	evt := bmsync.PunishmentEvent{
		EventType:     "issue",
		PunishmentID:  punishmentID,
		OrgID:         org.OrgID,
		ProfileID:     targetProfile.ID,
		MinecraftUUID: targetProfile.MinecraftUUID,
		Type:          punishType,
		Reason:        reason,
		IssuedBy:      &issuedBy,
		IssuedAt:      now,
	}
	payload, err := bmsync.MarshalEvent(evt)
	if err != nil {
		log.Printf("[audit] failed to marshal sync event: %v", err)
		return
	}
	if err := b.Bus.Publish(ctx, bmsync.ChannelPunishmentIssue, payload); err != nil {
		log.Printf("[audit] failed to publish sync event: %v", err)
	}
}

// publishAutoPunishment publishes a Redis sync event for an auto-punishment so Minecraft plugins
// mirror it in real time (the Discord audit log event will also fire, but this is faster).
func (b *Bot) publishAutoPunishment(ctx context.Context, orgID, profileID string, minecraftUUID *string, p *warn.Punishment) {
	if b.Bus == nil {
		return
	}
	issuedBy := "SYSTEM"
	evt := bmsync.PunishmentEvent{
		EventType:     "issue",
		PunishmentID:  p.ID,
		OrgID:         orgID,
		ProfileID:     profileID,
		MinecraftUUID: minecraftUUID,
		Type:          p.Type,
		Reason:        "Automatic: warning threshold reached",
		IssuedBy:      &issuedBy,
		ExpiresAt:     p.ExpiresAt,
		IssuedAt:      time.Now().UTC(),
	}
	payload, err := bmsync.MarshalEvent(evt)
	if err != nil {
		return
	}
	if err := b.Bus.Publish(ctx, bmsync.ChannelPunishmentIssue, payload); err != nil {
		log.Printf("[warn-threshold] failed to publish sync event: %v", err)
	}
}

// revokeByDiscordID marks any active punishment of the given type for a Discord user as revoked.
func (b *Bot) revokeByDiscordID(ctx context.Context, guildID, discordUserID, punishType, revokeReason string) {
	org, err := b.Profiles.OrgForGuild(ctx, guildID)
	if err != nil {
		return
	}

	targetProfile, err := b.Profiles.ResolveByDiscord(ctx, discordUserID)
	if err != nil {
		return
	}

	_, err = b.Profiles.DB().ExecContext(ctx, `
		UPDATE punishments
		SET minecraft_active = 0, revoked_at = NOW(3), revoke_reason = ?
		WHERE org_id = ? AND profile_id = ? AND type = ? AND minecraft_active = 1`,
		revokeReason, org.OrgID, targetProfile.ID, punishType,
	)
	if err != nil {
		log.Printf("[audit] failed to revoke %s for profile %s: %v", punishType, targetProfile.ID, err)
		return
	}

	if b.Bus == nil {
		return
	}

	evt := bmsync.PunishmentEvent{
		EventType:     "revoke",
		OrgID:         org.OrgID,
		ProfileID:     targetProfile.ID,
		MinecraftUUID: targetProfile.MinecraftUUID,
		Type:          punishType,
		Reason:        revokeReason,
		IssuedAt:      time.Now().UTC(),
	}
	payload, err := bmsync.MarshalEvent(evt)
	if err != nil {
		return
	}
	b.Bus.Publish(ctx, bmsync.ChannelPunishmentRevoke, payload)
}
