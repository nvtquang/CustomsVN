package answer

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/refs"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

func TestRouteRules(t *testing.T) {
	for q, want := range map[string]string{
		"Mã HS của xe ô tô điện là gì?":                          IntentHS,
		"Phân loại máy hút bụi cầm tay vào nhóm nào?":            IntentHS,
		"Thông tư 38/2015/TT-BTC còn hiệu lực không?":            IntentStatus,
		"Nghị định 08/2015/NĐ-CP đã được sửa đổi những lần nào?": IntentAmendments,
		"Hồ sơ hải quan hàng gia công gồm những gì?":             IntentProcedure,
		"Người khai hải quan có quyền gì?":                       IntentRegulation,
		// A provision named with a change word is about the provision.
		"Điều 25 Thông tư 38/2015/TT-BTC được sửa đổi thế nào?": IntentRegulation,
	} {
		if got := routeRules(q, refs.Parse(q)); got.Kind != want {
			t.Errorf("%q: got %s, want %s", q, got.Kind, want)
		}
	}
}

func withRouter(a *Answerer, replies ...string) *llm.Scripted {
	r := a.Router()
	s := &llm.Scripted{ProviderName: "cheap", Replies: replies}
	r.Set(llm.RoleRouter, llm.Target{Provider: s, Model: "small"})
	return s
}

func TestRouterModelRefusesOutOfScope(t *testing.T) {
	a, answer := answerer()
	withRouter(a, `{"intent": "out_of_scope", "clarify": ""}`)
	_, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Giá vàng hôm nay bao nhiêu?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if res.Refusal == nil || res.Refusal.Code != "out_of_scope" || len(answer.Calls) != 0 {
		t.Errorf("%+v, answer model called %d times", res.Refusal, len(answer.Calls))
	}
}

func TestRouterModelAsksBack(t *testing.T) {
	a, answer := answerer()
	withRouter(a, "```json\n"+`{"intent": "procedure", "clarify": "Hàng của bạn nhập theo loại hình kinh doanh hay gia công?"}`+"\n```")
	evs, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Thủ tục nhập khẩu nguyên liệu thế nào?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if res.Clarify == "" || res.Markdown() != res.Clarify || len(answer.Calls) != 0 {
		t.Errorf("clarify %q markdown %q calls %d", res.Clarify, res.Markdown(), len(answer.Calls))
	}
	if last := (*evs)[len(*evs)-1]; last.Type != "clarify" {
		t.Errorf("last event %s", last.Type)
	}
}

func TestRouterModelFailureKeepsTheRules(t *testing.T) {
	a, _ := answerer(`{"claim": "Có.", "citations": [{"source": 1, "quote": "Yêu cầu cơ quan hải quan xác định trước mã số"}]}`)
	s := withRouter(a)
	s.Err = &llm.Error{Provider: "cheap", Status: 500, Message: "down"}
	_, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Người khai hải quan có quyền gì?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if res.Intent.By != "rules" || len(res.Claims) != 1 {
		t.Errorf("intent %+v claims %d", res.Intent, len(res.Claims))
	}
}

func TestToolLoopAddsSources(t *testing.T) {
	a, fake := answerer(`{"claim": "Có quyền yêu cầu xác định trước mã số.", "citations": [{"source": 1, "quote": "Yêu cầu cơ quan hải quan xác định trước mã số"}]}`)
	a.Tools = &tools.Toolset{Engine: a.Engine}
	a.ToolLoop = true
	args, _ := json.Marshal(map[string]string{"reference": "Điều 18 Luật Hải quan"})
	fake.ToolTurns = [][]llm.ToolCall{{{ID: "c1", Name: tools.GetProvision, Arguments: args}}}
	evs, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Người khai hải quan được làm gì?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Steps) != 1 || res.Steps[0].Tool != tools.GetProvision || res.Steps[0].By != "model" || res.Steps[0].Error != "" {
		t.Fatalf("steps %+v", res.Steps)
	}
	if len(res.Claims) != 1 {
		t.Errorf("claims %+v dropped %+v", res.Claims, res.Dropped)
	}
	// Three model calls: the loop asking for a tool, the loop finishing, the compose.
	if len(fake.Calls) != 3 || len(fake.Calls[0].Tools) == 0 || !strings.Contains(fake.Calls[1].Messages[2].Content, "Điều 18") {
		t.Errorf("calls %d", len(fake.Calls))
	}
	hasStep := false
	for _, e := range *evs {
		hasStep = hasStep || e.Type == "step"
	}
	if !hasStep {
		t.Error("no step event")
	}
}

func TestNamedProvisionIsFetchedByThePlan(t *testing.T) {
	a, fake := answerer(`{"claim": "Có.", "citations": [{"source": 1, "quote": "Yêu cầu cơ quan hải quan xác định trước mã số"}]}`)
	a.Tools = &tools.Toolset{Engine: a.Engine}
	_, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Điểm b khoản 1 Điều 18 Luật Hải quan quy định gì?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Steps) != 1 || res.Steps[0].By != "plan" || res.Steps[0].Tool != tools.GetProvision {
		t.Errorf("steps %+v", res.Steps)
	}
	if !strings.Contains(fake.Calls[0].Messages[0].Content, "[1] 54/2014/QH13") {
		t.Error("the named provision is not source 1")
	}
}

func TestAnswersNeverLookUpHS(t *testing.T) {
	a, fake := answerer(`{"end": true}`)
	a.Tools = &tools.Toolset{Engine: a.Engine}
	a.ToolLoop = true
	args, _ := json.Marshal(map[string]string{"query": "6506.10.10"})
	fake.ToolTurns = [][]llm.ToolCall{{{ID: "c1", Name: tools.LookupHS, Arguments: args}}}
	_, emit := collect(t)
	res, err := a.Answer(context.Background(), Input{Question: "Nhập mũ bảo hiểm mã HS 6506.10.10 phải công bố hợp quy theo gì?"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	// The plan does not look the code up, the model is not offered the
	// tool, and a call it makes anyway fails without a card.
	if res.Intent.Kind != IntentHS || len(res.Steps) != 1 || res.Steps[0].By != "model" || res.Steps[0].Error == "" || len(res.Cards) != 0 {
		t.Errorf("intent %s steps %+v cards %d", res.Intent.Kind, res.Steps, len(res.Cards))
	}
	for _, tl := range fake.Calls[0].Tools {
		if tl.Name == tools.LookupHS {
			t.Error("lookup_hs offered to the answer's agent")
		}
	}
}

func TestCardsAnswerOnlyWhenTheyFoundSomething(t *testing.T) {
	empty := tools.Card{Type: "hs", Data: &tools.HSResult{}}
	line := tools.Card{Type: "hs", Data: &tools.HSResult{Lines: []tools.HSLine{{Code: "6506.10.10"}}}}
	for _, c := range []struct {
		kind  string
		cards []tools.Card
		tried bool
		want  bool
	}{
		{IntentStatus, []tools.Card{empty}, false, false},
		{IntentHS, []tools.Card{line}, false, false},
		{IntentRegulation, []tools.Card{line}, false, false},
		{IntentStatus, []tools.Card{{Type: "status"}}, false, true},
		// The composer refused or failed: the question asked more than the card.
		{IntentStatus, []tools.Card{{Type: "status"}}, true, false},
	} {
		res := &Result{Intent: Intent{Kind: c.kind}, Cards: c.cards}
		if got := cardsAnswer(res, c.tried); got != c.want {
			t.Errorf("%s %d cards tried=%v: got %v", c.kind, len(c.cards), c.tried, got)
		}
	}
}
