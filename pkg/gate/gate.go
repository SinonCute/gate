// Package gate is the main package for running one or more Minecraft proxy editions.
package gate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/robinbraemer/event"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"gate/pkg/bridge"
	"gate/pkg/edition"
	bproxy "gate/pkg/edition/bedrock/proxy"
	jconfig "gate/pkg/edition/java/config"
	jproxy "gate/pkg/edition/java/proxy"
	"gate/pkg/gate/config"
	"gate/pkg/internal/reload"
	"gate/pkg/runtime/process"
	errorsutil "gate/pkg/util/errs"
	"gate/pkg/util/interrupt"
)

// Options are Gate options.
type Options struct {
	// Config requires a valid Gate configuration.
	Config *config.Config
	// The event manager to use.
	// If none is set, no events are sent.
	EventMgr event.Manager
	// Dynamic config loader for runtime config updates
	DynamicConfigLoader *DynamicConfigLoader
}

// New returns a new Gate instance.
// The given Options requires a validated Config.
func New(options Options) (gate *Gate, err error) {
	if options.Config == nil {
		return nil, errorsutil.ErrMissingConfig
	}
	if options.DynamicConfigLoader == nil {
		return nil, fmt.Errorf("dynamic config loader is required")
	}

	eventMgr := options.EventMgr
	if eventMgr == nil {
		eventMgr = event.Nop
	}

	gate = &Gate{
		proc:   process.New(process.Options{AllOrNothing: true}),
		bridge: &bridge.Bridge{},
		loader: options.DynamicConfigLoader,
	}

	// Setup Java proxy if enabled
	if options.Config.Editions.Java.Enabled {
		// Get current config from loader for proxy setup
		_ = gate.loader.GetConfig() // Initial load, will be used by proxy logic
		gate.bridge.JavaProxy, err = jproxy.New(jproxy.Options{
			Config:   &options.Config.Editions.Java.Config,
			EventMgr: eventMgr,
		})
		if err != nil {
			return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Java, err)
		}
		if err = gate.proc.Add(process.RunnableFunc(func(ctx context.Context) error {
			ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("java"))
			return gate.bridge.JavaProxy.Start(ctx)
		})); err != nil {
			return nil, err
		}
	}

	// Setup Bedrock proxy if enabled
	if options.Config.Editions.Bedrock.Enabled {
		gate.bridge.BedrockProxy, err = bproxy.New(bproxy.Options{
			Config:   &options.Config.Editions.Bedrock.Config,
			EventMgr: eventMgr,
		})
		if err != nil {
			return nil, fmt.Errorf("error creating new %s proxy: %w", edition.Bedrock, err)
		}
		if err = gate.proc.Add(process.RunnableFunc(func(ctx context.Context) error {
			ctx = logr.NewContext(ctx, logr.FromContextOrDiscard(ctx).WithName("bedrock"))
			return gate.bridge.BedrockProxy.Start(ctx)
		})); err != nil {
			return nil, err
		}
	}

	if options.Config.Editions.Bedrock.Enabled && options.Config.Editions.Java.Enabled {
		// More than one edition was enabled, setup bridge between them
		if err = gate.bridge.Setup(); err != nil {
			return nil, fmt.Errorf("error setting up bridge between proxy editions: %w", err)
		}
	}

	return gate, nil
}

// Gate is the root holder of various child processes.
type Gate struct {
	bridge *bridge.Bridge       // The proxies.
	proc   process.Collection   // Parallel running proc.
	loader *DynamicConfigLoader // Dynamic config loader
}

// Java returns the Java edition proxy, or nil if none.
func (g *Gate) Java() *jproxy.Proxy {
	return g.bridge.JavaProxy
}

// Bedrock returns the Bedrock edition proxy, or nil if none.
func (g *Gate) Bedrock() *bproxy.Proxy {
	return g.bridge.BedrockProxy
}

// Start starts the Gate instance and all underlying proc.
func (g *Gate) Start(ctx context.Context) error { return g.proc.Start(ctx) }

// Viper is the default viper instance used by Start to load in a config.Config.
var Viper = viper.New()

// StartOption is an option for Start.
type StartOption func(o *startOptions)

type startOptions struct {
	conf                      *config.Config
	autoShutdownOnSignal      bool
	autoConfigReloadWatchPath string
	DynamicConfigLoader       *DynamicConfigLoader
}

// WithConfig is a StartOption for Start
// that uses the provided config.Config.
func WithConfig(c config.Config) StartOption {
	return func(o *startOptions) {
		o.conf = &c
	}
}

// WithAutoShutdownOnSignal is a StartOption for Start
// that automatically shuts down the Gate instance
// when a shutdown signal is received.
//
// This setting is enabled by default.
func WithAutoShutdownOnSignal(enabled bool) StartOption {
	return func(o *startOptions) {
		o.autoShutdownOnSignal = enabled
	}
}

// LoadConfigFunc is a function that loads in a config.Config.
type LoadConfigFunc func() (*config.Config, error)

// WithAutoConfigReload is a StartOption for Start
// that automatically reloads the config when a file change is detected.
//
// This setting is disabled by default.
func WithAutoConfigReload(path string) StartOption {
	return func(o *startOptions) {
		o.autoConfigReloadWatchPath = path
	}
}

