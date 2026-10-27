package prep

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/parquet-go/parquet-go"
	"github.com/tamnd/luatdo/law"
	"github.com/tamnd/luatdo/parse"
	"github.com/tamnd/luatdo/store"

	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/htmltable"
)

// Parsed is a document prep parsed itself, beside the plain text it was read
// from, so the chunks step can fall back to the text when the tree builder
// found no structure.
type Parsed struct {
	Doc     *law.Document `json:"doc"`
	Kind    string        `json:"kind"`
	Verdict string        `json:"verdict,omitempty"`
	Tier    int           `json:"tier,omitempty"`
	Text    string        `json:"text"`
	// TextSource names where Text came from when it is not the content
	// config: "legacy_content" for a consolidated text whose page at the
	// source is empty.
	TextSource string `json:"text_source,omitempty"`
}

// Status of a parsed document with no article structure.
const statusTextOnly = "text-only"

// HSDocID is TT 31/2022/TT-BTC, the Vietnamese nomenclature of goods.
const HSDocID = "vn:law:2022:31-2022-tt-btc"

type pick struct {
	kind    string
	meta    corpus.Meta
	verdict string
	tier    int
}

type contentRow struct {
	ID          string `parquet:"id"`
	ContentHTML string `parquet:"content_html"`
}

// content runs the steps that need the source HTML, in one pass over it.
func (r *run) content(steps []string) error {
	rows, err := parquet.ReadFile[corpus.Row](filepath.Join(r.rawData(), "metadata.parquet"))
	if err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}
	meta := make(map[string]*corpus.Row, len(rows))
	for i := range rows {
		meta[rows[i].ID] = &rows[i] // a repeated row: the last one wins
	}

	picks := map[string]pick{}
	stats := map[string]map[string]int{}
	allVBHN := map[string]bool{}
	for _, s := range steps {
		stats[s] = map[string]int{}
	}

	doVBHN, doTables := slices.Contains(steps, StepVBHN), slices.Contains(steps, StepTables)
	// tableItems maps a source row to the document whose tables it holds.
	tableItems := map[string]string{}
	if doVBHN || doTables {
		st := stats[StepVBHN]
		for id, row := range meta {
			if !corpus.IsVBHN(row) {
				continue
			}
			t := corpus.TierVBHN(row)
			if doVBHN {
				allVBHN[id] = true
				st["rows"]++
				st[fmt.Sprintf("rows_tier%d", t)]++
			}
			if t > corpus.Tier2 {
				continue
			}
			m := corpus.MetaOf(row, corpus.VBHNID(row))
			m.DocType = corpus.DocTypeVBHN
			if doVBHN {
				picks[id] = pick{kind: StepVBHN, meta: m, tier: t}
			}
			if doTables {
				tableItems[id] = m.ID
			}
		}
	}

	var hsTables [][][]string
	hsItem := ""
	if slices.Contains(steps, StepRescue) || slices.Contains(steps, StepHS) || doTables {
		domain, order, err := r.loadDomain()
		if err != nil {
			return err
		}
		if doTables {
			for _, id := range order {
				e := domain[id]
				if id == HSDocID || e.Tier > corpus.Tier2 || corpus.Force(&e.Meta, r.AsOf) == "" {
					continue
				}
				usable := e.Status == "parsed" || e.Status == "quarantined" && corpus.Rescued(corpus.Verdict(e.Quarantine))
				if item := corpus.ItemID(e.SourceURL); usable && item != "" {
					tableItems[item] = id
				}
			}
		}
		if slices.Contains(steps, StepRescue) {
			st := stats[StepRescue]
			for _, id := range order {
				e := domain[id]
				if e.Status != "quarantined" || e.Tier > corpus.Tier2 || !corpus.Live(&e.Meta) {
					continue
				}
				v := corpus.Verdict(e.Quarantine)
				st["quarantined_live_tier12"]++
				st["verdict_"+v]++
				if !corpus.Rescued(v) {
					continue
				}
				item := corpus.ItemID(e.SourceURL)
				if item == "" {
					st["no_source_row"]++
					continue
				}
				m := e.Meta
				m.SourceURL = corpus.SourceURL(item)
				picks[item] = pick{kind: StepRescue, meta: m, verdict: v, tier: e.Tier}
			}
		}
		if slices.Contains(steps, StepHS) {
			e := domain[HSDocID]
			if e == nil {
				return fmt.Errorf("%s is not in the store", HSDocID)
			}
			hsItem = corpus.ItemID(e.SourceURL)
		}
	}

	for _, s := range []string{StepVBHN, StepRescue, StepTables} {
		if slices.Contains(steps, s) {
			if err := resetDir(r.out(s)); err != nil {
				return err
			}
		}
	}

	r.logf("%v: streaming content for %d rows", steps, len(picks)+len(tableItems))
	err = eachContent(filepath.Join(r.rawData(), "content.parquet"), func(row contentRow) error {
		if docID, ok := tableItems[row.ID]; ok {
			delete(tableItems, row.ID)
			if err := r.writeTables(docID, row.ContentHTML, stats[StepTables]); err != nil {
				return err
			}
		}
		if row.ID == hsItem && hsTables == nil {
			for _, t := range htmltable.Parse(row.ContentHTML) {
				hsTables = append(hsTables, t.Rows)
			}
		}
		p, picked := picks[row.ID]
		if !picked && !allVBHN[row.ID] {
			return nil
		}
		text := parse.HTMLText(row.ContentHTML)
		if allVBHN[row.ID] {
			delete(allVBHN, row.ID)
			if utf8.RuneCountInString(text) < corpus.VBHNEmpty {
				stats[StepVBHN]["rows_empty"]++
			}
		}
		if !picked {
			return nil
		}
		delete(picks, row.ID) // the first content row of an item wins
		doc := document(&p.meta, text)
		stats[p.kind]["parsed_"+doc.Status]++
		if p.kind == StepVBHN && utf8.RuneCountInString(text) >= corpus.VBHNEmpty {
			stats[p.kind]["with_text"]++
		}
		return store.WriteJSON(r.out(p.kind, law.FileName(doc.ID)),
			Parsed{Doc: doc, Kind: p.kind, Verdict: p.verdict, Tier: p.tier, Text: text})
	})
	if err != nil {
		return err
	}
	for _, p := range picks {
		stats[p.kind]["no_content"]++
	}
	if doVBHN && r.Legacy {
		if err := r.fillFromLegacy(stats[StepVBHN]); err != nil {
			return fmt.Errorf("legacy: %w", err)
		}
	}

	if slices.Contains(steps, StepHS) {
		if hsTables == nil {
			return fmt.Errorf("no content for %s", HSDocID)
		}
		if err := r.writeHS(hsTables, stats[StepHS]); err != nil {
			return err
		}
	}
	for _, s := range steps {
		if err := r.summary(s, map[string]any{"as_of": r.AsOf, "revision": r.Revision, "counts": stats[s]}); err != nil {
			return err
		}
	}
	return nil
}

