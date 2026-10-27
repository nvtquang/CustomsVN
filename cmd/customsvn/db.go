package main

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"os/signal"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/config"
	"github.com/nvtquang/CustomsVN/internal/db"
	"github.com/nvtquang/CustomsVN/internal/ingest"
)

// openDB loads the config, migrates the schema and opens the pool. Every
// command that touches the database starts here, so none of them runs against
// an old schema.
func openDB(ctx context.Context, configPath string) (*config.Config, *pgxpool.Pool, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, nil, err
	}
	url, err := cfg.RequireDatabase()
	if err != nil {
		return nil, nil, err
	}
	if err := db.Migrate(ctx, url); err != nil {
		return nil, nil, err
	}
	pool, err := db.Open(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	return cfg, pool, nil
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}

func runMigrate(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file (default ./customsvn.yaml)")
	if ok, err := parse(fs, args); !ok {
		return err
	}
	ctx, cancel := signalContext()
	defer cancel()
	cfg, pool, err := openDB(ctx, *configPath)
	if err != nil {
		return err
	}
	pool.Close()
	v, err := db.Version(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	return printJSON(map[string]any{"schema_version": v})
}

func runIngest(args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file (default ./customsvn.yaml)")
	if ok, err := parse(fs, args); !ok {
		return err
	}
	ctx, cancel := signalContext()
	defer cancel()
	cfg, pool, err := openDB(ctx, *configPath)
	if err != nil {
		return err
	}
	defer pool.Close()
	res, err := ingest.Run(ctx, pool, ingest.Options{Prep: cfg.Prep, Data: cfg.Data, Log: os.Stderr})
	if err != nil {
		return err
	}
	return printJSON(res)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
