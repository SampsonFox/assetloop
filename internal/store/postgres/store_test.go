package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/SampsonFox/assetloop/internal/config"
	basestore "github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/postgres"
	"github.com/SampsonFox/assetloop/internal/store/storetest"
)

func TestStoreConformance(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	cfg := config.Database{Driver: "postgres", DSN: dsn}
	db, err := basestore.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := basestore.Migrate(context.Background(), db, cfg); err != nil {
		t.Fatal(err)
	}
	storetest.Run(t, postgres.New(db))
	storetest.AssertAssetEventsAppendOnly(t, db, "postgres")
	storetest.AssertBaseCurrencyLocked(t, db, "postgres")
}
