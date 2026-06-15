package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Ban/unban/kick/mute/unmute/lock/unlock/purge — Discord handles all persistent state for these.
// Our audit log listener (onAuditLogEntry) is the single point that writes to our DB
// and triggers cross-platform Minecraft sync.

func (b *Bot) handleBan(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.checkPerm(s, i, "punish.ban") {
		return
	}
	if !b.checkRateLimit(s, i, "ban", b.DB) {
		return
	}
	opts := optionMap(i)
	target := opts["user"].UserValue(s)
	reason := opts["reason"].StringValue()
	durationStr := ""
	if v, ok := opts["duration"]; ok {
		durationStr = v.StringValue()
	}
	deleteDays := 0
	if v, ok := opts["delete_messages"]; ok {
		deleteDays = int(v.IntValue())
	}

	expiresAt := parseDuration(durationStr)

	// Clamp duration to the member's max allowed
	expiresAt = b.clampDuration(i, "ban", expiresAt)

	if err := discordBan(s, i.GuildID, target.ID, reason, deleteDays); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to ban: %s", err))
		return
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       "🔨 User Banned",
		Description: fmt.Sprintf("**%s** has been banned.\n**Reason:** %s\n**Duration:** %s", target.Mention(), reason, formatExpiry(expiresAt)),
		Color:       0xED4245,
		Timestamp:   time.Now().Format(time.RFC3339),
	})
}

func (b *Bot) handleUnban(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.checkPerm(s, i, "punish.ban") {
		return
	}
	opts := optionMap(i)
	userID := opts["user_id"].StringValue()
	reason := ""
	if v, ok := opts["reason"]; ok {
		reason = v.StringValue()
	}

	if err := discordUnban(s, i.GuildID, userID); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to unban: %s", err))
		return
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       "✅ User Unbanned",
		Description: fmt.Sprintf("User `%s` has been unbanned.\n**Reason:** %s", userID, reason),
		Color:       0x57F287,
		Timestamp:   time.Now().Format(time.RFC3339),
	})
}

func (b *Bot) handleMute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.checkPerm(s, i, "punish.mute") {
		return
	}
	if !b.checkRateLimit(s, i, "mute", b.DB) {
		return
	}
	opts := optionMap(i)
	target := opts["user"].UserValue(s)
	reason := opts["reason"].StringValue()
	durationStr := opts["duration"].StringValue()

	expiresAt := parseDuration(durationStr)
	if expiresAt == nil {
		respondEphemeral(s, i, "Discord timeouts cannot be permanent (max 28 days). For indefinite mutes on Minecraft, use the Minecraft plugin directly.")
		return
	}

	expiresAt = b.clampDuration(i, "mute", expiresAt)

	if err := discordTimeout(s, i.GuildID, target.ID, reason, *expiresAt); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to mute: %s", err))
		return
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       "🔇 User Muted",
		Description: fmt.Sprintf("**%s** has been timed out.\n**Reason:** %s\n**Until:** %s", target.Mention(), reason, expiresAt.UTC().Format("2006-01-02 15:04 UTC")),
		Color:       0xFEE75C,
		Timestamp:   time.Now().Format(time.RFC3339),
	})
}

func (b *Bot) handleUnmute(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.checkPerm(s, i, "punish.mute") {
		return
	}
	opts := optionMap(i)
	target := opts["user"].UserValue(s)
	reason := ""
	if v, ok := opts["reason"]; ok {
		reason = v.StringValue()
	}

	if err := discordRemoveTimeout(s, i.GuildID, target.ID); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to unmute: %s", err))
		return
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       "🔊 User Unmuted",
		Description: fmt.Sprintf("**%s** timeout has been removed.\n**Reason:** %s", target.Mention(), reason),
		Color:       0x57F287,
		Timestamp:   time.Now().Format(time.RFC3339),
	})
}

func (b *Bot) handleKick(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.checkPerm(s, i, "punish.kick") {
		return
	}
	if !b.checkRateLimit(s, i, "kick", b.DB) {
		return
	}
	opts := optionMap(i)
	target := opts["user"].UserValue(s)
	reason := opts["reason"].StringValue()

	if err := discordKick(s, i.GuildID, target.ID, reason); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to kick: %s", err))
		return
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       "👟 User Kicked",
		Description: fmt.Sprintf("**%s** has been kicked.\n**Reason:** %s", target.Mention(), reason),
		Color:       0xEB459E,
		Timestamp:   time.Now().Format(time.RFC3339),
	})
}

