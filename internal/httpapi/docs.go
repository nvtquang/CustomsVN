package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// provision is one node of a document's structure tree.
type provision struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parent_id,omitempty"`
	Kind     string  `json:"kind"`
	Number   string  `json:"number"`
	Heading  string  `json:"heading,omitempty"`
	Text     string  `json:"text,omitempty"`
	Position int     `json:"position"`
}

// document returns a document, its structure tree, its force today, its
// chunks and the documents it is consolidated in or consolidates.
func (s *Server) document(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	c := s.Engine.Corpus()
	d := c.Docs[id]
	if d == nil {
		writeError(w, http.StatusNotFound, errors.New("no such document in the corpus"))
		return
	}
	rows, err := s.Pool.Query(ctx, `SELECT id, parent_id, kind, number, heading, text, position FROM provisions WHERE document_id = $1 ORDER BY position`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	provs := []provision{}
	for rows.Next() {
		var p provision
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Kind, &p.Number, &p.Heading, &p.Text, &p.Position); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		provs = append(provs, p)
	}
	type chunkRef struct {
		ID   string   `json:"id"`
		Path []string `json:"path"`
		Kind string   `json:"chunk_kind"`
	}
	chunks := []chunkRef{}
	text := map[string]string{}
	for _, ch := range c.ChunksOf(id) {
		chunks = append(chunks, chunkRef{ID: ch.ID, Path: ch.Path, Kind: ch.Kind})
		text[ch.ID] = ch.Text
	}
	today := s.Engine.Now().Format(time.DateOnly)
	related := map[string]any{}
	if d.ConsolidatedIn != "" {
		if v := c.Docs[d.ConsolidatedIn]; v != nil {
			related["consolidated_in"] = v
		}
	}
	if d.Consolidates != "" {
		if b := c.Docs[d.Consolidates]; b != nil {
			related["consolidates"] = b
		}
	}
	// A text-only document has chunks but no tree; the viewer shows chunks.
	out := map[string]any{"doc": d, "force_today": c.Force(d, today, today), "provisions": provs, "chunks": chunks, "related": related}
	if len(provs) == 0 {
		out["chunk_text"] = text
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) provision(w http.ResponseWriter, r *http.Request) {
	var p provision
	var doc string
	err := s.Pool.QueryRow(r.Context(), `SELECT id, parent_id, kind, number, heading, text, position, document_id FROM provisions WHERE id = $1`,
		r.PathValue("id")).Scan(&p.ID, &p.ParentID, &p.Kind, &p.Number, &p.Heading, &p.Text, &p.Position, &doc)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, errors.New("no such provision"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provision": p, "doc": s.Engine.Corpus().Docs[doc]})
}
