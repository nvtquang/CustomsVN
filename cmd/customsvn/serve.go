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
	"github.com/nvtquang/CustomsVN/internal/chat"
	"github.com/nvtquang/CustomsVN/internal/config"
	"github.com/nvtquang/CustomsVN/internal/embed"
	"github.com/nvtquang/CustomsVN/internal/evalx"
	"github.com/nvtquang/CustomsVN/internal/httpapi"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/llmconfig"
	"github.com/nvtquang/CustomsVN/internal/search"
	"github.com/nvtquang/CustomsVN/internal/secret"
	"github.com/nvtquang/CustomsVN/web"
)

// newEngine loads the corpus into memory and wires every embedding model and
// the reranker whose key is present. A model whose key is missing is skipped
// with a warning, so serve still runs BM25 on a machine with no keys.
func newEngine(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, log *slog.Logger) (*search.Engine, []string, bool, error) {
	start := time.Now()
	c, err := search.Load(ctx, pool)
	if err != nil {
		return nil, nil, false, err
	}
	log.Info("corpus loaded", "version", c.Version, "documents", len(c.Docs), "chunks", len(c.Chunks), "ms", time.Since(start).Milliseconds())
	e := search.NewEngine(c)
	e.Pool = pool
	var models []string
	for i := range cfg.Embedding {
		m := &cfg.Embedding[i]
		emb, err := embed.New(m)
		if err != nil {
			log.Warn("embedding model skipped", "model", m.Name, "err", err)
			continue
		}
		e.Embedders[m.Name] = emb
		models = append(models, m.Name)
		if e.DefaultModel == "" {
			e.DefaultModel = m.Name
		}
	}
	rerank := false
	if cfg.Rerank != nil {
		r, err := search.NewReranker(cfg.Rerank)
		if err != nil {
			log.Warn("reranker skipped", "err", err)
		} else {
			e.Reranker, rerank = r, true
		}
	}
	return e, models, rerank, nil
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
	e, models, rerank, err := newEngine(ctx, cfg, pool, log)
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
	srv := &httpapi.Server{Pool: pool, Engine: e, Models: models, Rerank: rerank, UI: web.Dist(), Log: log,
		Chat: &chat.Store{Pool: pool}, LLM: &llmconfig.Store{Pool: pool, Box: box}}
	srv.Answerer = &answer.Answerer{Engine: e, Router: srv.Router, Now: e.Now, Retrieval: srv.RetrievalSettings}
	if _, err := srv.ReloadLLM(ctx); err != nil {
		return err
	}
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
	e, _, _, err := newEngine(ctx, cfg, pool, log)
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
