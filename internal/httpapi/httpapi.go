// Package httpapi is the HTTP API of customsvn serve and the Web UI it ships.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/chat"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/llmconfig"
	"github.com/nvtquang/CustomsVN/internal/search"
)

// Server holds what the handlers need.
type Server struct {
	Pool   *pgxpool.Pool
	Engine *search.Engine
	// Models are the embedding models a search may name.
	Models []string
	// Rerank says whether a reranker is configured.
	Rerank bool
	// UI is the built Web UI; nil serves a note instead.
	UI  fs.FS
	Log *slog.Logger
	// Chat stores conversations; LLM stores providers and roles.
	Chat *chat.Store
	LLM  *llmconfig.Store
	// Answerer answers questions with the router ReloadLLM last built.
	Answerer *answer.Answerer
	router   atomic.Pointer[llm.Router]
	// ctx outlives a request, for batch runs.
	ctx context.Context
}

// ReloadLLM rebuilds the model router from the stored settings. Problems
// with single providers are returned; the rest of the router still works.
func (s *Server) ReloadLLM(ctx context.Context) ([]error, error) {
	r, problems, err := s.LLM.Router(ctx)
	if err != nil {
		return nil, err
	}
	s.router.Store(r)
	for _, p := range problems {
		s.Log.Warn("llm provider", "err", p)
	}
	return problems, nil
}

// Router is the current model router.
func (s *Server) Router() *llm.Router { return s.router.Load() }

// Handler returns the routes.
func (s *Server) Handler(ctx context.Context) http.Handler {
	s.ctx = ctx
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /api/meta", s.meta)
	mux.HandleFunc("POST /api/retrieval/test", s.retrievalTest)
	mux.HandleFunc("GET /api/settings/retrieval", s.getRetrievalSettings)
	mux.HandleFunc("PUT /api/settings/retrieval", s.putRetrievalSettings)
	mux.HandleFunc("GET /api/chunks/{id}", s.chunk)
	mux.HandleFunc("POST /api/goldsets", s.importGoldSet)
	mux.HandleFunc("GET /api/goldsets", s.listGoldSets)
	mux.HandleFunc("GET /api/goldsets/{id}", s.getGoldSet)
	mux.HandleFunc("POST /api/retrieval/runs", s.startRun)
	mux.HandleFunc("GET /api/retrieval/runs", s.listRuns)
	mux.HandleFunc("GET /api/retrieval/runs/{id}", s.getRun)
	mux.HandleFunc("POST /api/chat", s.chatStream)
	mux.HandleFunc("GET /api/conversations", s.listConversations)
	mux.HandleFunc("GET /api/conversations/{id}", s.getConversation)
	mux.HandleFunc("PATCH /api/conversations/{id}", s.renameConversation)
	mux.HandleFunc("DELETE /api/conversations/{id}", s.deleteConversation)
	mux.HandleFunc("POST /api/feedback", s.feedback)
	mux.HandleFunc("GET /api/documents/{id}", s.document)
	mux.HandleFunc("GET /api/provisions/{id}", s.provision)
	mux.HandleFunc("GET /api/settings/llm", s.getLLMSettings)
	mux.HandleFunc("PUT /api/settings/llm", s.putLLMSettings)
	mux.HandleFunc("POST /api/settings/llm/test", s.testLLM)
	mux.Handle("/", s.ui())
	return s.logRequests(mux)
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
	c := s.Engine.Corpus()
	var asOf string
	var created time.Time
	_ = s.Pool.QueryRow(r.Context(), `SELECT as_of::text, created_at FROM corpus_versions WHERE id = $1`, c.Version).Scan(&asOf, &created)
	writeJSON(w, http.StatusOK, map[string]any{
		"corpus_version": c.Version, "corpus_as_of": asOf, "ingested_at": created,
		"documents": len(c.Docs), "chunks": len(c.Chunks),
		"embedding_models": s.Models, "default_model": s.Engine.DefaultModel, "rerank": s.Rerank,
		"defaults": search.Defaults(), "today": s.Engine.Now().Format(time.DateOnly),
		"llm_ready": s.Router() != nil && s.Router().Has(llm.RoleAnswer),
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

// RetrievalSettings is what the settings screen edits: the filters and the
// configuration a search starts from.
type RetrievalSettings struct {
	Filters search.Filters `json:"filters"`
	Config  search.Config  `json:"config"`
}

func (s *Server) settings(ctx context.Context) RetrievalSettings {
	st := RetrievalSettings{Config: search.Defaults()}
	var raw []byte
	if err := s.Pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'retrieval'`).Scan(&raw); err == nil {
		_ = json.Unmarshal(raw, &st)
	}
	return st
}

// RetrievalSettings is the stored filters and configuration, for answers.
func (s *Server) RetrievalSettings(ctx context.Context) (search.Filters, search.Config) {
	st := s.settings(ctx)
	return st.Filters, st.Config
}

func (s *Server) getRetrievalSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settings(r.Context()))
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
