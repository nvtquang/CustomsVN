package search

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamnd/luatdo/law"
	"github.com/tamnd/luatdo/retrieve"

	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/refs"
)

// Doc is a corpus document as search needs it.
type Doc struct {
	ID             string `json:"id"`
	OfficialNumber string `json:"official_number"`
	DocType        string `json:"doc_type"`
	Title          string `json:"title"`
	IssuingBody    string `json:"issuing_body"`
	SignedOn       string `json:"signed_on,omitempty"`
	EffectiveFrom  string `json:"effective_from,omitempty"`
	ExpiredOn      string `json:"expired_on,omitempty"`
	ForceStatus    string `json:"force_status"`
	Status         string `json:"status"`
	Tier           int    `json:"tier"`
	SourceKind     string `json:"source_kind"`
	SourceURL      string `json:"source_url"`
	Consolidates   string `json:"consolidates,omitempty"`
	ConsolidatedIn string `json:"consolidated_in,omitempty"`
}

func (d *Doc) meta() corpus.Meta {
	return corpus.Meta{ID: d.ID, OfficialNumber: d.OfficialNumber, ForceStatus: d.ForceStatus,
		EffectiveFrom: d.EffectiveFrom, ExpiredOn: d.ExpiredOn, Status: d.Status}
}

// Chunk is one retrievable unit.
type Chunk struct {
	ID             string   `json:"id"`
	DocID          string   `json:"doc_id"`
	Path           []string `json:"path"`
	Text           string   `json:"text"`
	Kind           string   `json:"chunk_kind"`
	Tier           int      `json:"tier"`
	SourceKind     string   `json:"source_kind"`
	AmendmentNotes []string `json:"amendment_notes,omitempty"`
	NeedsReview    bool     `json:"needs_review,omitempty"`
	TextSource     string   `json:"text_source,omitempty"`
}

// Tail is the chunk's place below its document, "article-18:clause-2",
// without the part suffix of a cut chunk.
func (c *Chunk) Tail() string {
	t := strings.TrimPrefix(c.ID, c.DocID+":")
	if i := strings.Index(t, "#"); i >= 0 {
		t = t[:i]
	}
	return t
}

// Corpus is the searchable corpus held in memory: the BM25 index, the
// documents with their force, and the lookups references resolve through.
type Corpus struct {
	Version int64
	Docs    map[string]*Doc
	Chunks  map[string]*Chunk
	byDoc   map[string][]*Chunk
	byTail  map[string]map[string]*Chunk // doc -> tail -> chunk
	family  map[string][]*Chunk          // article -> its chunks, in document order
	numbers map[string][]*Doc            // normalised official number -> docs
	names   map[string][]*Doc            // folded law name -> docs
	index   *retrieve.Index
}

// NewCorpus indexes documents and chunks.
func NewCorpus(version int64, docs []Doc, chunks []Chunk) *Corpus {
	c := &Corpus{Version: version, Docs: map[string]*Doc{}, Chunks: map[string]*Chunk{}, byDoc: map[string][]*Chunk{},
		byTail: map[string]map[string]*Chunk{}, family: map[string][]*Chunk{}, numbers: map[string][]*Doc{}, names: map[string][]*Doc{}}
	for i := range docs {
		d := &docs[i]
		c.Docs[d.ID] = d
		n := corpus.NormNumber(d.OfficialNumber)
		c.numbers[n] = append(c.numbers[n], d)
		if key := lawName(d); key != "" {
			c.names[key] = append(c.names[key], d)
		}
	}
	// The BM25 index sees each chunk as one component of its document, with
	// the heading path as its heading so a question naming the chapter or
	// article still reaches it.
	ldocs := map[string]*law.Document{}
	var order []string
	for i := range chunks {
		ch := &chunks[i]
		if c.Docs[ch.DocID] == nil {
			continue
		}
		c.Chunks[ch.ID] = ch
		c.byDoc[ch.DocID] = append(c.byDoc[ch.DocID], ch)
		if c.byTail[ch.DocID] == nil {
			c.byTail[ch.DocID] = map[string]*Chunk{}
		}
		if _, ok := c.byTail[ch.DocID][ch.Tail()]; !ok {
			c.byTail[ch.DocID][ch.Tail()] = ch
		}
		if k := parentKey(ch.ID); k != "" {
			c.family[k] = append(c.family[k], ch)
		}
		ld := ldocs[ch.DocID]
		if ld == nil {
			d := c.Docs[ch.DocID]
			ld = &law.Document{ID: d.ID, Title: d.OfficialNumber + " " + d.Title}
			ldocs[ch.DocID] = ld
			order = append(order, ch.DocID)
		}
		ld.Provisions = append(ld.Provisions, law.Provision{ID: ch.ID, Kind: "chunk", Heading: strings.Join(ch.Path, " / "),
			Text: ch.Text, Position: len(ld.Provisions)})
	}
	in := retrieve.Input{}
	for _, id := range order {
		in.Docs = append(in.Docs, ldocs[id])
	}
	c.index = retrieve.Build(in)
	return c
}

