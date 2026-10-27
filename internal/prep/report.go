package prep

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tamnd/luatdo/store"

	"github.com/nvtquang/CustomsVN/internal/corpus"
)

// Coverage is what the corpus does with one mandatory document.
type Coverage struct {
	corpus.Mandatory
	DocID      string `json:"doc_id,omitempty"`
	StoreTitle string `json:"store_title,omitempty"`
	Status     string `json:"status"` // ok, expired, missing, check
	Chunks     int    `json:"chunks"`
	Source     string `json:"source,omitempty"`
	Reason     string `json:"reason"`
}

// Coverage statuses. Only "missing" and "check" need someone to act.
const (
	CoverOK      = "ok"      // in the corpus
	CoverExpired = "expired" // not in force on the as-of day, rightly absent
	CoverMissing = "missing" // in force, or unknown, and not in the corpus
	CoverCheck   = "check"   // the list itself may be wrong
)

// report writes the coverage report for the mandatory list and the list of
// old documents whose force an expert has to confirm.
func (r *run) report() error {
	domain, order, err := r.loadDomain()
	if err != nil {
		return err
	}
	chunks := map[string]int{}
	sources := map[string]string{}
	err = readJSONL(r.out("customs", "chunks.jsonl"), func(c struct {
		DocID  string `json:"doc_id"`
		Source string `json:"source_kind"`
	}) error {
		chunks[c.DocID]++
		sources[c.DocID] = c.Source
		return nil
	})
	if os.IsNotExist(err) {
		return fmt.Errorf("customs/chunks.jsonl is missing: run the chunks step first")
	}
	if err != nil {
		return err
	}
	// Which kept consolidated text stands in for a base document.
	consolidatedBy := map[string]Kept{}
	if err := readJSONL(r.out("customs", "documents.jsonl"), func(k Kept) error {
		if k.Kept && k.SourceKind == corpus.SourceConsolidated && k.Base != "" {
			consolidatedBy[corpus.NormNumber(k.Base)] = k
		}
		return nil
	}); err != nil {
		return err
	}
	byNumber := map[string][]*Entry{}
	for _, id := range order {
		e := domain[id]
		byNumber[corpus.NormNumber(e.OfficialNumber)] = append(byNumber[corpus.NormNumber(e.OfficialNumber)], e)
	}

	list, err := corpus.MandatoryList()
	if err != nil {
		return err
	}
	stats := map[string]int{"listed": len(list)}
	var rows []Coverage
	for _, m := range list {
		c := r.cover(m, byNumber[corpus.NormNumber(m.Number)], chunks, sources, consolidatedBy)
		stats[c.Status]++
		rows = append(rows, c)
	}
	if err := writeCoverage(r.out("report", "coverage"), rows, r.AsOf); err != nil {
		return err
	}

	// Documents issued 1995 to 2009 that the source still calls in force and
	// that made it into the corpus. Many of them were superseded without the
	// source noticing; an expert confirms each before an answer leans on it.
	var old bytes.Buffer
	w := csv.NewWriter(&old)
	_ = w.Write([]string{"official_number", "doc_type", "issuing_body", "signed_on", "effective_from", "force_status", "tier", "chunks", "title", "source_url", "expert_verdict", "expert_note"})
	for _, id := range order {
		e := domain[id]
		year := corpus.ISODate(e.SignedOn)
		if len(year) < 4 || year[:4] < "1995" || year[:4] > "2009" || chunks[e.ID] == 0 {
			continue
		}
		stats["old_in_force"]++
		_ = w.Write([]string{e.OfficialNumber, e.DocType, e.IssuingBody, corpus.ISODate(e.SignedOn), corpus.ISODate(e.EffectiveFrom),
			e.ForceStatus, strconv.Itoa(e.Tier), strconv.Itoa(chunks[e.ID]), e.Title, e.SourceURL, "", ""})
	}
	w.Flush()
	if err := store.WriteFile(r.out("report", "old_in_force.csv"), append([]byte(utf8BOM), old.Bytes()...)); err != nil {
		return err
	}
	return r.summary(StepReport, map[string]any{"as_of": r.AsOf, "counts": stats})
}