// WithDynamicConfigLoader is a StartOption for Start that sets the dynamic config loader.
func WithDynamicConfigLoader(loader *DynamicConfigLoader) StartOption {
	return func(o *startOptions) {
		o.DynamicConfigLoader = loader
	}
}

// Start is a convenience function to set up and run a Gate instance.
//
// It uses the logr.Logger from the provided context, reads in a Config,
// validates it and sets up os signal handling before starting the instance.
//
// The Gate is shutdown when the context is canceled or on occurrence of any
// significant error like severe configuration error or unable to bind to a port.
//
// Config validation warnings are logged but ignored.
func Start(ctx context.Context, opts ...StartOption) error {
	c := &startOptions{
		autoShutdownOnSignal: true,
	}
	for _, opt := range opts {
		opt(c)
	}

	log := logr.FromContextOrDiscard(ctx)

	// Require dynamic config loader
	if c.DynamicConfigLoader == nil {
		return fmt.Errorf("dynamic config loader is required")
	}

	// Get initial config from loader
	_ = c.DynamicConfigLoader.GetConfig() // Initial load, will be used by proxy logic

	// Setup new Gate instance with loaded config.
	eventMgr := event.New(event.WithLogger(log.WithName("event")))
	gate, err := New(Options{
		Config:   c.conf, // This will be used only for static config (DB/Redis)
		EventMgr: eventMgr,
	})
	if err != nil {
		return fmt.Errorf("error creating Gate instance: %w", err)
	}

	// Setup os signal channel to trigger Gate shutdown.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if c.autoShutdownOnSignal {
		go func() {
			defer cancel()
			select {
			case <-ctx.Done():
			case s := <-interrupt.Notify(ctx):
				log.Info("Received os signal", "signal", s)
			}
		}()
	}

	// Start everything
	return gate.Start(ctx)
}

// setupAutoConfigReload sets up auto config reload if enabled.
func setupAutoConfigReload(
	ctx context.Context,
	log logr.Logger,
	mgr event.Manager,
	path string,
	initialCfg *config.Config,
) error {
	if path == "" {
		return nil // No auto config reload
	}
	log.Info("auto config reload enabled", "path", path)
	prevCfg := initialCfg
	// Watch config file for changes
	return reload.Watch(ctx, path, func() error {
		cfg, err := LoadConfig(Viper)
		if err != nil {
			return err
		}
		if err = validateConfig(log, cfg); err != nil {
			return err
		}
		reload.FireConfigUpdate(mgr, cfg, prevCfg)
		prevCfg = cfg
		return nil
	})
}

// validateConfig validates the provided config.Config
// and logs any validation errors or warnings.
// If there are any hard errors, it returns an error.
func validateConfig(log logr.Logger, c *config.Config) error {
	// Validate Gate config
	warns, errs := c.Validate()
	for _, e := range errs {
		log.Info("config validation error", "error", e)
	}
	for _, w := range warns {
		log.Info("config validation warn", "warn", w)
	}
	if len(errs) != 0 {
		// Shouldn't run Gate with validation errors
		return fmt.Errorf("config validation errors "+
			"(errors: %d, warns: %d), inspect the logs for details",
			len(errs), len(warns))
	}
	return nil
}

// LoadConfig loads in config.Config from viper.
// It is used by Start with the packages Viper if no WithConfig option is given.
func LoadConfig(v *viper.Viper) (*config.Config, error) {
	// Clone default config
	cfg := func() config.Config { return config.DefaultConfig }()
	// Load in Gate config
	if err := fixedReadInConfig(v, &cfg); err != nil {
		return &cfg, fmt.Errorf("error loading config: %w", err)
	}
	// Override Java config by shorter alias
	if !reflect.DeepEqual(cfg.Config, jconfig.DefaultConfig) {
		cfg.Editions.Java.Config = cfg.Config
	}
	return &cfg, nil
}

// Workaround for https://github.com/minekube/gate/issues/218#issuecomment-1632800775
func fixedReadInConfig(v *viper.Viper, defaultConfig *config.Config) error {
	if defaultConfig == nil {
		return v.ReadInConfig()
	}

	configFile := v.ConfigFileUsed()
	if configFile == "" {
		// Try to find config file using Viper's config finder logic
		if err := v.ReadInConfig(); err != nil {
			return err
		}
		configFile = v.ConfigFileUsed()
		if configFile == "" {
			return nil // no config file found
		}
	}

	var (
		unmarshal func([]byte, any) error
		marshal   func(any) ([]byte, error)
	)
	switch path.Ext(configFile) {
	case ".yaml", ".yml":
		unmarshal = yaml.Unmarshal
		marshal = yaml.Marshal
	case ".json":
		unmarshal = json.Unmarshal
		marshal = json.Marshal
	default:
		return fmt.Errorf("unsupported config file format %q", configFile)
	}
	b, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("error reading config file %q: %w", configFile, err)
	}
	if err = unmarshal(b, defaultConfig); err != nil {
		return fmt.Errorf("error unmarshaling config file %q to %T: %w", configFile, defaultConfig, err)
	}
	if b, err = marshal(defaultConfig); err != nil {
		return fmt.Errorf("error marshaling config file %q: %w", configFile, err)
	}

	return v.ReadConfig(bytes.NewReader(b))
}
