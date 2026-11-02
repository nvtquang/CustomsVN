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

// TT 69/2025 ends TT 219/2013 and TT 193/2015 as wholes, ends only articles of
// TT 119/2014 and TT 09/2011, and mentions NĐ 209/2013 and TT 219/2013 inside
// the titles of the instruments it ends. NĐ 181/2025 ends NĐ 209/2013 in one
// line. The wording is the corpus's own (checked 2026-11-02).
func endedCorpus() *Corpus {
	docs := []Doc{
		{ID: "tt219", OfficialNumber: "219/2013/TT-BTC", ForceStatus: "Hết hiệu lực một phần", EffectiveFrom: "2014-01-01", Status: "parsed", SourceKind: corpus.SourceOriginal},
		{ID: "tt193", OfficialNumber: "193/2015/TT-BTC", ForceStatus: "Còn hiệu lực", EffectiveFrom: "2016-01-10", Status: "parsed", SourceKind: corpus.SourceOriginal},
		{ID: "tt119", OfficialNumber: "119/2014/TT-BTC", ForceStatus: "Còn hiệu lực", EffectiveFrom: "2014-10-09", Status: "parsed", SourceKind: corpus.SourceOriginal},
		{ID: "tt09", OfficialNumber: "09/2011/TT-BTC", ForceStatus: "Còn hiệu lực", EffectiveFrom: "2011-03-07", Status: "parsed", SourceKind: corpus.SourceOriginal},
		{ID: "nd209", OfficialNumber: "209/2013/NĐ-CP", ForceStatus: "Hết hiệu lực một phần", EffectiveFrom: "2014-02-01", Status: "parsed", SourceKind: corpus.SourceOriginal},
		{ID: "vbhn01", OfficialNumber: "01/VBHN-BTC", SourceKind: corpus.SourceConsolidated, Consolidates: "tt219", EffectiveFrom: "2024-01-04"},
		{ID: "tt69", OfficialNumber: "69/2025/TT-BTC", ForceStatus: "Còn hiệu lực", EffectiveFrom: "2025-07-01", Status: "parsed", SourceKind: corpus.SourceOriginal},
		{ID: "nd181", OfficialNumber: "181/2025/NĐ-CP", ForceStatus: "Còn hiệu lực", EffectiveFrom: "2025-07-01", Status: "parsed", SourceKind: corpus.SourceOriginal},
	}
	chunks := []Chunk{
		{ID: "tt69:article-10:clause-2", DocID: "tt69", Text: "Thông tư này thay thế:\n" +
			"a) Thông tư số 219/2013/TT-BTC ngày 31 tháng 12 năm 2013 của Bộ trưởng Bộ Tài chính hướng dẫn thi hành Luật Thuế giá trị gia tăng và Nghị định số 209/2013/NĐ-CP ngày 18 tháng 12 năm 2013.\n" +
			"b) Thông tư số 193/2015/TT-BTC ngày 24 tháng 11 năm 2015 sửa đổi, bổ sung Thông tư số 219/2013/TT-BTC ngày 31 tháng 12 năm 2013."},
		{ID: "tt69:article-10:clause-3", DocID: "tt69", Text: "Bãi bỏ quy định tại:\n" +
			"a) Chương 2 Thông tư số 09/2011/TT-BTC ngày 21 tháng 01 năm 2011 hướng dẫn về thuế giá trị gia tăng.\n" +
			"b) Điều 3 Thông tư số 119/2014/TT-BTC ngày 25 tháng 8 năm 2014 sửa đổi, bổ sung một số điều của Thông tư số 156/2013/TT-BTC."},
		{ID: "nd181:article-38:clause-2", DocID: "nd181", Text: "Nghị định này thay thế Nghị định số 209/2013/NĐ-CP ngày 18 tháng 12 năm 2013 của Chính phủ."},
	}
	c := NewCorpus(1, docs, chunks)
	c.SetAmendments([]Amendment{
		{From: "tt69", ToDoc: "tt219", Relation: "replaces"},
		{From: "tt69", ToDoc: "tt193", Relation: "expires"},
		{From: "tt69", ToDoc: "tt119", Relation: "expires"},
		{From: "tt69", ToDoc: "tt09", Relation: "repeals"},
		{From: "nd181", ToDoc: "nd209", Relation: "replaces"},
	})
	return c
}

