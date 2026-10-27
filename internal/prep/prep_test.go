package prep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/luatdo/cite"
	"github.com/tamnd/luatdo/law"
	"github.com/tamnd/luatdo/store"
	"github.com/tamnd/luatdo/subject"
)

// fakeStore writes a luatdo data directory with three documents: a customs law
// in force, a land law that customs documents cite, and an expired circular.
func fakeStore(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	doc := func(d law.Document) {
		if err := store.WriteJSON(filepath.Join(root, "docs", law.FileName(d.ID)), d); err != nil {
			t.Fatal(err)
		}
	}
	customs := "vn:law:2014:54-2014-qh13"
	doc(law.Document{ID: customs, OfficialNumber: "54/2014/QH13", Title: "Luật Hải quan", DocType: "Luật",
		IssuingBody: "Quốc hội", EffectiveFrom: "01/01/2015", ForceStatus: "Còn hiệu lực", Status: "parsed",
		Provisions: []law.Provision{
			{ID: customs + ":article-1", Kind: "article", Number: "1", Heading: "Phạm vi điều chỉnh", Text: "Luật này quy định về quản lý nhà nước về hải quan."},
			{ID: customs + ":article-2", Kind: "article", Number: "2"},
			{ID: customs + ":article-2:clause-1", Kind: "clause", Number: "1", Text: "Cơ quan hải quan."},
		}})
	doc(law.Document{ID: "vn:law:2015:2-2015-tt-btc", OfficialNumber: "2/2015/TT-BTC", Title: "Thông tư về hải quan",
		ForceStatus: "Còn hiệu lực", ExpiredOn: "01/01/2020", Status: "parsed",
		Provisions: []law.Provision{{ID: "vn:law:2015:2-2015-tt-btc:article-1", Kind: "article", Number: "1", Text: "Cũ."}}})
	doc(law.Document{ID: "vn:law:2013:45-2013-qh13", OfficialNumber: "45/2013/QH13", Title: "Luật Đất đai",
		ForceStatus: "Còn hiệu lực", Status: "parsed",
		Provisions: []law.Provision{{ID: "vn:law:2013:45-2013-qh13:article-1", Kind: "article", Number: "1", Text: "Đất đai."}}})

	data, err := subject.Encode([]subject.Record{{DocID: customs, Subjects: []subject.Assignment{{SubjectID: "thue/hai-quan"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteFile(filepath.Join(root, "subject", subject.AssignmentsFile), data); err != nil {
		t.Fatal(err)
	}
	// The expired circular is tier 1 too, so the land law is cited twice.
	for _, from := range []string{customs, "vn:law:2015:2-2015-tt-btc"} {
		links := []cite.Link{{FromDoc: from, ToNumber: "45/2013/QH13", ToDoc: "vn:law:2013:45-2013-qh13", Kind: "cites"}}
		if err := store.WriteJSON(filepath.Join(root, "cite", law.FileName(from)), links); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestTierAndChunks(t *testing.T) {
	root := fakeStore(t)
	out := filepath.Join(root, "out")
	for _, dir := range []string{StepVBHN, StepRescue} {
		if err := os.MkdirAll(filepath.Join(out, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	opts := Options{Data: root, Out: out, AsOf: "2026-10-27"}
	if err := Run(opts, StepTier, StepChunks); err != nil {
		t.Fatal(err)
	}
	chunks, err := os.ReadFile(filepath.Join(out, "customs", "chunks.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(chunks)), "\n")
	// Two articles of the customs law and the land law, promoted to tier 2 by
	// citation. The expired circular is gone.
	if len(lines) != 3 {
		t.Fatalf("got %d chunks:\n%s", len(lines), chunks)
	}
	if strings.Contains(string(chunks), "Cũ.") {
		t.Error("an expired document reached the corpus")
	}
	if !strings.Contains(lines[0], `"tier":2`) || !strings.Contains(lines[0], "Đất đai") {
		t.Errorf("land law: %s", lines[0])
	}
	if !strings.Contains(lines[2], `"text":"1. Cơ quan hải quan."`) {
		t.Errorf("clause chunk: %s", lines[2])
	}

	// A second run over the same inputs writes the same bytes.
	if err := Run(opts, StepTier, StepChunks); err != nil {
		t.Fatal(err)
	}
	again, err := os.ReadFile(filepath.Join(out, "customs", "chunks.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(chunks) {
		t.Error("second run wrote different chunks")
	}
}

func TestRunRejectsAnUnknownStep(t *testing.T) {
	if err := Run(Options{Data: t.TempDir()}, "embed"); err == nil || !strings.Contains(err.Error(), "unknown step") {
		t.Errorf("got %v", err)
	}
}
