// Package search is hybrid retrieval over the customs corpus.
//
// A question goes through five steps, each recorded in the response so the
// Retrieval Testing screen can show why a chunk ranked where it did:
//
//  1. References in the question ("khoản 2 Điều 18 Luật Hải quan") are looked
//     up directly and put first.
//  2. The scope is cut before anything is ranked: tiers, document types and
//     source kinds, and force on the as-of day, judged by expiry date and not
//     by the status label alone.
//  3. BM25 (luatdo retrieve) and, when a model is configured, pgvector each
//     rank the scope; an instrument named by number gets a third, restricted
//     BM25 list.
//  4. The lists are fused by reciprocal rank and optionally reranked.
//  5. An original chunk whose document has a consolidated text is replaced by
//     the consolidated chunk of the same provision.
package search

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamnd/luatdo/retrieve"

	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/refs"
)

// Modes.
const (
	ModeHybrid = "hybrid"
	ModeBM25   = "bm25"
	ModeVector = "vector"
)

// Filters cut the scope before ranking.
type Filters struct {
	// Tiers allowed. Empty means 1 and 2.
	Tiers []int `json:"tiers,omitempty"`
	// AsOf is the day force is judged on, YYYY-MM-DD. Empty means today.
	AsOf string `json:"as_of,omitempty"`
	// DocTypes restricts by document type ("Luật", "Thông tư", ...).
	DocTypes []string `json:"doc_types,omitempty"`
	// SourceKinds restricts to consolidated, original or rescued chunks.
	SourceKinds []string `json:"source_kinds,omitempty"`
	// Docs restricts to these documents.
	Docs []string `json:"docs,omitempty"`
	// IncludeExpired admits documents not in force on AsOf. For inspecting
	// the corpus only; an answer is never grounded on them.
	IncludeExpired bool `json:"include_expired,omitempty"`
}

// Config is how to rank. The zero value is filled with Defaults.
type Config struct {
	Mode         string  `json:"mode"`
	K            int     `json:"k"`
	BM25K        int     `json:"bm25_k"`
	VectorK      int     `json:"vector_k"`
	RRFK         float64 `json:"rrf_k"`
	BM25Weight   float64 `json:"bm25_weight"`
	VectorWeight float64 `json:"vector_weight"`
	Model        string  `json:"model,omitempty"`
	Rerank       bool    `json:"rerank"`
	RerankK      int     `json:"rerank_k"`
	// RerankModel names the reranker; empty means the default one.
	RerankModel string `json:"rerank_model,omitempty"`
	// MinScore drops hits scored below it, on a 0-1 scale: the reranker's
	// score when the hits were reranked, else the cosine similarity in a
	// vector search. Fused and BM25 scores have no such scale, so it does
	// not apply to them. Zero is off; a provision named in the question is
	// never dropped.
	MinScore float64 `json:"min_score,omitempty"`
	// ParentContext, when above zero, puts the rest of a hit's article after
	// it, nearest provisions first, up to this many characters in all: the
	// clause is what matched, the article is what the model reads. Each
	// added chunk is a real chunk, so a quote from it verifies as usual.
	ParentContext int `json:"parent_context,omitempty"`
	// KeepOriginals turns off step 5: original chunks are not replaced by
	// their consolidated wording.
	KeepOriginals bool `json:"keep_originals"`
	// IgnoreRefs turns off step 1.
	IgnoreRefs bool    `json:"ignore_refs"`
	Duplicates float64 `json:"duplicates"`
}

// Defaults is the configuration a search runs with unless told otherwise.
func Defaults() Config {
	return Config{Mode: ModeHybrid, K: 10, BM25K: 50, VectorK: 50, RRFK: 60, BM25Weight: 1, VectorWeight: 1, RerankK: 30}
}

// fill replaces zero values with the defaults.
func (c Config) fill() Config {
	d := Defaults()
	if c.Mode == "" {
		c.Mode = d.Mode
	}
	if c.K <= 0 {
		c.K = d.K
	}
	if c.BM25K <= 0 {
		c.BM25K = d.BM25K
	}
	if c.VectorK <= 0 {
		c.VectorK = d.VectorK
	}
	if c.RRFK <= 0 {
		c.RRFK = d.RRFK
	}
	if c.BM25Weight == 0 && c.VectorWeight == 0 {
		c.BM25Weight, c.VectorWeight = d.BM25Weight, d.VectorWeight
	}
	if c.RerankK <= 0 {
		c.RerankK = d.RerankK
	}
	return c
}