// handleWarn is a Bot method so it can access the warn service.
func (b *Bot) handleWarn(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.checkPerm(s, i, "punish.warn") {
		return
	}
	if !b.checkRateLimit(s, i, "warn", b.DB) {
		return
	}
	opts := optionMap(i)
	target := opts["user"].UserValue(s)
	reason := opts["reason"].StringValue()

	ctx := context.Background()

	org, err := b.Profiles.OrgForGuild(ctx, i.GuildID)
	if err != nil {
		respondEphemeral(s, i, "Server not linked to BetterModeration.")
		return
	}

	targetProfile, err := b.Profiles.ResolveByDiscord(ctx, target.ID)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to resolve profile: %s", err))
		return
	}

	orgID := org.OrgID
	profileID := targetProfile.ID
	issuedBy := i.Member.User.ID

	result, err := b.Warns.Issue(ctx, orgID, profileID, issuedBy, reason, org.ServerID)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to issue warning: %s", err))
		return
	}

	// Execute auto-punishment on Discord if a threshold was crossed
	if result.AutoPunish != nil {
		switch result.AutoPunish.Type {
		case "BAN":
			discordBan(s, i.GuildID, target.ID, "Automatic: warning threshold reached", 0)
		case "MUTE":
			if result.AutoPunish.ExpiresAt != nil {
				discordTimeout(s, i.GuildID, target.ID, "Automatic: warning threshold reached", *result.AutoPunish.ExpiresAt)
			}
		case "KICK":
			discordKick(s, i.GuildID, target.ID, "Automatic: warning threshold reached")
		}
		// Publish Minecraft sync event
		b.publishAutoPunishment(ctx, org.OrgID, targetProfile.ID, targetProfile.MinecraftUUID, result.AutoPunish)
	}

	// DM the warned user
	dmMsg := fmt.Sprintf(
		"⚠️ You have been warned in **%s**.\n**Reason:** %s\n**Active warnings:** %d",
		i.GuildID, reason, result.ActiveWarns,
	)
	if result.AutoPunish != nil {
		dmMsg += fmt.Sprintf("\n\n⛔ You have reached the warning threshold and received an automatic **%s**.", result.AutoPunish.Type)
		if result.AutoPunish.ExpiresAt != nil {
			dmMsg += fmt.Sprintf(" Expires: %s", result.AutoPunish.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC"))
		}
	}
	if dm, err := s.UserChannelCreate(target.ID); err == nil {
		s.ChannelMessageSend(dm.ID, dmMsg)
	}

	// Response embed
	desc := fmt.Sprintf("**%s** has been warned.\n**Reason:** %s\n**Active warnings:** %d",
		target.Mention(), reason, result.ActiveWarns)

	embed := &discordgo.MessageEmbed{
		Title:       "⚠️ User Warned",
		Description: desc,
		Color:       0xFEE75C,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	if result.AutoPunish != nil {
		expStr := "Permanent"
		if result.AutoPunish.ExpiresAt != nil {
			expStr = result.AutoPunish.ExpiresAt.UTC().Format("2006-01-02 15:04 UTC")
		}
		embed.Color = 0xED4245
		embed.Fields = []*discordgo.MessageEmbedField{{
			Name:  fmt.Sprintf("⛔ Threshold reached — auto %s issued", result.AutoPunish.Type),
			Value: "Expires: " + expStr,
		}}
	}

	respondEmbed(s, i, embed)
}

// History — reads from our DB (cross-platform history Discord doesn't know about).
func (b *Bot) handleHistory(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)
	target := opts["user"].UserValue(s)
	ctx := context.Background()

	targetProfile, err := b.Profiles.ResolveByDiscord(ctx, target.ID)
	if err != nil {
		respondEphemeral(s, i, "Failed to resolve profile.")
		return
	}

	punishments, err := b.Warns.GetPunishments(ctx, targetProfile.ID)
	if err != nil {
		respondEphemeral(s, i, "Failed to fetch punishment history.")
		return
	}

	desc := "*No punishments found.*"
	if len(punishments) > 0 {
		lines := make([]string, 0, len(punishments))
		for _, p := range punishments {
			exp := "permanent"
			if p.ExpiresAt != nil {
				exp = p.ExpiresAt.UTC().Format("2006-01-02")
			}
			lines = append(lines, fmt.Sprintf("**%s** — %s (exp: %s)", p.Type, p.Reason, exp))
		}
		desc = strings.Join(lines, "\n")
		if len(desc) > 4000 {
			desc = desc[:4000] + "\n*(truncated)*"
		}
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("📋 Punishment History — %s", target.Username),
		Description: desc,
		Color:       0x5865F2,
		Timestamp:   time.Now().Format(time.RFC3339),
	})
}

