package answer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/search"
)

const luat = "vn:law:2014:54-2014-qh13"

const article18 = "1. Người khai hải quan có quyền:\na) Được cơ quan hải quan cung cấp thông tin liên quan đến việc khai hải quan đối với hàng hóa, phương tiện vận tải;\nb) Yêu cầu cơ quan hải quan xác định trước mã số, xuất xứ, trị giá hải quan đối với hàng hóa khi đã cung cấp đầy đủ, chính xác thông tin cho cơ quan hải quan."

func engine() *search.Engine {
	c := search.NewCorpus(1, []search.Doc{
		{ID: luat, OfficialNumber: "54/2014/QH13", DocType: "Luật", Title: "LUẬT HẢI QUAN", EffectiveFrom: "2015-01-01",
			ForceStatus: "Còn hiệu lực", Status: "parsed", Tier: 1, SourceKind: "original"},
	}, []search.Chunk{
		{ID: luat + ":article-18", DocID: luat, Path: []string{"Điều 18. Quyền và nghĩa vụ của người khai hải quan"}, Text: article18, Tier: 1, SourceKind: "original"},
	})
	e := search.NewEngine(c)
	e.Now = func() time.Time { return time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC) }
	return e
}

func answerer(replies ...string) (*Answerer, *llm.Scripted) {
	s := &llm.Scripted{ProviderName: "fake", Replies: replies}
	r := llm.NewRouter()
	r.Set(llm.RoleAnswer, llm.Target{Provider: s, Model: "m"})
	return &Answerer{Engine: engine(), Router: func() *llm.Router { return r }}, s
}

func collect(t *testing.T) (*[]Event, func(Event) error) {
	t.Helper()
	var evs []Event
	return &evs, func(e Event) error { evs = append(evs, e); return nil }
}

func TestVerifiedClaimsAreShownAndOthersDropped(t *testing.T) {
	reply := `{"claim": "Người khai hải quan được yêu cầu xác định trước mã số hàng hóa [1].", "citations": [{"source": 1, "quote": "Yêu cầu cơ quan hải quan xác định trước mã số, xuất xứ, trị giá hải quan đối với hàng hóa"}]}
{"claim": "Người khai được miễn mọi loại phí.", "citations": [{"source": 1, "quote": "được miễn mọi loại phí hải quan"}]}
{"claim": "Không có trích dẫn.", "citations": []}
{"end": true}`
	a, fake := answerer(reply)
	evs, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Người khai hải quan có được xác định trước mã số không?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Claims) != 1 || len(res.Dropped) != 2 || res.Refusal != nil {
		t.Fatalf("claims %+v dropped %+v", res.Claims, res.Dropped)
	}
	c := res.Claims[0]
	if strings.Contains(c.Text, "[1]") || c.Citations[0].N != 1 || !c.Citations[0].Verified || c.Citations[0].ChunkID != luat+":article-18" {
		t.Errorf("claim %+v", c)
	}
	if res.Markdown() != "Người khai hải quan được yêu cầu xác định trước mã số hàng hóa. [1]" {
		t.Errorf("markdown %q", res.Markdown())
	}
	if res.Dropped[0].Reason != "quote_not_found" || res.Dropped[1].Reason != "no_citation" {
		t.Errorf("dropped %+v", res.Dropped)
	}
	// The composer saw the chunk and its force, and the claim event came
	// before the end.
	if !strings.Contains(fake.Calls[0].Messages[0].Content, "còn hiệu lực") {
		t.Error("the prompt does not state the force")
	}
	types := ""
	for _, e := range *evs {
		types += e.Type + " "
	}
	if !strings.Contains(types, "status status claim dropped dropped") {
		t.Errorf("events %s", types)
	}
}

func TestNothingVerifiedIsARefusal(t *testing.T) {
	a, _ := answerer(`{"claim": "Bịa.", "citations": [{"source": 1, "quote": "một câu không có trong văn bản nào cả"}]}`)
	_, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Quyền của người khai hải quan?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if res.Refusal == nil || res.Refusal.Code != "unverified" || len(res.Claims) != 0 {
		t.Errorf("%+v", res)
	}
}

func TestTariffRatesAreRefusedWithoutAModelCall(t *testing.T) {
	a, fake := answerer()
	_, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Thuế suất nhập khẩu của mã 8703.23.61 là bao nhiêu?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if res.Refusal == nil || res.Refusal.Code != "tariff" || len(fake.Calls) != 0 {
		t.Errorf("%+v calls %d", res.Refusal, len(fake.Calls))
	}
}