// Request is one search.
type Request struct {
	Query   string  `json:"query"`
	Filters Filters `json:"filters"`
	Config  Config  `json:"config"`
}

// Component is one ranker's view of a hit.
type Component struct {
	Rank  int     `json:"rank"`
	Score float64 `json:"score"`
}

// Hit is one result with every score that placed it.
type Hit struct {
	Rank       int        `json:"rank"`
	ChunkID    string     `json:"chunk_id"`
	Score      float64    `json:"score"`
	Direct     bool       `json:"direct,omitempty"`
	BM25       *Component `json:"bm25,omitempty"`
	Vector     *Component `json:"vector,omitempty"`
	Named      *Component `json:"named,omitempty"`
	RRF        float64    `json:"rrf"`
	Rerank     *float64   `json:"rerank,omitempty"`
	Replaced   string     `json:"replaced,omitempty"`   // the original chunk a consolidated one stands in for
	ContextOf  string     `json:"context_of,omitempty"` // added as part of this hit's article, not ranked
	Duplicates []string   `json:"duplicates,omitempty"`
	Chunk      *Chunk     `json:"chunk"`
	Doc        *Doc       `json:"doc"`
	Force      string     `json:"force"`
	Usable     bool       `json:"usable"`
}

// Resolved is a reference found in the question and where it led.
type Resolved struct {
	Ref     refs.Provision `json:"ref"`
	DocID   string         `json:"doc_id,omitempty"`
	ChunkID string         `json:"chunk_id,omitempty"`
	Note    string         `json:"note,omitempty"`
}

// Response is a search's result and the arithmetic behind it.
type Response struct {
	Query         string             `json:"query"`
	Filters       Filters            `json:"filters"`
	Config        Config             `json:"config"`
	CorpusVersion int64              `json:"corpus_version"`
	Refs          refs.Found         `json:"refs"`
	Resolved      []Resolved         `json:"resolved,omitempty"`
	InScopeDocs   int                `json:"in_scope_docs"`
	InScopeChunks int                `json:"in_scope_chunks"`
	Hits          []Hit              `json:"hits"`
	Notes         []string           `json:"notes,omitempty"`
	TimingMS      map[string]float64 `json:"timing_ms"`
	ExpiredInTopK int                `json:"expired_in_top_k"`
	// BelowThreshold counts the hits MinScore dropped.
	BelowThreshold int `json:"below_threshold,omitempty"`
}

// Notes and reference notes are codes, so the UI can say them in the
// reader's language.
const (
	NoteNoScope     = "no_scope"      // the filters left no document
	NoteBM25Only    = "bm25_only"     // hybrid without an embedding model
	NoteNoDatabase  = "no_database"   // vector search needs the database
	NoteNoReranker  = "no_reranker"   // rerank asked for, none configured
	NoteNoThreshold = "no_threshold"  // a score threshold with scores it cannot apply to
	RefNoInstrument = "no_instrument" // "Điều 5" with no instrument named
	RefNotInCorpus  = "not_in_corpus" // the instrument is not in the corpus
	RefNoProvision  = "no_provision"  // the instrument has no such provision
	RefOutOfScope   = "out_of_scope"  // found, but excluded by force or filters
)

// Reranker scores passages against a query, higher is better.
type Reranker interface {
	Rerank(ctx context.Context, query string, passages []string) ([]float64, error)
}

// Engine searches the corpus it holds. The corpus and the models can be
// swapped while searches run, after an ingest or a settings change.
type Engine struct {
	corpus atomic.Pointer[Corpus]
	models atomic.Pointer[Models]
	Pool   *pgxpool.Pool
	Now    func() time.Time
}

// NewEngine returns an engine over c, with no models.
func NewEngine(c *Corpus) *Engine {
	e := &Engine{Now: time.Now}
	e.corpus.Store(c)
	e.models.Store(NewModels())
	return e
}

// Models are the models searches currently use.
func (e *Engine) Models() *Models { return e.models.Load() }

