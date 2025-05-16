package db

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

//go:embed schema.sql
var schemaFS embed.FS

// InitDB initializes the database by creating the schema if it doesn't exist
func InitDB(ctx context.Context, db *sqlx.DB) error {
	// Read the schema file
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	// Split the schema into individual statements
	statements := strings.Split(string(schema), ";")

	// Execute each statement
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		_, err := db.ExecContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("failed to execute schema statement: %w", err)
		}
	}

	return nil
}

// NewDB creates a new database connection and initializes the schema
func NewDB(cfg *Config) (*sqlx.DB, error) {
	// Create the database connection
	db, err := sqlx.Connect("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure the connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	// Initialize the database schema
	if err := InitDB(context.Background(), db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database schema: %w", err)
	}

	return db, nil
}
