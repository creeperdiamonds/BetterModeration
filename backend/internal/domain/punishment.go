package domain

import "time"

type PunishmentType string

const (
	PunishmentBan  PunishmentType = "BAN"
	PunishmentMute PunishmentType = "MUTE"
	PunishmentKick PunishmentType = "KICK"
	PunishmentWarn PunishmentType = "WARN"
	PunishmentNote PunishmentType = "NOTE"
)

type Platform string

const (
	PlatformDiscord   Platform = "DISCORD"
	PlatformMinecraft Platform = "MINECRAFT"
	PlatformSystem    Platform = "SYSTEM"
)

type Punishment struct {
	ID        string         `json:"id"`
	ProfileID string         `json:"profile_id"`
	Type      PunishmentType `json:"type"`
	Reason    string         `json:"reason"`
	IssuedBy  string         `json:"issued_by"`
	IssuedAt  time.Time      `json:"issued_at"`
	ExpiresAt *time.Time     `json:"expires_at"`
	Platform  Platform       `json:"platform"`
	ServerID  string         `json:"server_id"`
	Active    bool           `json:"active"`
	Public    bool           `json:"public"`
}
