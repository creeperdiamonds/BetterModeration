package evasion

import (
	"crypto/md5"
	"fmt"
)

// OfflinePlayerUUID computes the deterministic UUID that Java's offline mode assigns
// to a player. Matches: UUID.nameUUIDFromBytes(("OfflinePlayer:"+name).getBytes("UTF-8"))
// which is a version-3 (MD5-based) UUID with no namespace prefix.
func OfflinePlayerUUID(username string) string {
	h := md5.Sum([]byte("OfflinePlayer:" + username))
	h[6] = (h[6] & 0x0f) | 0x30 // version 3 (MD5)
	h[8] = (h[8] & 0x3f) | 0x80 // IETF variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}
