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

func TestDisabledAuthRequiresLoopback(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("AUTH_MODE=disabled\nHTTP_ADDR=0.0.0.0:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected disabled authentication on a public listener to fail")
	}
	if err := os.WriteFile(path, []byte("AUTH_MODE=disabled\nHTTP_ADDR=127.0.0.1:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil || cfg.AuthMode != "disabled" {
		t.Fatalf("loopback disabled auth should be accepted: cfg=%+v err=%v", cfg, err)
	}
}

func TestMarketCredentialsLocalFileAndEnvironment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.zhuanzhuan.local"), []byte("ZHUANZHUAN_MCP_TOKEN=fixture-local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZHUANZHUAN_PRICE_UNIT_CONFIRMED", "false")
	cfg, err := Load(filepath.Join(dir, ".env"))
	if err != nil || cfg.Market.Token != "fixture-local" || cfg.Market.UnitsConfirmed {
		t.Fatal("local configuration or default unit gate failed", err)
	}
	t.Setenv("ZHUANZHUAN_MCP_TOKEN", "fixture-environment")
	t.Setenv("ZHUANZHUAN_PRICE_UNIT_CONFIRMED", "true")
	cfg, err = Load(filepath.Join(dir, ".env"))
	if err != nil || cfg.Market.Token != "fixture-environment" || !cfg.Market.UnitsConfirmed {
		t.Fatal("environment precedence failed", err)
	}
}