// document parses text with luatdo's own parser, as luatdo would have had it
// not left the row out.
func document(m *corpus.Meta, text string) *law.Document {
	doc := &law.Document{
		ID: m.ID, OfficialNumber: m.OfficialNumber, IssuingBody: m.IssuingBody, Title: m.Title,
		DocType: m.DocType, EffectiveFrom: m.EffectiveFrom, SignedOn: m.SignedOn, ExpiredOn: m.ExpiredOn,
		ForceStatus: m.ForceStatus, Source: "th1nhng0", SourceURL: m.SourceURL,
		SourceHash: store.HashBytes([]byte(text)), Status: "parsed",
	}
	doc.Provisions = parse.Fragment(m.ID, text)
	// Headings with no text under them are not structure. A page that lost its
	// auto-numbered "Điều" markers parses into chapters alone, and chunking
	// that tree would drop every word of the body.
	if !slices.ContainsFunc(doc.Provisions, func(p law.Provision) bool { return strings.TrimSpace(p.Text) != "" }) {
		doc.Provisions = nil
		doc.Status = statusTextOnly
	}
	return doc
}

// Tables is the data tables of one document, as grids of cell text.
type Tables struct {
	DocID  string       `json:"doc_id"`
	Tables [][][]string `json:"tables"`
}

