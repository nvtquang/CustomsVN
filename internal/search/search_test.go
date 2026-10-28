package search

import (
	"context"
	"slices"
	"testing"
	"time"
)

const (
	luat  = "vn:law:2014:54-2014-qh13"
	cu    = "vn:law:2001:29-2001-qh10"
	tt    = "vn:law:2015:38-2015-tt-btc"
	vbhn  = "vn:vbhn:2026:25-vbhn-btc:bo-tai-chinh:1"
	asOf  = "2026-10-27"
	quyen = "Người khai hải quan có quyền được cơ quan hải quan cung cấp thông tin liên quan đến việc khai hải quan"
)

func fixture() *Corpus {
	docs := []Doc{
		{ID: luat, OfficialNumber: "54/2014/QH13", DocType: "Luật", Title: "LUẬT HẢI QUAN", EffectiveFrom: "2015-01-01",
			ForceStatus: "Hết hiệu lực một phần", Status: "parsed", Tier: 1, SourceKind: "original"},
		{ID: cu, OfficialNumber: "29/2001/QH10", DocType: "Luật", Title: "Luật Hải quan", EffectiveFrom: "2002-01-01",
			ExpiredOn: "2015-01-01", ForceStatus: "Hết hiệu lực toàn bộ", Status: "parsed", Tier: 1, SourceKind: "original"},
		{ID: tt, OfficialNumber: "38/2015/TT-BTC", DocType: "Thông tư", Title: "Thông tư quy định về thủ tục hải quan",
			EffectiveFrom: "2015-04-01", ForceStatus: "Hết hiệu lực một phần", Status: "parsed", Tier: 1, SourceKind: "original",
			ConsolidatedIn: vbhn},
		{ID: vbhn, OfficialNumber: "25/VBHN-BTC", DocType: "Văn bản hợp nhất", Title: "Văn bản hợp nhất Thông tư thủ tục hải quan",
			ForceStatus: "", Status: "parsed", Tier: 1, SourceKind: "consolidated", Consolidates: tt},
	}
	chunks := []Chunk{
		{ID: luat + ":article-18", DocID: luat, Path: []string{"Điều 18. Quyền và nghĩa vụ của người khai hải quan"}, Text: "1. " + quyen + "\na) Được cung cấp thông tin", Tier: 1, SourceKind: "original"},
		{ID: luat + ":article-4", DocID: luat, Path: []string{"Điều 4. Giải thích từ ngữ"}, Text: "Thủ tục hải quan là các công việc mà người khai hải quan và công chức hải quan phải thực hiện", Tier: 1, SourceKind: "original"},
		{ID: cu + ":article-18", DocID: cu, Path: []string{"Điều 18"}, Text: quyen + " theo luật cũ", Tier: 1, SourceKind: "original"},
		{ID: tt + ":article-25", DocID: tt, Path: []string{"Điều 25. Khai hải quan"}, Text: "Khai hải quan điện tử trên hệ thống, wording cũ về thời hạn khai", Tier: 1, SourceKind: "original"},
		{ID: vbhn + ":article-25", DocID: vbhn, Path: []string{"Điều 25. Khai hải quan"}, Text: "Khai hải quan điện tử trên hệ thống, wording đã sửa đổi về thời hạn khai", Tier: 1, SourceKind: "consolidated"},
	}
	return NewCorpus(1, docs, chunks)
}

func engine() *Engine {
	e := NewEngine(fixture())
	e.Now = func() time.Time { t, _ := time.Parse(time.DateOnly, asOf); return t }
	return e
}

func TestSearchDropsExpiredDocuments(t *testing.T) {
	res, err := engine().Search(context.Background(), Request{Query: quyen})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res.Hits {
		if h.Chunk.DocID == cu {
			t.Errorf("an expired law reached the results: %s", h.ChunkID)
		}
		if !h.Usable {
			t.Errorf("hit %s is not usable", h.ChunkID)
		}
	}
	if res.Hits[0].ChunkID != luat+":article-18" || res.ExpiredInTopK != 0 {
		t.Errorf("top %s, expired %d", res.Hits[0].ChunkID, res.ExpiredInTopK)
	}
	if len(res.Notes) == 0 {
		t.Error("want a note that hybrid ran as BM25 only")
	}
}