// SetModels replaces the models.
func (e *Engine) SetModels(m *Models) { e.models.Store(m) }

// Corpus is the corpus searches currently run against.
func (e *Engine) Corpus() *Corpus { return e.corpus.Load() }

// Swap replaces the corpus.
func (e *Engine) Swap(c *Corpus) { e.corpus.Store(c) }

type ranked struct {
	id    string
	score float64
}

// Search runs one request.
func (e *Engine) Search(ctx context.Context, req Request) (*Response, error) {
	start := time.Now()
	c := e.Corpus()
	cfg := req.Config.fill()
	f := req.Filters
	if len(f.Tiers) == 0 {
		f.Tiers = []int{corpus.Tier1, corpus.Tier2}
	}
	today := e.Now().Format(time.DateOnly)
	if f.AsOf == "" {
		f.AsOf = today
	}
	if _, err := time.Parse(time.DateOnly, f.AsOf); err != nil {
		return nil, fmt.Errorf("as_of %q is not YYYY-MM-DD", f.AsOf)
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("empty query")
	}
	ms := e.Models()
	res := &Response{Query: query, Filters: f, Config: cfg, CorpusVersion: c.Version, TimingMS: map[string]float64{}}
	lap := func(name string, t time.Time) { res.TimingMS[name] = float64(time.Since(t).Microseconds()) / 1000 }

	// 2. Scope, by document.
	t := time.Now()
	allowed := map[string]bool{}
	force := map[string]string{}
	var scopeDocs []string
	for id, d := range c.Docs {
		fz := c.Force(d, f.AsOf, today)
		force[id] = fz
		if !slices.Contains(f.Tiers, d.Tier) ||
			len(f.DocTypes) > 0 && !slices.Contains(f.DocTypes, d.DocType) ||
			len(f.SourceKinds) > 0 && !slices.Contains(f.SourceKinds, d.SourceKind) ||
			len(f.Docs) > 0 && !slices.Contains(f.Docs, id) ||
			!f.IncludeExpired && !Usable(fz) {
			continue
		}
		allowed[id] = true
		scopeDocs = append(scopeDocs, id)
		res.InScopeChunks += len(c.byDoc[id])
	}
	sort.Strings(scopeDocs)
	res.InScopeDocs = len(scopeDocs)
	lap("scope", t)
	if len(scopeDocs) == 0 {
		res.Notes = append(res.Notes, NoteNoScope)
		res.TimingMS["total"] = float64(time.Since(start).Microseconds()) / 1000
		return res, nil
	}

	// 1. References.
	res.Refs = refs.Parse(query)
	var direct []string
	if !cfg.IgnoreRefs {
		for _, p := range res.Refs.Provisions {
			r := Resolved{Ref: p}
			if p.Number == "" && p.Name == "" {
				r.Note = RefNoInstrument
				res.Resolved = append(res.Resolved, r)
				continue
			}
			d := c.ResolveDoc(p.Number, p.Name, f.AsOf, today)
			if d == nil {
				r.Note = RefNotInCorpus
				res.Resolved = append(res.Resolved, r)
				continue
			}
			r.DocID = d.ID
			var ch *Chunk
			if !cfg.KeepOriginals && d.ConsolidatedIn != "" && allowed[d.ConsolidatedIn] {
				ch = c.CoveringChunk(d.ConsolidatedIn, p.Tail())
			}
			if ch == nil {
				ch = c.CoveringChunk(d.ID, p.Tail())
			}
			switch {
			case ch == nil:
				r.Note = RefNoProvision
			case !allowed[ch.DocID]:
				r.ChunkID, r.Note = ch.ID, RefOutOfScope
			default:
				r.ChunkID = ch.ID
				direct = append(direct, ch.ID)
			}
			res.Resolved = append(res.Resolved, r)
		}
	}

	// 3. Rankers.
	lists := map[string][]ranked{}
	weights := map[string]float64{}
	if cfg.Mode == ModeHybrid || cfg.Mode == ModeBM25 {
		t = time.Now()
		lists["bm25"] = c.bm25(query, scopeDocs, cfg.BM25K, cfg.Duplicates)
		weights["bm25"] = cfg.BM25Weight
		lap("bm25", t)
	}
	if cfg.Mode == ModeHybrid || cfg.Mode == ModeVector {
		model := cfg.Model
		if model == "" {
			model = ms.DefaultModel
		}
		emb := ms.Embedders[model]
		switch {
		case emb == nil && cfg.Mode == ModeVector:
			return nil, fmt.Errorf("vector search needs an embedding model; none named %q is configured", model)
		case emb == nil:
			res.Notes = append(res.Notes, NoteBM25Only)
		case e.Pool == nil:
			res.Notes = append(res.Notes, NoteNoDatabase)
		default:
			t = time.Now()
			list, err := e.vector(ctx, emb, query, scopeDocs, cfg.VectorK)
			if err != nil {
				return nil, fmt.Errorf("vector search: %w", err)
			}
			lists["vector"] = list
			weights["vector"] = cfg.VectorWeight
			res.Config.Model = model
			lap("vector", t)
		}
	}
	var named []string
	for _, n := range res.Refs.Numbers {
		if d := c.ResolveDoc(n, "", f.AsOf, today); d != nil && allowed[d.ID] && !slices.Contains(named, d.ID) {
			named = append(named, d.ID)
			if d.ConsolidatedIn != "" && allowed[d.ConsolidatedIn] {
				named = append(named, d.ConsolidatedIn)
			}
		}
	}
	if len(named) > 0 {
		lists["named"] = c.bm25(query, named, cfg.BM25K, cfg.Duplicates)
		weights["named"] = 1
	}

	// 4. Fusion.
	t = time.Now()
	hits := map[string]*Hit{}
	for name, list := range lists {
		for i, r := range list {
			h := hits[r.id]
			if h == nil {
				h = &Hit{ChunkID: r.id}
				hits[r.id] = h
			}
			comp := &Component{Rank: i + 1, Score: r.score}
			switch name {
			case "bm25":
				h.BM25 = comp
			case "vector":
				h.Vector = comp
			case "named":
				h.Named = comp
			}
			h.RRF += weights[name] / (cfg.RRFK + float64(i+1))
		}
	}
	order := make([]*Hit, 0, len(hits))
	for _, h := range hits {
		h.Score = h.RRF
		order = append(order, h)
	}
	sortHits(order)
	lap("fuse", t)

	reranker := ms.Reranker(cfg.RerankModel)
	if cfg.Rerank && reranker != nil && len(order) > 0 {
		t = time.Now()
		top := order[:min(cfg.RerankK, len(order))]
		passages := make([]string, len(top))
		for i, h := range top {
			passages[i] = c.Chunks[h.ChunkID].Text
		}
		scores, err := reranker.Rerank(ctx, query, passages)
		if err != nil {
			return nil, fmt.Errorf("rerank: %w", err)
		}
		for i, h := range top {
			s := scores[i]
			h.Rerank, h.Score = &s, s
		}
		sortHits(top)
		order = append(top, order[len(top):]...)
		if res.Config.RerankModel == "" {
			res.Config.RerankModel = ms.DefaultReranker
		}
		lap("rerank", t)
	} else if cfg.Rerank {
		res.Notes = append(res.Notes, NoteNoReranker)
	}

	// Direct references go first, in the order the question gave them.
	if len(direct) > 0 {
		var head []*Hit
		for _, id := range direct {
			h := hits[id]
			if h == nil {
				h = &Hit{ChunkID: id}
				hits[id] = h
			}
			h.Direct = true
			head = append(head, h)
		}
		rest := slices.DeleteFunc(order, func(h *Hit) bool { return h.Direct })
		order = append(head, rest...)
	}

	// 5. Consolidated wording first, duplicates folded, the threshold
	// applied, cut to K.
	threshold := cfg.MinScore > 0
	if threshold && (cfg.Rerank && reranker == nil || !cfg.Rerank && cfg.Mode != ModeVector) {
		res.Notes = append(res.Notes, NoteNoThreshold)
		threshold = false
	}
	seen := map[string]*Hit{}
	for _, h := range order {
		if len(res.Hits) == cfg.K {
			break
		}
		if threshold && !h.Direct && belowThreshold(h, cfg.MinScore) {
			res.BelowThreshold++
			continue
		}
		ch := c.Chunks[h.ChunkID]
		if !cfg.KeepOriginals {
			if v := c.Consolidated(ch); v != nil && allowed[v.DocID] {
				h.Replaced, h.ChunkID, ch = ch.ID, v.ID, v
			}
		}
		// Wording an amendment has since rewritten is out like an expired
		// document: the amendment is the source now.
		stale := c.Superseded(ch.ID, f.AsOf)
		if stale && !f.IncludeExpired {
			continue
		}
		if prev := seen[h.ChunkID]; prev != nil {
			prev.Duplicates = append(prev.Duplicates, firstNonEmpty(h.Replaced, h.ChunkID))
			continue
		}
		h.Chunk, h.Doc = ch, c.Docs[ch.DocID]
		h.Force = force[ch.DocID]
		h.Usable = Usable(h.Force) && !stale
		if !h.Usable {
			res.ExpiredInTopK++
		}
		h.Rank = len(res.Hits) + 1
		res.Hits = append(res.Hits, *h)
		seen[h.ChunkID] = &res.Hits[len(res.Hits)-1]
	}
	if cfg.ParentContext > 0 {
		res.Hits = c.withParents(res.Hits, cfg.ParentContext)
	}
	res.TimingMS["total"] = float64(time.Since(start).Microseconds()) / 1000
	return res, nil
}

