package bot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jmoiron/sqlx"
	"creeperdiamonds.xyz/bettermoderation/internal/appeals"
	"creeperdiamonds.xyz/bettermoderation/internal/automod"
	"creeperdiamonds.xyz/bettermoderation/internal/permission"
	"creeperdiamonds.xyz/bettermoderation/internal/profile"
	"creeperdiamonds.xyz/bettermoderation/internal/reports"
	bmsync "creeperdiamonds.xyz/bettermoderation/internal/sync"
	"creeperdiamonds.xyz/bettermoderation/internal/warn"
)

type Bot struct {
	Session        *discordgo.Session
	Warns          *warn.Service
	Profiles       *profile.Service
	Perms          *permission.Loader
	Bus            *bmsync.EventBus
	AutoMod        *automod.Engine
	Appeals        *appeals.Service
	Reports        *reports.Service
	DB             *sqlx.DB
	registeredCmds []*discordgo.ApplicationCommand
	handlers       map[string]func(*discordgo.Session, *discordgo.InteractionCreate)
	logChannels    map[string]string // guildID → log channel ID, loaded from DB on link
}

func New(token string, warnSvc *warn.Service, profileSvc *profile.Service, permLoader *permission.Loader, bus *bmsync.EventBus, autoModEngine *automod.Engine, appealsSvc *appeals.Service, reportsSvc *reports.Service, db *sqlx.DB) (*Bot, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("creating discord session: %w", err)
	}

	s.Identify.Intents = discordgo.IntentsGuilds |
		discordgo.IntentGuildMembers |
		discordgo.IntentGuildBans |
		discordgo.IntentGuildMessages |
		discordgo.IntentMessageContent

	b := &Bot{
		Session:     s,
		Warns:       warnSvc,
		Profiles:    profileSvc,
		Perms:       permLoader,
		Bus:         bus,
		AutoMod:     autoModEngine,
		Appeals:     appealsSvc,
		Reports:     reportsSvc,
		DB:          db,
		logChannels: make(map[string]string),
	}
	b.buildHandlers()

	s.AddHandler(b.onReady)
	s.AddHandler(b.onInteraction)
	s.AddHandler(b.onAuditLogEntry)
	s.AddHandler(b.onGuildBanAdd)
	s.AddHandler(b.onGuildBanRemove)
	s.AddHandler(b.onMessageCreate)

	return b, nil
}

func (b *Bot) Start() error {
	if err := b.Session.Open(); err != nil {
		return fmt.Errorf("opening discord session: %w", err)
	}
	return nil
}

func (b *Bot) Stop() {
	for _, cmd := range b.registeredCmds {
		if err := b.Session.ApplicationCommandDelete(b.Session.State.User.ID, "", cmd.ID); err != nil {
			log.Printf("warn: failed to delete command /%s: %v", cmd.Name, err)
		}
	}
	b.Session.Close()
}

// SetLogChannel sets the moderation log channel for a guild.
// Called when a guild links up or changes its log channel config.
func (b *Bot) SetLogChannel(guildID, channelID string) {
	b.logChannels[guildID] = channelID
}

func (b *Bot) onReady(s *discordgo.Session, e *discordgo.Ready) {
	log.Printf("BetterModeration bot online as %s#%s (%s)", e.User.Username, e.User.Discriminator, e.User.ID)
	b.syncCommands(s)
}

func (b *Bot) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}
	if h, ok := b.handlers[i.ApplicationCommandData().Name]; ok {
		h(s, i)
	}
}

// onAuditLogEntry fires when any moderation action is taken in a guild,
// including actions taken via Discord's built-in UI (not the bot's commands).
// It writes to the punishments DB, publishes a sync event, and sends a log embed.
func (b *Bot) onAuditLogEntry(s *discordgo.Session, e *discordgo.GuildAuditLogEntryCreate) {
	ctx := context.Background()

	var punishType string
	var embed *discordgo.MessageEmbed

	switch e.ActionType {
	case discordgo.AuditLogActionMemberBanAdd:
		punishType = "BAN"
		embed = b.buildLogEmbed(s, e, "🔨 Member Banned (built-in)", 0xED4245, "")

	case discordgo.AuditLogActionMemberBanRemove:
		// Unban — revoke active ban in DB
		b.revokeByDiscordID(ctx, e.GuildID, e.TargetID, "BAN", "Unbanned via Discord")
		embed = b.buildLogEmbed(s, e, "✅ Member Unbanned (built-in)", 0x57F287, "")

	case discordgo.AuditLogActionMemberKick:
		punishType = "KICK"
		embed = b.buildLogEmbed(s, e, "👟 Member Kicked (built-in)", 0xEB459E, "")

	case discordgo.AuditLogActionMemberUpdate:
		timedOut, removed := detectTimeoutChange(e.Changes)
		if timedOut {
			punishType = "MUTE"
			embed = b.buildLogEmbed(s, e, "🔇 Member Timed Out (built-in)", 0xFEE75C, "")
		} else if removed {
			b.revokeByDiscordID(ctx, e.GuildID, e.TargetID, "MUTE", "Timeout removed via Discord")
			embed = b.buildLogEmbed(s, e, "🔊 Timeout Removed (built-in)", 0x57F287, "")
		}
	}

	// Write to DB if this was a new punishment (not a revoke)
	if punishType != "" {
		b.recordNativePunishment(ctx, e, punishType)
	}

	if embed == nil {
		return
	}

	logCh := b.logChannels[e.GuildID]
	if logCh != "" {
		if _, err := s.ChannelMessageSendEmbed(logCh, embed); err != nil {
			log.Printf("warn: failed to send log embed to channel %s: %v", logCh, err)
		}
	}
}

