package bot

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
)

// discordBan issues a native Discord guild ban and optionally deletes message history.
func discordBan(s *discordgo.Session, guildID, userID, reason string, deleteMessageDays int) error {
	return s.GuildBanCreateWithReason(guildID, userID, reason, deleteMessageDays)
}

// discordUnban removes a native Discord guild ban.
func discordUnban(s *discordgo.Session, guildID, userID string) error {
	return s.GuildBanDelete(guildID, userID)
}

// discordKick kicks a member from the guild with a reason.
func discordKick(s *discordgo.Session, guildID, userID, reason string) error {
	return s.GuildMemberDeleteWithReason(guildID, userID, reason)
}

// discordTimeout applies Discord's native member timeout (communication disabled).
// Max duration Discord allows is 28 days. Returns an error if duration exceeds that.
func discordTimeout(s *discordgo.Session, guildID, userID, reason string, until time.Time) error {
	maxTimeout := time.Now().Add(28 * 24 * time.Hour)
	if until.After(maxTimeout) {
		return fmt.Errorf("discord timeout cannot exceed 28 days; use a DB-only mute for longer durations")
	}
	_, err := s.GuildMemberEditComplex(guildID, userID, &discordgo.GuildMemberParams{
		CommunicationDisabledUntil: &until,
	})
	return err
}

// discordRemoveTimeout clears a Discord native timeout from a member.
func discordRemoveTimeout(s *discordgo.Session, guildID, userID string) error {
	zero := time.Time{}
	_, err := s.GuildMemberEditComplex(guildID, userID, &discordgo.GuildMemberParams{
		CommunicationDisabledUntil: &zero,
	})
	return err
}

// discordLockChannel denies @everyone the ability to send messages in a channel.
func discordLockChannel(s *discordgo.Session, guildID, channelID string) error {
	guild, err := s.Guild(guildID)
	if err != nil {
		return err
	}
	// Deny SendMessages for the @everyone role (same ID as the guild)
	return s.ChannelPermissionSet(channelID, guild.ID, discordgo.PermissionOverwriteTypeRole,
		0, discordgo.PermissionSendMessages)
}

// discordUnlockChannel restores @everyone's ability to send messages.
func discordUnlockChannel(s *discordgo.Session, guildID, channelID string) error {
	guild, err := s.Guild(guildID)
	if err != nil {
		return err
	}
	// Neutral (neither allow nor deny) — falls back to role/default permissions
	return s.ChannelPermissionSet(channelID, guild.ID, discordgo.PermissionOverwriteTypeRole,
		0, 0)
}

// parseDuration converts strings like "1h", "7d", "2w", "1mo", "1y", "forever"
// to an absolute time.Time. Returns nil for "forever" (permanent).
func parseDuration(s string) *time.Time {
	if s == "" || s == "forever" || s == "permanent" {
		return nil
	}
	suffixes := []struct {
		suffix string
		mult   time.Duration
	}{
		{"mo", 30 * 24 * time.Hour},
		{"y", 365 * 24 * time.Hour},
		{"w", 7 * 24 * time.Hour},
		{"d", 24 * time.Hour},
		{"h", time.Hour},
	}
	for _, sf := range suffixes {
		if len(s) > len(sf.suffix) && s[len(s)-len(sf.suffix):] == sf.suffix {
			n := 0
			valid := true
			for _, c := range s[:len(s)-len(sf.suffix)] {
				if c < '0' || c > '9' {
					valid = false
					break
				}
				n = n*10 + int(c-'0')
			}
			if valid && n > 0 {
				t := time.Now().Add(time.Duration(n) * sf.mult)
				return &t
			}
		}
	}
	return nil
}

// respondEphemeral sends an ephemeral (only-you-can-see) reply to an interaction.
func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, msg string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: msg,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

// respondEmbed sends a public embed reply to an interaction.
func respondEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}
