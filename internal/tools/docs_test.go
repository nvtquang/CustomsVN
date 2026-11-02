package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nvtquang/CustomsVN/internal/search"
)

// Two documents share the number 18/VBHN-BTC: a 2023 text on cars of
// privileged persons (consolidating TT 19/2014, partly expired) and a 2026 one
// on VAT (consolidating NĐ 181/2025, in force). vbpl.vn labels both "Chưa xác
// định".
func vbhnToolset() *Toolset {
	docs := []search.Doc{
		{ID: "vn:law:2025:181-2025-nd-cp", OfficialNumber: "181/2025/NĐ-CP", ForceStatus: "Còn hiệu lực", EffectiveFrom: "2025-07-01", Status: "parsed", Tier: 2, SourceKind: "original"},
		{ID: "vn:law:2014:19-2014-tt-btc", OfficialNumber: "19/2014/TT-BTC", ForceStatus: "Hết hiệu lực một phần", EffectiveFrom: "2014-03-01", Status: "parsed", Tier: 1, SourceKind: "original"},
		{ID: "vn:law:2013:ended-nd-cp", OfficialNumber: "9/2013/NĐ-CP", ForceStatus: "Hết hiệu lực toàn bộ", EffectiveFrom: "2013-01-01", ExpiredOn: "2020-01-01", Status: "parsed", Tier: 2, SourceKind: "original"},
		{ID: "vn:vbhn:2026:18", OfficialNumber: "18/VBHN-BTC", ForceStatus: "Chưa xác định", EffectiveFrom: "2026-06-04", Status: "parsed", Tier: 2, SourceKind: "consolidated", Consolidates: "vn:law:2025:181-2025-nd-cp"},
		{ID: "vn:vbhn:2023:18", OfficialNumber: "18/VBHN-BTC", ForceStatus: "Chưa xác định", EffectiveFrom: "2023-12-06", Status: "parsed", Tier: 1, SourceKind: "consolidated", Consolidates: "vn:law:2014:19-2014-tt-btc"},
		{ID: "vn:vbhn:2024:orphan", OfficialNumber: "05/VBHN-BTC", ForceStatus: "Chưa xác định", Status: "parsed", Tier: 2, SourceKind: "consolidated", Consolidates: "vn:law:1999:not-in-corpus"},
		{ID: "vn:vbhn:2024:dead", OfficialNumber: "06/VBHN-BTC", ForceStatus: "Chưa xác định", Status: "parsed", Tier: 2, SourceKind: "consolidated", Consolidates: "vn:law:2013:ended-nd-cp"},
	}
	e := search.NewEngine(search.NewCorpus(1, docs, nil))
	e.Now = func() time.Time { return time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC) }
	return &Toolset{Engine: e}
}

func TestConsolidatedTextTakesTheForceOfItsBase(t *testing.T) {
	ts := vbhnToolset()
	cases := []struct {
		id        string
		force     string
		noteHas   string
		noteEmpty bool
	}{
		// The label says "Chưa xác định"; the base is in force, so is the VBHN.
		{"vn:vbhn:2026:18", "in_force", "181/2025/NĐ-CP", false},
		// Partly expired base: still usable, and the card says whose force it is.
		{"vn:vbhn:2023:18", "partly_expired", "19/2014/TT-BTC", false},
		// A base the corpus lacks: no force can be stated, and the card says why.
		{"vn:vbhn:2024:orphan", "", "văn bản gốc không có trong kho", false},
		// An ended base ends its consolidation.
		{"vn:vbhn:2024:dead", "", "9/2013/NĐ-CP", false},
		// An original keeps its own label, with nothing to explain.
		{"vn:law:2025:181-2025-nd-cp", "in_force", "", true},
	}
	for _, c := range cases {
		// The registry row carries the label an original is judged by; a
		// consolidated text's own label must make no difference.
		force, note := ts.forceOf(Doc{ID: c.id, ForceStatus: "Còn hiệu lực", EffectiveFrom: "2025-07-01"}, "2026-10-31")
		if strings.Contains(c.id, "vbhn") {
			force, note = ts.forceOf(Doc{ID: c.id, ForceStatus: "Chưa xác định"}, "2026-10-31")
		}
		if force != c.force {
			t.Errorf("%s: force %q, want %q", c.id, force, c.force)
		}
		if c.noteEmpty && note != "" || !strings.Contains(note, c.noteHas) {
			t.Errorf("%s: note %q, want it to hold %q", c.id, note, c.noteHas)
		}
	}
}

