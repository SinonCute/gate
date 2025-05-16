-- gates table
CREATE TABLE IF NOT EXISTS gates (
    gate_id VARCHAR(36) PRIMARY KEY,
    public_address VARCHAR(255) NOT NULL,
    internal_address VARCHAR(255),
    role VARCHAR(50) NOT NULL CHECK (role IN ('GLOBAL', 'REGIONAL')),
    region VARCHAR(100),
    status VARCHAR(50) NOT NULL DEFAULT 'OFFLINE' 
        CHECK (status IN ('HEALTHY', 'DEGRADED', 'UNDER_ATTACK', 'OFFLINE', 'STARTING')),
    current_players INT NOT NULL DEFAULT 0,
    max_players INT,
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    group_name VARCHAR(100),
    priority INT DEFAULT 100,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for gates table
CREATE INDEX IF NOT EXISTS idx_gates_role ON gates(role);
CREATE INDEX IF NOT EXISTS idx_gates_status ON gates(status);
CREATE INDEX IF NOT EXISTS idx_gates_region ON gates(region);
CREATE INDEX IF NOT EXISTS idx_gates_last_heartbeat ON gates(last_heartbeat);
CREATE INDEX IF NOT EXISTS idx_gates_group_name ON gates(group_name);

-- shields table
CREATE TABLE IF NOT EXISTS shields (
    shield_id VARCHAR(36) PRIMARY KEY,
    shield_name VARCHAR(255) UNIQUE NOT NULL,
    allow_offline_motd BOOLEAN DEFAULT FALSE,
    allow_regional_transfer BOOLEAN DEFAULT TRUE,
    default_regional_gate_group_name VARCHAR(100),
    fallback_global_gate_group_name VARCHAR(100),
    default_target_game_server_name VARCHAR(255),
    attack_mitigation_mode VARCHAR(100) DEFAULT 'ROUTE_TO_GLOBAL_FALLBACK'
        CHECK (attack_mitigation_mode IN ('ROUTE_TO_GLOBAL_FALLBACK', 'BLOCK_REGIONAL_ACCESS', 'MAINTENANCE_MOTD')),
    domains_limit INT,
    tunnel_limit INT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- endpoints table
CREATE TABLE IF NOT EXISTS endpoints (
    endpoint_id VARCHAR(36) PRIMARY KEY,
    shield_id VARCHAR(36) NOT NULL REFERENCES shields(shield_id) ON DELETE CASCADE,
    game_server_name VARCHAR(255) NOT NULL,
    backend_ip VARCHAR(255) NOT NULL,
    backend_port INT NOT NULL,
    protocol_type VARCHAR(10) NOT NULL DEFAULT 'TCP' CHECK (protocol_type IN ('TCP', 'UDP')),
    proxy_protocol BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (shield_id, game_server_name)
);

-- Indexes for endpoints table
CREATE INDEX IF NOT EXISTS idx_endpoints_shield_id ON endpoints(shield_id);
CREATE INDEX IF NOT EXISTS idx_endpoints_game_server_name ON endpoints(game_server_name);

-- routing_rules table
CREATE TABLE IF NOT EXISTS routing_rules (
    rule_id VARCHAR(36) PRIMARY KEY,
    shield_id VARCHAR(36) NOT NULL REFERENCES shields(shield_id) ON DELETE CASCADE,
    domains TEXT[] NOT NULL,
    target_endpoint_id VARCHAR(36) REFERENCES endpoints(endpoint_id),
    target_endpoint_name_pattern VARCHAR(255),
    edition_type VARCHAR(20) NOT NULL DEFAULT 'JAVA' CHECK (edition_type IN ('JAVA', 'BEDROCK')),
    priority INT DEFAULT 100,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for routing_rules table
CREATE INDEX IF NOT EXISTS idx_routing_rules_shield_id ON routing_rules(shield_id);
CREATE INDEX IF NOT EXISTS idx_routing_rules_domains ON routing_rules USING GIN (domains);

-- Add triggers for updated_at timestamps
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_gates_updated_at
    BEFORE UPDATE ON gates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_shields_updated_at
    BEFORE UPDATE ON shields
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_endpoints_updated_at
    BEFORE UPDATE ON endpoints
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_routing_rules_updated_at
    BEFORE UPDATE ON routing_rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column(); 