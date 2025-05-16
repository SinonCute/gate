package db

import (
	"fmt"
)

// Config holds the database configuration
type Config struct {
	// Host is the database host
	Host string `json:"host" yaml:"host"`
	// Port is the database port
	Port int `json:"port" yaml:"port"`
	// User is the database user
	User string `json:"user" yaml:"user"`
	// Password is the database password
	Password string `json:"password" yaml:"password"`
	// Database is the database name
	Database string `json:"database" yaml:"database"`
	// SSLMode is the SSL mode for the connection
	SSLMode string `json:"sslMode" yaml:"sslMode"`
	// MaxOpenConns is the maximum number of open connections to the database
	MaxOpenConns int `json:"maxOpenConns" yaml:"maxOpenConns"`
	// MaxIdleConns is the maximum number of idle connections in the pool
	MaxIdleConns int `json:"maxIdleConns" yaml:"maxIdleConns"`
	// ConnMaxLifetime is the maximum amount of time a connection may be reused
	ConnMaxLifetime int `json:"connMaxLifetime" yaml:"connMaxLifetime"`
}

// DefaultConfig returns the default database configuration
func DefaultConfig() *Config {
	return &Config{
		Host:            "localhost",
		Port:            5432,
		User:            "postgres",
		Password:        "",
		Database:        "gate",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    25,
		ConnMaxLifetime: 300,
	}
}

// DSN returns the database connection string
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}