// buildLogEmbed creates a moderation log embed from an audit log entry.
func (b *Bot) buildLogEmbed(s *discordgo.Session, e *discordgo.GuildAuditLogEntryCreate, title string, color int, extra string) *discordgo.MessageEmbed {
	// Resolve moderator name
	modName := e.UserID
	if mod, err := s.User(e.UserID); err == nil {
		modName = fmt.Sprintf("%s (`%s`)", mod.Username, mod.ID)
	}

	// Resolve target name
	targetName := e.TargetID
	if target, err := s.User(e.TargetID); err == nil {
		targetName = fmt.Sprintf("%s (`%s`)", target.Username, target.ID)
	}

	reason := "No reason provided"
	if e.Reason != nil && *e.Reason != "" {
		reason = *e.Reason
	}

	desc := fmt.Sprintf("**Target:** %s\n**Moderator:** %s\n**Reason:** %s",
		targetName, modName, reason)
	if extra != "" {
		desc += "\n" + extra
	}

	return &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Color:       color,
		Footer:      &discordgo.MessageEmbedFooter{Text: "Detected via Discord audit log"},
		Timestamp:   time.Now().Format(time.RFC3339),
	}
}

// detectTimeoutChange checks whether an audit log entry's Changes indicate a
// communication_disabled_until being set (timeout added) or cleared (timeout removed).
func detectTimeoutChange(changes []*discordgo.AuditLogChange) (added bool, removed bool) {
	for _, c := range changes {
		if c.Key != "communication_disabled_until" {
			continue
		}
		newVal, _ := c.New.(string)
		oldVal, _ := c.Old.(string)
		if newVal != "" && newVal != "0001-01-01T00:00:00+00:00" {
			return true, false // timeout was set
		}
		if oldVal != "" && (newVal == "" || newVal == "0001-01-01T00:00:00+00:00") {
			return false, true // timeout was cleared
		}
	}
	return false, false
}

// onMessageCreate evaluates messages against AutoMod rules.
func (b *Bot) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || b.AutoMod == nil {
		return
	}
	ctx := context.Background()

	org, err := b.Profiles.OrgForGuild(ctx, m.GuildID)
	if err != nil {
		return // guild not linked
	}

	action := b.AutoMod.Evaluate(ctx, org.OrgID, m.Content, m.Author.ID)
	if action == nil {
		return
	}

	logCh := b.logChannels[m.GuildID]

	if action.TestMode {
		if logCh != "" {
			s.ChannelMessageSend(logCh, fmt.Sprintf(
				"[AutoMod TEST] Rule **%s** would fire on message by <@%s>: `%s`",
				action.Rule.Name, m.Author.ID, truncate(m.Content, 100),
			))
		}
		return
	}

	// Delete the message
	s.ChannelMessageDelete(m.ChannelID, m.ID)

	// Notify log channel
	if logCh != "" {
		s.ChannelMessageSend(logCh, fmt.Sprintf(
			"[AutoMod] Rule **%s** triggered → **%s** on <@%s>",
			action.Rule.Name, action.Rule.ActionType, m.Author.ID,
		))
	}

	// Execute the action
	switch action.Rule.ActionType {
	case "WARN":
		profile, err := b.Profiles.ResolveByDiscord(ctx, m.Author.ID)
		if err != nil {
			return
		}
		b.Warns.Issue(ctx, org.OrgID, profile.ID, "AUTOMOD", "AutoMod: "+action.Rule.Name, org.ServerID)

	case "MUTE":
		duration := 10 * time.Minute
		if action.Rule.ActionDuration != nil {
			duration = time.Duration(*action.Rule.ActionDuration) * time.Second
		}
		until := time.Now().UTC().Add(duration)
		discordTimeout(s, m.GuildID, m.Author.ID, "AutoMod: "+action.Rule.Name, until)

	case "KICK":
		discordKick(s, m.GuildID, m.Author.ID, "AutoMod: "+action.Rule.Name)

	case "BAN":
		discordBan(s, m.GuildID, m.Author.ID, "AutoMod: "+action.Rule.Name, 0)

	case "NOTIFY_STAFF":
		// Already logged above — no further action needed
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// onGuildBanAdd / onGuildBanRemove serve as secondary signals in case the
// audit log gateway event is delayed or missing.
func (b *Bot) onGuildBanAdd(s *discordgo.Session, e *discordgo.GuildBanAdd) {
	log.Printf("[event] ban add — user=%s guild=%s", e.User.ID, e.GuildID)
}

func (b *Bot) onGuildBanRemove(s *discordgo.Session, e *discordgo.GuildBanRemove) {
	log.Printf("[event] ban remove — user=%s guild=%s", e.User.ID, e.GuildID)
}

func (b *Bot) syncCommands(s *discordgo.Session) {
	defs := allCommands()
	registered := make([]*discordgo.ApplicationCommand, 0, len(defs))
	for _, def := range defs {
		cmd, err := s.ApplicationCommandCreate(s.State.User.ID, "", def)
		if err != nil {
			log.Printf("failed to register command /%s: %v", def.Name, err)
			continue
		}
		registered = append(registered, cmd)
	}
	b.registeredCmds = registered
	log.Printf("registered %d slash commands", len(registered))
}

func (b *Bot) buildHandlers() {
	b.handlers = map[string]func(*discordgo.Session, *discordgo.InteractionCreate){
		"ban":     b.handleBan,
		"unban":   b.handleUnban,
		"mute":    b.handleMute,
		"unmute":  b.handleUnmute,
		"kick":    b.handleKick,
		"warn":    b.handleWarn,
		"history": b.handleHistory,
		"lookup":  b.handleLookup,
		"purge":   b.handlePurge,
		"lock":    b.handleLock,
		"unlock":  b.handleUnlock,
		"appeal":  b.handleAppeal,
		"report":  b.handleReport,
	}
}