// Lookup — reads from our DB to show linked Minecraft account, warns, appeals etc.
func (b *Bot) handleLookup(s *discordgo.Session, i *discordgo.InteractionCreate) {
	opts := optionMap(i)
	ctx := context.Background()

	org, err := b.Profiles.OrgForGuild(ctx, i.GuildID)
	if err != nil {
		respondEphemeral(s, i, "Server not linked to BetterModeration.")
		return
	}

	var desc string

	if v, ok := opts["user"]; ok {
		discordUser := v.UserValue(s)
		prof, err := b.Profiles.ResolveByDiscord(ctx, discordUser.ID)
		if err != nil {
			respondEphemeral(s, i, "Failed to resolve profile.")
			return
		}
		mc := "*not linked*"
		if prof.MinecraftUUID != nil {
			mc = "`" + *prof.MinecraftUUID + "`"
		}
		activeWarns, _ := b.Warns.CountActive(ctx, org.OrgID, prof.ID)
		alts, _ := b.Profiles.FindAlts(ctx, prof.ID)
		desc = fmt.Sprintf(
			"**Discord:** %s (`%s`)\n**Minecraft UUID:** %s\n**Active Warns:** %d\n**Alt accounts:** %d",
			discordUser.Username, discordUser.ID, mc, activeWarns, len(alts),
		)
	} else if v, ok := opts["minecraft_uuid"]; ok {
		uuid := v.StringValue()
		prof, err := b.Profiles.ResolveByMinecraft(ctx, uuid)
		if err != nil {
			respondEphemeral(s, i, "Failed to resolve profile.")
			return
		}
		discord := "*not linked*"
		if prof.DiscordID != nil {
			discord = "`" + *prof.DiscordID + "`"
		}
		activeWarns, _ := b.Warns.CountActive(ctx, org.OrgID, prof.ID)
		desc = fmt.Sprintf(
			"**Minecraft UUID:** `%s`\n**Discord ID:** %s\n**Active Warns:** %d",
			uuid, discord, activeWarns,
		)
	} else {
		respondEphemeral(s, i, "Provide either a Discord user or a Minecraft UUID.")
		return
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       "🔎 Profile Lookup",
		Description: desc,
		Color:       0x5865F2,
		Timestamp:   time.Now().Format(time.RFC3339),
	})
}

// Appeal — lets a user submit an appeal for one of their punishments.
func (b *Bot) handleAppeal(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Member == nil {
		respondEphemeral(s, i, "This command must be used inside a server.")
		return
	}
	opts := optionMap(i)
	punishmentID := opts["punishment_id"].StringValue()
	reason := opts["reason"].StringValue()
	var evidence *string
	if v, ok := opts["evidence"]; ok {
		ev := v.StringValue()
		evidence = &ev
	}

	ctx := context.Background()

	// Resolve the caller's profile
	callerProfile, err := b.Profiles.ResolveByDiscord(ctx, i.Member.User.ID)
	if err != nil {
		respondEphemeral(s, i, "Could not find your BetterModeration profile. Make sure your Discord account is linked.")
		return
	}

	appeal, err := b.Appeals.Submit(ctx, punishmentID, callerProfile.ID, reason, evidence)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       "📋 Appeal Submitted",
		Description: fmt.Sprintf("Your appeal for punishment `%s` has been submitted.\n**Appeal ID:** `%s`\n\nOur moderation team will review it and get back to you.", punishmentID, appeal.ID),
		Color:       0x57F287,
		Timestamp:   time.Now().Format(time.RFC3339),
	})
}

// Report — lets a user report another member.
func (b *Bot) handleReport(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Member == nil {
		respondEphemeral(s, i, "This command must be used inside a server.")
		return
	}
	opts := optionMap(i)
	target := opts["user"].UserValue(s)
	category := opts["category"].StringValue()
	description := opts["description"].StringValue()
	var evidence *string
	if v, ok := opts["evidence"]; ok {
		ev := v.StringValue()
		evidence = &ev
	}

	if target.ID == i.Member.User.ID {
		respondEphemeral(s, i, "You cannot report yourself.")
		return
	}

	ctx := context.Background()

	org, err := b.Profiles.OrgForGuild(ctx, i.GuildID)
	if err != nil {
		respondEphemeral(s, i, "Server not linked to BetterModeration.")
		return
	}

	reporterProfile, err := b.Profiles.ResolveByDiscord(ctx, i.Member.User.ID)
	if err != nil {
		respondEphemeral(s, i, "Could not find your profile. Make sure your Discord account is linked.")
		return
	}

	targetProfile, err := b.Profiles.ResolveByDiscord(ctx, target.ID)
	if err != nil {
		respondEphemeral(s, i, "Could not find that user's profile.")
		return
	}

	serverID := org.ServerID
	rep, err := b.Reports.Submit(ctx, org.OrgID, reporterProfile.ID, targetProfile.ID, category, description, evidence, "DISCORD", &serverID)
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("❌ %s", err.Error()))
		return
	}

	// Alert staff log channel
	logCh := b.logChannels[i.GuildID]
	if logCh != "" {
		s.ChannelMessageSend(logCh, fmt.Sprintf(
			"📩 **New Report** [%s]\n**Reporter:** <@%s>\n**Target:** <@%s>\n**Category:** %s\n**Description:** %s\n**ID:** `%s`",
			category, i.Member.User.ID, target.ID, category, description, rep.ID,
		))
	}

	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title:       "📩 Report Submitted",
		Description: fmt.Sprintf("Your report against **%s** has been submitted.\n**Report ID:** `%s`\n\nThank you — our moderation team will review it.", target.Username, rep.ID),
		Color:       0x57F287,
		Timestamp:   time.Now().Format(time.RFC3339),
	})
}