func TestFollowUpIsRewritten(t *testing.T) {
	a, fake := answerer("Người khai hải quan có quyền yêu cầu xác định trước mã số không?",
		`{"claim": "Có.", "citations": [{"source": 1, "quote": "Yêu cầu cơ quan hải quan xác định trước mã số"}]}`)
	_, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Còn mã số thì sao?",
		History: []Turn{{Role: "user", Content: "Người khai hải quan có quyền gì?"}, {Role: "assistant", Content: "..."}}}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if res.Standalone != "Người khai hải quan có quyền yêu cầu xác định trước mã số không?" || len(res.Claims) != 1 {
		t.Errorf("%+v", res)
	}
	if len(fake.Calls) != 2 || fake.Calls[0].System != DefaultPrompts().rewrite() {
		t.Errorf("calls %d", len(fake.Calls))
	}
}

func TestNoModel(t *testing.T) {
	a := &Answerer{Engine: engine(), Router: func() *llm.Router { return llm.NewRouter() }}
	_, emit := collect(t)
	if _, err := a.Answer(context.Background(), Input{Question: "x"}, emit); !IsNoModel(err) {
		t.Errorf("got %v", err)
	}
}

func TestVerbatim(t *testing.T) {
	src := "Người khai hải quan có quyền:\na) Được cơ quan hải quan  cung cấp thông tin liên quan đến việc khai hải quan;"
	for q, want := range map[string]bool{
		"Được cơ quan hải quan cung cấp thông tin":                                     true, // whitespace differs
		"“Được cơ quan hải quan cung cấp thông tin”":                                   true, // typographic quotes around
		"Người khai hải quan có quyền: ... cung cấp thông tin liên quan đến việc khai": true,
		"Được cơ quan thuế cung cấp thông tin":                                         false, // a word changed
		"cung cấp thông tin ... Người khai hải quan có quyền":                          false, // out of order
		"Người khai hải quan có quyền ... việc":                                        false, // a piece too short
		"": false,
	} {
		if got := Verbatim(src, q); got != want {
			t.Errorf("Verbatim(%q) = %v", q, got)
		}
	}
}

func TestGapsAreShownBesideClaims(t *testing.T) {
	reply := `{"claim": "Có, người khai được yêu cầu xác định trước mã số.", "citations": [{"source": 1, "quote": "Yêu cầu cơ quan hải quan xác định trước mã số, xuất xứ, trị giá hải quan đối với hàng hóa"}]}
{"gap": "Thời hạn cơ quan hải quan trả lời"}
{"gap": "Xem thêm tại https://evil.example"}
{"end": true}`
	a, _ := answerer(reply)
	evs, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Có được xác định trước mã số không, bao lâu có kết quả?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Claims) != 1 || len(res.Gaps) != 1 || res.Gaps[0] != "Thời hạn cơ quan hải quan trả lời" || len(res.Dropped) != 1 {
		t.Fatalf("claims %d gaps %q dropped %+v", len(res.Claims), res.Gaps, res.Dropped)
	}
	if !strings.HasSuffix(res.Markdown(), "Chưa có căn cứ: Thời hạn cơ quan hải quan trả lời") {
		t.Errorf("markdown %q", res.Markdown())
	}
	gap := false
	for _, e := range *evs {
		gap = gap || e.Type == "gap"
	}
	if !gap {
		t.Error("no gap event")
	}
}

func TestStoredPromptsReplaceTheDefaultsButNotTheContract(t *testing.T) {
	a, fake := answerer(`{"end": true}`)
	a.Prompts = func(context.Context) Prompts { return Prompts{Compose: "Trả lời thật ngắn."} }
	_, emit := collect(t)
	if _, err := a.Answer(context.Background(), Input{Question: "Người khai hải quan có quyền gì?"}, emit); err != nil {
		t.Fatal(err)
	}
	sys := fake.Calls[len(fake.Calls)-1].System
	if !strings.HasPrefix(sys, "Trả lời thật ngắn.") || !strings.Contains(sys, `{"end": true}`) || strings.Contains(sys, "chuyên viên tư vấn") {
		t.Errorf("compose system %q", sys)
	}
}
