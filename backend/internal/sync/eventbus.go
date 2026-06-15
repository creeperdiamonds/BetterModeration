package sync

import (
	"context"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

// EventBus provides pub/sub messaging via Redis.
type EventBus struct {
	client *redis.Client
}

// NewEventBus creates a new EventBus using the provided Redis client.
func NewEventBus(client *redis.Client) *EventBus {
	return &EventBus{client: client}
}

// Publish sends a payload to the given channel.
func (eb *EventBus) Publish(ctx context.Context, channel, payload string) error {
	if err := eb.client.Publish(ctx, channel, payload).Err(); err != nil {
		return fmt.Errorf("eventbus publish to %q: %w", channel, err)
	}
	return nil
}

// Subscribe listens on the given channel and calls handler for each message.
// It blocks until the context is cancelled. Intended to be run in a goroutine.
func (eb *EventBus) Subscribe(ctx context.Context, channel string, handler func(payload string)) {
	pubsub := eb.client.Subscribe(ctx, channel)
	defer func() {
		if err := pubsub.Close(); err != nil {
			log.Printf("eventbus: error closing subscription on %q: %v", channel, err)
		}
	}()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			handler(msg.Payload)
		}
	}
}
