package config

import (
	"fmt"

	bconfig "gate/pkg/edition/bedrock/config"
	jconfig "gate/pkg/edition/java/config"
	"gate/pkg/gate/db"
	"gate/pkg/util/validation"
)

// DefaultConfig is a default Config.
var DefaultConfig = Config{
	Config: jconfig.DefaultConfig,
	Editions: Editions{
		Java: Java{
			Enabled: true,
			Config:  jconfig.DefaultConfig,
		},
		Bedrock: Bedrock{
			Enabled: false,
			Config:  bconfig.DefaultConfig,
		},
	},
	HealthService: HealthService{
		Enabled: false,
		Bind:    "0.0.0.0:9090",
	},
	API: API{
		Enabled: false,
		Config:  api.DefaultConfig,
	},
	Database: *db.DefaultConfig(),
	Redis:    *DefaultRedisConfig(),
	Gate:     *DefaultGateInstanceConfig(),
}

type GateInstanceConfig struct {
	// If empty, "default" will be used.
	GateID        string `json:"gateId" yaml:"gateId"`
	Role          string `json:"role" yaml:"role"`
	PublicAddress string `json:"publicAddress" yaml:"publicAddress"`
}

func DefaultGateInstanceConfig() *GateInstanceConfig {
	return &GateInstanceConfig{
		GateID:        "default",
		Role:          "regional",
		PublicAddress: "",
	}
}

// RedisConfig holds Redis connection and pubsub info.
type RedisConfig struct {
	Host     string `json:"host" yaml:"host"`
	Port     int    `json:"port" yaml:"port"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
	Channel  string `json:"channel" yaml:"channel"`
}

func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Password: "",
		DB:       0,
		Channel:  "gate_config_updates",
	}
}

// Config is the root configuration of Gate.
type Config struct {
	// Config is the Java edition configuration.
	// It is an alias for Editions.Java.Config.
	Config jconfig.Config `json:"config,omitempty" yaml:"config,omitempty"`
	// See Editions struct.
	Editions Editions `json:"editions,omitempty" yaml:"editions,omitempty"`
	// See HealthService struct.
	HealthService HealthService `json:"healthService,omitempty" yaml:"healthService,omitempty"`
	// See Database struct.
	Database db.Config `json:"database,omitempty" yaml:"database,omitempty"`
	// See Redis struct.
	Redis RedisConfig `json:"redis,omitempty" yaml:"redis,omitempty"`
	// See GateInstanceConfig struct.
	Gate GateInstanceConfig `json:"gate,omitempty" yaml:"gate,omitempty"`
}

// Editions provides Minecraft edition specific configs.
// If multiple editions are enabled, cross-play is activated.
// If no edition is enabled, all will be enabled.
type Editions struct {
	Java    Java    `json:"java,omitempty" yaml:"java,omitempty"`
	Bedrock Bedrock `json:"bedrock,omitempty" yaml:"bedrock,omitempty"`
}

// Java edition.
type Java struct {
	Enabled bool           `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Config  jconfig.Config `json:"config,omitempty" yaml:"config,omitempty"`
}

// Bedrock edition.
type Bedrock struct {
	Enabled bool           `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Config  bconfig.Config `json:"config,omitempty" yaml:"config,omitempty"`
}

// HealthService is a GRPC health probe service for use with Kubernetes pods.
// (https://github.com/grpc-ecosystem/grpc-health-probe)
type HealthService struct {
	Enabled bool   `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Bind    string `json:"bind,omitempty" yaml:"bind,omitempty"`
}

// Validate validates a Config and all enabled edition configs (Java / Bedrock).
func (c *Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }
	if c == nil {
		e("config must not be nil")
		return
	}

	if c.HealthService.Enabled {
		if err := validation.ValidHostPort(c.HealthService.Bind); err != nil {
			e("Invalid health probe bind address %q: %v", c.HealthService.Bind, err)
		}
	}

	prefix := func(p string, errs []error) (pErrs []error) {
		for _, err := range errs {
			pErrs = append(pErrs, fmt.Errorf("%s: %w", p, err))
		}
		return
	}

	// Validate edition configs
	if c.Editions.Java.Enabled {
		warns2, errs2 := c.Editions.Java.Config.Validate()
		warns = append(warns, prefix("java", warns2)...)
		errs = append(errs, prefix("java", errs2)...)
	}
	//if c.Editions.Bedrock.Enabled {
	//	warns2, errs2 := c.Editions.Bedrock.Config.Validate()
	//	warns = append(warns, prefix("bedrock", warns2)...)
	//	errs = append(errs, prefix("bedrock", errs2)...)
	//}
	return
}
