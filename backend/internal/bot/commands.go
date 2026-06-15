package bot

import "github.com/bwmarrin/discordgo"

func allCommands() []*discordgo.ApplicationCommand {
	str := func(s string) *string { return &s }
	minLen := func(n float64) *float64 { return &n }

	return []*discordgo.ApplicationCommand{
		{
			Name:        "ban",
			Description: "Ban a user from this server",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionBanMembers),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to ban", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for the ban", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "duration", Description: "Duration: 1h, 7d, 2w, 1mo, 1y, forever (default: forever)"},
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "delete_messages", Description: "Delete message history (days, 0–7)", MinValue: minLen(0), MaxValue: minLen(7)},
			},
		},
		{
			Name:        "unban",
			Description: "Unban a user from this server",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionBanMembers),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "user_id", Description: "Discord user ID to unban", Required: true, MinLength: str("17"), MaxLength: str("20")},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for the unban"},
			},
		},
		{
			Name:        "mute",
			Description: "Timeout (mute) a user using Discord's native timeout — max 28 days",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionModerateMembers),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to mute", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "duration", Description: "Duration: 1h, 7d, 2w (max 28d)", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for the mute", Required: true},
			},
		},
		{
			Name:        "unmute",
			Description: "Remove a timeout from a user",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionModerateMembers),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to unmute", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for removal"},
			},
		},
		{
			Name:        "kick",
			Description: "Kick a user from this server",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionKickMembers),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to kick", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for the kick", Required: true},
			},
		},
		{
			Name:        "warn",
			Description: "Warn a user (logged and counts toward thresholds)",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionModerateMembers),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to warn", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for the warning", Required: true},
			},
		},
		{
			Name:        "history",
			Description: "View punishment history for a user",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionModerateMembers),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to look up", Required: true},
			},
		},
		{
			Name:        "lookup",
			Description: "Look up a user's BetterModeration profile",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionModerateMembers),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Discord user to look up"},
				{Type: discordgo.ApplicationCommandOptionString, Name: "minecraft_uuid", Description: "Minecraft UUID to look up"},
			},
		},
		{
			Name:        "purge",
			Description: "Bulk delete messages in this channel",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionManageMessages),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionInteger, Name: "amount", Description: "Number of messages to delete (1–100)", Required: true, MinValue: minLen(1), MaxValue: minLen(100)},
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "Only delete messages from this user"},
			},
		},
		{
			Name:        "lock",
			Description: "Lock a channel so only staff can send messages",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionManageChannels),
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Reason for locking the channel"},
			},
		},
		{
			Name:        "unlock",
			Description: "Unlock a previously locked channel",
			DefaultMemberPermissions: int64ptr(discordgo.PermissionManageChannels),
		},
		{
			Name:        "appeal",
			Description: "Submit an appeal for one of your punishments",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionString, Name: "punishment_id", Description: "ID of the punishment to appeal", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "reason", Description: "Why should this punishment be reconsidered?", Required: true, MaxLength: str("2000")},
				{Type: discordgo.ApplicationCommandOptionString, Name: "evidence", Description: "Evidence links (optional)"},
			},
		},
		{
			Name:        "report",
			Description: "Report another member to the moderation team",
			Options: []*discordgo.ApplicationCommandOption{
				{Type: discordgo.ApplicationCommandOptionUser, Name: "user", Description: "User to report", Required: true},
				{Type: discordgo.ApplicationCommandOptionString, Name: "category", Description: "Report category", Required: true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Harassment", Value: "HARASSMENT"},
						{Name: "Spam", Value: "SPAM"},
						{Name: "Cheating", Value: "CHEATING"},
						{Name: "Hate speech", Value: "HATE_SPEECH"},
						{Name: "Scam / impersonation", Value: "SCAM"},
						{Name: "Other", Value: "OTHER"},
					},
				},
				{Type: discordgo.ApplicationCommandOptionString, Name: "description", Description: "Describe the issue", Required: true, MaxLength: str("1000")},
				{Type: discordgo.ApplicationCommandOptionString, Name: "evidence", Description: "Evidence links (optional)"},
			},
		},
	}
}

func int64ptr(v int64) *int64 { return &v }