// belowThreshold: a reranked hit is judged by its rerank score, any other by
// its cosine similarity; a hit the vector search did not return has none.
func belowThreshold(h *Hit, min float64) bool {
	switch {
	case h.Rerank != nil:
		return *h.Rerank < min
	case h.Vector != nil:
		return h.Vector.Score < min
	}
	return true
}

// withParents puts after each hit the other chunks of its article that are
// not hits already, nearest first, while the article stays within budget
// characters, then in document order.
func (c *Corpus) withParents(hits []Hit, budget int) []Hit {
	have := map[string]bool{}
	for _, h := range hits {
		have[h.ChunkID] = true
	}
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, h)
		fam := c.family[parentKey(h.ChunkID)]
		at := slices.IndexFunc(fam, func(x *Chunk) bool { return x.ID == h.ChunkID })
		if at < 0 || h.Chunk == nil {
			continue
		}
		used := utf8.RuneCountInString(h.Chunk.Text)
		var picked []int
		for d := 1; d < len(fam); d++ {
			for _, i := range []int{at - d, at + d} {
				if i < 0 || i >= len(fam) || have[fam[i].ID] {
					continue
				}
				n := utf8.RuneCountInString(fam[i].Text)
				if used+n > budget {
					continue
				}
				used += n
				picked = append(picked, i)
				have[fam[i].ID] = true
			}
		}
		slices.Sort(picked)
		for _, i := range picked {
			ch := fam[i]
			out = append(out, Hit{ChunkID: ch.ID, Chunk: ch, Doc: h.Doc, Force: h.Force, Usable: h.Usable, ContextOf: h.ChunkID})
		}
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

