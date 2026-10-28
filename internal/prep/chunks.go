package prep

import (
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/tamnd/luatdo/law"

	"github.com/nvtquang/CustomsVN/internal/corpus"
)

// Kept is one line of the chunks step's manifest: a document that was or was
// not put into the corpus, and why.
type Kept struct {
	ID             string   `json:"id"`
	Kept           bool     `json:"kept"`
	Reason         string   `json:"reason,omitempty"`
	SourceKind     string   `json:"source_kind,omitempty"`
	OfficialNumber string   `json:"official_number,omitempty"`
	Title          string   `json:"title,omitempty"`
	Tier           int      `json:"tier,omitempty"`
	Force          string   `json:"force,omitempty"`
	Base           string   `json:"base,omitempty"`
	AmendedBy      []string `json:"amended_by,omitempty"`
	Notes          int      `json:"notes,omitempty"`
	ConsolidatedIn string   `json:"consolidated_in,omitempty"`
	Verdict        string   `json:"verdict,omitempty"`
}

// batch is how many store documents are held in memory at once.
const batch = 256

func (r *run) chunks() error {
	domain, order, err := r.loadDomain()
	if err != nil {
		return err
	}
	chunks, manifest := newJSONL(), newJSONL()
	// A chunk identifier is the primary key the corpus is ingested under, so
	// two chunks claiming one is a defect to stop on, not to paper over.
	seen := map[string]bool{}
	ch := corpus.NewChunker(func(c corpus.Chunk) error {
		if seen[c.ID] {
			return fmt.Errorf("chunk %s emitted twice", c.ID)
		}
		seen[c.ID] = true
		return chunks.add(c)
	})
	ch.Chunking = r.Chunking.Fill()
	stats := ch.Stats

	tables, err := r.loadTables()
	if err != nil {
		return err
	}

	// 1. Consolidated texts, the latest consolidation of each base.
	vbhn, err := readParsed(r.out(StepVBHN))
	if err != nil {
		return err
	}
	type consolidation struct {
		p *Parsed
		corpus.Consolidation
	}
	latest := map[string]*consolidation{}
	var keys []string
	for i := range vbhn {
		p := &vbhn[i]
		if utf8.RuneCountInString(p.Text) < corpus.VBHNEmpty {
			stats["vbhn_empty_at_source"]++
			continue
		}
		c := &consolidation{p, corpus.ReadConsolidation(p.Doc.Title, p.Text)}
		key := p.Doc.ID
		if c.Base != "" {
			key = corpus.NormNumber(c.Base)
		}
		prev, seen := latest[key]
		if !seen {
			keys = append(keys, key)
		}
		if !seen || corpus.ISODate(p.Doc.SignedOn) > corpus.ISODate(prev.p.Doc.SignedOn) {
			latest[key] = c
		}
	}
	stats["vbhn_superseded_by_newer_consolidation"] = len(vbhn) - stats["vbhn_empty_at_source"] - len(latest)

	consolidatedIn := map[string]string{}
	for _, key := range keys {
		c := latest[key]
		d := c.p.Doc
		var base *Entry
		if c.Base != "" {
			base = domain[corpus.StoreID(c.Base)]
		}
		force := "unknown_base"
		src := &corpus.Source{Kind: corpus.SourceConsolidated, Tier: c.p.Tier, Subjects: []string{}, TextSource: c.p.TextSource,
			Consolidates: &corpus.Consolidates{Base: c.Base, AmendedBy: nonNil(c.Amenders)}}
		if src.Tier == 0 {
			src.Tier = corpus.Tier2
		}
		if base != nil {
			force = corpus.Force(&base.Meta, r.AsOf)
			if force == "" {
				// The base is gone, so this consolidation of it is history.
				stats["vbhn_base_expired"]++
				if err := manifest.add(Kept{ID: d.ID, Reason: "base " + c.Base + " no longer in force"}); err != nil {
					return err
				}
				continue
			}
			consolidatedIn[base.ID] = d.ID
			src.Subjects = base.Subjects
			src.Consolidates.BaseDocID = base.ID
			stats["vbhn_base_resolved"]++
		} else {
			stats["vbhn_base_unresolved"]++
		}
		src.Force = force
		stats["vbhn_kept"]++
		if err := manifest.add(Kept{ID: d.ID, Kept: true, SourceKind: src.Kind, OfficialNumber: d.OfficialNumber,
			Title: d.Title, Tier: src.Tier, Force: force, Base: c.Base, AmendedBy: c.Amenders, Notes: len(c.Notes)}); err != nil {
			return err
		}
		if err := r.chunkParsed(ch, c.p, src, c.Notes, tables); err != nil {
			return err
		}
	}

	// 2. In-force originals of tier 1 and 2.
	var picked []*Entry
	for _, id := range order {
		e := domain[id]
		if e.Tier <= corpus.Tier2 && e.Status == "parsed" && corpus.Force(&e.Meta, r.AsOf) != "" {
			picked = append(picked, e)
		}
	}
	r.logf("chunks: %d consolidated, %d originals", stats["vbhn_kept"], len(picked))
	for start := 0; start < len(picked); start += batch {
		group := picked[start:min(start+batch, len(picked))]
		paths := make([]string, len(group))
		for i, e := range group {
			paths[i] = filepath.Join(r.store.Docs(), law.FileName(e.ID))
		}
		docs, err := readAll[law.Document](paths)
		if err != nil {
			return err
		}
		for i, e := range group {
			src := &corpus.Source{Kind: corpus.SourceOriginal, Tier: e.Tier, Force: corpus.Force(&e.Meta, r.AsOf),
				Subjects: e.Subjects, ConsolidatedIn: consolidatedIn[e.ID]}
			stats[fmt.Sprintf("docs_original_tier%d", e.Tier)]++
			if err := manifest.add(Kept{ID: e.ID, Kept: true, SourceKind: src.Kind, Tier: src.Tier, Force: src.Force,
				ConsolidatedIn: src.ConsolidatedIn}); err != nil {
				return err
			}
			if err := ch.Tree(&e.Meta, docs[i].Provisions, src, nil); err != nil {
				return err
			}
			if err := ch.Tables(&e.Meta, tables[e.ID], docs[i].Provisions, src); err != nil {
				return err
			}
		}
	}

	// 3. Rescued from quarantine.
	rescued, err := readParsed(r.out(StepRescue))
	if err != nil {
		return err
	}
	for i := range rescued {
		p := &rescued[i]
		m := metaOf(p.Doc)
		force := corpus.Force(&m, r.AsOf)
		if force == "" {
			continue
		}
		src := &corpus.Source{Kind: corpus.SourceRescued, Tier: corpus.Tier2, Force: force, Subjects: []string{},
			RescueReason: p.Verdict, NeedsReview: p.Verdict == corpus.VerdictYearTypo}
		if e := domain[m.ID]; e != nil {
			src.Tier, src.Subjects = e.Tier, e.Subjects
		}
		stats["docs_rescued_"+p.Doc.Status]++
		if err := manifest.add(Kept{ID: m.ID, Kept: true, SourceKind: src.Kind, Tier: src.Tier, Force: force,
			Verdict: p.Verdict}); err != nil {
			return err
		}
		if err := r.chunkParsed(ch, p, src, nil, tables); err != nil {
			return err
		}
	}

	chunksHash, err := chunks.write(r.out("customs", "chunks.jsonl"))
	if err != nil {
		return err
	}
	manifestHash, err := manifest.write(r.out("customs", "documents.jsonl"))
	if err != nil {
		return err
	}
	return r.summary(StepChunks, map[string]any{"as_of": r.AsOf, "counts": stats, "chunking": ch.Chunking,
		"chunks_sha256": chunksHash, "documents_sha256": manifestHash})
}

