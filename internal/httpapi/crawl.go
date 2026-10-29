package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/crawl"
)

// LoadSchedule reads the weekly crawl's schedule; none stored is the default.
func LoadSchedule(ctx context.Context, pool *pgxpool.Pool) crawl.Schedule {
	s := crawl.DefaultSchedule()
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'crawl'`).Scan(&raw); err == nil {
		_ = json.Unmarshal(raw, &s)
	}
	if s.Validate() != nil {
		return crawl.DefaultSchedule()
	}
	return s
}

type crawlView struct {
	crawl.Schedule
	// Installed says whether the dataset's crawler is on this server.
	Installed bool `json:"installed"`
}

func (s *Server) crawlView(ctx context.Context) crawlView {
	_, err := os.Stat(s.CrawlerDir)
	return crawlView{Schedule: LoadSchedule(ctx, s.Pool), Installed: s.CrawlerDir != "" && err == nil}
}

func (s *Server) getCrawl(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.crawlView(r.Context()))
}

func (s *Server) putCrawl(w http.ResponseWriter, r *http.Request) {
	var in crawl.Schedule
	if !readJSON(w, r, &in) {
		return
	}
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	raw, _ := json.Marshal(in)
	if _, err := s.Pool.Exec(r.Context(), `INSERT INTO settings (key, value) VALUES ('crawl', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, raw); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.crawlView(r.Context()))
}
