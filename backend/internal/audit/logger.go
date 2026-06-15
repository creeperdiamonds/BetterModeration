package audit

import (
	"context"
	"encoding/json"
	"log"

	"github.com/jmoiron/sqlx"
)

// Log writes an append-only audit log entry. It fires-and-forgets in a goroutine
// so callers are never blocked by the audit write.
func Log(ctx context.Context, db *sqlx.DB, orgID, actorID, actorType, action, targetID, punishmentID string, details map[string]any, ip, source string) {
	go func() {
		detailsJSON, _ := json.Marshal(details)
		_, err := db.ExecContext(ctx, `
			INSERT INTO audit_log
				(org_id, actor_id, actor_type, action, target_id, punishment_id, details, ip_address, source, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(3))`,
			orgID,
			nullStr(actorID),
			actorType,
			action,
			nullStr(targetID),
			nullStr(punishmentID),
			string(detailsJSON),
			nullStr(ip),
			source,
		)
		if err != nil {
			log.Printf("[audit] failed to write log entry (action=%s): %v", action, err)
		}
	}()
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
