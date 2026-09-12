package config

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Database struct {
	Driver string
	DSN    string
}

type Blob struct {
	DefaultStore string
	LocalRoot    string
	OSS          OSS
}

type OSS struct{ Endpoint, Region, Bucket, AccessKeyID, AccessKeySecret, PathPrefix string }

type Market struct {
	Token string
}

type Config struct {
	Market      Market
	Environment string
	HTTPAddr    string
	LogLevel    string
	AuthMode    string
	Database    Database
	Blob        Blob
	MCP         MCP
}

type MCP struct {
	Enabled                       bool
	Issuer, ClientID, RedirectURI string
}

func Load(dotenvPath string) (Config, error) {
	values := map[string]string{
		"ZHUANZHUAN_MCP_TOKEN":     "",
		"APP_ENV":                  "local",
		"HTTP_ADDR":                "127.0.0.1:8080",
		"LOG_LEVEL":                "info",
		"AUTH_MODE":                "local",
		"MCP_ENABLED":              "false",
		"MCP_ISSUER":               "",
		"MCP_CLIENT_ID":            "codex-local",
		"MCP_REDIRECT_URI":         "http://127.0.0.1/callback",
		"DB_DRIVER":                "sqlite",
		"DB_DSN":                   "./data/assetloop.db",
		"ATTACHMENT_DEFAULT_STORE": "local",
		"ATTACHMENT_LOCAL_ROOT":    "./data/blobs",
		"ALIYUN_OSS_ENDPOINT":      "", "ALIYUN_OSS_REGION": "", "ALIYUN_OSS_BUCKET": "",
		"ALIYUN_OSS_ACCESS_KEY_ID": "", "ALIYUN_OSS_ACCESS_KEY_SECRET": "", "ALIYUN_OSS_PATH_PREFIX": "",
	}
	if err := loadDotenv(dotenvPath, values); err != nil {
		return Config{}, err
	}
	// A separate ignored provider file is useful for existing local installs.
	if err := loadDotenv(filepath.Join(filepath.Dir(dotenvPath), ".env.zhuanzhuan.local"), values); err != nil {
		return Config{}, err
	}
	for key := range values {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}

	driver := strings.ToLower(strings.TrimSpace(values["DB_DRIVER"]))
	if driver != "sqlite" && driver != "postgres" {
		return Config{}, fmt.Errorf("DB_DRIVER must be sqlite or postgres, got %q", driver)
	}
	if strings.TrimSpace(values["DB_DSN"]) == "" {
		return Config{}, errors.New("DB_DSN must not be empty")
	}
	if strings.TrimSpace(values["HTTP_ADDR"]) == "" {
		return Config{}, errors.New("HTTP_ADDR must not be empty")
	}
	authMode := strings.ToLower(strings.TrimSpace(values["AUTH_MODE"]))
	if authMode != "local" && authMode != "disabled" {
		return Config{}, fmt.Errorf("AUTH_MODE must be local or disabled, got %q", authMode)
	}
	if authMode == "disabled" && !isLoopbackAddress(values["HTTP_ADDR"]) {
		return Config{}, errors.New("AUTH_MODE=disabled requires a loopback HTTP_ADDR")
	}
	mcp, err := loadMCP(values, authMode)
	if err != nil {
		return Config{}, err
	}
	defaultStore := strings.ToLower(strings.TrimSpace(values["ATTACHMENT_DEFAULT_STORE"]))
	if defaultStore != "local" && defaultStore != "aliyun" {
		return Config{}, errors.New("ATTACHMENT_DEFAULT_STORE must be local or aliyun")
	}
	if strings.TrimSpace(values["ATTACHMENT_LOCAL_ROOT"]) == "" {
		return Config{}, errors.New("ATTACHMENT_LOCAL_ROOT must not be empty")
	}
	if defaultStore == "aliyun" {
		for _, key := range []string{"ALIYUN_OSS_REGION", "ALIYUN_OSS_BUCKET", "ALIYUN_OSS_ACCESS_KEY_ID", "ALIYUN_OSS_ACCESS_KEY_SECRET"} {
			if strings.TrimSpace(values[key]) == "" {
				return Config{}, fmt.Errorf("%s is required for Aliyun OSS", key)
			}
		}
	}

	return Config{
		Market:      Market{Token: strings.TrimSpace(values["ZHUANZHUAN_MCP_TOKEN"])},
		Environment: values["APP_ENV"],
		HTTPAddr:    values["HTTP_ADDR"],
		LogLevel:    values["LOG_LEVEL"],
		AuthMode:    authMode,
		MCP:         mcp,
		Database: Database{
			Driver: driver,
			DSN:    values["DB_DSN"],
		},
		Blob: Blob{DefaultStore: defaultStore, LocalRoot: values["ATTACHMENT_LOCAL_ROOT"], OSS: OSS{Endpoint: values["ALIYUN_OSS_ENDPOINT"], Region: values["ALIYUN_OSS_REGION"], Bucket: values["ALIYUN_OSS_BUCKET"], AccessKeyID: values["ALIYUN_OSS_ACCESS_KEY_ID"], AccessKeySecret: values["ALIYUN_OSS_ACCESS_KEY_SECRET"], PathPrefix: values["ALIYUN_OSS_PATH_PREFIX"]}},
	}, nil
}

func loadMCP(values map[string]string, authMode string) (MCP, error) {
	enabled, err := strconv.ParseBool(strings.TrimSpace(values["MCP_ENABLED"]))
	if err != nil {
		return MCP{}, errors.New("MCP_ENABLED must be true or false")
	}
	cfg := MCP{Enabled: enabled, Issuer: strings.TrimSpace(values["MCP_ISSUER"]), ClientID: strings.TrimSpace(values["MCP_CLIENT_ID"]), RedirectURI: strings.TrimSpace(values["MCP_REDIRECT_URI"])}
	if !enabled {
		return cfg, nil
	}
	if authMode != "local" {
		return MCP{}, errors.New("MCP requires AUTH_MODE=local")
	}
	issuer, err := url.Parse(cfg.Issuer)
	if err != nil || !validMCPURL(issuer) || issuer.Path != "" || issuer.RawQuery != "" || issuer.ForceQuery || strings.Contains(cfg.Issuer, "#") {
		return MCP{}, errors.New("MCP_ISSUER must be an HTTPS origin or an HTTP loopback IP origin")
	}
	if cfg.ClientID == "" || len(cfg.ClientID) > 128 || strings.ContainsAny(cfg.ClientID, " \t\r\n") {
		return MCP{}, errors.New("MCP_CLIENT_ID must be a nonempty identifier")
	}
	redirect, err := url.Parse(cfg.RedirectURI)
	if err != nil || !validMCPURL(redirect) || strings.Contains(cfg.RedirectURI, "#") {
		return MCP{}, errors.New("MCP_REDIRECT_URI must be an HTTPS or HTTP loopback IP callback")
	}
	return cfg, nil
}

func validMCPURL(u *url.URL) bool {
	if u == nil || u.Host == "" || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return false
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return false
		}
	}
	ip := net.ParseIP(u.Hostname())
	return u.Scheme == "https" || u.Scheme == "http" && ip != nil && ip.IsLoopback()
}

func isLoopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func loadDotenv(path string, values map[string]string) error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open dotenv: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("dotenv line %d must be KEY=VALUE", lineNo)
		}
		if _, known := values[key]; known {
			values[key] = strings.Trim(strings.TrimSpace(value), "\"'")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read dotenv: %w", err)
	}
	return nil
}
