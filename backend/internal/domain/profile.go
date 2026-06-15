package domain

import "time"

type Profile struct {
	ID            string    `json:"id"`
	DiscordID     string    `json:"discord_id"`
	MinecraftUUID string    `json:"minecraft_uuid"`
	LinkedAt      time.Time `json:"linked_at"`
	IPHistory     []string  `json:"ip_history"`
	CreatedAt     time.Time `json:"created_at"`
}