func (r *run) cover(m corpus.Mandatory, entries []*Entry, chunks map[string]int, sources map[string]string, consolidatedBy map[string]Kept) Coverage {
	c := Coverage{Mandatory: m}
	if len(entries) == 0 {
		c.Status, c.Reason = CoverMissing, "Không có trong nguồn dữ liệu (th1nhng0 "+r.Revision[:7]+")"
		return c
	}
	// A number can belong to several documents only across bodies; prefer the
	// one whose title carries the cue.
	e := entries[0]
	for _, x := range entries {
		if m.TitleMatches(x.Title) {
			e = x
			break
		}
	}
	c.DocID, c.StoreTitle, c.Chunks, c.Source = e.ID, e.Title, chunks[e.ID], sources[e.ID]
	vbhn, hasVBHN := consolidatedBy[corpus.NormNumber(m.Number)]
	switch {
	case !m.TitleMatches(e.Title):
		c.Status, c.Reason = CoverCheck, "Tiêu đề trong nguồn không chứa \""+m.Cue+"\": kiểm tra lại số hiệu"
	case c.Chunks > 0 && hasVBHN:
		c.Status, c.Reason = CoverOK, "Có; ưu tiên văn bản hợp nhất "+vbhn.OfficialNumber
	case c.Chunks > 0:
		c.Status, c.Reason = CoverOK, "Có ("+c.Source+")"
	case hasVBHN:
		c.Status, c.Reason = CoverOK, "Có qua văn bản hợp nhất "+vbhn.OfficialNumber
	case corpus.Force(&e.Meta, r.AsOf) == "":
		c.Status = CoverExpired
		c.Reason = "Không còn hiệu lực ngày " + r.AsOf + " (" + strings.TrimSpace(e.ForceStatus)
		if e.ExpiredOn != "" {
			c.Reason += ", hết hiệu lực " + corpus.ISODate(e.ExpiredOn)
		}
		c.Reason += ")"
	case e.Status == "metadata":
		c.Status, c.Reason = CoverMissing, "Nguồn chỉ có metadata, không có toàn văn"
	case e.Status == "quarantined":
		c.Status, c.Reason = CoverMissing, "Bị luatdo cách ly và không cứu được: "+e.Quarantine
	case e.Tier > corpus.Tier2:
		c.Status, c.Reason = CoverMissing, "Còn hiệu lực nhưng bị xếp tầng 3"
	default:
		c.Status, c.Reason = CoverMissing, "Còn hiệu lực nhưng không có chunk"
	}
	return c
}

// utf8BOM lets Excel open a CSV of Vietnamese text as UTF-8.
const utf8BOM = "\xef\xbb\xbf"

var coverLabel = map[string]string{
	CoverOK: "Có", CoverExpired: "Hết hiệu lực", CoverMissing: "Thiếu", CoverCheck: "Cần kiểm tra",
}

func writeCoverage(base string, rows []Coverage, asOf string) error {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"number", "group", "note", "status", "reason", "chunks", "doc_id", "store_title"})
	for _, c := range rows {
		_ = w.Write([]string{c.Number, c.Group, c.Note, c.Status, c.Reason, strconv.Itoa(c.Chunks), c.DocID, c.StoreTitle})
	}
	w.Flush()
	if err := store.WriteFile(base+".csv", append([]byte(utf8BOM), b.Bytes()...)); err != nil {
		return err
	}

	counts := map[string]int{}
	for _, c := range rows {
		counts[c.Status]++
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# Báo cáo độ phủ danh mục văn bản bắt buộc\n\n")
	fmt.Fprintf(&md, "Sinh bởi `customsvn prep report`, hiệu lực tính tại ngày %s. Danh mục: `internal/corpus/mandatory.tsv` (bản nháp, chờ chuyên gia duyệt).\n\n", asOf)
	fmt.Fprintf(&md, "| Trạng thái | Số văn bản |\n| --- | --- |\n")
	for _, s := range []string{CoverOK, CoverExpired, CoverMissing, CoverCheck} {
		fmt.Fprintf(&md, "| %s | %d |\n", coverLabel[s], counts[s])
	}
	fmt.Fprintf(&md, "| Tổng | %d |\n\n", len(rows))
	fmt.Fprintf(&md, "| Số hiệu | Nhóm | Nội dung | Trạng thái | Chunk | Lý do |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, c := range rows {
		fmt.Fprintf(&md, "| %s | %s | %s | %s | %d | %s |\n", c.Number, c.Group, c.Note, coverLabel[c.Status], c.Chunks, strings.ReplaceAll(c.Reason, "|", "/"))
	}
	return store.WriteFile(base+".md", []byte(md.String()))
}