func (r *run) chunkParsed(ch *corpus.Chunker, p *Parsed, src *corpus.Source, notes map[string]string, tables map[string][][][]string) error {
	m := metaOf(p.Doc)
	var err error
	if p.Doc.Status == "parsed" {
		err = ch.Tree(&m, p.Doc.Provisions, src, notes)
	} else {
		err = ch.Text(&m, p.Text, src)
	}
	if err != nil {
		return err
	}
	return ch.Tables(&m, tables[m.ID], p.Doc.Provisions, src)
}

// loadTables reads the tables step's output. A corpus built without that step
// simply has no table chunks.
func (r *run) loadTables() (map[string][][][]string, error) {
	paths, err := jsonFiles(r.out(StepTables))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	all, err := readAll[Tables](paths)
	if err != nil {
		return nil, err
	}
	out := make(map[string][][][]string, len(all))
	for _, t := range all {
		out[t.DocID] = t.Tables
	}
	return out, nil
}

func readParsed(dir string) ([]Parsed, error) {
	paths, err := jsonFiles(dir)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%s is missing: run the %s step first", dir, filepath.Base(dir))
	}
	if err != nil {
		return nil, err
	}
	return readAll[Parsed](paths)
}

func metaOf(d *law.Document) corpus.Meta {
	return corpus.Meta{
		ID: d.ID, OfficialNumber: d.OfficialNumber, IssuingBody: d.IssuingBody, Title: d.Title, DocType: d.DocType,
		EffectiveFrom: d.EffectiveFrom, SignedOn: d.SignedOn, ExpiredOn: d.ExpiredOn, ForceStatus: d.ForceStatus,
		SourceURL: d.SourceURL, Status: d.Status, Quarantine: d.Quarantine,
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
