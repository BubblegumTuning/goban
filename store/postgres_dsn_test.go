package store

import (
	"strings"
	"testing"

	"goban/config"
)

func TestPostgresDSN_DefaultDisable(t *testing.T) {
	dsn, err := postgresDSN(config.Config{
		DBHost: "localhost", DBPort: 5432, DBUser: "goban", DBPassword: "secret", DBName: "goban",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "sslmode=disable") {
		t.Fatalf("expected sslmode=disable, got %q", dsn)
	}
}

func TestPostgresDSN_Require(t *testing.T) {
	dsn, err := postgresDSN(config.Config{
		DBHost: "db.example", DBPort: 5432, DBUser: "u", DBPassword: "p", DBName: "n", DBSSLMode: "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dsn, "sslmode=require") {
		t.Fatalf("got %q", dsn)
	}
}

func TestPostgresDSN_RejectsUnknownMode(t *testing.T) {
	_, err := postgresDSN(config.Config{DBSSLMode: "not-a-mode"})
	if err == nil {
		t.Fatal("expected error")
	}
}
