package domain

import "time"

type Server struct {
	ID       string    `json:"id"`
	OrgID    string    `json:"org_id"`
	Name     string    `json:"name"`
	Platform Platform  `json:"platform"`
	Online   bool      `json:"online"`
	LastSeen time.Time `json:"last_seen"`
	LinkedAt time.Time `json:"linked_at"`
}
