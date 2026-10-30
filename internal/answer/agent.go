package answer

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/refs"
	"github.com/nvtquang/CustomsVN/internal/search"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

// Intents a question is routed to.
const (
	IntentRegulation = "regulation" // what a rule says
	IntentProcedure  = "procedure"  // how to do something
	IntentHS         = "hs"         // classification, an HS code
	IntentStatus     = "status"     // is a document in force
	IntentAmendments = "amendments" // how a document changed over time
	IntentOutOfScope = "out_of_scope"
)

// Intent is where a question was routed and by what.
type Intent struct {
	Kind    string `json:"kind"`
	Clarify string `json:"clarify,omitempty"`
	By      string `json:"by"` // rules or model
}

// Step is one tool call of the agent, for the trace the reader can unfold.
type Step struct {
	N       int             `json:"n"`
	Tool    string          `json:"tool"`
	Args    json.RawMessage `json:"args"`
	Summary string          `json:"summary,omitempty"`
	Error   string          `json:"error,omitempty"`
	By      string          `json:"by"` // plan or model
	MS      float64         `json:"ms"`
}

// MaxSteps bounds the model's tool loop. Six calls answer a question that
// needs a status, a timeline and three provisions; past that the loop is
// circling.
const MaxSteps = 6

// maxSources is how many chunks the composer sees at most.
const maxSources = 12

// maxExceptions is how many exception provisions (see search.Exceptions) may
// join the sources.
const maxExceptions = 2

var (
	asksHS         = regexp.MustCompile(`(?i)mã hs|mã số hàng hóa|mã số hàng hoá|phân loại|áp mã|thuộc (?:nhóm|phân nhóm|chương)|mã hàng`)
	asksStatus     = regexp.MustCompile(`(?i)(?:còn|hết|có|chưa có) hiệu lực|bị thay thế|bị bãi bỏ|còn áp dụng|còn dùng được`)
	asksAmendments = regexp.MustCompile(`(?i)sửa đổi|bổ sung|lịch sử|những lần|các văn bản (?:sửa|thay)|hợp nhất`)
	asksProcedure  = regexp.MustCompile(`(?i)thủ tục|hồ sơ|trình tự|các bước|cách thức|quy trình`)
)

// routeRules routes by the words of the question. It runs always; a router
// model may override it, never the other way round when the model fails.
func routeRules(q string, found refs.Found) Intent {
	names := len(found.Numbers) > 0 || hasLawName(found)
	switch {
	case len(found.HSCodes) > 0 || asksHS.MatchString(q):
		return Intent{Kind: IntentHS, By: "rules"}
	case names && asksStatus.MatchString(q):
		return Intent{Kind: IntentStatus, By: "rules"}
	case names && asksAmendments.MatchString(q) && len(found.Provisions) == 0:
		return Intent{Kind: IntentAmendments, By: "rules"}
	case asksProcedure.MatchString(q):
		return Intent{Kind: IntentProcedure, By: "rules"}
	}
	return Intent{Kind: IntentRegulation, By: "rules"}
}

func hasLawName(found refs.Found) bool {
	for _, p := range found.Provisions {
		if p.Name != "" || p.Number != "" {
			return true
		}
	}
	return false
}

// route decides the intent: the rules first, then the router model when one
// is configured. A model that fails or answers nonsense leaves the rules'
// decision in place.
func (a *Answerer) route(ctx context.Context, router *llm.Router, p Prompts, q string, found refs.Found) (Intent, llm.Usage) {
	in := routeRules(q, found)
	if !router.Has(llm.RoleRouter) {
		return in, llm.Usage{}
	}
	resp, err := router.Stream(ctx, llm.RoleRouter, llm.Request{System: p.router(), MaxTokens: 200,
		Messages: []llm.Message{{Role: "user", Content: questionBlock(q)}}}, nil)
	if err != nil {
		return in, llm.Usage{}
	}
	var out struct {
		Intent  string `json:"intent"`
		Clarify string `json:"clarify"`
	}
	text := strings.TrimSpace(resp.Text)
	if i, j := strings.Index(text, "{"), strings.LastIndex(text, "}"); i >= 0 && j > i {
		text = text[i : j+1]
	}
	if json.Unmarshal([]byte(text), &out) != nil {
		return in, resp.Usage
	}
	switch out.Intent {
	case IntentRegulation, IntentProcedure, IntentHS, IntentStatus, IntentAmendments, IntentOutOfScope:
		// A question that names a document's status or an HS code stays where
		// the rules put it: the words are explicit and the tools are exact.
		if in.Kind == IntentRegulation || in.Kind == IntentProcedure {
			in.Kind, in.By = out.Intent, "model"
		}
	}
	if c := strings.TrimSpace(out.Clarify); c != "" && len([]rune(c)) < 300 && in.Kind != IntentOutOfScope {
		in.Clarify, in.By = c, "model"
	}
	return in, resp.Usage
}

// chatTools are the tools the answer's agent may call: all but the HS
// lookup. An answer gives the rules and the provisions behind them, not a
// classification; HS lines are for the HS screen, MCP and the API.
func chatTools() []llm.Tool {
	var out []llm.Tool
	for _, t := range tools.Specs() {
		if t.Name != tools.LookupHS {
			out = append(out, t)
		}
	}
	return out
}