func TestEndedAsAWhole(t *testing.T) {
	c := endedCorpus()
	cases := map[string]bool{
		"tt219": true,  // an item of "Thông tư này thay thế:"
		"tt193": true,  // the second item; its title also names 219/2013, which is not its number
		"nd209": true,  // one line, and named inside the title of item a) of another text
		"tt119": false, // "Điều 3 Thông tư số 119/2014": an article, not the instrument
		"tt09":  false, // "Chương 2 Thông tư số 09/2011": a chapter
	}
	for id, want := range cases {
		_, got := c.EndedBy(id, "2026-11-02")
		if got != want {
			t.Errorf("%s ended %v, want %v", id, got, want)
		}
		if f := c.Force(c.Docs[id], "2026-11-02", "2026-11-02"); (f == "") != want {
			t.Errorf("%s force %q, ended want %v", id, f, want)
		}
	}
	if e, _ := c.EndedBy("tt219", "2026-11-02"); e.By != "tt69" || e.On != "2025-07-01" {
		t.Errorf("ended by %+v", e)
	}
	// The VBHN of an ended instrument ends with it.
	if f := c.Force(c.Docs["vbhn01"], "2026-11-02", "2026-11-02"); f != "" {
		t.Errorf("VBHN of an ended instrument has force %q", f)
	}
	// Before the new text took effect the old one was law.
	if _, gone := c.EndedBy("tt219", "2025-06-30"); gone {
		t.Error("ended before the replacing text took effect")
	}
	if f := c.Force(c.Docs["tt219"], "2025-06-30", "2026-11-02"); f != "partly_expired" {
		t.Errorf("a day before the replacement: force %q", f)
	}
}

func TestEndedNeedsBothTheEdgeAndTheText(t *testing.T) {
	c := endedCorpus()
	// The text lists TT 219/2013, but vbpl.vn recorded only an amendment.
	c.SetAmendments([]Amendment{{From: "tt69", ToDoc: "tt219", Relation: "amends"}})
	if _, gone := c.EndedBy("tt219", "2026-11-02"); gone {
		t.Error("an amendment edge ended a document")
	}
	// An edge whose amending text does not list the instrument.
	c.SetAmendments([]Amendment{{From: "nd181", ToDoc: "tt219", Relation: "replaces"}})
	if _, gone := c.EndedBy("tt219", "2026-11-02"); gone {
		t.Error("an edge with no text behind it ended a document")
	}
}

// The transition article of decree 181/2025 points back at Article 30, and an
// answer on refunds for investment projects needs it.
func TestTransitionsFindTheArticleThatSaysWhatHappensToOldCases(t *testing.T) {
	docs := []Doc{{ID: "nd", OfficialNumber: "181/2025/NĐ-CP", SourceKind: corpus.SourceOriginal}}
	chunks := []Chunk{
		{ID: "nd:article-30:clause-1", DocID: "nd", Path: []string{"Điều 30. Hoàn thuế đối với dự án đầu tư", "Khoản 1"}, Text: "Cơ sở kinh doanh có dự án đầu tư được hoàn thuế."},
		{ID: "nd:article-39:clause-1", DocID: "nd", Path: []string{"Điều 39. Điều khoản chuyển tiếp", "Khoản 1"},
			Text: "Dự án đã được đầu tư trước ngày 01 tháng 7 năm 2025 thì được áp dụng quy định hoàn thuế tại Điều 30 Nghị định này."},
		{ID: "nd:article-12", DocID: "nd", Path: []string{"Điều 12. Khác"}, Text: "Theo Điều 30 Nghị định này thì được hoàn thuế."},
	}
	c := NewCorpus(1, docs, chunks)
	got := c.Transitions([]string{"nd:article-30:clause-1"})
	if len(got) != 1 || got[0] != "nd:article-39:clause-1" {
		t.Errorf("transitions %v", got)
	}
	// An article that is not named has none, and one already in hand is not repeated.
	if got := c.Transitions([]string{"nd:article-12"}); len(got) != 0 {
		t.Errorf("transitions of an unnamed article %v", got)
	}
	if got := c.Transitions([]string{"nd:article-30:clause-1", "nd:article-39:clause-1"}); len(got) != 0 {
		t.Errorf("transition already in the sources: %v", got)
	}
	// Exceptions stay apart from transitions.
	if got := c.Exceptions([]string{"nd:article-30:clause-1"}); len(got) != 0 {
		t.Errorf("a transition article taken for an exception: %v", got)
	}
}
