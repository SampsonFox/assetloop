package config

import (
	"path/filepath"
	"testing"
)

func TestMCPConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]string
		valid  bool
	}{
		{"default off", map[string]string{}, true},
		{"explicit local", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "http://127.0.0.1:8081"}, true},
		{"https", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "https://app.example.invalid"}, true},
		{"missing issuer", map[string]string{"MCP_ENABLED": "true"}, false},
		{"disabled account", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "http://127.0.0.1:8081", "AUTH_MODE": "disabled"}, false},
		{"bad boolean", map[string]string{"MCP_ENABLED": "maybe"}, false},
		{"public plaintext", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "http://example.invalid"}, false},
		{"issuer path", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "https://example.invalid/path"}, false},
		{"issuer fragment", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "https://example.invalid#"}, false},
		{"issuer query", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "https://example.invalid?"}, false},
		{"bad port", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "http://127.0.0.1:99999"}, false},
		{"no client", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "http://127.0.0.1:8081", "MCP_CLIENT_ID": ""}, false},
		{"callback credentials", map[string]string{"MCP_ENABLED": "true", "MCP_ISSUER": "http://127.0.0.1:8081", "MCP_REDIRECT_URI": "http://user@127.0.0.1/callback"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range map[string]string{"MCP_ENABLED": "false", "MCP_ISSUER": "", "MCP_CLIENT_ID": "codex-local", "MCP_REDIRECT_URI": "http://127.0.0.1/callback", "AUTH_MODE": "local", "HTTP_ADDR": "127.0.0.1:8081"} {
				t.Setenv(k, v)
			}
			for k, v := range tc.values {
				t.Setenv(k, v)
			}
			cfg, err := Load(filepath.Join(t.TempDir(), "missing.env"))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
			if tc.name == "default off" && cfg.MCP.Enabled {
				t.Fatal("MCP enabled by default")
			}
		})
	}
}
