package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nvtquang/CustomsVN/internal/evalx"
	"github.com/nvtquang/CustomsVN/internal/search"
)

// goldSetImport is the body of POST /api/goldsets. With dry_run the set is
// parsed and resolved and every problem returned, and nothing is stored: the
// screen shows that as the preview.
type goldSetImport struct {
	Name    string `json:"name"`
	Format  string `json:"format"` // csv, json, or empty to detect
	Content string `json:"content"`
	Note    string `json:"note"`
	DryRun  bool   `json:"dry_run"`
	// Force stores the rows that resolved even when others did not.
	Force bool `json:"force"`
}

func (s *Server) importGoldSet(w http.ResponseWriter, r *http.Request) {
	var in goldSetImport
	if !readJSON(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" && !in.DryRun {
		writeError(w, http.StatusBadRequest, errors.New("a gold set needs a name"))
		return
	}
	items, rowErrs, err := evalx.Parse(in.Format, []byte(in.Content))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	rowErrs = append(rowErrs, evalx.Resolve(s.Engine.Corpus(), items, s.Engine.Now().Format(time.DateOnly))...)
	out := map[string]any{"items": items, "errors": rowErrs, "count": len(items)}
	if in.DryRun || len(rowErrs) > 0 && !in.Force {
		status := http.StatusOK
		if !in.DryRun {
			status = http.StatusUnprocessableEntity
		}
		writeJSON(w, status, out)
		return
	}
	format := in.Format
	if format == "" {
		format = "auto"
	}
	g, err := evalx.SaveGoldSet(r.Context(), s.Pool, in.Name, format, in.Note, items)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out["gold_set"] = g
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) listGoldSets(w http.ResponseWriter, r *http.Request) {
	sets, err := evalx.ListGoldSets(r.Context(), s.Pool)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, sets)
}

func (s *Server) getGoldSet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	g, items, err := evalx.LoadGoldSet(r.Context(), s.Pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"gold_set": g, "items": items})
}

type runStart struct {
	GoldSetID int64          `json:"gold_set_id"`
	Label     string         `json:"label"`
	Request   search.Request `json:"request"`
}

func (s *Server) startRun(w http.ResponseWriter, r *http.Request) {
	var in runStart
	if !readJSON(w, r, &in) {
		return
	}
	id, err := evalx.StartRun(s.ctx, s.Pool, s.Engine, in.GoldSetID, in.Label, in.Request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int64{"id": id})
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := evalx.ListRuns(r.Context(), s.Pool, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	run, results, err := evalx.GetRun(r.Context(), s.Pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "results": results})
}
