package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// Store defines the interface for database operations
type Store interface {
	// Gate operations
	CreateGate(ctx context.Context, gate *Gate) error
	GetGate(ctx context.Context, gateID string) (*Gate, error)
	UpdateGate(ctx context.Context, gate *Gate) error
	DeleteGate(ctx context.Context, gateID string) error
	ListGates(ctx context.Context) ([]*Gate, error)
	UpdateGateStatus(ctx context.Context, gateID string, status GateStatus, currentPlayers int) error

	// Shield operations
	CreateShield(ctx context.Context, shield *Shield) error
	GetShield(ctx context.Context, shieldID string) (*Shield, error)
	UpdateShield(ctx context.Context, shield *Shield) error
	DeleteShield(ctx context.Context, shieldID string) error
	ListShields(ctx context.Context) ([]*Shield, error)

	// Endpoint operations
	CreateEndpoint(ctx context.Context, endpoint *Endpoint) error
	GetEndpoint(ctx context.Context, endpointID string) (*Endpoint, error)
	UpdateEndpoint(ctx context.Context, endpoint *Endpoint) error
	DeleteEndpoint(ctx context.Context, endpointID string) error
	ListEndpoints(ctx context.Context, shieldID string) ([]*Endpoint, error)

	// RoutingRule operations
	CreateRoutingRule(ctx context.Context, rule *RoutingRule) error
	GetRoutingRule(ctx context.Context, ruleID string) (*RoutingRule, error)
	UpdateRoutingRule(ctx context.Context, rule *RoutingRule) error
	DeleteRoutingRule(ctx context.Context, ruleID string) error
	ListRoutingRules(ctx context.Context, shieldID string) ([]*RoutingRule, error)
}

// store implements the Store interface
type store struct {
	db *sqlx.DB
}

// NewStore creates a new database store
func NewStore(cfg *Config) (Store, error) {
	db, err := NewDB(cfg)
	if err != nil {
		return nil, err
	}
	return &store{db: db}, nil
}

// Gate operations
func (s *store) CreateGate(ctx context.Context, gate *Gate) error {
	query := `
		INSERT INTO gates (
			gate_id, public_address, internal_address, role, region,
			status, current_players, max_players, last_heartbeat,
			group_name, priority, created_at, updated_at
		) VALUES (
			:gate_id, :public_address, :internal_address, :role, :region,
			:status, :current_players, :max_players, :last_heartbeat,
			:group_name, :priority, :created_at, :updated_at
		)`
	_, err := s.db.NamedExecContext(ctx, query, gate)
	return err
}

