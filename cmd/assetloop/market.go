package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/SampsonFox/assetloop/internal/application"
	"github.com/SampsonFox/assetloop/internal/config"
	"github.com/SampsonFox/assetloop/internal/market/frankfurter"
	"github.com/SampsonFox/assetloop/internal/market/zhuanzhuan"
	"github.com/SampsonFox/assetloop/internal/store"
	"github.com/SampsonFox/assetloop/internal/store/postgres"
	"github.com/SampsonFox/assetloop/internal/store/sqlite"
	"os"
	"path/filepath"
	"time"
)

func newMarketService(st application.MarketStore, cfg config.Config) *application.MarketService {
	var provider application.MarketDataProvider
	if cfg.Market.Token != "" {
		provider = zhuanzhuan.New(cfg.Market.Token)
	}
	return application.NewMarketService(st, provider, frankfurter.New(), application.MarketOptions{UnitsConfirmed: cfg.Market.UnitsConfirmed, MinInterval: 2 * time.Second})
}
func refreshMarketCommand(args []string) error {
	flags := flag.NewFlagSet("refresh-market", flag.ContinueOnError)
	tenant := flags.String("tenant", "", "optional tenant ID")
	item := flags.String("item", "", "optional market item ID")
	dotenv := flags.String("config", ".env", "configuration file")
	if e := flags.Parse(args); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	absolute, e := filepath.Abs(*dotenv)
	if e != nil {
		return e
	}
	if e = os.Chdir(filepath.Dir(absolute)); e != nil {
		return e
	}
	cfg, e := config.Load(absolute)
	if e != nil {
		return e
	}
	db, e := store.Open(cfg.Database)
	if e != nil {
		return e
	}
	defer db.Close()
	ctx := context.Background()
	// Scheduled commands never perform schema upgrades while a Web process is active.
	if e = store.CheckSchema(ctx, db, cfg.Database.Driver); e != nil {
		return e
	}
	var st application.MarketStore
	if cfg.Database.Driver == "sqlite" {
		st = sqlite.New(db)
	} else {
		st = postgres.New(db)
	}
	n, e := newMarketService(st, cfg).RefreshDue(ctx, *tenant, *item)
	fmt.Fprintf(os.Stdout, "market refresh: %d updated\n", n)
	if e != nil {
		return fmt.Errorf("%s", application.MarketErrorCode(e))
	}
	return nil
}
