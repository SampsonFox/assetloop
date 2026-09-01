package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Database struct {
	Driver string
	DSN    string
}

type Config struct {
	Environment string
	HTTPAddr    string
	LogLevel    string
	Database    Database
}

func Load(dotenvPath string) (Config, error) {
	values := map[string]string{
		"APP_ENV":   "local",
		"HTTP_ADDR": "127.0.0.1:8080",
		"LOG_LEVEL": "info",
		"DB_DRIVER": "sqlite",
		"DB_DSN":    "./data/assetloop.db",
	}
	if err := loadDotenv(dotenvPath, values); err != nil {
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

	return Config{
		Environment: values["APP_ENV"],
		HTTPAddr:    values["HTTP_ADDR"],
		LogLevel:    values["LOG_LEVEL"],
		Database: Database{
			Driver: driver,
			DSN:    values["DB_DSN"],
		},
	}, nil
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
