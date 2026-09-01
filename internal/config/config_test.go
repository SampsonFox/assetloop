package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("HTTP_ADDR=localhost:9000\nDB_DRIVER=postgres\nDB_DSN=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DB_DSN", "from-environment")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "localhost:9000" || cfg.Database.Driver != "postgres" || cfg.Database.DSN != "from-environment" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}