func TestSearchOnAnEarlierDay(t *testing.T) {
	res, err := engine().Search(context.Background(), Request{Query: quyen, Filters: Filters{AsOf: "2010-06-01"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) == 0 || res.Hits[0].Chunk.DocID != cu {
		t.Fatalf("in 2010 the 2001 law was in force: %+v", res.Hits)
	}
}

func TestSearchResolvesADirectReference(t *testing.T) {
	res, err := engine().Search(context.Background(), Request{Query: "điểm a khoản 1 Điều 18 Luật Hải quan nói gì?"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Hits[0].Direct || res.Hits[0].ChunkID != luat+":article-18" {
		t.Errorf("got %+v", res.Hits[0])
	}
	if len(res.Resolved) != 1 || res.Resolved[0].DocID != luat {
		t.Errorf("resolved %+v (the 2001 law of the same name is expired)", res.Resolved)
	}
}

func TestSearchPrefersTheConsolidatedWording(t *testing.T) {
	res, err := engine().Search(context.Background(), Request{Query: "khai hải quan điện tử thời hạn"})
	if err != nil {
		t.Fatal(err)
	}
	top := res.Hits[0]
	if top.ChunkID != vbhn+":article-25" {
		t.Fatalf("top %s", top.ChunkID)
	}
	for _, h := range res.Hits[1:] {
		if h.ChunkID == tt+":article-25" {
			t.Error("the stale original is still listed beside its consolidated wording")
		}
	}
	// The consolidated text has the force of its base.
	if top.Force != "partly_expired" {
		t.Errorf("force %q", top.Force)
	}
	off, _ := engine().Search(context.Background(), Request{Query: "khai hải quan điện tử thời hạn", Config: Config{KeepOriginals: true}, Filters: Filters{SourceKinds: []string{"original"}}})
	if off.Hits[0].ChunkID != tt+":article-25" {
		t.Errorf("originals only: top %s", off.Hits[0].ChunkID)
	}
}

func TestCovers(t *testing.T) {
	c := fixture()
	ch := c.Chunks[vbhn+":article-25"]
	for id, want := range map[string]bool{
		tt + ":article-25":          true, // consolidated wording speaks for the base
		tt + ":article-25:clause-2": true, // the article holds its clauses
		tt + ":article-2":           false,
		luat + ":article-25":        false,
	} {
		if got := c.Covers(ch, id); got != want {
			t.Errorf("Covers(%s) = %v", id, got)
		}
	}
}

func TestVectorModeNeedsAModel(t *testing.T) {
	if _, err := engine().Search(context.Background(), Request{Query: "x", Config: Config{Mode: ModeVector}}); err == nil {
		t.Error("want an error")
	}
}

func TestParentKey(t *testing.T) {
	for id, want := range map[string]string{
		luat + ":article-18":                        luat + ":article-18",
		luat + ":chapter-2:article-18:clause-2":     luat + ":chapter-2:article-18",
		luat + ":annex-1:article-3:clause-1#part-2": luat + ":annex-1:article-3",
		luat + ":table-1":                           "",
	} {
		if got := parentKey(id); got != want {
			t.Errorf("%s: got %q, want %q", id, got, want)
		}
	}
}

func TestParentContextAddsTheRestOfTheArticle(t *testing.T) {
	art := tt + ":article-30"
	c := fixture()
	chunks := []Chunk{}
	for _, ch := range c.Chunks {
		chunks = append(chunks, *ch)
	}
	chunks = append(chunks,
		Chunk{ID: art, DocID: tt, Text: "Kiểm tra sau thông quan", Tier: 1, SourceKind: "original"},
		Chunk{ID: art + ":clause-1", DocID: tt, Text: "1. Hồ sơ kiểm tra sau thông quan gồm quyết định kiểm tra", Tier: 1, SourceKind: "original"},
		Chunk{ID: art + ":clause-2", DocID: tt, Text: "2. Thời hạn kiểm tra là năm ngày làm việc", Tier: 1, SourceKind: "original"})
	docs := []Doc{}
	for _, d := range c.Docs {
		docs = append(docs, *d)
	}
	e := NewEngine(NewCorpus(1, docs, chunks))
	e.Now = engine().Now
	req := Request{Query: "hồ sơ kiểm tra sau thông quan quyết định", Config: Config{K: 1, KeepOriginals: true}}
	res, err := e.Search(context.Background(), req)
	if err != nil || len(res.Hits) != 1 || res.Hits[0].ChunkID != art+":clause-1" {
		t.Fatalf("without context: %+v %v", res, err)
	}
	req.Config.ParentContext = 6000
	res, err = e.Search(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range res.Hits {
		got = append(got, h.ChunkID+"<"+h.ContextOf)
	}
	want := []string{art + ":clause-1<", art + "<" + art + ":clause-1", art + ":clause-2<" + art + ":clause-1"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("hit %d: got %s, want %s", i, got[i], want[i])
		}
	}
	// A budget that holds only the hit adds nothing.
	req.Config.ParentContext = 60
	if res, _ = e.Search(context.Background(), req); len(res.Hits) != 1 {
		t.Errorf("small budget: %d hits", len(res.Hits))
	}
}

type fixedReranker map[string]float64

func (f fixedReranker) Rerank(_ context.Context, _ string, passages []string) ([]float64, error) {
	out := make([]float64, len(passages))
	for i, p := range passages {
		for prefix, s := range f {
			if len(p) >= len(prefix) && p[:len(prefix)] == prefix {
				out[i] = s
			}
		}
	}
	return out, nil
}

func TestMinScoreDropsLowRerankScores(t *testing.T) {
	e := engine()
	m := NewModels()
	m.AddReranker("test/r", fixedReranker{"1. " + quyen[:10]: 0.9})
	e.SetModels(m)
	req := Request{Query: "người khai hải quan", Config: Config{Rerank: true, MinScore: 0.5}}
	res, err := e.Search(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Hits) != 1 || res.Hits[0].ChunkID != luat+":article-18" || res.BelowThreshold == 0 {
		t.Errorf("hits %d, below %d", len(res.Hits), res.BelowThreshold)
	}
	if res.Config.RerankModel != "test/r" {
		t.Errorf("rerank model %q", res.Config.RerankModel)
	}
	// Fused BM25 scores have no 0-1 scale: the threshold is not applied.
	req.Config.Rerank = false
	res, _ = e.Search(context.Background(), req)
	if res.BelowThreshold != 0 || len(res.Hits) < 2 || !slices.Contains(res.Notes, NoteNoThreshold) {
		t.Errorf("without rerank: hits %d, notes %v", len(res.Hits), res.Notes)
	}
}
