// Package store provides database abstraction for Goban persistence.
package store

import (
	"fmt"
	"log"

	"goban/config"
)

// New creates a new TicketStore based on configuration.
// Returns SQLite store by default, or PostgreSQL if DBType is "postgres".
func New(cfg config.Config) (TicketStore, error) {
	if config.Debug {
		log.Printf("[STORE.DEBUG] cfg.DBType = '%s' (len=%d)", cfg.DBType, len(cfg.DBType))
	}

	if cfg.DBType == "postgres" || cfg.DBType == "postgresql" {
		log.Printf("Creating PostgreSQL store for %s:%d/%s",
			cfg.DBHost, cfg.DBPort, cfg.DBName)

		store := &PostgresStore{config: cfg}
		if err := store.Init(); err != nil {
			return nil, fmt.Errorf("failed to initialize PostgreSQL store: %w", err)
		}
		return store, nil
	}

	// Default to SQLite
	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath = "./goban.db"
	}
	log.Printf("Creating SQLite store for %s", dbPath)

	store := &SQLiteStore{config: cfg}
	if err := store.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize SQLite store: %w", err)
	}
	return store, nil
}