// gather collects what the composer may cite and the cards the reader sees:
// the search, the tools the intent calls for, then the model's own tool loop.
func (a *Answerer) gather(ctx context.Context, router *llm.Router, p Prompts, res *Result, found refs.Found, emit func(Event) error) ([]search.Hit, error) {
	t := time.Now()
	if err := emit(Event{Type: "status", Stage: "retrieve"}); err != nil {
		return nil, err
	}
	var filters search.Filters
	var cfg search.Config
	if a.Retrieval != nil {
		filters, cfg = a.Retrieval(ctx)
	}
	// The answer's top K is the knowledge screen's (cfg.K), unless the
	// answerer fixes one.
	k := a.K
	if k <= 0 {
		k = cfg.K
	}
	if k <= 0 {
		k = 8
	}
	// An answer is never grounded on a document out of force, whatever the
	// settings screen allows for inspection.
	filters.AsOf, filters.IncludeExpired, cfg.K = res.AsOf, false, k
	sr, err := a.Engine.Search(ctx, search.Request{Query: res.Standalone, Filters: filters, Config: cfg})
	if err != nil {
		return nil, err
	}
	res.CorpusVersion = sr.CorpusVersion
	var first, rest []search.Hit // provisions asked for by name go first
	for _, h := range sr.Hits {
		if h.Usable { // the scope already ensures it; this is the second lock
			rest = append(rest, h)
		}
	}
	res.TimingMS["retrieve"] = ms(t)
	if a.Tools == nil {
		return rest, nil
	}

	call := func(by, tool string, args map[string]any) (*tools.Result, error) {
		raw, _ := json.Marshal(args)
		st := Step{N: len(res.Steps) + 1, Tool: tool, Args: raw, By: by}
		t0 := time.Now()
		var r *tools.Result
		var err error
		if tool == tools.LookupHS {
			// Not offered to the model; one that calls it anyway is told so.
			err = fmt.Errorf("%s is not available when answering", tool)
		} else {
			r, err = a.Tools.Call(ctx, tool, raw, res.AsOf)
		}
		st.MS = ms(t0)
		if err != nil {
			st.Error = err.Error()
		} else {
			st.Summary = r.Summary
		}
		res.Steps = append(res.Steps, st)
		if e := emit(Event{Type: "step", Step: &res.Steps[len(res.Steps)-1]}); e != nil {
			return nil, e
		}
		// An empty card the model asked for is a search that missed, not
		// something the reader asked to see; the trace still records it.
		if err == nil && r.Card != nil && !(by == "model" && r.Card.Empty()) {
			res.Cards = append(res.Cards, *r.Card)
			if e := emit(Event{Type: "card", Card: r.Card}); e != nil {
				return nil, e
			}
		}
		return r, err
	}

	// The plan: what the intent needs, called by code, not chosen by a model.
	t = time.Now()
	doc := ""
	if len(found.Numbers) > 0 {
		doc = found.Numbers[0]
	} else {
		for _, p := range found.Provisions {
			if p.Name != "" {
				doc = p.Name
				break
			}
		}
	}
	switch res.Intent.Kind {
	case IntentStatus:
		if doc != "" {
			if _, err := call("plan", tools.GetDocumentStatus, map[string]any{"document": doc}); err != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
	case IntentAmendments:
		if doc != "" {
			if _, err := call("plan", tools.GetAmendments, map[string]any{"document": doc}); err != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
	}
	for i, p := range found.Provisions {
		if i == 3 {
			break
		}
		if p.Number == "" && p.Name == "" {
			continue
		}
		r, err := call("plan", tools.GetProvision, map[string]any{"reference": p.Span})
		if err == nil {
			first = append(first, r.Sources...)
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	res.TimingMS["plan"] = ms(t)

	// The model's loop, for questions that may need more than one search. A
	// question about goods or an HS code is answered like a rule: which
	// licences, inspections and limits apply to those goods.
	var fetched, extra []search.Hit
	if a.ToolLoop && (res.Intent.Kind == IntentRegulation || res.Intent.Kind == IntentProcedure || res.Intent.Kind == IntentHS) {
		t = time.Now()
		if err := emit(Event{Type: "status", Stage: "agent"}); err != nil {
			return nil, err
		}
		msgs := []llm.Message{{Role: "user", Content: questionBlock(res.Standalone) + "\nNGÀY ÁP DỤNG: " + res.AsOf +
			"\n\nĐÃ CÓ CÁC ĐIỀU KHOẢN SAU (từ tìm kiếm ban đầu):\n" + listHits(append(first, rest...))}}
		// The loop only gathers sources, so it can run on a faster model
		// than the one that writes the answer.
		agentRole := llm.RoleAgent
		if !router.Has(agentRole) {
			agentRole = llm.RoleAnswer
		}
		for turn := 0; turn < MaxSteps; turn++ {
			resp, err := router.Stream(ctx, agentRole, llm.Request{System: p.agent(), Messages: msgs, Tools: chatTools(), MaxTokens: 800}, nil)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				// A provider that cannot call tools, or fails here, leaves
				// the answer with what the plan gathered.
				res.Steps = append(res.Steps, Step{N: len(res.Steps) + 1, Tool: "agent", By: "model", Error: err.Error()})
				break
			}
			addUsage(res, "agent", resp.Usage)
			if len(resp.ToolCalls) == 0 {
				break
			}
			msgs = append(msgs, llm.Message{Role: "assistant", Content: resp.Text, ToolCalls: resp.ToolCalls})
			for _, tc := range resp.ToolCalls {
				var args map[string]any
				_ = json.Unmarshal(tc.Arguments, &args)
				r, err := call("model", tc.Name, args)
				content := ""
				switch {
				case err != nil && ctx.Err() != nil:
					return nil, ctx.Err()
				case err != nil:
					content = "Lỗi: " + err.Error()
				case tc.Name == tools.GetProvision:
					content = r.Text
					fetched = append(fetched, r.Sources...)
				default:
					content = r.Text
					extra = append(extra, r.Sources...)
				}
				msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: tc.ID, Content: truncate(content, 6000)})
			}
			if len(res.Steps) >= MaxSteps+4 {
				break
			}
		}
		res.TimingMS["agent"] = ms(t)
	}

	out := pickSources(first, fetched, rest, extra)
	// The provisions that carve exceptions out of what was found go in ahead
	// of the searches, at most two, so a penalty is never quoted without its
	// "no penalty when".
	ids := make([]string, len(out))
	for i, h := range out {
		ids[i] = h.ChunkID
	}
	var exc []search.Hit
	for _, id := range a.Engine.Corpus().Exceptions(ids) {
		if h := a.Engine.HitFor(id, res.AsOf); h != nil && h.Usable {
			exc = append(exc, *h)
			if len(exc) == maxExceptions {
				break
			}
		}
	}
	if len(exc) > 0 {
		out = pickSources(first, append(append([]search.Hit(nil), fetched...), exc...), rest, extra)
	}
	return out, nil
}

// pickSources orders what the composer sees, each chunk once and at most
// maxSources: provisions named in the question, then provisions the model
// fetched by reference, then the first search, then the model's own
// searches. A provision the model asked for by name is its most deliberate
// find; behind the first search it fell past the cap, and the composer
// refused for want of the very provision the trace shows was fetched.
func pickSources(named, fetched, search1, searched []search.Hit) []search.Hit {
	var out []search.Hit
	seen := map[string]bool{}
	for _, list := range [][]search.Hit{named, fetched, search1, searched} {
		for _, h := range list {
			if seen[h.ChunkID] || !h.Usable {
				continue
			}
			seen[h.ChunkID] = true
			out = append(out, h)
			if len(out) == maxSources {
				return out
			}
		}
	}
	return out
}

// listHits is the short list the agent model starts from.
func listHits(hits []search.Hit) string {
	var b strings.Builder
	for _, h := range hits {
		fmt.Fprintf(&b, "- %s | %s\n", fence(h.Doc.OfficialNumber), fence(strings.Join(h.Chunk.Path, " › ")))
	}
	if b.Len() == 0 {
		return "(chưa có)\n"
	}
	return b.String()
}

// cardsNote tells the composer what the reader already sees as cards, so it
// does not restate a status or a timeline in words it cannot cite.
func cardsNote(cards []tools.Card) string {
	if len(cards) == 0 {
		return ""
	}
	kinds := map[string]string{"status": "trạng thái hiệu lực", "amendments": "chuỗi sửa đổi", "hs": "kết quả tra mã HS", "graph": "quan hệ văn bản"}
	var parts []string
	for _, c := range cards {
		parts = append(parts, kinds[c.Type])
	}
	return "Người dùng đã thấy " + strings.Join(parts, ", ") + " dưới dạng thẻ lấy từ cơ sở dữ liệu. Không nhắc lại các dữ kiện trong thẻ, nhưng vẫn phải trả lời mọi phần khác của câu hỏi bằng nhận định có trích dẫn từ NGUỒN; phần nào NGUỒN không có thì ghi một dòng gap."
}

// cardsAnswer reports whether the cards alone answer the question: a lookup
// of a status or a timeline that found something, and a composer that
// neither refused nor wrote a claim that failed verification. When it did,
// the question asked more than the cards say, and the reader is told so
// instead of being shown the cards as if they were the whole answer. Gaps are
// shown either way.
func cardsAnswer(res *Result, composerTried bool) bool {
	switch res.Intent.Kind {
	case IntentStatus, IntentAmendments:
	default:
		return false
	}
	if composerTried {
		return false
	}
	for _, c := range res.Cards {
		if !c.Empty() {
			return true
		}
	}
	return false
}

func addUsage(res *Result, purpose string, u llm.Usage) {
	cur := res.Usage[purpose]
	cur.InputTokens += u.InputTokens
	cur.OutputTokens += u.OutputTokens
	if u.Provider != "" {
		cur.Provider, cur.Model = u.Provider, u.Model
	}
	res.Usage[purpose] = cur
}

func ms(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }
