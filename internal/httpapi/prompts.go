package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/answer"
)

// Prompts returns the stored system prompts. A prompt left empty, or saved
// equal to the default, is stored empty, so a better default in a later
// release reaches every install that never changed it.
func (s *Server) Prompts(ctx context.Context) answer.Prompts { return StoredPrompts(ctx, s.Pool) }

// StoredPrompts reads the prompts from the database, for callers without a
// Server (the MCP command).
func StoredPrompts(ctx context.Context, pool *pgxpool.Pool) answer.Prompts {
	var p answer.Prompts
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'prompts'`).Scan(&raw); err == nil {
		_ = json.Unmarshal(raw, &p)
	}
	return p
}

func (s *Server) savePrompts(ctx context.Context, in answer.Prompts) error {
	d := answer.DefaultPrompts()
	for _, x := range []struct {
		name     string
		v        *string
		fallback string
	}{{"compose", &in.Compose, d.Compose}, {"agent", &in.Agent, d.Agent}, {"router", &in.Router, d.Router}, {"rewrite", &in.Rewrite, d.Rewrite}} {
		*x.v = strings.TrimSpace(*x.v)
		if len([]rune(*x.v)) > answer.MaxPrompt {
			return fmt.Errorf("%s: at most %d characters", x.name, answer.MaxPrompt)
		}
		if *x.v == strings.TrimSpace(x.fallback) {
			*x.v = ""
		}
	}
	raw, _ := json.Marshal(in)
	_, err := s.Pool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('prompts', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, raw)
	return err
}

type promptsView struct {
	// Prompts are the ones in use, defaults filled in.
	Prompts answer.Prompts `json:"prompts"`
	// Custom says which prompts differ from the default.
	Custom   map[string]bool `json:"custom"`
	Defaults answer.Prompts  `json:"defaults"`
	// Contracts are appended by code to each prompt and cannot be edited.
	Contracts answer.Prompts `json:"contracts"`
}

func (s *Server) promptsView(ctx context.Context) promptsView {
	stored := s.Prompts(ctx)
	return promptsView{Prompts: stored.Fill(), Defaults: answer.DefaultPrompts(), Contracts: answer.Contracts(),
		Custom: map[string]bool{"compose": stored.Compose != "", "agent": stored.Agent != "", "router": stored.Router != "", "rewrite": stored.Rewrite != ""}}
}

func (s *Server) getPrompts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.promptsView(r.Context()))
}

func (s *Server) putPrompts(w http.ResponseWriter, r *http.Request) {
	var in answer.Prompts
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.savePrompts(r.Context(), in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.promptsView(r.Context()))
}