func TestSameNumberNoteNamesTheOtherDocuments(t *testing.T) {
	if n := sameNumberNote(nil); n != "" {
		t.Errorf("no other document, note %q", n)
	}
	n := sameNumberNote([]Doc{{Number: "18/VBHN-BTC", SignedOn: "2023-12-06", Title: "Văn bản hợp nhất số 18/VBHN-BTC về xe ô tô"}})
	for _, want := range []string{"18/VBHN-BTC", "2023", "xe ô tô"} {
		if !strings.Contains(n, want) {
			t.Errorf("note %q lacks %q", n, want)
		}
	}
}

func TestDocHintsTravelInTheContext(t *testing.T) {
	if h := docHints(context.Background()); h != nil {
		t.Errorf("no hints set, got %v", h)
	}
	h := docHints(WithDocHints(context.Background(), []string{"a", "b"}))
	if !h["a"] || !h["b"] || h["c"] {
		t.Errorf("hints %v", h)
	}
}

func TestSharedNumberPrefersTheDocumentTheAnswerWorksFrom(t *testing.T) {
	vat := candidate{Doc: Doc{ID: "2026", SignedOn: "2026-06-04", InCorpus: true}, tier: 2, usable: true}
	cars := candidate{Doc: Doc{ID: "2023", SignedOn: "2023-12-06", InCorpus: true}, tier: 1, usable: true}
	first := func(cs ...candidate) string { rankCandidates(cs); return cs[0].ID }

	// Alone, the lower tier wins, as it does for a provincial number.
	if got := first(vat, cars); got != "2023" {
		t.Errorf("no hint: %s", got)
	}
	// The 2026 text is the one the question's sources came from.
	vat.hinted = true
	if got := first(cars, vat); got != "2026" {
		t.Errorf("hinted: %s", got)
	}
	// A hint does not make a document outside the corpus beat one inside.
	ghost := candidate{Doc: Doc{ID: "ghost"}, tier: 1, usable: true}
	vat.hinted = false
	if got := first(ghost, vat); got != "2026" {
		t.Errorf("corpus first: %s", got)
	}
	// In force beats ended, then tier, then the newer signing.
	ended := candidate{Doc: Doc{ID: "ended", SignedOn: "2027-01-01", InCorpus: true}, tier: 1}
	if got := first(ended, vat); got != "2026" {
		t.Errorf("usable first: %s", got)
	}
	older := candidate{Doc: Doc{ID: "older", SignedOn: "2020-01-01", InCorpus: true}, tier: 2, usable: true}
	if got := first(older, vat); got != "2026" {
		t.Errorf("newer first: %s", got)
	}
}

func TestForceNoteSaysWhoEndedADocument(t *testing.T) {
	docs := []search.Doc{
		{ID: "old", OfficialNumber: "219/2013/TT-BTC", ForceStatus: "Hết hiệu lực một phần", EffectiveFrom: "2014-01-01", Status: "parsed", SourceKind: "original"},
		{ID: "new", OfficialNumber: "69/2025/TT-BTC", ForceStatus: "Còn hiệu lực", EffectiveFrom: "2025-07-01", Status: "parsed", SourceKind: "original"},
		{ID: "vbhn", OfficialNumber: "01/VBHN-BTC", ForceStatus: "Chưa xác định", Status: "parsed", SourceKind: "consolidated", Consolidates: "old"},
	}
	chunks := []search.Chunk{{ID: "new:article-10:clause-2", DocID: "new", Text: "Thông tư này thay thế:\na) Thông tư số 219/2013/TT-BTC ngày 31 tháng 12 năm 2013 hướng dẫn thuế."}}
	c := search.NewCorpus(1, docs, chunks)
	c.SetAmendments([]search.Amendment{{From: "new", ToDoc: "old", Relation: "replaces"}})
	e := search.NewEngine(c)
	e.Now = func() time.Time { return time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC) }
	ts := &Toolset{Engine: e}
	for _, id := range []string{"old", "vbhn"} {
		force, note := ts.forceOf(Doc{ID: id, ForceStatus: "Hết hiệu lực một phần"}, "2026-11-02")
		if force != "" || !strings.Contains(note, "69/2025/TT-BTC") || !strings.Contains(note, "2025-07-01") {
			t.Errorf("%s: force %q, note %q", id, force, note)
		}
	}
	// A day before the new text, the old one is law and no note says otherwise.
	if force, note := ts.forceOf(Doc{ID: "old", ForceStatus: "Hết hiệu lực một phần"}, "2025-06-30"); force != "partly_expired" || note != "" {
		t.Errorf("before: force %q, note %q", force, note)
	}
}
