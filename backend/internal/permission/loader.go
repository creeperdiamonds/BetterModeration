package permission

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

const cacheTTL = 5 * time.Minute

// Loader resolves and caches permission sets for guild members.
type Loader struct {
	db    *sqlx.DB
	redis *redis.Client
}

func NewLoader(db *sqlx.DB, redis *redis.Client) *Loader {
	return &Loader{db: db, redis: redis}
}

// Load returns a resolved permission Set for a member in a guild.
// Checks Redis first; on miss, queries the DB and caches the result.
func (l *Loader) Load(ctx context.Context, guildID string, discordRoleIDs []string) (*Set, error) {
	key := cacheKey(guildID, discordRoleIDs)

	// Check cache
	cached, err := l.redis.Get(ctx, key).Bytes()
	if err == nil {
		return deserialize(cached)
	}
	if err != redis.Nil {
		// Redis error — log and fall through to DB
		_ = err
	}

	// Cache miss — load from DB
	nodes, err := l.loadFromDB(ctx, guildID, discordRoleIDs)
	if err != nil {
		return nil, fmt.Errorf("loading permission nodes: %w", err)
	}

	set := NewSet(nodes)

	// Cache the serialized set
	if data, err := serialize(nodes); err == nil {
		l.redis.Set(ctx, key, data, cacheTTL)
	}

	return set, nil
}

// Invalidate clears cached permission sets for a guild.
// Call this whenever mod role nodes are changed in the dashboard.
func (l *Loader) Invalidate(ctx context.Context, guildID string) error {
	pattern := fmt.Sprintf("perms:%s:*", guildID)
	keys, err := l.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return l.redis.Del(ctx, keys...).Err()
}

// loadFromDB fetches all permission nodes for the given Discord role IDs in a guild.
// Joins mod_roles (which map Discord role IDs) → role_permission_nodes.
func (l *Loader) loadFromDB(ctx context.Context, guildID string, discordRoleIDs []string) ([]Node, error) {
	if len(discordRoleIDs) == 0 {
		return nil, nil
	}

	query, args, err := sqlx.In(`
		SELECT rpn.node, rpn.value
		FROM role_permission_nodes rpn
		JOIN mod_roles mr ON mr.id = rpn.role_id
		JOIN servers s ON s.org_id = mr.org_id
		WHERE s.platform = 'DISCORD'
		  AND s.platform_id = ?
		  AND mr.discord_role_id IN (?)
	`, guildID, discordRoleIDs)
	if err != nil {
		return nil, err
	}

	rows, err := l.db.QueryxContext(ctx, l.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var nodeStr string
		var val bool
		if err := rows.Scan(&nodeStr, &val); err != nil {
			return nil, err
		}
		nodes = append(nodes, Node{Node: nodeStr, Value: val})
	}
	return nodes, rows.Err()
}

// cacheKey builds a stable Redis key for a guild + role set combination.
// Sorts role IDs so order doesn't matter.
func cacheKey(guildID string, roleIDs []string) string {
	sorted := make([]string, len(roleIDs))
	copy(sorted, roleIDs)
	sort.Strings(sorted)

	h := sha256.New()
	for _, r := range sorted {
		h.Write([]byte(r))
	}
	return fmt.Sprintf("perms:%s:%x", guildID, h.Sum(nil)[:8])
}

func serialize(nodes []Node) ([]byte, error) {
	return json.Marshal(nodes)
}

func deserialize(data []byte) (*Set, error) {
	var nodes []Node
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, err
	}
	return NewSet(nodes), nil
}
