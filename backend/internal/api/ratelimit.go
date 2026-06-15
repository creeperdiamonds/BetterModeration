package api

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimit returns middleware that enforces a sliding-window rate limit.
// key extracts the rate-limit identity from the request (e.g. IP or profile ID).
// If Redis is unavailable the request is allowed through and the error is logged.
func RateLimit(rdb *redis.Client, maxReq int, window time.Duration, key func(*http.Request) string) func(http.Handler) http.Handler {
	windowSecs := int(window.Seconds())
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k := key(r)
			if k == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Bucket key changes every [window] so old counts expire naturally
			bucket := time.Now().UTC().Truncate(window).Unix()
			redisKey := fmt.Sprintf("bm:rl:%s:%d", k, bucket)

			ctx := r.Context()
			count, err := rdb.Incr(ctx, redisKey).Result()
			if err != nil {
				log.Printf("[ratelimit] redis error for key %s: %v — allowing request", redisKey, err)
				next.ServeHTTP(w, r)
				return
			}
			if count == 1 {
				rdb.Expire(ctx, redisKey, window+5*time.Second)
			}

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(maxReq))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(max(0, maxReq-int(count))))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().UTC().Truncate(window).Add(window).Unix(), 10))

			if int(count) > maxReq {
				w.Header().Set("Retry-After", strconv.Itoa(windowSecs))
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded — try again later")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// KeyByIP extracts the real client IP, respecting X-Forwarded-For from trusted proxies.
func KeyByIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain (the original client)
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			return "ip:" + ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "ip:" + r.RemoteAddr
	}
	return "ip:" + host
}

// KeyByServerID uses the authenticated server ID from context as the rate-limit key.
// Falls back to IP if the server ID is not present.
func KeyByServerID(r *http.Request) string {
	if id := serverIDFromCtx(r.Context()); id != "" {
		return "server:" + id
	}
	return KeyByIP(r)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── Input validation ──────────────────────────────────────────────────────────

// ErrTooLong is returned when a field exceeds its maximum allowed length.
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

// validateLen returns a ValidationError if s is longer than max bytes.
func validateLen(field, s string, maxBytes int) error {
	if len(s) > maxBytes {
		return &ValidationError{fmt.Sprintf("%s must not exceed %d characters", field, maxBytes)}
	}
	return nil
}

// validatePunishment request fields.
func validatePunishmentInput(reason string) error {
	if strings.TrimSpace(reason) == "" {
		return &ValidationError{"reason is required"}
	}
	return validateLen("reason", reason, 1000)
}

// validateAppealInput validates appeal fields.
func validateAppealInput(reason, evidence string) error {
	if err := validateLen("reason", reason, 2000); err != nil {
		return err
	}
	return validateLen("evidence", evidence, 4000)
}

// validateReportInput validates report fields.
func validateReportInput(description, evidence string) error {
	if err := validateLen("description", description, 2000); err != nil {
		return err
	}
	return validateLen("evidence", evidence, 4000)
}

// ── Suspicious activity detection ─────────────────────────────────────────────

// flagSuspicious increments a counter that tracks suspicious activity from a key.
// When the count exceeds threshold, it logs a warning. Does not block the request.
func flagSuspicious(ctx context.Context, rdb *redis.Client, key string, threshold int64) {
	redisKey := "bm:suspicious:" + key
	count, err := rdb.Incr(ctx, redisKey).Result()
	if err != nil {
		return
	}
	if count == 1 {
		rdb.Expire(ctx, redisKey, 24*time.Hour)
	}
	if count >= threshold {
		log.Printf("[anti-abuse] suspicious activity detected: key=%s count=%d", key, count)
	}
}
