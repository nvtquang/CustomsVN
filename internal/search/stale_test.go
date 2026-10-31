package search

import (
	"testing"

	"github.com/nvtquang/CustomsVN/internal/corpus"
)

// A decree, its 2019 VBHN, and a 2025 amendment that rewrites Article 21 and
// two clauses of Article 20, plus a decree amending several instruments.
func staleCorpus() *Corpus {
	docs := []Doc{
		{ID: "base", OfficialNumber: "08/2015/NĐ-CP", Title: "Nghị định quy định chi tiết Luật Hải quan", SignedOn: "2015-01-21", EffectiveFrom: "2015-03-15", ForceStatus: "Còn hiệu lực", Status: "parsed", SourceKind: corpus.SourceOriginal},
		{ID: "vbhn", OfficialNumber: "46/VBHN-BTC", Title: "Văn bản hợp nhất Nghị định 08/2015/NĐ-CP", SignedOn: "2019-09-26", SourceKind: corpus.SourceConsolidated, Consolidates: "base"},
		{ID: "am", OfficialNumber: "167/2025/NĐ-CP", Title: "Sửa đổi, bổ sung một số điều của Nghị định số 08/2015/NĐ-CP", SignedOn: "2025-06-30", EffectiveFrom: "2025-08-15", ForceStatus: "Còn hiệu lực", Status: "parsed", SourceKind: corpus.SourceOriginal},
		{ID: "multi", OfficialNumber: "05/2025/NĐ-CP", Title: "Sửa đổi Nghị định số 08/2022/NĐ-CP và Nghị định số 08/2015/NĐ-CP", SignedOn: "2025-01-06", EffectiveFrom: "2025-01-06", ForceStatus: "Còn hiệu lực", Status: "parsed", SourceKind: corpus.SourceOriginal},
	}
	chunks := []Chunk{
		{ID: "base:article-20:clause-1", DocID: "base", Text: "1. Trị giá hàng xuất khẩu cũ."},
		{ID: "base:article-20:clause-3", DocID: "base", Text: "3. Khoản không bị sửa."},
		{ID: "base:article-21", DocID: "base", Text: "Điều 21. Tỷ giá cũ."},
		{ID: "base:article-22", DocID: "base", Text: "Điều 22. Cả điều, trong đó có khoản 3 bị sửa."},
		{ID: "base:article-25", DocID: "base", Text: "Điều 25. Không bị sửa."},
		{ID: "base:article-22:clause-3", DocID: "base", Text: "3. Khoản cũ."},
		{ID: "base:article-24:clause-3", DocID: "base", Text: "3. Khoản cũ."},
		{ID: "base:article-24:clause-5", DocID: "base", Text: "5. Khoản không bị sửa."},
		{ID: "base:article-24:clause-7", DocID: "base", Text: "7. Khoản cũ."},
		{ID: "base:article-30", DocID: "base", Text: "Điều 30. Chỉ văn bản sửa nhiều nghị định nhắc tới, trong điều của nghị định khác."},
		{ID: "vbhn:article-21:clause-5", DocID: "vbhn", Text: "5. Tỷ giá cũ."},
		{ID: "vbhn:article-22", DocID: "vbhn", Text: "Điều 22. Không bị sửa."},
		{ID: "am:article-1:clause-8", DocID: "am", Path: []string{"Điều 1. Sửa đổi, bổ sung một số điều của Nghị định số 08/2015/NĐ-CP"},
			Text: "8. Sửa đổi, bổ sung khoản 1, khoản 2 Điều 20 như sau:\na) Sửa đổi, bổ sung khoản 1 như sau:\n\"1. Theo khoản 2 Điều 25 Nghị định này.\""},
		{ID: "am:article-1:clause-9~2", DocID: "am", Path: []string{"Điều 1. Sửa đổi, bổ sung một số điều của Nghị định số 08/2015/NĐ-CP"},
			Text: "9. Sửa đổi, bổ sung Điều 21 như sau:\n\"Điều 21. Tỷ giá mới.\"\n11. Sửa đổi, bổ sung các khoản 2, 3 và 7 Điều 24 như sau:"},
		{ID: "multi:article-1:clause-1", DocID: "multi", Path: []string{"Điều 1. Sửa đổi Nghị định số 08/2022/NĐ-CP"},
			Text: "1. Sửa đổi Điều 30 như sau:"},
		{ID: "multi:article-2:clause-3", DocID: "multi", Path: []string{"Điều 2. Sửa đổi các nghị định có liên quan", "Khoản 3"},
			Text: "Sửa đổi, bổ sung khoản 3 Điều 22 của Nghị định số 08/2015/NĐ-CP như sau:\n\"3. Mới.\""},
	}
	c := NewCorpus(1, docs, chunks)
	c.SetAmendments([]Amendment{{From: "am", ToDoc: "base"}, {From: "multi", ToNumber: "08/2015/NĐ-CP"}})
	return c
}

