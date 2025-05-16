package types

import "gate/pkg/gate/db"

// DynamicConfig holds the in-memory view of all dynamic config
// loaded from the persistent database.
type DynamicConfig struct {
	Gate         *db.Gate
	Shields      map[string]*db.Shield
	Endpoints    map[string]*db.Endpoint
	RoutingRules map[string]*db.RoutingRule
}
