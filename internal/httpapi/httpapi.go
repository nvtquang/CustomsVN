// Package httpapi is the HTTP API of customsvn serve and the Web UI it ships.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/auth"
	"github.com/nvtquang/CustomsVN/internal/chat"
	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/evalx"
	"github.com/nvtquang/CustomsVN/internal/jobs"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/llmconfig"
	"github.com/nvtquang/CustomsVN/internal/ratelimit"
	"github.com/nvtquang/CustomsVN/internal/search"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

// Server holds what the handlers need.
type Server struct {
	Pool   *pgxpool.Pool
	Engine *search.Engine
	// FileModels are the embedding models and reranker of the config file;
	// the settings screen's are added to them on every reload.
	FileModels *search.Models
	// UI is the built Web UI; nil serves a note instead.
	UI  fs.FS
	Log *slog.Logger
	// Chat stores conversations; LLM stores providers and roles.
	Chat *chat.Store
	LLM  *llmconfig.Store
	// Answerer answers questions with the router ReloadLLM last built.
	Answerer *answer.Answerer
	// Tools serve the agent, MCP and the HS screen.
	Tools *tools.Toolset
	// Version is reported to MCP clients.
	Version string
	// Auth holds users and sessions.
	Auth *auth.Store
	// Jobs runs prep, ingest and embed in the background; nil disables them.
	Jobs *jobs.Runner
	// Answers runs answer evaluations.
	Answers *evalx.AnswerRunner
	// PrepDir is where prep writes, for the coverage report.
	PrepDir string
	// reloadMu serializes reloads: each reads the settings after the save
	// that asked for it, so the last to finish holds the latest.
	reloadMu sync.Mutex
	router   atomic.Pointer[llm.Router]
	prices   atomic.Pointer[llmconfig.Prices]
	// loginLimit slows password guessing; questionLimit bounds model spend.
	loginLimit, questionLimit *ratelimit.Limiter
	// ctx outlives a request, for batch runs.
	ctx context.Context
}

// ReloadLLM rebuilds the model router, the embedding models and the
// rerankers from the stored settings. Problems with single providers are
// returned; the rest still works.
func (s *Server) ReloadLLM(ctx context.Context) ([]error, error) {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	r, problems, err := s.LLM.Router(ctx)
	if err != nil {
		return nil, err
	}
	models, more, err := s.LLM.Models(ctx)
	if err != nil {
		return nil, err
	}
	problems = append(problems, more...)
	s.Engine.SetModels(s.FileModels.Merge(models))
	st, err := s.LLM.Load(ctx)
	if err != nil {
		return nil, err
	}
	s.router.Store(r)
	s.prices.Store(&st.Prices)
	for _, p := range problems {
		s.Log.Warn("llm provider", "err", p)
	}
	return problems, nil
}

// Router is the current model router.
func (s *Server) Router() *llm.Router { return s.router.Load() }

// Cost prices one call's usage with the stored prices.
func (s *Server) Cost(u llm.Usage) float64 {
	if p := s.prices.Load(); p != nil {
		return p.Cost(u)
	}
	return 0
}

// RecordUsage stores the usage of an answer given outside a conversation.
func (s *Server) RecordUsage(ctx context.Context, user int64, prefix string, res *answer.Result) {
	if err := s.Chat.RecordUsage(context.WithoutCancel(ctx), user, prefix, res); err != nil {
		s.Log.Warn("record usage", "err", err)
	}
}

