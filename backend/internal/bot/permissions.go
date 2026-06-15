package bot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jmoiron/sqlx"
)

// commandActions maps each slash command to the action type it performs.
// Used to check mod role permissions server-side.
var commandActions = map[string]string{
	"ban":     "punish.ban",
	"unban":   "punish.ban",
	"mute":    "punish.mute",
	"unmute":  "punish.mute",
	"kick":    "punish.kick",
	"warn":    "punish.warn",
	"purge":   "purge",
	"lock":    "channel.lock",
	"unlock":  "channel.lock",
	"history": "history.view",
	"lookup":  "lookup",
}

// SyncCommandPermissions pushes Discord command visibility rules for a guild
// based on its mod role configuration. Commands are hidden from roles that
// don't hold the required bban node — but handlers always re-check server-side.
//
// nodeRoleMap: bban node → list of Discord role IDs that hold it.
// e.g. {"punish.ban": ["123456789"], "punish.warn": ["123456789", "987654321"]}
func (b *Bot) SyncCommandPermissions(guildID string, nodeRoleMap map[string][]string) error {
	appID := b.Session.State.User.ID

	cmds, err := b.Session.ApplicationCommands(appID, "")
	if err != nil {
		return fmt.Errorf("fetching commands: %w", err)
	}

	cmdByName := make(map[string]*discordgo.ApplicationCommand, len(cmds))
	for _, c := range cmds {
		cmdByName[c.Name] = c
	}

	for cmdName, node := range commandActions {
		cmd, ok := cmdByName[cmdName]
		if !ok {
			continue
		}

		roleIDs := nodeRoleMap[node]
		if len(roleIDs) == 0 {
			continue
		}

		perms := make([]*discordgo.ApplicationCommandPermissions, len(roleIDs))
		for idx, roleID := range roleIDs {
			perms[idx] = &discordgo.ApplicationCommandPermissions{
				ID:         roleID,
				Type:       discordgo.ApplicationCommandPermissionTypeRole,
				Permission: true,
			}
		}

		if _, err := b.Session.ApplicationCommandPermissionsEdit(appID, guildID, cmd.ID,
			&discordgo.ApplicationCommandPermissionsList{Permissions: perms},
		); err != nil {
			log.Printf("[perms] failed to sync /%s for guild %s: %v", cmdName, guildID, err)
		} else {
			log.Printf("[perms] synced /%s for guild %s (%d roles)", cmdName, guildID, len(perms))
		}
	}
	return nil
}

// checkPerm is the server-side permission gate called at the top of every handler.
// Loads the member's permission set from Redis (or DB on cache miss) and checks
// the given node. Admins bypass all checks. Always runs regardless of Discord UI visibility.
func (b *Bot) checkPerm(s *discordgo.Session, i *discordgo.InteractionCreate, node string) bool {
	if i.Member == nil {
		respondEphemeral(s, i, "This command can only be used in a server.")
		return false
	}

	// Server admins always pass
	if memberHasPerm(i.Member, discordgo.PermissionAdministrator) {
		return true
	}

	permSet, err := b.Perms.Load(context.Background(), i.GuildID, i.Member.Roles)
	if err != nil {
		log.Printf("[perm] failed to load permission set for guild %s: %v", i.GuildID, err)
		// Fail closed — deny on error rather than accidentally granting access
		respondEphemeral(s, i, "Permission check failed. Please try again.")
		return false
	}

	if !permSet.Has(node) {
		respondEphemeral(s, i, fmt.Sprintf("You don't have permission: `%s`", node))
		return false
	}

	return true
}

// checkRateLimit verifies the member hasn't exceeded their rate limit for the given action
// (e.g. "ban"). It reads the limit from their permission set, counts recent uses in the DB,
// and records this use if allowed. Returns true if the action is permitted.
func (b *Bot) checkRateLimit(s *discordgo.Session, i *discordgo.InteractionCreate, action string, db *sqlx.DB) bool {
	if i.Member == nil {
		return true
	}
	if memberHasPerm(i.Member, discordgo.PermissionAdministrator) {
		return true // admins have no rate limit
	}

	permSet, err := b.Perms.Load(context.Background(), i.GuildID, i.Member.Roles)
	if err != nil {
		return true // fail open on rate limit errors to avoid blocking valid actions
	}

	maxCount, windowSecs := permSet.RateLimit(action)
	if maxCount == 0 {
		return true // no rate limit configured
	}

	modID := i.Member.User.ID
	org, err := b.Profiles.OrgForGuild(context.Background(), i.GuildID)
	if err != nil {
		return true
	}

	windowStart := time.Now().UTC().Add(-time.Duration(windowSecs) * time.Second)

	var count int
	row := db.QueryRowxContext(context.Background(), `
		SELECT COUNT(*) FROM mod_role_usage
		WHERE org_id = ? AND mod_id = ? AND action = ? AND used_at >= ?`,
		org.OrgID, modID, action, windowStart,
	)
	if err := row.Scan(&count); err != nil {
		return true
	}

	if count >= maxCount {
		respondEphemeral(s, i, fmt.Sprintf("You have reached your `%s` limit (%d per window). Try again later.", action, maxCount))
		return false
	}

	// Record this use
	db.ExecContext(context.Background(), `
		INSERT INTO mod_role_usage (org_id, mod_id, action, used_at)
		VALUES (?, ?, ?, NOW(3))`,
		org.OrgID, modID, action,
	)
	return true
}

// memberHasPerm checks whether a guild member's computed permissions include the given bit.
func memberHasPerm(m *discordgo.Member, perm int64) bool {
	if m.Permissions&discordgo.PermissionAdministrator != 0 {
		return true // admins bypass everything
	}
	return m.Permissions&perm != 0
}

func allUniqueRoles(m map[string][]string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, roles := range m {
		for _, r := range roles {
			if _, ok := seen[r]; !ok {
				seen[r] = struct{}{}
				out = append(out, r)
			}
		}
	}
	return out
}
