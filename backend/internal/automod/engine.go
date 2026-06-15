package automod

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
)

type Rule struct {
	ID              string  `db:"id"`
	OrgID           string  `db:"org_id"`
	Name            string  `db:"name"`
	Enabled         bool    `db:"enabled"`
	TestMode        bool    `db:"test_mode"`
	TriggerType     string  `db:"trigger_type"`
	TriggerValue    *string `db:"trigger_value"`
	ActionType      string  `db:"action_type"`
	ActionDuration  *int64  `db:"action_duration_seconds"`
	Platform        string  `db:"platform"`
	Priority        int     `db:"priority"`
}

// Action is returned by Evaluate when a rule fires.
type Action struct {
	Rule     Rule
	TestMode bool
}

// Engine loads AutoMod rules from DB (cached in Redis) and evaluates messages.
type Engine struct {
	db    *sqlx.DB
	redis *redis.Client
	mu    sync.RWMutex
	cache map[string][]Rule // guildID → rules
	ttl   map[string]time.Time
}

const cacheTTL = 5 * time.Minute

func NewEngine(db *sqlx.DB, redis *redis.Client) *Engine {
	return &Engine{
		db:    db,
		redis: redis,
		cache: make(map[string][]Rule),
		ttl:   make(map[string]time.Time),
	}
}

// Evaluate runs all enabled rules for the guild against the given message.
// Returns the first matching Action (sorted by priority), or nil if no rule fires.
func (e *Engine) Evaluate(ctx context.Context, orgID, message, authorID string) *Action {
	rules, err := e.rules(ctx, orgID)
	if err != nil {
		log.Printf("[automod] failed to load rules for org %s: %v", orgID, err)
		return nil
	}

	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if r.Platform != "DISCORD" && r.Platform != "ALL" {
			continue
		}
		if matches(r, message) {
			return &Action{Rule: r, TestMode: r.TestMode}
		}
	}
	return nil
}

func matches(r Rule, message string) bool {
	switch r.TriggerType {
	case "WORD_MATCH":
		if r.TriggerValue == nil {
			return false
		}
		lower := strings.ToLower(message)
		for _, word := range strings.Split(*r.TriggerValue, ",") {
			if strings.Contains(lower, strings.TrimSpace(strings.ToLower(word))) {
				return true
			}
		}

	case "REGEX":
		if r.TriggerValue == nil {
			return false
		}
		re, err := regexp.Compile(*r.TriggerValue)
		if err != nil {
			return false
		}
		return re.MatchString(message)

	case "CAPS":
		if len(message) < 10 {
			return false
		}
		upper := 0
		total := 0
		for _, ch := range message {
			if unicode.IsLetter(ch) {
				total++
				if unicode.IsUpper(ch) {
					upper++
				}
			}
		}
		return total > 0 && float64(upper)/float64(total) > 0.7

	case "LINK":
		lower := strings.ToLower(message)
		return strings.Contains(lower, "http://") || strings.Contains(lower, "https://")

	case "INVITE":
		lower := strings.ToLower(message)
		return strings.Contains(lower, "discord.gg/") || strings.Contains(lower, "discord.com/invite/")

	case "MENTION_SPAM":
		count := strings.Count(message, "<@")
		return count >= 5

	case "SPAM":
		// Basic: message over 500 chars with lots of repetition
		if len(message) < 100 {
			return false
		}
		// Check if any 10-char substring repeats 3+ times
		if len(message) >= 10 {
			sub := message[:10]
			return strings.Count(message, sub) >= 3
		}
	}
	return false
}

func (e *Engine) rules(ctx context.Context, orgID string) ([]Rule, error) {
	e.mu.RLock()
	rules, ok := e.cache[orgID]
	exp := e.ttl[orgID]
	e.mu.RUnlock()

	if ok && time.Now().Before(exp) {
		return rules, nil
	}

	// Try Redis
	key := "automod:rules:" + orgID
	cached, err := e.redis.Get(ctx, key).Result()
	if err == nil {
		var loaded []Rule
		if json.Unmarshal([]byte(cached), &loaded) == nil {
			e.mu.Lock()
			e.cache[orgID] = loaded
			e.ttl[orgID] = time.Now().Add(cacheTTL)
			e.mu.Unlock()
			return loaded, nil
		}
	}

	// Load from DB
	rows, err := e.db.QueryxContext(ctx, `
		SELECT id, org_id, name, enabled, test_mode, trigger_type, trigger_value,
		       action_type, action_duration_seconds, platform, priority
		FROM automod_rules
		WHERE org_id = ? AND enabled = 1
		ORDER BY priority ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var loaded []Rule
	for rows.Next() {
		var r Rule
		if err := rows.StructScan(&r); err != nil {
			continue
		}
		loaded = append(loaded, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Cache in Redis + local
	if b, err := json.Marshal(loaded); err == nil {
		e.redis.Set(ctx, key, string(b), cacheTTL)
	}
	e.mu.Lock()
	e.cache[orgID] = loaded
	e.ttl[orgID] = time.Now().Add(cacheTTL)
	e.mu.Unlock()

	return loaded, nil
}

// Invalidate clears the cached rules for an org (call after rule changes).
func (e *Engine) Invalidate(ctx context.Context, orgID string) {
	e.redis.Del(ctx, "automod:rules:"+orgID)
	e.mu.Lock()
	delete(e.cache, orgID)
	delete(e.ttl, orgID)
	e.mu.Unlock()
}
