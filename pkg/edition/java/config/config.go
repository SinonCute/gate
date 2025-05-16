package config

import (
	"fmt"
	"strings"
	"time"

	"gate/pkg/util/componentutil"
	"gate/pkg/util/configutil"
	"gate/pkg/util/favicon"
	"gate/pkg/util/validation"
	"go.minekube.com/gate/pkg/edition/java/proto/version"
)

// DefaultConfig is a default Config.
var DefaultConfig = Config{
	Bind:              "0.0.0.0:25565",
	ConnectionTimeout: configutil.Duration(5000 * time.Millisecond),
	ReadTimeout:       configutil.Duration(30000 * time.Millisecond),
	Quota: Quota{
		Connections: QuotaSettings{
			Enabled:    true,
			OPS:        5,
			Burst:      10,
			MaxEntries: 1000,
		},
		Logins: QuotaSettings{
			Enabled:    true,
			OPS:        0.4,
			Burst:      3,
			MaxEntries: 1000,
		},
	},
	ProxyProtocol:  false,
	Debug:          false,
	ShutdownReason: defaultShutdownReason(),
}

func defaultMotd() *configutil.TextComponent {
	return text("§bA Gate Proxy\n§bVisit ➞ §fgithub.com/minekube/gate")
}
func defaultShutdownReason() *configutil.TextComponent {
	return text("§cGate proxy is shutting down...\nPlease reconnect in a moment!")
}

// Config is the configuration of the proxy.
type Config struct { // TODO use https://github.com/projectdiscovery/yamldoc-go for generating output yaml and markdown for the docs
	Bind string `yaml:"bind"` // The address to listen for connections.

	Forwarding Forwarding `yaml:"forwarding,omitempty" json:"forwarding,omitempty"` // Player info forwarding settings.

	ConnectionTimeout configutil.Duration `yaml:"connectionTimeout,omitempty" json:"connectionTimeout,omitempty"` // Write timeout
	ReadTimeout       configutil.Duration `yaml:"readTimeout,omitempty" json:"readTimeout,omitempty"`             // Read timeout

	Quota         Quota       `yaml:"quota,omitempty" json:"quota,omitempty"` // Rate limiting settings
	Compression   Compression `yaml:"compression,omitempty" json:"compression,omitempty"`
	ProxyProtocol bool        `yaml:"proxyProtocol,omitempty" json:"proxyProtocol,omitempty"` // Enable HA-Proxy protocol mode

	AcceptTransfers bool `yaml:"acceptTransfers,omitempty" json:"acceptTransfers,omitempty"` // Whether to accept transfers from other hosts via transfer packet

	Debug          bool                      `yaml:"debug,omitempty" json:"debug,omitempty"` // Enable debug mode
	ShutdownReason *configutil.TextComponent `yaml:"shutdownReason,omitempty" json:"shutdownReason,omitempty"`
}

type (
	ForcedHosts map[string][]string // virtualhost:server names
	Status      struct {
		ShowMaxPlayers  int                       `yaml:"showMaxPlayers"`
		Motd            *configutil.TextComponent `yaml:"motd"`
		Favicon         favicon.Favicon           `yaml:"favicon"`
		LogPingRequests bool                      `yaml:"logPingRequests"`
	}
	Query struct {
		Enabled     bool `yaml:"enabled"`
		Port        int  `yaml:"port"`
		ShowPlugins bool `yaml:"showPlugins"`
	}
	Forwarding struct {
		Mode              ForwardingMode `yaml:"mode"`
		VelocitySecret    string         `yaml:"velocitySecret"`    // Used with "velocity" mode
		BungeeGuardSecret string         `yaml:"bungeeGuardSecret"` // Used with "bungeeguard" mode
	}
	Compression struct {
		Threshold int `yaml:"threshold"`
		Level     int `yaml:"level"`
	}
	// Quota is the config for rate limiting.
	Quota struct {
		Connections QuotaSettings `yaml:"connections"` // Limits new connections per second, per IP block.
		Logins      QuotaSettings `yaml:"logins"`      // Limits logins per second, per IP block.
		// Maybe add a bytes-per-sec limiter, or should be managed by a higher layer.
	}
	QuotaSettings struct {
		Enabled    bool    `yaml:"enabled"`    // If false, there is no such limiting.
		OPS        float32 `yaml:"ops"`        // Allowed operations/events per second, per IP block
		Burst      int     `yaml:"burst"`      // The maximum events per second, per block; the size of the token bucket
		MaxEntries int     `yaml:"maxEntries"` // Maximum number of IP blocks to keep track of in cache
	}
	// Auth is the config for authentication.
	Auth struct {
		// SessionServerURL is the base URL for the Mojang session server to authenticate online mode players.
		// Defaults to https://sessionserver.mojang.com/session/minecraft/hasJoined
		SessionServerURL *configutil.URL `yaml:"sessionServerUrl"` // TODO support multiple urls configutil.SingleOrMulti[URL]
	}
)

// ForwardingMode is a player info forwarding mode.
type ForwardingMode string

const (
	NoneForwardingMode   ForwardingMode = "none"
	LegacyForwardingMode ForwardingMode = "legacy"
	// VelocityForwardingMode is a forwarding mode specified by the Velocity java proxy and
	// supported by PaperSpigot for versions starting at 1.13.
	VelocityForwardingMode ForwardingMode = "velocity"
	// BungeeGuardForwardingMode is a forwarding mode used by versions lower than 1.13
	BungeeGuardForwardingMode ForwardingMode = "bungeeguard"
)

// Validate validates Config.
func (c *Config) Validate() (warns []error, errs []error) {
	e := func(m string, args ...any) { errs = append(errs, fmt.Errorf(m, args...)) }

	if c == nil {
		e("config must not be nil")
		return
	}

	if strings.TrimSpace(c.Bind) == "" {
		e("Bind is empty")
	} else {
		if err := validation.ValidHostPort(c.Bind); err != nil {
			e("Invalid bind %q: %v", c.Bind, err)
		}
	}

	for _, quota := range []QuotaSettings{c.Quota.Connections, c.Quota.Logins} {
		if quota.Enabled {
			if quota.OPS <= 0 {
				e("Invalid quota ops %d, use a number > 0", quota.OPS)
			}
			if quota.Burst < 1 {
				e("Invalid quota burst %d, use a number >= 1", quota.Burst)
			}
			if quota.MaxEntries < 1 {
				e("Invalid quota max entries %d, use a number >= 1", quota.Burst)
			}
		}
	}

	return
}

func text(s string) *configutil.TextComponent {
	return (*configutil.TextComponent)(must(componentutil.ParseTextComponent(
		version.MinimumVersion.Protocol, s)))
}

func must[T any](t T, err error) T {
	if err != nil {
		panic(err)
	}
	return t
}
