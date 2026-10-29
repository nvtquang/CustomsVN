package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/config"
	"github.com/nvtquang/CustomsVN/internal/crawl"
	"github.com/nvtquang/CustomsVN/internal/httpapi"
	"github.com/nvtquang/CustomsVN/internal/jobs"
)

// Where the dataset's crawler and its interpreter are. The image installs
// them (see the Dockerfile); elsewhere set the variables.
const (
	CrawlerEnv = "CUSTOMSVN_CRAWLER"
	PythonEnv  = "CUSTOMSVN_PYTHON"
	LuatdoEnv  = "CUSTOMSVN_LUATDO"
	// ActionEnv overrides the vbpl.vn catalogue action when the portal moves it.
	ActionEnv = "CUSTOMSVN_VBPL_ACTION"
)

func crawlerDir() string { return orDefault(os.Getenv(CrawlerEnv), "/opt/vbpl-crawler") }

// crawlJob is the weekly update: the crawl, then luatdo's parse, cite and
// subjects over the new revision, then prep, ingest and the embeddings of the
// new chunks, each step's output in the job's log. A crawl that finds nothing
// in scope stops after the crawl.
func crawlJob(cfg *config.Config, pool *pgxpool.Pool, kinds map[string]jobs.Func) jobs.Func {
	return func(ctx context.Context, raw json.RawMessage, log io.Writer) error {
		var p struct {
			MaxPages int `json:"max_pages"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		sched := httpapi.LoadSchedule(ctx, pool)
		if p.MaxPages == 0 {
			p.MaxPages = sched.MaxPages
		}
		// Monthly runs fetch every in-scope document again; weekly ones a
		// quarter each, so statuses are never more than a month old.
		res, err := crawl.Run(ctx, crawl.Options{Data: cfg.Data, Crawler: crawlerDir(), Python: os.Getenv(PythonEnv),
			MaxPages: p.MaxPages, RefreshParts: sched.RecheckParts(), CatalogAction: os.Getenv(ActionEnv), Log: log})
		if err != nil {
			return err
		}
		out, _ := json.Marshal(res)
		fmt.Fprintf(log, "crawl result: %s\n", out)
		if res.Revision == "" {
			return nil
		}
		luatdo := orDefault(os.Getenv(LuatdoEnv), "luatdo")
		for _, step := range [][]string{
			{"parse", "--data", cfg.Data, "--dataset", "th1nhng0"},
			{"cite", "--data", cfg.Data},
			{"subjects", "--data", cfg.Data},
		} {
			fmt.Fprintf(log, "luatdo %s\n", step[0])
			cmd := exec.CommandContext(ctx, luatdo, step...)
			cmd.Stdout, cmd.Stderr = log, log
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("luatdo %s: %w", step[0], err)
			}
		}
		for _, k := range []string{"prep", "ingest"} {
			fmt.Fprintf(log, "--- %s\n", k)
			if err := kinds[k](ctx, json.RawMessage(`{}`), log); err != nil {
				return fmt.Errorf("%s: %w", k, err)
			}
		}
		var index string
		_ = pool.QueryRow(ctx, `SELECT coalesce(value->>'index', '') FROM settings WHERE key = 'retrieval'`).Scan(&index)
		if index == httpapi.IndexEconomy {
			fmt.Fprintln(log, "embed skipped: the knowledge screen is set to economy (keywords only)")
			return nil
		}
		fmt.Fprintln(log, "--- embed")
		if err := kinds["embed"](ctx, json.RawMessage(`{}`), log); err != nil {
			// The corpus is in: new chunks are found by keywords until an
			// embed job runs.
			fmt.Fprintf(log, "embed skipped: %v\n", err)
		}
		return nil
	}
}

// scheduleCrawls queues the crawl when its weekly or monthly slot has passed and none
// has been queued since, checked every few minutes, so a server that was
// down at the slot catches up when it starts.
func scheduleCrawls(ctx context.Context, pool *pgxpool.Pool, runner *jobs.Runner, log *slog.Logger) {
	if _, err := os.Stat(crawlerDir()); err != nil {
		log.Info("weekly crawl off: the dataset's crawler is not installed", "dir", crawlerDir(), "env", CrawlerEnv)
		return
	}
	started := time.Now()
	check := func() {
		s := httpapi.LoadSchedule(ctx, pool)
		if !s.Enabled {
			return
		}
		slot := s.LastSlot(time.Now())
		var since, ever int
		if err := pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE created_at >= $1), count(*) FROM ingest_jobs WHERE kind = 'crawl'`, slot).Scan(&since, &ever); err != nil {
			log.Warn("crawl schedule", "err", err)
			return
		}
		// A missed slot is caught up only once crawls are under way: a
		// server's first start does not crawl at once for a slot it never saw.
		if since > 0 || (ever == 0 && slot.Before(started)) {
			return
		}
		id, err := runner.Enqueue(ctx, "crawl", json.RawMessage(`{"max_pages": `+strconv.Itoa(s.MaxPages)+`}`), 0)
		if err != nil {
			log.Warn("crawl schedule", "err", err)
			return
		}
		log.Info("weekly crawl queued", "job", id, "slot", slot)
	}
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		check()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// runCrawl runs the crawl alone and prints what it found. luatdo, prep and
// ingest are left to run after it, as the weekly job runs them.
func runCrawl(args []string) error {
	fs := flag.NewFlagSet("crawl", flag.ContinueOnError)
	maxPages := fs.Int("max-pages", crawl.DefaultSchedule().MaxPages, "catalogue pages walked, newest first")
	rotate := fs.Bool("rotate", true, "also fetch again this week's share of the in-scope documents")
	if ok, err := parse(fs, args); !ok {
		return err
	}
	data := os.Getenv("LUATDO_DATA")
	if data == "" {
		return errors.New("set LUATDO_DATA to the data directory")
	}
	ctx, cancel := signalContext()
	defer cancel()
	res, err := crawl.Run(ctx, crawl.Options{Data: data, Crawler: crawlerDir(), Python: os.Getenv(PythonEnv),
		MaxPages: *maxPages, NoRotation: !*rotate, CatalogAction: os.Getenv(ActionEnv), Log: os.Stderr})
	if err != nil {
		return err
	}
	return printJSON(res)
}
