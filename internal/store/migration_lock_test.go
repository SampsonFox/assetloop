package store

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/SampsonFox/assetloop/internal/config"
)

func TestMigrationLockSurvivesProcessCrash(t *testing.T) {
	if path := os.Getenv("ASSETLOOP_LOCK_TEST_PATH"); path != "" {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if locked, err := tryFileLock(file); !locked || err != nil {
			t.Fatalf("child lock: %v %v", locked, err)
		}
		os.Stdout.WriteString("locked\n")
		os.Stdin.Read(make([]byte, 1))
		return
	}
	path := filepath.Join(t.TempDir(), "lock.db")
	db, err := Open(config.Database{Driver: "sqlite", DSN: path})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	child := exec.Command(os.Args[0], "-test.run=^TestMigrationLockSurvivesProcessCrash$")
	child.Env = append(os.Environ(), "ASSETLOOP_LOCK_TEST_PATH="+path+".migration.lock")
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if child.ProcessState == nil {
			child.Process.Kill()
			child.Wait()
		}
	})
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("child lock signal: %q %v", line, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if unlock, err := migrationLock(ctx, db, "sqlite"); err == nil {
		unlock()
		t.Fatal("second process bypassed lock")
	}
	child.Process.Kill()
	child.Wait()
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	unlock, err := migrationLock(ctx2, db, "sqlite")
	if err != nil {
		t.Fatalf("crash left stale lock: %v", err)
	}
	unlock()
}
