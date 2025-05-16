package gate

import (
	"context"
	"log"
	"sync"
	"time"

	"gate/pkg/gate/db"

	"github.com/redis/go-redis/v9"
)

// DynamicConfig holds the in-memory view of all dynamic config
// loaded from the persistent database.
type DynamicConfig struct {
	Gate         *db.Gate
	Shields      map[string]*db.Shield
	Endpoints    map[string]*db.Endpoint
	RoutingRules map[string]*db.RoutingRule
}

// DynamicConfigLoader loads and caches config from the database.
type DynamicConfigLoader struct {
	store  db.Store
	mu     sync.RWMutex
	config DynamicConfig
}

// NewDynamicConfigLoader creates a new loader and loads initial config from DB.
func NewDynamicConfigLoader(ctx context.Context, store db.Store) (*DynamicConfigLoader, error) {
	loader := &DynamicConfigLoader{store: store}
	if err := loader.Refresh(ctx); err != nil {
		return nil, err
	}
	return loader, nil
}

// Refresh reloads all config from the database.
func (l *DynamicConfigLoader) Refresh(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	gate, err := l.store.GetGate(ctx, l.config.Gate.GateID)
	if err != nil {
		return err
	}
	shields, err := l.store.ListShields(ctx)
	if err != nil {
		return err
	}
	endpoints := make([]*db.Endpoint, 0)
	for _, shield := range shields {
		eps, err := l.store.ListEndpoints(ctx, shield.ShieldID)
		if err != nil {
			return err
		}
		endpoints = append(endpoints, eps...)
	}
	rules := make([]*db.RoutingRule, 0)
	for _, shield := range shields {
		rs, err := l.store.ListRoutingRules(ctx, shield.ShieldID)
		if err != nil {
			return err
		}
		rules = append(rules, rs...)
	}

	cfg := DynamicConfig{
		Gate:         gate,
		Shields:      make(map[string]*db.Shield),
		Endpoints:    make(map[string]*db.Endpoint),
		RoutingRules: make(map[string]*db.RoutingRule),
	}
	for _, s := range shields {
		cfg.Shields[s.ShieldID] = s
	}
	for _, e := range endpoints {
		cfg.Endpoints[e.EndpointID] = e
	}
	for _, r := range rules {
		cfg.RoutingRules[r.RuleID] = r
	}

	l.config = cfg
	return nil
}

// GetConfig returns a copy of the current dynamic config.
func (l *DynamicConfigLoader) GetConfig() DynamicConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.config
}

// ListenAndRefreshOnRedis subscribes to a Redis channel and refreshes config on message.
func (l *DynamicConfigLoader) ListenAndRefreshOnRedis(ctx context.Context, redisOpts *redis.Options, channel string) {
	client := redis.NewClient(redisOpts)
	pubsub := client.Subscribe(ctx, channel)
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			_ = pubsub.Close()
			return
		case msg := <-ch:
			log.Printf("[DynamicConfigLoader] Received Redis pubsub message: %s", msg.Payload)
			// Debounce rapid updates
			time.Sleep(100 * time.Millisecond)
			if err := l.Refresh(ctx); err != nil {
				log.Printf("[DynamicConfigLoader] Error refreshing config: %v", err)
			} else {
				log.Printf("[DynamicConfigLoader] Config refreshed from database.")
			}
		}
	}
}
