package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type subscription struct {
	ID     string `db:"id"`
	URL    string `db:"url"`
	Secret string `db:"secret"`
	Events string `db:"events"` // JSON array stored as string
}

// Dispatcher sends HMAC-signed webhook payloads to registered URLs.
type Dispatcher struct {
	db     *sqlx.DB
	client *http.Client
}

func NewDispatcher(db *sqlx.DB) *Dispatcher {
	return &Dispatcher{
		db:     db,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Dispatch fires webhooks for all subscriptions in an org that include eventType.
// Runs asynchronously — callers never block.
func (d *Dispatcher) Dispatch(ctx context.Context, orgID, eventType string, payload any) {
	go func() {
		subs, err := d.loadSubs(ctx, orgID, eventType)
		if err != nil {
			log.Printf("[webhook] failed to load subscriptions for org %s: %v", orgID, err)
			return
		}
		body, err := json.Marshal(map[string]any{
			"event":   eventType,
			"org_id":  orgID,
			"payload": payload,
			"ts":      time.Now().UTC().Unix(),
		})
		if err != nil {
			return
		}
		for _, sub := range subs {
			d.deliver(sub, body)
		}
	}()
}

func (d *Dispatcher) loadSubs(ctx context.Context, orgID, eventType string) ([]subscription, error) {
	rows, err := d.db.QueryxContext(ctx, `
		SELECT id, url, secret, events FROM webhook_subscriptions
		WHERE org_id = ? AND enabled = 1`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []subscription
	for rows.Next() {
		var s subscription
		if err := rows.StructScan(&s); err != nil {
			continue
		}
		// Filter: check if eventType is in the events JSON array
		var events []string
		if err := json.Unmarshal([]byte(s.Events), &events); err != nil {
			continue
		}
		for _, e := range events {
			if strings.EqualFold(e, eventType) || e == "*" {
				out = append(out, s)
				break
			}
		}
	}
	return out, rows.Err()
}

func (d *Dispatcher) deliver(sub subscription, body []byte) {
	sig := sign(sub.Secret, body)

	req, err := http.NewRequest(http.MethodPost, sub.URL, bytes.NewReader(body))
	if err != nil {
		log.Printf("[webhook] bad URL for sub %s: %v", sub.ID, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BM-Signature", "sha256="+sig)
	req.Header.Set("X-BM-Event", "webhook")

	resp, err := d.client.Do(req)
	if err != nil {
		log.Printf("[webhook] delivery failed to %s: %v", sub.URL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[webhook] delivered to %s (sub %s) → %d", sub.URL, sub.ID, resp.StatusCode)
	} else {
		log.Printf("[webhook] non-2xx from %s (sub %s): %d", sub.URL, sub.ID, resp.StatusCode)
	}
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature lets receivers verify that a webhook came from us.
func VerifySignature(secret, sigHeader string, body []byte) bool {
	expected := "sha256=" + sign(secret, body)
	return hmac.Equal([]byte(sigHeader), []byte(expected))
}

// TestPayload returns a dummy payload for testing webhook delivery.
func TestPayload(orgID string) map[string]any {
	return map[string]any{
		"event":   "test",
		"org_id":  orgID,
		"message": fmt.Sprintf("Test webhook from BetterModeration at %s", time.Now().UTC().Format(time.RFC3339)),
	}
}
