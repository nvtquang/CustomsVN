package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/chat"
	"github.com/nvtquang/CustomsVN/internal/httpapi"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/llmconfig"
	"github.com/nvtquang/CustomsVN/internal/secret"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

// askItem is one question of a batch and what came of it.
type askItem struct {
	ID       string          `json:"id"`
	Question string          `json:"question"`
	Answer   string          `json:"answer"`
	Outcome  string          `json:"outcome"` // answered, cards, refused, clarified, error
	Error    string          `json:"error,omitempty"`
	Result   *answer.Result  `json:"result,omitempty"`
	Cost     float64         `json:"cost_usd"`
	Elapsed  float64         `json:"elapsed_s"`
	Extra    json.RawMessage `json:"extra,omitempty"`
}

// runAsk puts a list of questions through the whole answer, each in a fresh
// conversation, the way the chat screen does, and writes what a reader would
// have seen. It needs no login, so a test set can be run in one go and graded
// from the file.
func runAsk(args []string) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file (default ./customsvn.yaml)")
	in := fs.String("questions", "", "JSON file: a list of {id, question}")
	out := fs.String("out", "", "write the answers to this JSON file")
	asOf := fs.String("as-of", "", "the day law is judged on, YYYY-MM-DD (default today)")
	only := fs.String("only", "", "comma-separated ids to run (default all)")
	parallel := fs.Int("parallel", 1, "questions answered at once, each in its own conversation")
	if ok, err := parse(fs, args); !ok {
		return err
	}
	if *in == "" || *out == "" {
		return errors.New("ask needs -questions and -out")
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	var items []askItem
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("%s: %w", *in, err)
	}
	if *only != "" {
		want := map[string]bool{}
		for _, id := range strings.Split(*only, ",") {
			want[strings.TrimSpace(id)] = true
		}
		var kept []askItem
		for _, it := range items {
			if want[it.ID] {
				kept = append(kept, it)
			}
		}
		items = kept
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
	srv := &httpapi.Server{Pool: pool, Engine: e, FileModels: fileModels, Log: log,
		Chat: &chat.Store{Pool: pool}, LLM: &llmconfig.Store{Pool: pool, Box: box}}
	srv.Tools = &tools.Toolset{Pool: pool, Engine: e}
	srv.Answerer = &answer.Answerer{Engine: e, Router: srv.Router, Now: e.Now, Retrieval: srv.RetrievalSettings,
		Prompts: srv.Prompts, Tools: srv.Tools, ToolLoop: true}
	if _, err := srv.ReloadLLM(ctx); err != nil {
		return err
	}
	if !srv.Router().Has(llm.RoleAnswer) {
		return errors.New("no answer model configured: set one in Settings › LLM")
	}

	if *parallel < 1 {
		*parallel = 1
	}
	var mu sync.Mutex // the output file and the progress line
	next := make(chan int)
	go func() {
		defer close(next)
		for i := range items {
			select {
			case next <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	var wg sync.WaitGroup
	var writeErr error
	for w := 0; w < *parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				ask(ctx, srv, &items[i], *asOf)
				mu.Lock()
				it := &items[i]
				fmt.Fprintf(os.Stderr, "%s %-9s %5.1fs $%.4f %s\n", it.ID, it.Outcome, it.Elapsed, it.Cost, truncateRunes(it.Question, 60))
				b, err := json.MarshalIndent(items, "", " ")
				if err == nil {
					err = os.WriteFile(*out, b, 0o644) // after each question: a long run survives a crash
				}
				if err != nil && writeErr == nil {
					writeErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if writeErr != nil {
		return writeErr
	}
	return ctx.Err()
}

// ask puts one question through the answer and records what came of it.
func ask(ctx context.Context, srv *httpapi.Server, it *askItem, asOf string) {
	start := time.Now()
	res, err := srv.Answerer.Answer(ctx, answer.Input{Question: it.Question, AsOf: asOf}, func(answer.Event) error { return nil })
	it.Elapsed = time.Since(start).Seconds()
	switch {
	case err != nil:
		it.Outcome, it.Error = "error", err.Error()
	case res.Refusal != nil:
		it.Outcome = "refused"
	case res.Clarify != "":
		it.Outcome = "clarified"
	case len(res.Claims) > 0:
		it.Outcome = "answered"
	default:
		it.Outcome = "cards"
	}
	if res != nil {
		it.Result = res
		it.Answer = res.Markdown() + statusLines(res)
		for _, u := range res.Usage {
			it.Cost += srv.Cost(u)
		}
	}
}

// statusLines is what the status cards of an answer say, as text: the web
// shows them as cards beside the claims, and a graded file has no cards.
func statusLines(res *answer.Result) string {
	var b strings.Builder
	for _, c := range res.Cards {
		st, ok := c.Data.(tools.Status)
		if c.Type != "status" || !ok {
			continue
		}
		state := map[string]string{"in_force": "còn hiệu lực", "partly_expired": "hết hiệu lực một phần", "not_yet_in_force": "chưa có hiệu lực", "": "không còn hiệu lực"}[st.Force]
		fmt.Fprintf(&b, "\n\n[Thẻ trạng thái] %s: %s tại %s.", st.Number, state, st.AsOf)
		if st.Note != "" {
			b.WriteString(" " + st.Note)
		}
		for i, l := range st.ReplacedBy {
			if i == 3 {
				break
			}
			b.WriteString(" Thay bởi: " + l.Number + ".")
		}
	}
	return b.String()
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