// Handler returns the routes.
func (s *Server) Handler(ctx context.Context) http.Handler {
	s.ctx = ctx
	s.loginLimit, s.questionLimit = newLimiters()
	if l := s.limits(ctx); l != DefaultLimits {
		s.questionLimit.Set(l.PerMinute, l.PerMinute)
	}
	if s.Chat.Cost == nil {
		s.Chat.Cost = s.Cost
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	// Accounts.
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.logout)
	mux.HandleFunc("GET /api/auth/me", s.me)
	mux.Handle("POST /api/auth/password", s.authed(s.changePassword))
	// Everyone logged in: ask, read, look up.
	mux.Handle("GET /api/meta", s.authed(s.meta))
	mux.Handle("GET /api/chunks/{id}", s.authed(s.chunk))
	mux.Handle("POST /api/chat", s.authed(s.chatStream))
	mux.Handle("GET /api/conversations", s.authed(s.listConversations))
	mux.Handle("GET /api/conversations/{id}", s.authed(s.getConversation))
	mux.Handle("PATCH /api/conversations/{id}", s.authed(s.renameConversation))
	mux.Handle("DELETE /api/conversations/{id}", s.authed(s.deleteConversation))
	mux.Handle("POST /api/feedback", s.authed(s.feedback))
	mux.Handle("GET /api/documents/{id}", s.authed(s.document))
	mux.Handle("GET /api/provisions/{id}", s.authed(s.provision))
	mux.Handle("GET /api/hs", s.authed(s.hs))
	mux.Handle("POST /api/tools/{name}", s.authed(s.tool))
	// Admins: settings, labs, data, review, users.
	mux.Handle("POST /api/retrieval/test", s.admin(s.retrievalTest))
	mux.Handle("GET /api/settings/retrieval", s.admin(s.getRetrievalSettings))
	mux.Handle("PUT /api/settings/retrieval", s.admin(s.putRetrievalSettings))
	mux.Handle("POST /api/goldsets", s.admin(s.importGoldSet))
	mux.Handle("GET /api/goldsets", s.admin(s.listGoldSets))
	mux.Handle("GET /api/goldsets/{id}", s.admin(s.getGoldSet))
	mux.Handle("POST /api/retrieval/runs", s.admin(s.startRun))
	mux.Handle("GET /api/retrieval/runs", s.admin(s.listRuns))
	mux.Handle("GET /api/retrieval/runs/{id}", s.admin(s.getRun))
	mux.Handle("POST /api/answers/runs", s.admin(s.startAnswerRun))
	mux.Handle("GET /api/answers/runs", s.admin(s.listAnswerRuns))
	mux.Handle("GET /api/answers/runs/{id}", s.admin(s.getAnswerRun))
	mux.Handle("POST /api/answers/runs/{id}/cancel", s.admin(s.cancelAnswerRun))
	mux.Handle("GET /api/settings/llm", s.admin(s.getLLMSettings))
	mux.Handle("PUT /api/settings/llm", s.admin(s.putLLMSettings))
	mux.Handle("POST /api/settings/llm/test", s.admin(s.testLLM))
	mux.Handle("POST /api/settings/llm/models", s.admin(s.listModels))
	mux.Handle("GET /api/settings/limits", s.admin(s.getLimits))
	mux.Handle("PUT /api/settings/limits", s.admin(s.putLimits))
	mux.Handle("GET /api/settings/export", s.admin(s.exportConfig))
	mux.Handle("POST /api/settings/import", s.admin(s.importConfig))
	mux.Handle("GET /api/admin/corpus", s.admin(s.adminCorpus))
	mux.Handle("GET /api/admin/jobs", s.admin(s.listJobs))
	mux.Handle("POST /api/admin/jobs", s.admin(s.startJob))
	mux.Handle("GET /api/admin/jobs/{id}", s.admin(s.getJob))
	mux.Handle("POST /api/admin/jobs/{id}/cancel", s.admin(s.cancelJob))
	mux.Handle("GET /api/admin/feedback", s.admin(s.listFeedback))
	mux.Handle("GET /api/admin/feedback/{id}", s.admin(s.getFeedback))
	mux.Handle("PATCH /api/admin/feedback/{id}", s.admin(s.reviewFeedback))
	mux.Handle("POST /api/admin/feedback/{id}/gold", s.admin(s.feedbackToGold))
	mux.Handle("GET /api/admin/dashboard", s.admin(s.dashboard))
	mux.Handle("GET /api/admin/users", s.admin(s.listUsers))
	mux.Handle("POST /api/admin/users", s.admin(s.createUser))
	mux.Handle("PATCH /api/admin/users/{id}", s.admin(s.updateUser))
	mux.Handle("DELETE /api/admin/users/{id}", s.admin(s.deleteUser))
	// Other programs: the API token or a session.
	mux.Handle("/mcp", s.apiAccess(s.mcpHandler(s.Version)))
	mux.Handle("GET /v1/models", s.apiAccess(http.HandlerFunc(s.models)))
	mux.Handle("POST /v1/chat/completions", s.apiAccess(http.HandlerFunc(s.chatCompletions)))
	mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, errors.New("no such endpoint"))
	}))
	mux.Handle("/", s.ui())
	return s.logRequests(securityHeaders(mux))
}

