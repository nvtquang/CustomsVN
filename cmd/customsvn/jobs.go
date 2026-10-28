package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/config"
	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/embed"
	"github.com/nvtquang/CustomsVN/internal/ingest"
	"github.com/nvtquang/CustomsVN/internal/jobs"
	"github.com/nvtquang/CustomsVN/internal/prep"
	"github.com/nvtquang/CustomsVN/internal/search"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

// jobKinds are the data tasks the admin screen can start: the same work as
// the prep, ingest and embed commands, with their output in the job's log.
func jobKinds(cfg *config.Config, pool *pgxpool.Pool, e *search.Engine) map[string]jobs.Func {
	return map[string]jobs.Func{
		"prep": func(ctx context.Context, raw json.RawMessage, log io.Writer) error {
			var p struct {
				AsOf   string `json:"as_of"`
				Legacy *bool  `json:"legacy"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return err
			}
			if p.AsOf != "" {
				if _, err := time.Parse(time.DateOnly, p.AsOf); err != nil {
					return errors.New("as_of must be YYYY-MM-DD")
				}
			}
			legacy := p.Legacy == nil || *p.Legacy
			if cfg.Data == "" {
				return errors.New("no luatdo data directory: set LUATDO_DATA or data in the config")
			}
			// The documents are cut as the knowledge screen says.
			var st struct {
				Chunking corpus.Chunking `json:"chunking"`
			}
			var stored []byte
			if err := pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'retrieval'`).Scan(&stored); err == nil {
				_ = json.Unmarshal(stored, &st)
			}
			chunking := st.Chunking.Fill()
			fmt.Fprintf(log, "chunking: %s, max length %d\n", chunking.Mode, chunking.MaxLength)
			// prep takes no context and is not stopped halfway: an ingest
			// queued behind it would read half-written files.
			return prep.Run(prep.Options{Data: cfg.Data, Out: cfg.Prep, AsOf: p.AsOf, Legacy: legacy, Chunking: chunking, Log: log})
		},
		"ingest": func(ctx context.Context, _ json.RawMessage, log io.Writer) error {
			res, err := ingest.Run(ctx, pool, ingest.Options{Prep: cfg.Prep, Data: cfg.Data, Log: log})
			if err != nil {
				return err
			}
			out, _ := json.Marshal(res)
			fmt.Fprintf(log, "result: %s\n", out)
			return nil
		},
		"embed": func(ctx context.Context, raw json.RawMessage, log io.Writer) error {
			var p struct {
				Model  string `json:"model"`
				Target string `json:"target"`
				Limit  int    `json:"limit"`
			}
			if err := json.Unmarshal(raw, &p); err != nil {
				return err
			}
			// Any model the server has loaded: the config file's or the
			// settings screen's.
			models := e.Models()
			emb := models.Embedder(p.Model)
			if emb == nil {
				if p.Model == "" {
					return errors.New("no embedding model: add one in the model settings or the config file")
				}
				return fmt.Errorf("no embedding model %q is loaded", p.Model)
			}
			src := models.Sources[emb.Name()]
			opts := embed.StoreOptions{Limit: p.Limit, Log: log}
			var res *embed.StoreResult
			var err error
			switch p.Target {
			case "", "chunks":
				res, err = embed.EmbedChunks(ctx, pool, emb, src.Provider, src.Model, opts)
			case "hs":
				res, err = embed.EmbedHS(ctx, pool, emb, src.Provider, src.Model, opts)
			default:
				return fmt.Errorf("target must be chunks or hs, not %q", p.Target)
			}
			if res != nil {
				out, _ := json.Marshal(res)
				fmt.Fprintf(log, "result: %s\n", out)
			}
			return err
		},
	}
}

// afterJob loads a new corpus into the running server once an ingest has
// written one, so answers use it without a restart. Loading takes the better
// part of a minute of CPU, so an ingest that changed nothing skips it.
func afterJob(pool *pgxpool.Pool, e *search.Engine, ts *tools.Toolset) func(context.Context, string) error {
	return func(ctx context.Context, kind string) error {
		if kind != "ingest" {
			return nil
		}
		var latest int64
		if err := pool.QueryRow(ctx, `SELECT coalesce(max(id), 0) FROM corpus_versions`).Scan(&latest); err != nil {
			return err
		}
		ts.Reset() // HS lines are rewritten by every ingest
		if latest == e.Corpus().Version {
			return nil
		}
		c, err := search.Load(ctx, pool)
		if err != nil {
			return err
		}
		e.Swap(c)
		ts.Reset()
		return nil
	}
}
