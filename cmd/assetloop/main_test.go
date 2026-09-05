package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/migrations"
	"github.com/pressly/goose/v3"
)

func TestNoArgumentsSelectsStartup(t *testing.T) {
	t.Setenv("DB_DRIVER", "invalid-driver")
	err := run(nil)
	if err == nil || !strings.Contains(err.Error(), "DB_DRIVER") {
		t.Fatalf("no arguments must reach startup configuration, got %v", err)
	}
}

func TestDefaultStartupMigratesThenServes(t *testing.T) {
	if os.Getenv("ASSETLOOP_STARTUP_TEST_CHILD") == "1" {
		if err := run(nil); err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, previous := range []bool{false, true} {
		name := "fresh"
		if previous {
			name = "upgrade"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "startup.db")
			cfg := config.Database{Driver: "sqlite", DSN: path}
			if previous {
				db, err := basestore.Open(cfg)
				if err != nil {
					t.Fatal(err)
				}
				migration, err := migrations.FS.ReadFile("sqlite/00001_initial.sql")
				if err != nil {
					t.Fatal(err)
				}
				provider, err := goose.NewProvider(goose.DialectSQLite3, db, fstest.MapFS{"00001_initial.sql": {Data: migration}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := provider.Up(context.Background()); err != nil {
					t.Fatal(err)
				}
				db.Close()
			}
			start := func() {
				t.Helper()
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				addr := listener.Addr().String()
				listener.Close()
				child := exec.Command(os.Args[0], "-test.run=^TestDefaultStartupMigratesThenServes$")
				child.Dir = t.TempDir()
				child.Env = append(os.Environ(), "ASSETLOOP_STARTUP_TEST_CHILD=1", "DB_DRIVER=sqlite", "DB_DSN="+path, "HTTP_ADDR="+addr, "AUTH_MODE=local", "APP_ENV=local")
				log, err := os.Create(filepath.Join(t.TempDir(), "startup.log"))
				if err != nil {
					t.Fatal(err)
				}
				defer log.Close()
				child.Stdout = log
				child.Stderr = log
				if err := child.Start(); err != nil {
					t.Fatal(err)
				}
				defer func() { child.Process.Kill(); child.Wait() }()
				client := &http.Client{Timeout: time.Second}
				ready := false
				for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
					response, err := client.Get("http://" + addr + "/healthz")
					if err == nil {
						body, _ := io.ReadAll(response.Body)
						response.Body.Close()
						if response.StatusCode == 200 && strings.TrimSpace(string(body)) == "ok" {
							ready = true
							break
						}
					}
					time.Sleep(25 * time.Millisecond)
				}
				if !ready {
					content, _ := os.ReadFile(log.Name())
					t.Fatalf("default startup did not serve: %s", content)
				}
				response, err := client.Get("http://" + addr + "/setup")
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 200 {
					t.Fatalf("setup status: %d", response.StatusCode)
				}
			}
			start()
			db, err := basestore.Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := basestore.CheckSchema(context.Background(), db, "sqlite"); err != nil {
				t.Fatal(err)
			}
			db.Close()
			before, err := filepath.Glob(path + ".backup-*")
			if err != nil {
				t.Fatal(err)
			}
			if previous && len(before) != 1 {
				t.Fatalf("upgrade should create one backup: %v", before)
			}
			start()
			after, _ := filepath.Glob(path + ".backup-*")
			if len(before) != len(after) {
				t.Fatal("restarting added an unnecessary backup")
			}
		})
	}
}
