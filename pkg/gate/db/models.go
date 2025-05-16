package db

import (
	"time"

	"github.com/google/uuid"
)

// Gate represents a proxy instance in the network
type Gate struct {
	GateID          string     `db:"gate_id"`
	PublicAddress   string     `db:"public_address"`
	InternalAddress string     `db:"internal_address"`
	Role            GateRole   `db:"role"`
	Region          string     `db:"region"`
	Status          GateStatus `db:"status"`
	CurrentPlayers  int        `db:"current_players"`
	MaxPlayers      int        `db:"max_players"`
	LastHeartbeat   time.Time  `db:"last_heartbeat"`
	GroupName       string     `db:"group_name"`
	Priority        int        `db:"priority"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
}

// GateRole represents the role of a gate in the network
type GateRole string

const (
	GateRoleGlobal   GateRole = "GLOBAL"
	GateRoleRegional GateRole = "REGIONAL"
)

// GateStatus represents the operational status of a gate
type GateStatus string

const (
	GateStatusHealthy     GateStatus = "HEALTHY"
	GateStatusDegraded    GateStatus = "DEGRADED"
	GateStatusUnderAttack GateStatus = "UNDER_ATTACK"
	GateStatusOffline     GateStatus = "OFFLINE"
	GateStatusStarting    GateStatus = "STARTING"
)

// Shield represents a policy and feature set for routing
type Shield struct {
	ShieldID                 string    `db:"shield_id"`
	ShieldName               string    `db:"shield_name"`
	AllowOfflineMotd         bool      `db:"allow_offline_motd"`
	AllowRegionalTransfer    bool      `db:"allow_regional_transfer"`
	DefaultRegionalGateGroup string    `db:"default_regional_gate_group_name"`
	FallbackGlobalGateGroup  string    `db:"fallback_global_gate_group_name"`
	DefaultTargetGameServer  string    `db:"default_target_game_server_name"`
	AttackMitigationMode     string    `db:"attack_mitigation_mode"`
	DomainsLimit             int       `db:"domains_limit"`
	TunnelLimit              int       `db:"tunnel_limit"`
	CreatedAt                time.Time `db:"created_at"`
	UpdatedAt                time.Time `db:"updated_at"`
}

// Endpoint represents a backend game server
type Endpoint struct {
	EndpointID     string    `db:"endpoint_id"`
	ShieldID       string    `db:"shield_id"`
	GameServerName string    `db:"game_server_name"`
	BackendIP      string    `db:"backend_ip"`
	BackendPort    int       `db:"backend_port"`
	ProtocolType   string    `db:"protocol_type"`
	ProxyProtocol  bool      `db:"proxy_protocol"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

// RoutingRule represents a rule for routing traffic
type RoutingRule struct {
	RuleID                string    `db:"rule_id"`
	ShieldID              string    `db:"shield_id"`
	Domains               []string  `db:"domains"`
	TargetEndpointID      string    `db:"target_endpoint_id"`
	TargetEndpointPattern string    `db:"target_endpoint_name_pattern"`
	EditionType           string    `db:"edition_type"`
	Priority              int       `db:"priority"`
	CreatedAt             time.Time `db:"created_at"`
	UpdatedAt             time.Time `db:"updated_at"`
}

// NewGate creates a new Gate instance with default values
func NewGate() *Gate {
	return &Gate{
		GateID:         uuid.New().String(),
		Status:         GateStatusStarting,
		CurrentPlayers: 0,
		Priority:       100,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// NewShield creates a new Shield instance with default values
func NewShield() *Shield {
	return &Shield{
		ShieldID:              uuid.New().String(),
		AllowRegionalTransfer: true,
		AttackMitigationMode:  "ROUTE_TO_GLOBAL_FALLBACK",
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
}

// NewEndpoint creates a new Endpoint instance with default values
func NewEndpoint() *Endpoint {
	return &Endpoint{
		EndpointID:   uuid.New().String(),
		ProtocolType: "TCP",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

// NewRoutingRule creates a new RoutingRule instance with default values
func NewRoutingRule() *RoutingRule {
	return &RoutingRule{
		RuleID:      uuid.New().String(),
		EditionType: "JAVA",
		Priority:    100,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}