func sortHits(h []*Hit) {
	sort.SliceStable(h, func(i, j int) bool {
		if h[i].Score != h[j].Score {
			return h[i].Score > h[j].Score
		}
		return h[i].ChunkID < h[j].ChunkID
	})
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// bm25 ranks the chunks of the given documents with luatdo's BM25.
func (c *Corpus) bm25(query string, docs []string, k int, duplicates float64) []ranked {
	r := c.index.Search(retrieve.Query{Text: query, K: k, Scope: retrieve.Scope{Docs: docs}, Duplicates: duplicates})
	out := make([]ranked, len(r.Hits))
	for i, h := range r.Hits {
		out[i] = ranked{id: h.ID, score: h.Score}
	}
	return out
}

// HitFor is a chunk as a search hit, with its document's force on asOf. It
// is how a tool that fetches one provision hands it to the composer in the
// same shape a search would.
func (e *Engine) HitFor(chunkID, asOf string) *Hit {
	c := e.Corpus()
	ch := c.Chunks[chunkID]
	if ch == nil {
		return nil
	}
	d := c.Docs[ch.DocID]
	today := e.Now().Format(time.DateOnly)
	if asOf == "" {
		asOf = today
	}
	f := c.Force(d, asOf, today)
	return &Hit{ChunkID: ch.ID, Chunk: ch, Doc: d, Force: f, Usable: Usable(f) && !c.Superseded(ch.ID, asOf)}
}
