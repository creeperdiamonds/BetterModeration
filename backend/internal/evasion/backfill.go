package evasion

import (
	"context"
	"fmt"
	"log"
)

// BackfillBannedOfflineUUIDs seeds banned_offline_uuids for all existing BAN
// punishments that have known usernames in player_usernames. Safe to call
// multiple times — uses INSERT IGNORE on the (offline_uuid, org_id) PK.
// Returns the total number of offline UUID rows written.
func (s *Service) BackfillBannedOfflineUUIDs(ctx context.Context) (int, error) {
	// Fetch every (profile_id, org_id) pair that has an active or historical BAN.
	rows, err := s.db.QueryxContext(ctx, `
		SELECT DISTINCT p.profile_id, p.org_id
		FROM punishments p
		WHERE p.type = 'BAN'`)
	if err != nil {
		return 0, fmt.Errorf("query banned profiles: %w", err)
	}
	defer rows.Close()

	type row struct {
		ProfileID string `db:"profile_id"`
		OrgID     string `db:"org_id"`
	}

	var profiles []row
	for rows.Next() {
		var r row
		if err := rows.StructScan(&r); err != nil {
			log.Printf("[evasion/backfill] scan error: %v", err)
			continue
		}
		profiles = append(profiles, r)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()

	total := 0
	for _, p := range profiles {
		// Fetch all known usernames for this profile
		urows, err := s.db.QueryxContext(ctx,
			`SELECT username FROM player_usernames WHERE profile_id = ?`, p.ProfileID)
		if err != nil {
			log.Printf("[evasion/backfill] query usernames for %s: %v", p.ProfileID, err)
			continue
		}
		for urows.Next() {
			var username string
			if err := urows.Scan(&username); err != nil {
				continue
			}
			offlineUUID := OfflinePlayerUUID(username)
			res, err := s.db.ExecContext(ctx, `
				INSERT IGNORE INTO banned_offline_uuids
					(offline_uuid, profile_id, org_id, username, computed_at)
				VALUES (?, ?, ?, ?, NOW(3))`,
				offlineUUID, p.ProfileID, p.OrgID, username,
			)
			if err != nil {
				log.Printf("[evasion/backfill] insert %s/%s: %v", p.ProfileID, username, err)
				continue
			}
			n, _ := res.RowsAffected()
			total += int(n)
		}
		urows.Close()
	}

	log.Printf("[evasion/backfill] wrote %d banned_offline_uuids rows for %d profiles", total, len(profiles))
	return total, nil
}
