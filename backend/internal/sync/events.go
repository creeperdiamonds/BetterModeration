package sync

import (
	"encoding/json"
	"fmt"
	"time"
)

// Channel names used for Redis pub/sub.
const (
	ChannelPunishmentIssue  = "bm:punishment:issue"
	ChannelPunishmentRevoke = "bm:punishment:revoke"
	ChannelJoinFlagged      = "bm:join:flagged"
)

// PunishmentEvent is published when a punishment is issued or revoked.
// Minecraft plugins subscribe and apply/remove bans/mutes as needed.
type PunishmentEvent struct {
	EventType     string     `json:"event_type"`      // "issue" | "revoke" | "expire"
	PunishmentID  string     `json:"punishment_id"`
	OrgID         string     `json:"org_id"`
	ProfileID     string     `json:"profile_id"`
	MinecraftUUID *string    `json:"minecraft_uuid,omitempty"`
	Type          string     `json:"type"`            // BAN | MUTE | KICK | WARN
	Reason        string     `json:"reason"`
	IssuedBy      *string    `json:"issued_by,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	IssuedAt      time.Time  `json:"issued_at"`
}

// JoinFlaggedEvent is published when a suspicious join scores ≥ FLAG threshold.
// The Discord bot subscribes and posts a staff alert embed.
type JoinFlaggedEvent struct {
	ProfileID      string    `json:"profile_id"`
	MinecraftUUID  string    `json:"minecraft_uuid"`
	Username       string    `json:"username"`
	IP             string    `json:"ip"`
	OrgID          string    `json:"org_id"`
	ServerID       string    `json:"server_id"`
	SuspicionScore int       `json:"suspicion_score"`
	Flags          []string  `json:"flags"`
	Action         string    `json:"action"`
	JoinedAt       time.Time `json:"joined_at"`
}

// MarshalEvent encodes a PunishmentEvent to JSON for pub/sub.
func MarshalEvent(e PunishmentEvent) (string, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("marshal event: %w", err)
	}
	return string(b), nil
}

// UnmarshalEvent decodes a pub/sub payload into a PunishmentEvent.
func UnmarshalEvent(payload string) (PunishmentEvent, error) {
	var e PunishmentEvent
	if err := json.Unmarshal([]byte(payload), &e); err != nil {
		return e, fmt.Errorf("unmarshal event: %w", err)
	}
	return e, nil
}
