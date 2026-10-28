package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/auth"
	"github.com/nvtquang/CustomsVN/internal/chat"
	"github.com/nvtquang/CustomsVN/internal/config"
	"github.com/nvtquang/CustomsVN/internal/embed"
	"github.com/nvtquang/CustomsVN/internal/evalx"
	"github.com/nvtquang/CustomsVN/internal/httpapi"
	"github.com/nvtquang/CustomsVN/internal/jobs"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/llmconfig"
	"github.com/nvtquang/CustomsVN/internal/search"
	"github.com/nvtquang/CustomsVN/internal/secret"
	"github.com/nvtquang/CustomsVN/internal/tools"
	"github.com/nvtquang/CustomsVN/web"
)

// newEngine loads the corpus into memory and wires every embedding model and
// the reranker of the config file whose key is present. A model whose key is
// missing is skipped with a warning, so serve still runs BM25 on a machine
// with no keys. The models of the settings screen are added by the server.
func newEngine(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (*search.Engine, *search.Models, error) {
	start := time.Now()
	c, err := search.Load(ctx, pool)
	if err != nil {
		return nil, nil, err
	}
	log.Info("corpus loaded", "version", c.Version, "documents", len(c.Docs), "chunks", len(c.Chunks), "ms", time.Since(start).Milliseconds())
	e := search.NewEngine(c)
	e.Pool = pool
	models := search.NewModels()
	for i := range cfg.Embedding {
		m := &cfg.Embedding[i]
		emb, err := embed.New(m)
		if err != nil {
			log.Warn("embedding model skipped", "model", m.Name, "err", err)
			continue
		}
		models.AddEmbedder(emb, search.Source{Provider: m.Provider, Model: m.Model})
	}
	if cfg.Rerank != nil {
		r, err := search.NewReranker(cfg.Rerank)
		if err != nil {
			log.Warn("reranker skipped", "err", err)
		} else {
			models.AddReranker(cfg.Rerank.Provider+"/"+cfg.Rerank.Model, r)
		}
	}
	e.SetModels(models)
	return e, models, nil
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file (default ./customsvn.yaml)")
	listen := fs.String("listen", "", "address to listen on (default from config, 127.0.0.1:8080)")
	if ok, err := parse(fs, args); !ok {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, cancel := signalContext()
	defer cancel()
	cfg, pool, err := openDB(ctx, *configPath)
	if err != nil {
		return err
	}
	defer pool.Close()
	if *listen != "" {
		cfg.Listen = *listen
	}
	e, fileModels, err := newEngine(ctx, cfg, pool, log)
	if err != nil {
		return err
	}
	box, err := secret.FromEnv()
	if err != nil {
		if !errors.Is(err, secret.ErrNoMasterKey) {
			return err
		}
		log.Warn("no master key: LLM API keys can come only from environment variables", "env", secret.Env)
	}
	srv := &httpapi.Server{Pool: pool, Engine: e, FileModels: fileModels, UI: web.Dist(), Log: log,
		Chat: &chat.Store{Pool: pool}, LLM: &llmconfig.Store{Pool: pool, Box: box}}
	srv.Tools = &tools.Toolset{Pool: pool, Engine: e}
	srv.Version = version
	srv.Answerer = &answer.Answerer{Engine: e, Router: srv.Router, Now: e.Now, Retrieval: srv.RetrievalSettings,
		Tools: srv.Tools, ToolLoop: true}
	if _, err := srv.ReloadLLM(ctx); err != nil {
		return err
	}
	srv.Auth = &auth.Store{Pool: pool}
	created, password, err := srv.Auth.EnsureAdmin(ctx, os.Getenv(AdminEmailEnv), os.Getenv(AdminPasswordEnv))
	if err != nil {
		return err
	}
	if created {
		if os.Getenv(AdminPasswordEnv) == "" {
			// Written to stderr once, never stored in clear; change it in the
			// UI or reset it with customsvn user passwd.
			fmt.Fprintf(os.Stderr, "\nFirst admin created. Log in with:\n  email:    %s\n  password: %s\nChange the password after logging in.\n\n",
				orDefault(os.Getenv(AdminEmailEnv), "admin"), password)
		} else {
			log.Info("first admin created", "email", orDefault(os.Getenv(AdminEmailEnv), "admin"))
		}
	}
	go pruneSessions(ctx, srv.Auth, log)
	if err := evalx.MarkInterrupted(ctx, pool); err != nil {
		return err
	}
	srv.PrepDir = cfg.Prep
	srv.Jobs = &jobs.Runner{Pool: pool, Log: log, Kinds: jobKinds(cfg, pool, e), After: afterJob(pool, e, srv.Tools)}
	if err := srv.Jobs.Start(ctx); err != nil {
		return err
	}
	srv.Answers = &evalx.AnswerRunner{Pool: pool, Answerer: srv.Answerer, LLM: srv.LLM, Cost: srv.Cost,
		Record: func(ctx context.Context, res *answer.Result) error { return srv.Chat.RecordUsage(ctx, 0, "eval:", res) }}
	if !srv.Router().Has(llm.RoleAnswer) {
		log.Warn("no answer model configured: set one in Settings › LLM")
	}
	if srv.UI == nil {
		log.Warn("Web UI not built; serving the API only")
	}
	hs := &http.Server{Addr: cfg.Listen, Handler: srv.Handler(ctx), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		_ = hs.Shutdown(sctx)
	}()
	log.Info("listening", "addr", "http://"+cfg.Listen)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// AdminEmailEnv and AdminPasswordEnv set the first admin, created when the
// database has no user. Without a password one is generated and printed once.
const (
	AdminEmailEnv    = "CUSTOMSVN_ADMIN_EMAIL"
	AdminPasswordEnv = "CUSTOMSVN_ADMIN_PASSWORD"
)

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func pruneSessions(ctx context.Context, a *auth.Store, log *slog.Logger) {
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		if err := a.Prune(ctx); err != nil && ctx.Err() == nil {
			log.Warn("prune sessions", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func runEval(args []string) error {
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file (default ./customsvn.yaml)")
	goldset := fs.String("goldset", "", "gold set file, CSV or JSON")
	mode := fs.String("mode", "", "hybrid, bm25 or vector (default hybrid)")
	model := fs.String("model", "", "embedding model for hybrid or vector")
	k := fs.Int("k", 10, "results per question")
	keep := fs.Bool("keep-originals", false, "do not replace original chunks by their consolidated wording")
	ignoreRefs := fs.Bool("ignore-refs", false, "do not look references in the question up directly")
	rerank := fs.Bool("rerank", false, "rerank with the configured reranker")
	jsonOut := fs.String("json", "", "write every result to this file")
	if ok, err := parse(fs, args); !ok {
		return err
	}
	if *goldset == "" {
		return errors.New("eval needs -goldset")
	}
	data, err := os.ReadFile(*goldset)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, cancel := signalContext()
	defer cancel()
	cfg, pool, err := openDB(ctx, *configPath)
	if err != nil {
		return err
	}
	defer pool.Close()
	e, _, err := newEngine(ctx, cfg, pool, log)
	if err != nil {
		return err
	}
	items, rowErrs, err := evalx.Parse("", data)
	if err != nil {
		return err
	}
	rowErrs = append(rowErrs, evalx.Resolve(e.Corpus(), items, e.Now().Format(time.DateOnly))...)
	for _, re := range rowErrs {
		fmt.Fprintln(os.Stderr, "gold set:", re.Error())
	}
	req := search.Request{Config: search.Config{Mode: *mode, Model: *model, K: *k, KeepOriginals: *keep, IgnoreRefs: *ignoreRefs, Rerank: *rerank}}
	m, results := evalx.Run(ctx, e, items, req, nil)
	fmt.Printf("corpus version %d, %d questions (%d scored, %d to refuse, %d errors)\n", e.Corpus().Version, m.Questions, m.Scored, m.Refuse, m.Errors)
	fmt.Printf("recall@1 %.3f  @3 %.3f  @5 %.3f  @10 %.3f   hit@10 %.3f   MRR %.3f   nDCG@10 %.3f\n",
		m.Recall["1"], m.Recall["3"], m.Recall["5"], m.Recall["10"], m.HitAt10, m.MRR, m.NDCG10)
	fmt.Printf("expired in top-10 %.3f   latency p50 %.1f ms  p95 %.1f ms\n", m.ExpiredRate, m.LatencyP50, m.LatencyP95)
	var missed []string
	for _, r := range results {
		switch {
		case r.Error != "":
			missed = append(missed, r.Item.ExtID+" error: "+r.Error)
		case r.Missed() && len(r.Item.ExpectedIDs) > 0:
			missed = append(missed, r.Item.ExtID+" "+r.Item.Question)
		}
	}
	if len(missed) > 0 {
		fmt.Printf("missed in top-10 (%d):\n  %s\n", len(missed), strings.Join(missed, "\n  "))
	}
	if *jsonOut != "" {
		return writeJSONFile(*jsonOut, map[string]any{"metrics": m, "results": results, "request": req})
	}
	return nil
}

func runEmbed(args []string) error {
	fs := flag.NewFlagSet("embed", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file (default ./customsvn.yaml)")
	model := fs.String("model", "", "embedding model name from the config (default the first)")
	limit := fs.Int("limit", 0, "stop after this many chunks (0 = all)")
	target := fs.String("target", "chunks", "what to embed: chunks or hs (HS descriptions)")
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
	if *model == "" {
		if len(cfg.Embedding) == 0 {
			return errors.New("no embedding models in the config; see customsvn.example.yaml")
		}
		*model = cfg.Embedding[0].Name
	}
	m, err := cfg.EmbeddingModel(*model)
	if err != nil {
		return err
	}
	emb, err := embed.New(m)
	if err != nil {
		return err
	}
	if *target == "hs" {
		res, err := embed.EmbedHS(ctx, pool, emb, m.Provider, m.Model, embed.StoreOptions{Limit: *limit, Log: os.Stderr})
		if res != nil {
			_ = printJSON(res)
		}
		return err
	}
	res, err := embed.EmbedChunks(ctx, pool, emb, m.Provider, m.Model, embed.StoreOptions{Limit: *limit, Log: os.Stderr})
	if res != nil {
		_ = printJSON(res)
	}
	return err
}

func writeJSONFile(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(v); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
