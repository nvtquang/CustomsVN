package evalx

import (
	"context"
	"testing"
	"time"

	"github.com/nvtquang/CustomsVN/internal/search"
)

const luat = "vn:law:2014:54-2014-qh13"

func engine() *search.Engine {
	c := search.NewCorpus(1, []search.Doc{
		{ID: luat, OfficialNumber: "54/2014/QH13", DocType: "Luật", Title: "LUẬT HẢI QUAN", EffectiveFrom: "2015-01-01",
			ForceStatus: "Còn hiệu lực", Status: "parsed", Tier: 1, SourceKind: "original"},
	}, []search.Chunk{
		{ID: luat + ":article-18", DocID: luat, Path: []string{"Điều 18. Quyền và nghĩa vụ của người khai hải quan"}, Text: "Người khai hải quan có quyền được cung cấp thông tin", Tier: 1, SourceKind: "original"},
		{ID: luat + ":article-28", DocID: luat, Path: []string{"Điều 28. Xác định trước mã số"}, Text: "Xác định trước mã số, xuất xứ, trị giá hải quan", Tier: 1, SourceKind: "original"},
	})
	e := search.NewEngine(c)
	e.Now = func() time.Time { return time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC) }
	return e
}

const csvSet = "id,question,expected_provisions,expected_answer,as_of_date,category,should_refuse,notes\n" +
	"q1,Người khai hải quan có quyền gì?,khoản 1 Điều 18 Luật Hải quan 54/2014/QH13,,2026-10-27,quyen,false,\n" +
	"q2,Xác định trước mã số hàng hóa,Điều 28 Luật Hải quan,,,thu-tuc,false,\n" +
	"q3,Thuế suất mã 8703.23.61?,,,,thue-suat,true,chưa có biểu thuế\n" +
	"q4,,Điều 99 Luật Hải quan,,27/10/2026,,false,\n"

func TestParseReportsEveryBadRow(t *testing.T) {
	items, errs, err := Parse("", []byte("\xef\xbb\xbf"+csvSet))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || !items[2].ShouldRefuse {
		t.Fatalf("items %+v", items)
	}
	if len(errs) != 2 { // q4: empty question, bad date
		t.Errorf("errors %v", errs)
	}
	rerrs := Resolve(engine().Corpus(), items, "2026-10-27")
	if len(rerrs) != 1 || rerrs[0].ID != "q4" { // Điều 99 is not in the corpus
		t.Errorf("resolve errors %v", rerrs)
	}
	if items[0].ExpectedIDs[0] != luat+":article-18:clause-1" {
		t.Errorf("ids %v", items[0].ExpectedIDs)
	}
}

func TestRunScores(t *testing.T) {
	e := engine()
	items, _, _ := Parse("csv", []byte(csvSet))
	items = items[:3]
	Resolve(e.Corpus(), items, "2026-10-27")
	m, results := Run(context.Background(), e, items, search.Request{}, nil)
	if m.Scored != 2 || m.Refuse != 1 {
		t.Errorf("metrics %+v", m)
	}
	if results[0].Ranks[0] != 1 || results[0].RR != 1 || results[0].NDCG10 != 1 {
		t.Errorf("q1 %+v", results[0])
	}
	if m.Recall["10"] != 1 || m.ExpiredRate != 0 {
		t.Errorf("metrics %+v", m)
	}
}