// Purge — Discord deletes the messages, nothing to store.
func (b *Bot) handlePurge(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.checkPerm(s, i, "purge") {
		return
	}
	opts := optionMap(i)
	amount := int(opts["amount"].IntValue())

	var filterUser *discordgo.User
	if v, ok := opts["user"]; ok {
		filterUser = v.UserValue(s)
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})

	msgs, err := s.ChannelMessages(i.ChannelID, 100, "", "", "")
	if err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: strPtr(fmt.Sprintf("Failed to fetch messages: %s", err))})
		return
	}

	var ids []string
	for _, m := range msgs {
		if filterUser != nil && m.Author.ID != filterUser.ID {
			continue
		}
		ids = append(ids, m.ID)
		if len(ids) >= amount {
			break
		}
	}

	if len(ids) == 0 {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: strPtr("No messages found to delete.")})
		return
	}

	if err := s.ChannelMessagesBulkDelete(i.ChannelID, ids); err != nil {
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: strPtr(fmt.Sprintf("Failed to delete: %s", err))})
		return
	}

	userStr := "all users"
	if filterUser != nil {
		userStr = filterUser.Username
	}
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: strPtr(fmt.Sprintf("Deleted %d messages from %s.", len(ids), userStr)),
	})
}

// Lock/unlock — Discord stores channel permission overwrites, nothing to persist.
func (b *Bot) handleLock(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.checkPerm(s, i, "purge") {
		return
	}
	opts := optionMap(i)
	reason := ""
	if v, ok := opts["reason"]; ok {
		reason = v.StringValue()
	}

	if err := discordLockChannel(s, i.GuildID, i.ChannelID); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to lock channel: %s", err))
		return
	}

	desc := "🔒 This channel has been locked."
	if reason != "" {
		desc += "\n**Reason:** " + reason
	}
	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title: "🔒 Channel Locked", Description: desc, Color: 0xED4245,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

func (b *Bot) handleUnlock(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if !b.checkPerm(s, i, "purge") {
		return
	}
	if err := discordUnlockChannel(s, i.GuildID, i.ChannelID); err != nil {
		respondEphemeral(s, i, fmt.Sprintf("Failed to unlock channel: %s", err))
		return
	}
	respondEmbed(s, i, &discordgo.MessageEmbed{
		Title: "🔓 Channel Unlocked", Description: "This channel has been unlocked.",
		Color: 0x57F287, Timestamp: time.Now().Format(time.RFC3339),
	})
}

// clampDuration enforces the member's max duration node for an action.
// If the requested time exceeds what their permissions allow, it is silently clamped.
// nil (permanent) is clamped to the max allowed duration unless permanent is permitted.
func (b *Bot) clampDuration(i *discordgo.InteractionCreate, action string, requested *time.Time) *time.Time {
	if i.Member == nil || memberHasPerm(i.Member, discordgo.PermissionAdministrator) {
		return requested
	}
	permSet, err := b.Perms.Load(context.Background(), i.GuildID, i.Member.Roles)
	if err != nil {
		return requested
	}
	maxSecs := permSet.MaxDuration(action)
	if maxSecs == -1 {
		return requested // permanent allowed
	}
	if maxSecs == 0 {
		// No duration nodes granted — allow whatever (perm check already passed)
		return requested
	}
	maxExpiry := time.Now().UTC().Add(time.Duration(maxSecs) * time.Second)
	if requested == nil || requested.After(maxExpiry) {
		return &maxExpiry
	}
	return requested
}

func optionMap(i *discordgo.InteractionCreate) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	m := make(map[string]*discordgo.ApplicationCommandInteractionDataOption)
	for _, opt := range i.ApplicationCommandData().Options {
		m[opt.Name] = opt
	}
	return m
}

func formatExpiry(t *time.Time) string {
	if t == nil {
		return "Permanent"
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func strPtr(s string) *string { return &s }

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "Permanent"
	}
	var parts []string
	if days := int(d.Hours()) / 24; days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours := int(d.Hours()) % 24; hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	return strings.Join(parts, " ")
}