// securityHeaders keeps the UI out of frames on other sites and stops the
// browser from guessing content types.
func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		h.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.Log.Info("request", "method", r.Method, "path", r.URL.Path, "ms", time.Since(start).Milliseconds())
		}
	})
}

// ui serves the single-page app: a file when it exists, index.html for every
// other path so client routes survive a reload.
func (s *Server) ui() http.Handler {
	if s.UI == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Web UI not built: run npm run build in web/ and rebuild customsvn", http.StatusNotFound)
		})
	}
	files := http.FileServerFS(s.UI)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := s.UI.Open(p); err == nil {
				_ = f.Close()
				if strings.HasPrefix(p, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, s.UI, "index.html")
	})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	out := map[string]any{"status": "ok", "corpus_version": s.Engine.Corpus().Version}
	if err := s.Pool.Ping(ctx); err != nil {
		out["status"], out["database"] = "degraded", err.Error()
		writeJSON(w, http.StatusServiceUnavailable, out)
		return
	}
	out["database"] = "ok"
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	c, ms := s.Engine.Corpus(), s.Engine.Models()
	var asOf string
	var created time.Time
	_ = s.Pool.QueryRow(r.Context(), `SELECT as_of::text, created_at FROM corpus_versions WHERE id = $1`, c.Version).Scan(&asOf, &created)
	writeJSON(w, http.StatusOK, map[string]any{
		"corpus_version": c.Version, "corpus_as_of": asOf, "ingested_at": created,
		"documents": len(c.Docs), "chunks": len(c.Chunks),
		"embedding_models": ms.EmbeddingNames(), "default_model": ms.DefaultModel,
		"rerank": len(ms.Rerankers) > 0, "rerank_models": ms.RerankNames(),
		"defaults": search.Defaults(), "today": s.Engine.Now().Format(time.DateOnly),
		"llm_ready": s.Router() != nil && s.Router().Has(llm.RoleAnswer),
		"user":      userOf(r), "version": s.Version,
	})
}