// lawName is the folded name a law is cited by, "luat-hai-quan", or "" for an
// instrument that is cited by number.
func lawName(d *Doc) string {
	switch d.DocType {
	case "Luật", "Bộ luật", "Pháp lệnh":
	default:
		return ""
	}
	t := strings.TrimSpace(d.Title)
	if i := strings.Index(t, " số "); i > 0 {
		t = t[:i]
	}
	key := refs.NameKey(t)
	prefix := refs.NameKey(d.DocType)
	if !strings.HasPrefix(key, prefix+"-") {
		key = prefix + "-" + key
	}
	return key
}

// Load reads the current corpus from the database.
func Load(ctx context.Context, pool *pgxpool.Pool) (*Corpus, error) {
	var version int64
	if err := pool.QueryRow(ctx, `SELECT coalesce(max(id), 0) FROM corpus_versions`).Scan(&version); err != nil {
		return nil, err
	}
	if version == 0 {
		return nil, fmt.Errorf("the database holds no corpus: run customsvn ingest")
	}
	rows, err := pool.Query(ctx, `SELECT id, official_number, doc_type, title, issuing_body, signed_on, effective_from,
		expired_on, force_status, status, tier, source_kind, source_url, coalesce(consolidates, ''), coalesce(consolidated_in, '')
		FROM documents ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var docs []Doc
	for rows.Next() {
		var d Doc
		var signed, eff, exp *time.Time
		if err := rows.Scan(&d.ID, &d.OfficialNumber, &d.DocType, &d.Title, &d.IssuingBody, &signed, &eff, &exp,
			&d.ForceStatus, &d.Status, &d.Tier, &d.SourceKind, &d.SourceURL, &d.Consolidates, &d.ConsolidatedIn); err != nil {
			return nil, err
		}
		d.SignedOn, d.EffectiveFrom, d.ExpiredOn = isoOf(signed), isoOf(eff), isoOf(exp)
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows, err = pool.Query(ctx, `SELECT id, document_id, path, text, chunk_kind, tier, source_kind, amendment_notes,
		needs_review, coalesce(text_source, '') FROM chunks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var chunks []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.DocID, &c.Path, &c.Text, &c.Kind, &c.Tier, &c.SourceKind, &c.AmendmentNotes,
			&c.NeedsReview, &c.TextSource); err != nil {
			return nil, err
		}
		chunks = append(chunks, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return NewCorpus(version, docs, chunks), nil
}

func isoOf(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.DateOnly)
}

// Force is the force of a document on asOf, judged on today's knowledge.
// A consolidated text has the force of the instrument it consolidates; one
// whose base is not in the corpus has none that can be stated, and is treated
// as not in force.
func (c *Corpus) Force(d *Doc, asOf, today string) string {
	if d.SourceKind == corpus.SourceConsolidated {
		base := c.Docs[d.Consolidates]
		if base == nil {
			return ""
		}
		return ForceAt(base.meta(), asOf, today)
	}
	return ForceAt(d.meta(), asOf, today)
}

// ForceAt is a document's force on a day that may be in the past.
//
// The status label describes today, so it only decides today's question. For
// an earlier day the dates decide: a document was in force from its effective
// date until its expiry date. A document the label calls ended but that
// carries no expiry date ended on a day nobody recorded, and is not in force
// on any day it can be asked about.
func ForceAt(m corpus.Meta, asOf, today string) string {
	if eff := corpus.ISODate(m.EffectiveFrom); eff != "" && eff > asOf {
		return corpus.ForceNotYet
	}
	if asOf >= today {
		return corpus.Force(&m, asOf)
	}
	partly := strings.TrimSpace(m.ForceStatus) == corpus.StatusPartly
	if exp := corpus.ISODate(m.ExpiredOn); exp != "" {
		if exp > asOf {
			return corpus.ForceInForce
		}
		if partly {
			return corpus.ForcePartly
		}
		return ""
	}
	return corpus.Force(&m, asOf)
}