func (r *run) writeTables(docID, html string, st map[string]int) error {
	t := Tables{DocID: docID}
	for _, tb := range htmltable.Parse(html) {
		if corpus.IsDataTable(tb.Rows) {
			t.Tables = append(t.Tables, tb.Rows)
		}
	}
	st["documents_read"]++
	if len(t.Tables) == 0 {
		return nil
	}
	st["documents_with_tables"]++
	st["tables"] += len(t.Tables)
	return store.WriteJSON(r.out(StepTables, law.FileName(docID)), t)
}

func (r *run) writeHS(tables [][][]string, st map[string]int) error {
	rows := corpus.HSRows(tables)
	out := newJSONL()
	eight := map[string]bool{}
	for _, row := range rows {
		if err := out.add(row); err != nil {
			return err
		}
		if corpus.IsTariffLine(row.Code) {
			st["tariff_lines"]++
			eight[row.Code] = true
		}
	}
	st["tables"] = len(tables)
	st["rows"] = len(rows)
	st["tariff_codes"] = len(eight)
	_, err := out.write(r.out("hs", "hs_31-2022.jsonl"))
	return err
}

// eachContent streams the content config row by row; it is 3.5 GB of HTML
// once decompressed and is never held whole.
func eachContent(path string, visit func(contentRow) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	rd := parquet.NewGenericReader[contentRow](f)
	defer rd.Close()
	buf := make([]contentRow, 64)
	for {
		n, err := rd.Read(buf)
		for _, row := range buf[:n] {
			if err := visit(row); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
	}
}

type legacyMeta struct {
	ID     int64  `parquet:"id"`
	Number string `parquet:"document_number"`
	Date   string `parquet:"issuance_date"`
}

type legacyContent struct {
	ID      int64  `parquet:"id"`
	Content string `parquet:"content"`
}

// fillFromLegacy gives an empty consolidated text the text the older crawl in
// the same dataset holds for it. A VBHN number repeats across years and
// ministries, so a row matches only on number and signing date together.
func (r *run) fillFromLegacy(st map[string]int) error {
	dir := filepath.Join(r.store.Raw(), "th1nhng0", r.Revision, "legacy")
	if _, err := os.Stat(filepath.Join(dir, "content.parquet")); os.IsNotExist(err) {
		r.logf("vbhn: no legacy_content under %s; empty consolidated texts stay empty", dir)
		st["legacy_missing"] = 1
		return nil
	}
	paths, err := jsonFiles(r.out(StepVBHN))
	if err != nil {
		return err
	}
	parsed, err := readAll[Parsed](paths)
	if err != nil {
		return err
	}
	empty := map[string]*Parsed{}
	for i := range parsed {
		p := &parsed[i]
		if utf8.RuneCountInString(p.Text) < corpus.VBHNEmpty {
			empty[corpus.NormNumber(p.Doc.OfficialNumber)+"|"+corpus.ISODate(p.Doc.SignedOn)] = p
		}
	}
	rows, err := parquet.ReadFile[legacyMeta](filepath.Join(dir, "metadata.parquet"))
	if err != nil {
		return fmt.Errorf("read legacy metadata (download the legacy_metadata and legacy_content configs into %s): %w", dir, err)
	}
	want := map[int64]*Parsed{}
	for _, row := range rows {
		if p, ok := empty[corpus.NormNumber(row.Number)+"|"+corpus.ISODate(row.Date)]; ok {
			want[row.ID] = p
		}
	}
	st["legacy_matched"] = len(want)
	f, err := os.Open(filepath.Join(dir, "content.parquet"))
	if err != nil {
		return err
	}
	defer f.Close()
	rd := parquet.NewGenericReader[legacyContent](f)
	defer rd.Close()
	buf := make([]legacyContent, 64)
	for {
		n, err := rd.Read(buf)
		for _, row := range buf[:n] {
			p, ok := want[row.ID]
			if !ok {
				continue
			}
			delete(want, row.ID)
			text := parse.HTMLText(row.Content)
			if utf8.RuneCountInString(text) < corpus.VBHNEmpty {
				continue
			}
			m := metaOf(p.Doc)
			doc := document(&m, text)
			st["legacy_filled_"+doc.Status]++
			if err := store.WriteJSON(r.out(StepVBHN, law.FileName(doc.ID)),
				Parsed{Doc: doc, Kind: p.Kind, Tier: p.Tier, Text: text, TextSource: "legacy_content"}); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