func (s *Server) retrievalTest(w http.ResponseWriter, r *http.Request) {
	var req search.Request
	if !readJSON(w, r, &req) {
		return
	}
	res, err := s.Engine.Search(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// RetrievalSettings is what the knowledge screen edits: how documents are
// cut, and the filters and configuration a search starts from.
type RetrievalSettings struct {
	Filters search.Filters `json:"filters"`
	Config  search.Config  `json:"config"`
	// TopK is how many passages an answer is given; Config.K is the
	// Retrieval Testing screen's. Zero means DefaultTopK.
	TopK     int             `json:"top_k"`
	Chunking corpus.Chunking `json:"chunking"`
	// Index is "high_quality" (embeddings beside keywords) or "economy"
	// (keywords only, no embedding calls); economy searches BM25 whatever
	// the mode says.
	Index string `json:"index,omitempty"`
	// Prepared is how the last prep cut the documents, read from its
	// summary; it is reported, never stored.
	Prepared *corpus.Chunking `json:"prepared,omitempty"`
}

// DefaultTopK is how many passages an answer is given unless set otherwise.
const DefaultTopK = 8

// Index methods.
const (
	IndexHighQuality = "high_quality"
	IndexEconomy     = "economy"
)

func (s *Server) settings(ctx context.Context) RetrievalSettings {
	st := RetrievalSettings{Config: search.Defaults()}
	var raw []byte
	if err := s.Pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'retrieval'`).Scan(&raw); err == nil {
		_ = json.Unmarshal(raw, &st)
	}
	if st.TopK <= 0 {
		st.TopK = DefaultTopK
	}
	st.Chunking = st.Chunking.Fill()
	st.Prepared = nil
	return st
}

// RetrievalSettings is the stored filters and configuration, for answers:
// cut to the answer's top K, and with the article put back around a clause
// when documents are cut parent and child.
func (s *Server) RetrievalSettings(ctx context.Context) (search.Filters, search.Config) {
	st := s.settings(ctx)
	st.Config.K = st.TopK
	if st.Index == IndexEconomy {
		st.Config.Mode = search.ModeBM25
	}
	if st.Chunking.Mode == corpus.ChunkParentChild {
		st.Config.ParentContext = st.Chunking.ParentLength
	}
	return st.Filters, st.Config
}

// prepared reads how the last prep cut the documents. A summary written
// before chunking was a setting has none: those runs were general.
func (s *Server) prepared() *corpus.Chunking {
	if s.PrepDir == "" {
		return nil
	}
	raw, err := os.ReadFile(filepath.Join(s.PrepDir, "summary", "chunks.json"))
	if err != nil {
		return nil
	}
	var sum struct {
		Chunking corpus.Chunking `json:"chunking"`
	}
	if json.Unmarshal(raw, &sum) != nil {
		return nil
	}
	c := sum.Chunking.Fill()
	return &c
}

func (s *Server) getRetrievalSettings(w http.ResponseWriter, r *http.Request) {
	st := s.settings(r.Context())
	st.Prepared = s.prepared()
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) putRetrievalSettings(w http.ResponseWriter, r *http.Request) {
	var st RetrievalSettings
	if !readJSON(w, r, &st) {
		return
	}
	if st.Config.Mode != "" && st.Config.Mode != search.ModeHybrid && st.Config.Mode != search.ModeBM25 && st.Config.Mode != search.ModeVector {
		writeError(w, http.StatusBadRequest, errors.New("mode must be hybrid, bm25 or vector"))
		return
	}
	if st.Config.K < 0 || st.Config.K > 100 || st.Config.BM25K < 0 || st.Config.BM25K > 500 || st.Config.VectorK < 0 || st.Config.VectorK > 500 {
		writeError(w, http.StatusBadRequest, errors.New("k must be 1-100 and candidate counts 1-500"))
		return
	}
	if st.TopK < 0 || st.TopK > 30 {
		writeError(w, http.StatusBadRequest, errors.New("top_k must be 1-30"))
		return
	}
	if st.Config.MinScore < 0 || st.Config.MinScore >= 1 {
		writeError(w, http.StatusBadRequest, errors.New("min_score must be from 0 to below 1"))
		return
	}
	if err := st.Chunking.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if st.Index != "" && st.Index != IndexHighQuality && st.Index != IndexEconomy {
		writeError(w, http.StatusBadRequest, errors.New("index must be high_quality or economy"))
		return
	}
	// The answer's parent context comes from the chunking, never from a
	// stored configuration.
	st.Config.ParentContext, st.Prepared = 0, nil
	raw, _ := json.Marshal(st)
	if _, err := s.Pool.Exec(r.Context(), `INSERT INTO settings (key, value) VALUES ('retrieval', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, raw); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) chunk(w http.ResponseWriter, r *http.Request) {
	c := s.Engine.Corpus()
	ch := c.Chunks[r.PathValue("id")]
	if ch == nil {
		writeError(w, http.StatusNotFound, errors.New("no such chunk"))
		return
	}
	d := c.Docs[ch.DocID]
	writeJSON(w, http.StatusOK, map[string]any{"chunk": ch, "doc": d,
		"force_today": c.Force(d, s.Engine.Now().Format(time.DateOnly), s.Engine.Now().Format(time.DateOnly))})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