// Usable reports whether a force lets a document ground an answer on the day:
// in force or partly expired. A document not yet in force is not law yet.
func Usable(force string) bool {
	return force == corpus.ForceInForce || force == corpus.ForcePartly
}

// ResolveDoc finds the corpus document a reference names, preferring the one
// in force on asOf. A name is matched by the longest known law name it starts
// with, so "Luật Hải quan, người khai..." finds the Customs Law and "Luật Thuế
// xuất khẩu, thuế nhập khẩu" is not cut at its comma.
func (c *Corpus) ResolveDoc(number, name, asOf, today string) *Doc {
	var cands []*Doc
	if number != "" {
		cands = c.numbers[corpus.NormNumber(number)]
	}
	if len(cands) == 0 && name != "" {
		q, best := law.Slug(name), ""
		for key := range c.names {
			if (q == key || strings.HasPrefix(q, key+"-")) && len(key) > len(best) {
				best = key
			}
		}
		cands = c.names[best]
	}
	if len(cands) == 0 {
		return nil
	}
	sorted := slices.Clone(cands)
	slices.SortStableFunc(sorted, func(a, b *Doc) int {
		ua, ub := Usable(c.Force(a, asOf, today)), Usable(c.Force(b, asOf, today))
		if ua != ub {
			if ua {
				return -1
			}
			return 1
		}
		return strings.Compare(b.SignedOn, a.SignedOn)
	})
	return sorted[0]
}

// ChunksOf returns a document's chunks in corpus order.
func (c *Corpus) ChunksOf(docID string) []*Chunk { return c.byDoc[docID] }

// CoveringChunk is the chunk that holds a provision of a document: the chunk
// of that provision, of an ancestor (an article holds its clauses), or the
// first chunk of a descendant when the provision was split.
func (c *Corpus) CoveringChunk(docID, tail string) *Chunk {
	tails := c.byTail[docID]
	if tails == nil {
		return nil
	}
	for t := tail; t != ""; {
		if ch := tails[t]; ch != nil {
			return ch
		}
		i := strings.LastIndex(t, ":")
		if i < 0 {
			break
		}
		t = t[:i]
	}
	for _, ch := range c.byDoc[docID] {
		if strings.HasPrefix(ch.Tail(), tail+":") {
			return ch
		}
	}
	return nil
}

// Consolidated is the chunk of the consolidated text that stands for an
// original chunk, or nil when the original is not consolidated or the
// consolidated text has no such provision.
func (c *Corpus) Consolidated(ch *Chunk) *Chunk {
	d := c.Docs[ch.DocID]
	if d == nil || d.ConsolidatedIn == "" || ch.Kind == "table" {
		return nil
	}
	return c.CoveringChunk(d.ConsolidatedIn, ch.Tail())
}

// Canonical is where a chunk sits in the instrument it speaks for: a
// consolidated chunk speaks for its base. Gold sets name provisions of the
// base, and a hit on the consolidated wording of the same article is a hit.
func (c *Corpus) Canonical(ch *Chunk) (docID, tail string) {
	d := c.Docs[ch.DocID]
	if d != nil && d.SourceKind == corpus.SourceConsolidated && d.Consolidates != "" {
		return d.Consolidates, ch.Tail()
	}
	return ch.DocID, ch.Tail()
}

// Covers reports whether a chunk answers for an expected provision: the same
// provision, an ancestor of it, or a part of it.
func (c *Corpus) Covers(ch *Chunk, provisionID string) bool {
	doc, tail := c.Canonical(ch)
	if !strings.HasPrefix(provisionID, doc+":") {
		return false
	}
	want := strings.TrimPrefix(provisionID, doc+":")
	return tail == want || strings.HasPrefix(want, tail+":") || strings.HasPrefix(tail, want+":")
}

// parentKey is the article a chunk belongs to: its identifier up to the last
// article, "…:article-18" for "…:article-18:clause-2" and for the article
// itself. A chunk outside any article, a document's table or a piece of a
// text with no structure, has none.
func parentKey(id string) string {
	if i := strings.Index(id, "#"); i >= 0 {
		id = id[:i]
	}
	i := strings.LastIndex(id, ":article-")
	if i < 0 {
		return ""
	}
	if j := strings.Index(id[i+1:], ":"); j >= 0 {
		return id[:i+1+j]
	}
	return id
}
