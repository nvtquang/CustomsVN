package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/chat"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/llmconfig"
	"github.com/nvtquang/CustomsVN/internal/mcpserver"
	"github.com/nvtquang/CustomsVN/internal/secret"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

// runMCP serves MCP over stdin and stdout, for a client that starts the
// server itself (Claude Desktop's "command" entry). Everything but the
// protocol goes to stderr.
func runMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file (default ./customsvn.yaml)")
	if ok, err := parse(fs, args); !ok {
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
	e, fileModels, err := newEngine(ctx, cfg, pool, log)
	if err != nil {
		return err
	}
	box, err := secret.FromEnv()
	if err != nil && !errors.Is(err, secret.ErrNoMasterKey) {
		return err
	}
	ts := &tools.Toolset{Pool: pool, Engine: e}
	router, problems, err := (&llmconfig.Store{Pool: pool, Box: box}).Router(ctx)
	if err != nil {
		return err
	}
	models, more, err := (&llmconfig.Store{Pool: pool, Box: box}).Models(ctx)
	if err != nil {
		return err
	}
	e.SetModels(fileModels.Merge(models))
	problems = append(problems, more...)
	for _, p := range problems {
		log.Warn("llm provider", "err", p)
	}
	a := &answer.Answerer{Engine: e, Router: func() *llm.Router { return router }, Now: e.Now, Tools: ts, ToolLoop: true}
	llms := &llmconfig.Store{Pool: pool, Box: box}
	st, err := llms.Load(ctx)
	if err != nil {
		return err
	}
	store := &chat.Store{Pool: pool, Cost: st.Prices.Cost}
	record := func(ctx context.Context, res *answer.Result) {
		if err := store.RecordUsage(ctx, 0, "mcp:", res); err != nil {
			log.Warn("record usage", "err", err)
		}
	}
	return mcpserver.New(version, ts, a, record).Run(ctx, &mcp.StdioTransport{})
}
