package prep

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tamnd/luatdo/cite"
	"github.com/tamnd/luatdo/law"
	"github.com/tamnd/luatdo/subject"

	"github.com/nvtquang/CustomsVN/internal/corpus"
)

// Entry is one store document as the tier step leaves it in domain.jsonl:
// its header, its subjects and its tier. Later steps read this file instead of
// the 3 GB of documents.
type Entry struct {
	corpus.Meta
	Subjects []string `json:"subjects"`
	corpus.Tiering
}

func (r *run) domainPath() string { return r.out("domain.jsonl") }

func (r *run) tier() error {
	paths, err := jsonFiles(r.store.Docs())
	if err != nil {
		return err
	}
	r.logf("tier: reading %d documents", len(paths))
	metas, err := readAll[corpus.Meta](paths)
	if err != nil {
		return err
	}
	records, err := subject.ReadRecords(filepath.Join(r.store.Subject(), subject.AssignmentsFile))
	if err != nil {
		return fmt.Errorf("subjects (run luatdo subjects first): %w", err)
	}
	subjects := map[string][]string{}
	for _, rec := range records {
		var ids []string
		for _, a := range rec.Subjects {
			if !slices.Contains(ids, a.SubjectID) {
				ids = append(ids, a.SubjectID)
			}
		}
		slices.Sort(ids)
		subjects[rec.DocID] = ids
	}

	entries := make([]Entry, len(metas))
	for i := range metas {
		s := subjects[metas[i].ID]
		if s == nil {
			s = []string{}
		}
		entries[i] = Entry{Meta: metas[i], Subjects: s, Tiering: corpus.TierOf(&metas[i], s)}
	}
	// File names sort like identifiers except where ":" became "_".
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.ID, b.ID) })
	byID := make(map[string]*Entry, len(entries))
	for i := range entries {
		byID[entries[i].ID] = &entries[i]
	}

	// The national acts customs leans on: cited by two or more tier 1
	// documents, issued centrally, and not already tiered by their title.
	cited := map[string]int{}
	for i := range entries {
		e := &entries[i]
		if e.Tier != corpus.Tier1 {
			continue
		}
		var links []cite.Link
		path := filepath.Join(r.store.Cite(), law.FileName(e.ID))
		if err := readJSONFile(path, &links); err != nil {
			return err
		}
		for _, l := range links {
			if l.ToDoc != "" && l.ToDoc != e.ID {
				cited[l.ToDoc]++
			}
		}
	}
	promoted := 0
	for id, n := range cited {
		e := byID[id]
		if e != nil && n >= corpus.PromoteCited && e.Tier == corpus.Tier3 && !corpus.Provincial(e.IssuingBody) {
			e.Tiering = corpus.Tiering{Tier: corpus.Tier2, Why: []string{fmt.Sprintf("cited by %d tier-1 documents", n)}}
			promoted++
		}
	}

	out := newJSONL()
	stats := map[string]int{"documents": len(entries), "promoted_by_citation": promoted}
	drops := map[string]int{}
	for i := range entries {
		e := &entries[i]
		if err := out.add(e); err != nil {
			return err
		}
		stats["documents_"+e.Status]++
		stats[fmt.Sprintf("tier%d", e.Tier)]++
		if force, why := corpus.Kept(&e.Meta, r.AsOf); force != "" {
			stats["in_force_kept"]++
		} else {
			stats["in_force_dropped"]++
			drops[why]++
		}
		if e.Tier <= corpus.Tier2 && corpus.Force(&e.Meta, r.AsOf) != "" && e.Status == "parsed" {
			stats[fmt.Sprintf("in_force_tier%d_parsed", e.Tier)]++
		}
		if e.Tier <= corpus.Tier2 && corpus.Live(&e.Meta) && e.Status == "quarantined" {
			stats["quarantined_live_tier12"]++
		}
	}
	hash, err := out.write(r.domainPath())
	if err != nil {
		return err
	}
	return r.summary(StepTier, map[string]any{"as_of": r.AsOf, "counts": stats, "drop_reasons": drops, "domain_sha256": hash})
}

// readJSONFile reads a JSON file into v. A missing file leaves v untouched:
// luatdo writes no citation file for a document that cites nothing.
func readJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// loadDomain reads the tier step's output.
func (r *run) loadDomain() (map[string]*Entry, []string, error) {
	byID := map[string]*Entry{}
	var order []string
	err := readJSONL(r.domainPath(), func(e Entry) error {
		byID[e.ID] = &e
		order = append(order, e.ID)
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("%s is missing: run the tier step first", r.domainPath())
	}
	return byID, order, err
}

func (r *run) summary(step string, v map[string]any) error {
	r.logf("%s: %v", step, v["counts"])
	return writeJSON(r.out("summary", step+".json"), v)
}
