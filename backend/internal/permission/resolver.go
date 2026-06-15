package permission

import (
	"strconv"
	"strings"
)

// Node represents a permission node assigned to a role.
type Node struct {
	Node  string // e.g. "punish.ban.max.7d"
	Value bool   // true = allow, false = deny (negation)
}

// Set is a resolved collection of permission nodes for a subject (user/role).
type Set struct {
	nodes []Node
}

// NewSet builds a Set from a slice of nodes, sorting them so more specific
// nodes take priority over wildcards (longest path wins).
func NewSet(nodes []Node) *Set {
	sorted := make([]Node, len(nodes))
	copy(sorted, nodes)
	// Sort descending by specificity (segment count) so longest match is checked first
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if specificity(sorted[j].Node) > specificity(sorted[i].Node) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return &Set{nodes: sorted}
}

// Has returns true if the given node is granted (directly or via wildcard),
// and not overridden by a denial.
func (s *Set) Has(node string) bool {
	best, found := s.resolve(node)
	return found && best
}

// MaxDuration returns the highest max duration (in seconds) granted by nodes
// matching the pattern "punish.<action>.max.<value>".
// Returns -1 if punish.<action>.permanent is granted.
// Returns 0 if no duration node is found (action not allowed).
func (s *Set) MaxDuration(action string) int64 {
	prefix := "punish." + action + ".max."
	permNode := "punish." + action + ".permanent"

	if s.Has(permNode) {
		return -1 // permanent
	}

	var best int64 = 0
	for _, n := range s.nodes {
		if !n.Value {
			continue
		}
		if !strings.HasPrefix(n.Node, prefix) {
			continue
		}
		suffix := n.Node[len(prefix):]
		secs := parseDurationNode(suffix)
		if secs > best {
			best = secs
		}
	}
	return best
}

// RateLimit returns the max number of actions allowed per window (in seconds)
// for a given action. Pattern: "ratelimit.<action>.<count>.<window>".
// Returns count=0 if no rate limit node is found (unlimited).
func (s *Set) RateLimit(action string) (count int, windowSeconds int64) {
	prefix := "ratelimit." + strings.ToLower(action) + "."

	for _, n := range s.nodes {
		if !n.Value || !strings.HasPrefix(n.Node, prefix) {
			continue
		}
		parts := strings.Split(n.Node[len(prefix):], ".")
		if len(parts) != 2 {
			continue
		}
		c, err1 := strconv.Atoi(parts[0])
		w := parseDurationNode(parts[1])
		if err1 == nil && w > 0 {
			// Take the most permissive rate limit (highest count)
			if c > count {
				count = c
				windowSeconds = w
			}
		}
	}
	return
}

// resolve finds the most specific matching node for the given permission.
// Returns (value, found).
func (s *Set) resolve(node string) (bool, bool) {
	node = strings.ToLower(node)

	// Exact match first
	for _, n := range s.nodes {
		if strings.ToLower(n.Node) == node {
			return n.Value, true
		}
	}

	// Wildcard match — walk up the node tree checking for wildcards
	// e.g. "punish.ban" checks:
	//   "punish.ban.*"  (not applicable here but for children)
	//   "punish.*"
	//   "*"
	parts := strings.Split(node, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		wildcard := strings.Join(parts[:i], ".") + ".*"
		for _, n := range s.nodes {
			if strings.ToLower(n.Node) == wildcard {
				return n.Value, true
			}
		}
	}

	return false, false
}

// specificity returns the number of segments in a node path.
// Higher = more specific = higher priority.
func specificity(node string) int {
	if strings.HasSuffix(node, ".*") {
		return strings.Count(node, ".") - 1
	}
	return strings.Count(node, ".")
}

// parseDurationNode converts a duration string like "1h", "7d", "30d", "1y"
// into seconds.
func parseDurationNode(s string) int64 {
	if len(s) < 2 {
		return 0
	}
	unit := s[len(s)-1]
	val, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
	if err != nil || val <= 0 {
		return 0
	}
	switch unit {
	case 'h':
		return val * 3600
	case 'd':
		return val * 86400
	case 'w':
		return val * 604800
	case 'm': // months
		return val * 2592000
	case 'y':
		return val * 31536000
	}
	return 0
}