func (s *store) GetGate(ctx context.Context, gateID string) (*Gate, error) {
	var gate Gate
	err := s.db.GetContext(ctx, &gate, "SELECT * FROM gates WHERE gate_id = $1", gateID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &gate, err
}

func (s *store) UpdateGate(ctx context.Context, gate *Gate) error {
	query := `
		UPDATE gates SET
			public_address = :public_address,
			internal_address = :internal_address,
			role = :role,
			region = :region,
			status = :status,
			current_players = :current_players,
			max_players = :max_players,
			last_heartbeat = :last_heartbeat,
			group_name = :group_name,
			priority = :priority,
			updated_at = :updated_at
		WHERE gate_id = :gate_id`
	_, err := s.db.NamedExecContext(ctx, query, gate)
	return err
}

func (s *store) DeleteGate(ctx context.Context, gateID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM gates WHERE gate_id = $1", gateID)
	return err
}

func (s *store) ListGates(ctx context.Context) ([]*Gate, error) {
	var gates []*Gate
	err := s.db.SelectContext(ctx, &gates, "SELECT * FROM gates")
	return gates, err
}

func (s *store) UpdateGateStatus(ctx context.Context, gateID string, status GateStatus, currentPlayers int) error {
	query := `
		UPDATE gates SET
			status = $1,
			current_players = $2,
			last_heartbeat = $3,
			updated_at = $3
		WHERE gate_id = $4`
	_, err := s.db.ExecContext(ctx, query, status, currentPlayers, time.Now(), gateID)
	return err
}

// Shield operations
func (s *store) CreateShield(ctx context.Context, shield *Shield) error {
	query := `
		INSERT INTO shields (
			shield_id, shield_name, allow_offline_motd, allow_regional_transfer,
			default_regional_gate_group_name, fallback_global_gate_group_name,
			default_target_game_server_name, attack_mitigation_mode,
			domains_limit, tunnel_limit, created_at, updated_at
		) VALUES (
			:shield_id, :shield_name, :allow_offline_motd, :allow_regional_transfer,
			:default_regional_gate_group_name, :fallback_global_gate_group_name,
			:default_target_game_server_name, :attack_mitigation_mode,
			:domains_limit, :tunnel_limit, :created_at, :updated_at
		)`
	_, err := s.db.NamedExecContext(ctx, query, shield)
	return err
}

func (s *store) GetShield(ctx context.Context, shieldID string) (*Shield, error) {
	var shield Shield
	err := s.db.GetContext(ctx, &shield, "SELECT * FROM shields WHERE shield_id = $1", shieldID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &shield, err
}

func (s *store) UpdateShield(ctx context.Context, shield *Shield) error {
	query := `
		UPDATE shields SET
			shield_name = :shield_name,
			allow_offline_motd = :allow_offline_motd,
			allow_regional_transfer = :allow_regional_transfer,
			default_regional_gate_group_name = :default_regional_gate_group_name,
			fallback_global_gate_group_name = :fallback_global_gate_group_name,
			default_target_game_server_name = :default_target_game_server_name,
			attack_mitigation_mode = :attack_mitigation_mode,
			domains_limit = :domains_limit,
			tunnel_limit = :tunnel_limit,
			updated_at = :updated_at
		WHERE shield_id = :shield_id`
	_, err := s.db.NamedExecContext(ctx, query, shield)
	return err
}

func (s *store) DeleteShield(ctx context.Context, shieldID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM shields WHERE shield_id = $1", shieldID)
	return err
}

func (s *store) ListShields(ctx context.Context) ([]*Shield, error) {
	var shields []*Shield
	err := s.db.SelectContext(ctx, &shields, "SELECT * FROM shields")
	return shields, err
}

// Endpoint operations
func (s *store) CreateEndpoint(ctx context.Context, endpoint *Endpoint) error {
	query := `
		INSERT INTO endpoints (
			endpoint_id, shield_id, game_server_name, backend_ip,
			backend_port, protocol_type, proxy_protocol,
			created_at, updated_at
		) VALUES (
			:endpoint_id, :shield_id, :game_server_name, :backend_ip,
			:backend_port, :protocol_type, :proxy_protocol,
			:created_at, :updated_at
		)`
	_, err := s.db.NamedExecContext(ctx, query, endpoint)
	return err
}

func (s *store) GetEndpoint(ctx context.Context, endpointID string) (*Endpoint, error) {
	var endpoint Endpoint
	err := s.db.GetContext(ctx, &endpoint, "SELECT * FROM endpoints WHERE endpoint_id = $1", endpointID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &endpoint, err
}

func (s *store) UpdateEndpoint(ctx context.Context, endpoint *Endpoint) error {
	query := `
		UPDATE endpoints SET
			shield_id = :shield_id,
			game_server_name = :game_server_name,
			backend_ip = :backend_ip,
			backend_port = :backend_port,
			protocol_type = :protocol_type,
			proxy_protocol = :proxy_protocol,
			updated_at = :updated_at
		WHERE endpoint_id = :endpoint_id`
	_, err := s.db.NamedExecContext(ctx, query, endpoint)
	return err
}

func (s *store) DeleteEndpoint(ctx context.Context, endpointID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM endpoints WHERE endpoint_id = $1", endpointID)
	return err
}

func (s *store) ListEndpoints(ctx context.Context, shieldID string) ([]*Endpoint, error) {
	var endpoints []*Endpoint
	err := s.db.SelectContext(ctx, &endpoints, "SELECT * FROM endpoints WHERE shield_id = $1", shieldID)
	return endpoints, err
}

// RoutingRule operations
func (s *store) CreateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	query := `
		INSERT INTO routing_rules (
			rule_id, shield_id, domains, target_endpoint_id,
			target_endpoint_name_pattern, edition_type, priority,
			created_at, updated_at
		) VALUES (
			:rule_id, :shield_id, :domains, :target_endpoint_id,
			:target_endpoint_name_pattern, :edition_type, :priority,
			:created_at, :updated_at
		)`
	_, err := s.db.NamedExecContext(ctx, query, rule)
	return err
}

func (s *store) GetRoutingRule(ctx context.Context, ruleID string) (*RoutingRule, error) {
	var rule RoutingRule
	err := s.db.GetContext(ctx, &rule, "SELECT * FROM routing_rules WHERE rule_id = $1", ruleID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &rule, err
}

func (s *store) UpdateRoutingRule(ctx context.Context, rule *RoutingRule) error {
	query := `
		UPDATE routing_rules SET
			shield_id = :shield_id,
			domains = :domains,
			target_endpoint_id = :target_endpoint_id,
			target_endpoint_name_pattern = :target_endpoint_name_pattern,
			edition_type = :edition_type,
			priority = :priority,
			updated_at = :updated_at
		WHERE rule_id = :rule_id`
	_, err := s.db.NamedExecContext(ctx, query, rule)
	return err
}

func (s *store) DeleteRoutingRule(ctx context.Context, ruleID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM routing_rules WHERE rule_id = $1", ruleID)
	return err
}

func (s *store) ListRoutingRules(ctx context.Context, shieldID string) ([]*RoutingRule, error) {
	var rules []*RoutingRule
	err := s.db.SelectContext(ctx, &rules, "SELECT * FROM routing_rules WHERE shield_id = $1", shieldID)
	return rules, err
}
