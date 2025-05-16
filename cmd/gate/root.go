package gate

import (
	"fmt"
	"os"
	"strings"

	"gate/pkg/gate"
	"gate/pkg/gate/config"
	"gate/pkg/gate/db"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Execute runs App() and calls os.Exit when finished.
func Execute() {
	if err := App().Run(os.Args); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func App() *cli.App {
	app := cli.NewApp()
	app.Name = "gate"
	app.Usage = "Gate is an extensible Minecraft proxy."
	app.Description = `A high performant & paralleled Minecraft proxy server with
	scalability, flexibility & excelled server version support.

Visit the website https://gate.minekube.com/ for more information.`

	var (
		debug      bool
		configFile string
		verbosity  int
	)
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "config",
			Aliases:     []string{"c"},
			Usage:       `config file (default: ./config.yml) Supports: yaml, json, env`,
			EnvVars:     []string{"GATE_CONFIG"},
			Destination: &configFile,
		},
		&cli.BoolFlag{
			Name:        "debug",
			Aliases:     []string{"d"},
			Usage:       "Enable debug mode and highest log verbosity",
			Destination: &debug,
			EnvVars:     []string{"GATE_DEBUG"},
		},
		&cli.IntFlag{
			Name:        "verbosity",
			Aliases:     []string{"v"},
			Usage:       "The higher the verbosity the more logs are shown",
			EnvVars:     []string{"GATE_VERBOSITY"},
			Destination: &verbosity,
		},
	}
	app.Action = func(c *cli.Context) error {
		// Init viper
		v, err := initViper(c, configFile)
		if err != nil {
			return cli.Exit(err, 1)
		}
		// Load config (only DB/Redis/gate_id)
		var cfg config.Config = config.DefaultConfig
		if err := v.Unmarshal(&cfg); err != nil {
			return cli.Exit(fmt.Errorf("error reading config file: %w", err), 2)
		}

		// Create logger
		log, err := newLogger(debug, verbosity)
		if err != nil {
			return cli.Exit(fmt.Errorf("error creating zap logger: %w", err), 1)
		}
		c.Context = logr.NewContext(c.Context, log)

		log.Info("logging verbosity", "verbosity", verbosity)
		log.Info("using config file", "config", v.ConfigFileUsed())

		// Initialize database store
		store, err := db.NewStore(&cfg.Database)
		if err != nil {
			return cli.Exit(fmt.Errorf("error initializing database store: %w", err), 1)
		}

		// Create default gate instance if it doesn't exist
		gateID := os.Getenv("GATE_ID")
		if gateID == "" {
			gateID = "default"
		}

		existingGate, err := store.GetGate(c.Context, gateID)
		if err != nil {
			return cli.Exit(fmt.Errorf("error checking existing gate: %w", err), 1)
		}

		if existingGate == nil {
			// Create new gate instance
			newGate := db.NewGate()
			newGate.GateID = cfg.Gate.GateID
			newGate.Role = db.GateRole(cfg.Gate.Role)
			newGate.Region = cfg.Gate.Region
			newGate.GroupName = cfg.Gate.GroupName
			newGate.PublicAddress = cfg.Gate.PublicAddress
			newGate.InternalAddress = cfg.Gate.InternalAddress

			if err := store.CreateGate(c.Context, newGate); err != nil {
				return cli.Exit(fmt.Errorf("error creating gate instance: %w", err), 1)
			}
			log.Info("created new gate instance", "gate_id", newGate.GateID)
		}

		// Initialize dynamic config loader
		loader, err := gate.NewDynamicConfigLoader(c.Context, store)
		if err != nil {
			return cli.Exit(fmt.Errorf("error initializing dynamic config loader: %w", err), 1)
		}

		// Start Redis pubsub listener for config updates
		redisOpts := &redis.Options{
			Addr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		}
		go loader.ListenAndRefreshOnRedis(c.Context, redisOpts, cfg.Redis.Channel)

		// Start Gate (pass loader as needed)
		if err = gate.Start(c.Context,
			gate.WithDynamicConfigLoader(loader),
		); err != nil {
			return cli.Exit(fmt.Errorf("error running Gate: %w", err), 1)
		}
		return nil
	}
	return app
}

func initViper(c *cli.Context, configFile string) (*viper.Viper, error) {
	v := gate.Viper
	if c.IsSet("config") {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
	}
	// Load Environment Variables
	v.SetEnvPrefix("GATE")
	v.AutomaticEnv() // read in environment variables that match
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	return v, nil
}

// newLogger returns a new zap logger with a modified production
// or development default config to ensure human readability.
func newLogger(debug bool, v int) (l logr.Logger, err error) {
	var cfg zap.Config
	if debug {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.Level = zap.NewAtomicLevelAt(zapcore.Level(-v))

	cfg.Encoding = "console"
	cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	zl, err := cfg.Build()
	if err != nil {
		return logr.Discard(), err
	}
	return zapr.NewLogger(zl), nil
}