func TestSupersededByAmendment(t *testing.T) {
	c := staleCorpus()
	cases := map[string]bool{
		"base:article-20:clause-1": true,  // clause rewritten
		"base:article-20:clause-3": false, // clause untouched
		"base:article-21":          true,  // whole article rewritten
		"base:article-22":          true,  // a whole article holding a rewritten clause
		"base:article-25":          false, // cited only inside the new wording
		"base:article-22:clause-3": true,  // named in the first line of a multi-target clause
		"base:article-30":          false, // the multi-target decree changes another decree's Article 30
		"vbhn:article-21:clause-5": true,  // the 2019 VBHN predates the amendment
		"vbhn:article-22":          true,  // holds clause 3, rewritten in 2025
		"am:article-1:clause-9~2":  false, // the amendment itself is the law
		"base:article-24:clause-3": true,  // "các khoản 2, 3 và 7": the middle of a run
		"base:article-24:clause-7": true,  // and its last number
		"base:article-24:clause-5": false, // not in the run
	}
	for id, want := range cases {
		if got := c.Superseded(id, "2026-10-27"); got != want {
			t.Errorf("%s superseded %v, want %v", id, got, want)
		}
	}
	// Before the amendment took effect the old wording was the law.
	if c.Superseded("base:article-21", "2025-08-14") {
		t.Error("superseded before the amendment took effect")
	}
}

func TestExceptionsFindTheNoPenaltyClause(t *testing.T) {
	docs := []Doc{{ID: "nd", OfficialNumber: "169/2026/NĐ-CP", SourceKind: corpus.SourceOriginal}}
	chunks := []Chunk{
		{ID: "nd:article-7:clause-2", DocID: "nd", Path: []string{"Điều 7. Những trường hợp không xử phạt vi phạm hành chính", "Khoản 2"},
			Text: "a) Người nộp thuế có hành vi khai sai theo quy định tại Điều 10 Nghị định này nhưng đã khai bổ sung trong thời hạn 60 ngày."},
		{ID: "nd:article-10:clause-2", DocID: "nd", Path: []string{"Điều 10. Vi phạm về khai thuế", "Khoản 2"}, Text: "Phạt 10% số tiền thuế khai thiếu."},
		{ID: "nd:article-12", DocID: "nd", Path: []string{"Điều 12. Vi phạm về kiểm tra"}, Text: "Trừ trường hợp quy định tại Điều 10 Nghị định này, phạt tiền."},
	}
	c := NewCorpus(1, docs, chunks)
	got := c.Exceptions([]string{"nd:article-10:clause-2"})
	if len(got) != 1 || got[0] != "nd:article-7:clause-2" {
		t.Errorf("exceptions %v, want the no-penalty clause only", got)
	}
	if got := c.Exceptions([]string{"nd:article-7:clause-2"}); len(got) != 0 {
		t.Errorf("an exception has exceptions %v", got)
	}
}
