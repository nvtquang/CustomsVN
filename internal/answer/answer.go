// Package answer turns a question into an answer whose every claim carries
// a quote verified word for word against the chunk it cites, or a refusal.
//
// The steps: rewrite a follow-up into a standalone question, retrieve, ask the
// composer for claims as JSON Lines, and verify each claim as soon as its
// line is complete. A claim is shown only after all its quotes are found in
// the cited chunks and those chunks are in force on the as-of day; a claim
// that fails is dropped and counted. No claim left means a refusal.
package answer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/refs"
	"github.com/nvtquang/CustomsVN/internal/search"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

// Turn is an earlier exchange of the conversation.
type Turn struct {
	Role    string `json:"role"` // user or assistant
	Content string `json:"content"`
}

// Input is one question.
type Input struct {
	Question string
	History  []Turn
	AsOf     string // YYYY-MM-DD; empty means today
}

// Citation is one verified quote.
type Citation struct {
	N        int    `json:"n"` // the [n] shown to the reader, numbered per answer
	ChunkID  string `json:"chunk_id"`
	DocID    string `json:"doc_id"`
	Number   string `json:"number"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	Quote    string `json:"quote"`
	Force    string `json:"force"`
	Verified bool   `json:"verified"`
}

// Claim is one verified statement.
type Claim struct {
	Text      string     `json:"text"`
	Citations []Citation `json:"citations"`
}

// Dropped is a claim the verifier refused, and why.
type Dropped struct {
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// Refusal is an answer that declines, with ways to ask again.
type Refusal struct {
	Code        string   `json:"code"` // no_sources, out_of_scope, tariff, unverified
	Reason      string   `json:"reason"`
	Suggestions []string `json:"suggestions,omitempty"`
}

// Event is one server-sent event of an answer in progress.
type Event struct {
	Type    string      `json:"type"` // status, rewrite, intent, step, card, clarify, claim, gap, dropped, refusal
	Stage   string      `json:"stage,omitempty"`
	Text    string      `json:"text,omitempty"`
	Intent  *Intent     `json:"intent,omitempty"`
	Step    *Step       `json:"step,omitempty"`
	Card    *tools.Card `json:"card,omitempty"`
	Claim   *Claim      `json:"claim,omitempty"`
	Dropped *Dropped    `json:"dropped,omitempty"`
	Refusal *Refusal    `json:"refusal,omitempty"`
	Sources int         `json:"sources,omitempty"`
}

// Result is a finished answer.
type Result struct {
	Question   string    `json:"question"`
	Standalone string    `json:"standalone"`
	AsOf       string    `json:"as_of"`
	Claims     []Claim   `json:"claims"`
	Dropped    []Dropped `json:"dropped,omitempty"`
	// Gaps name the parts of the question the sources do not answer. They
	// state an absence, never a rule, and carry no citation.
	Gaps    []string `json:"gaps,omitempty"`
	Refusal *Refusal `json:"refusal,omitempty"`
	// Clarify is a question back to the reader when the answer depends on
	// something the question left out.
	Clarify       string               `json:"clarify,omitempty"`
	Intent        Intent               `json:"intent"`
	Steps         []Step               `json:"steps,omitempty"`
	Cards         []tools.Card         `json:"cards,omitempty"`
	Retrieved     []string             `json:"retrieved"`
	CorpusVersion int64                `json:"corpus_version"`
	Provider      string               `json:"provider,omitempty"`
	Model         string               `json:"model,omitempty"`
	Usage         map[string]llm.Usage `json:"usage,omitempty"`
	TimingMS      map[string]float64   `json:"timing_ms"`
	// FirstClaimMS is the time to the first verified claim, the moment the
	// reader first sees an answer.
	FirstClaimMS float64 `json:"first_claim_ms,omitempty"`
}

// Markdown renders the answer as it is stored and shown: each claim followed
// by its citation markers.
func (r *Result) Markdown() string {
	if r.Refusal != nil {
		return r.Refusal.Reason
	}
	if r.Clarify != "" {
		return r.Clarify
	}
	if len(r.Claims) == 0 {
		// An answer made of cards: the history keeps what they said.
		var parts []string
		for _, s := range r.Steps {
			if s.Summary != "" && s.Error == "" && s.Tool != "search_legal" && s.Tool != "get_provision" {
				parts = append(parts, s.Summary)
			}
		}
		return strings.Join(parts, "; ")
	}
	var b strings.Builder
	for i, c := range r.Claims {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(strings.TrimSpace(c.Text))
		for _, n := range markers(c) {
			fmt.Fprintf(&b, " [%d]", n)
		}
	}
	for _, g := range r.Gaps {
		b.WriteString("\n\nChưa có căn cứ: " + g)
	}
	return b.String()
}

func markers(c Claim) []int {
	var out []int
	for _, ct := range c.Citations {
		dup := false
		for _, n := range out {
			dup = dup || n == ct.N
		}
		if !dup {
			out = append(out, ct.N)
		}
	}
	return out
}

// Answerer holds what an answer needs.
type Answerer struct {
	Engine *search.Engine
	// Router returns the current router; settings can change between calls.
	Router func() *llm.Router
	// Retrieval returns the stored retrieval settings; nil means defaults.
	// The as-of day and K are set per answer.
	Retrieval func(context.Context) (search.Filters, search.Config)
	// Tools are the agent's tools; nil answers from the search alone.
	Tools *tools.Toolset
	// ToolLoop lets the answer model call tools before composing, up to
	// MaxSteps times, for questions about rules and procedures.
	ToolLoop bool
	// Prompts returns the stored system prompts; nil, or an empty prompt,
	// means the default.
	Prompts func(context.Context) Prompts
	// K is how many chunks the search hands the composer.
	K   int
	Now func() time.Time
}

// ErrNoModel is returned when no answer model is configured.
var ErrNoModel = llm.ErrNoModel

var (
	tariffAsk = regexp.MustCompile(`(?i)thuế suất|mức thuế|biểu thuế|thuế nhập khẩu (?:là )?bao nhiêu|thuế xuất khẩu (?:là )?bao nhiêu`)
	// selfMarker is an [n] the model wrote into a claim. Markers are added
	// from the verified citations only, so a written one could point at a
	// source nothing was verified against.
	selfMarker = regexp.MustCompile(`\s*\[\d+\]`)
)

// Answer answers one question, calling emit for every event. emit errors stop
// the answer (the reader went away). When the model fails after some claims
// were verified, both the partial result and the error are returned.
func (a *Answerer) Answer(ctx context.Context, in Input, emit func(Event) error) (*Result, error) {
	start := time.Now()
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	asOf := in.AsOf
	if asOf == "" {
		asOf = now().Format(time.DateOnly)
	}
	res := &Result{Question: strings.TrimSpace(in.Question), AsOf: asOf, Claims: []Claim{}, TimingMS: map[string]float64{},
		Usage: map[string]llm.Usage{}}
	lap := func(name string, t time.Time) { res.TimingMS[name] = float64(time.Since(t).Microseconds()) / 1000 }
	defer func() { res.TimingMS["total"] = float64(time.Since(start).Microseconds()) / 1000 }()
	router := a.Router()
	if router == nil || !router.Has(llm.RoleAnswer) {
		return nil, ErrNoModel
	}
	prompts := DefaultPrompts()
	if a.Prompts != nil {
		prompts = a.Prompts(ctx).Fill()
	}

	// 1. A follow-up becomes a question that stands alone.
	res.Standalone = res.Question
	if len(in.History) > 0 {
		t := time.Now()
		if err := emit(Event{Type: "status", Stage: "rewrite"}); err != nil {
			return nil, err
		}
		role := llm.RoleRewrite
		if !router.Has(role) {
			role = llm.RoleAnswer
		}
		var hist strings.Builder
		for _, h := range lastTurns(in.History, 6) {
			fmt.Fprintf(&hist, "%s: %s\n", map[string]string{"user": "Người dùng", "assistant": "Trợ lý"}[h.Role], truncate(h.Content, 1200))
		}
		resp, err := router.Stream(ctx, role, llm.Request{System: prompts.rewrite(), MaxTokens: 300,
			Messages: []llm.Message{{Role: "user", Content: "CUỘC TRÒ CHUYỆN:\n" + fence(hist.String()) + "\nCÂU HỎI CUỐI:\n<question>\n" + fence(res.Question) + "\n</question>"}}}, nil)
		if err != nil {
			return nil, err
		}
		res.Usage[llm.RoleRewrite] = resp.Usage
		if q := strings.TrimSpace(strings.Trim(resp.Text, `"`)); q != "" && len([]rune(q)) < 600 {
			res.Standalone = q
		}
		lap("rewrite", t)
		if err := emit(Event{Type: "rewrite", Text: res.Standalone}); err != nil {
			return nil, err
		}
	}

	// Deterministic refusals come before any model is asked: duty rates are
	// not in the corpus, and a rate guessed from a decree's text is a wrong
	// answer that looks right.
	found := refs.Parse(res.Standalone)
	if tariffAsk.MatchString(res.Standalone) && (len(found.HSCodes) > 0 || strings.Contains(strings.ToLower(res.Standalone), "mã")) {
		return a.refuse(res, emit, &Refusal{Code: "tariff",
			Reason: "CustomsVN chưa có dữ liệu biểu thuế xuất nhập khẩu, nên không trả lời được thuế suất của một mã hàng cụ thể.",
			Suggestions: []string{"Tra thuế suất trên Biểu thuế xuất nhập khẩu do Chính phủ ban hành hoặc cổng thông tin của Tổng cục Hải quan.",
				"Hỏi về nguyên tắc áp dụng thuế suất, ví dụ: Khi nào được áp dụng thuế suất ưu đãi đặc biệt?"}})
	}

	// 2. Route. Rules first, then the router model if one is configured.
	t := time.Now()
	intent, usage := a.route(ctx, router, prompts, res.Standalone, found)
	res.Intent = intent
	addUsage(res, llm.RoleRouter, usage)
	res.TimingMS["route"] = ms(t)
	if err := emit(Event{Type: "intent", Intent: &res.Intent}); err != nil {
		return nil, err
	}
	switch {
	case intent.Kind == IntentOutOfScope:
		return a.refuse(res, emit, &Refusal{Code: "out_of_scope",
			Reason:      "Câu hỏi nằm ngoài phạm vi pháp luật hải quan, xuất nhập khẩu và thuế liên quan mà CustomsVN tra cứu.",
			Suggestions: []string{"Hỏi về thủ tục hải quan, thuế xuất nhập khẩu, xuất xứ, trị giá, mã HS hoặc xử phạt trong lĩnh vực hải quan."}})
	case intent.Clarify != "":
		res.Clarify = intent.Clarify
		if err := emit(Event{Type: "clarify", Text: intent.Clarify}); err != nil {
			return nil, err
		}
		return res, nil
	}

	// 3. Gather sources and cards.
	hits, err := a.gather(ctx, router, prompts, res, found, emit)
	if err != nil {
		return nil, err
	}
	for _, h := range hits {
		res.Retrieved = append(res.Retrieved, h.ChunkID)
	}
	if len(hits) == 0 {
		if cardsAnswer(res, false) {
			return res, nil
		}
		return a.refuse(res, emit, &Refusal{Code: "no_sources",
			Reason:      "Không tìm thấy điều khoản nào còn hiệu lực liên quan đến câu hỏi.",
			Suggestions: []string{"Nêu rõ thủ tục, loại hàng hóa hoặc văn bản bạn quan tâm.", "Kiểm tra lại mốc ngày áp dụng."}})
	}

	// 3. Compose, verifying each claim as its line completes.
	t = time.Now()
	if err := emit(Event{Type: "status", Stage: "compose", Sources: len(hits)}); err != nil {
		return nil, err
	}
	numbers := map[string]int{} // chunk id -> [n] shown to the reader
	var line strings.Builder
	var refusal *Refusal
	handle := func(raw string) error {
		raw = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "```json"))
		if raw == "" || strings.HasPrefix(raw, "```") {
			return nil
		}
		var out struct {
			Claim     string `json:"claim"`
			Citations []struct {
				Source int    `json:"source"`
				Quote  string `json:"quote"`
			} `json:"citations"`
			Refuse      string   `json:"refuse"`
			Suggestions []string `json:"suggestions"`
			Gap         string   `json:"gap"`
			End         bool     `json:"end"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			res.Dropped = append(res.Dropped, Dropped{Text: truncate(raw, 200), Reason: "not_json"})
			return emit(Event{Type: "dropped", Dropped: &res.Dropped[len(res.Dropped)-1]})
		}
		switch {
		case out.Gap != "" && out.Claim == "":
			// A gap is the model's free text: short, with nothing to click,
			// and at most three of them.
			g := strings.TrimSpace(selfMarker.ReplaceAllString(out.Gap, ""))
			if g == "" || len([]rune(g)) > 300 || !Unlinked(g) || len(res.Gaps) == 3 {
				res.Dropped = append(res.Dropped, Dropped{Text: truncate(out.Gap, 300), Reason: "gap"})
				return emit(Event{Type: "dropped", Dropped: &res.Dropped[len(res.Dropped)-1]})
			}
			res.Gaps = append(res.Gaps, g)
			return emit(Event{Type: "gap", Text: g})
		case out.Refuse != "":
			refusal = &Refusal{Code: "out_of_scope", Reason: out.Refuse}
			if !Unlinked(out.Refuse) {
				// The reason is the model's free text, shown as it is.
				refusal.Reason = "Câu hỏi nằm ngoài phạm vi các văn bản đang có."
			}
			for _, s := range out.Suggestions {
				if Unlinked(s) {
					refusal.Suggestions = append(refusal.Suggestions, s)
				}
			}
			return nil
		case out.End || out.Claim == "":
			return nil
		}
		claim := Claim{Text: strings.TrimSpace(selfMarker.ReplaceAllString(out.Claim, ""))}
		reason := ""
		if len(out.Citations) == 0 {
			reason = "no_citation"
		}
		for _, c := range out.Citations {
			if c.Source < 1 || c.Source > len(hits) {
				reason = "unknown_source"
				break
			}
			h := hits[c.Source-1]
			if !Verbatim(h.Chunk.Text, c.Quote) {
				reason = "quote_not_found"
				break
			}
			if !search.Usable(h.Force) {
				reason = "not_in_force"
				break
			}
			n, ok := numbers[h.ChunkID]
			if !ok {
				n = len(numbers) + 1
				numbers[h.ChunkID] = n
			}
			claim.Citations = append(claim.Citations, Citation{N: n, ChunkID: h.ChunkID, DocID: h.Doc.ID, Number: h.Doc.OfficialNumber,
				Title: h.Doc.Title, Path: strings.Join(h.Chunk.Path, " › "), Quote: strings.TrimSpace(c.Quote), Force: h.Force, Verified: true})
		}
		if reason == "" {
			var cited []string
			for _, c := range out.Citations {
				cited = append(cited, hits[c.Source-1].Chunk.Text)
			}
			if !Unlinked(claim.Text, cited...) {
				reason = "link"
			}
		}
		if reason != "" {
			res.Dropped = append(res.Dropped, Dropped{Text: truncate(out.Claim, 300), Reason: reason})
			return emit(Event{Type: "dropped", Dropped: &res.Dropped[len(res.Dropped)-1]})
		}
		res.Claims = append(res.Claims, claim)
		if res.FirstClaimMS == 0 {
			res.FirstClaimMS = float64(time.Since(start).Microseconds()) / 1000
		}
		return emit(Event{Type: "claim", Claim: &res.Claims[len(res.Claims)-1]})
	}
	resp, err := router.Stream(ctx, llm.RoleAnswer, llm.Request{System: prompts.compose(), MaxTokens: 3000,
		Messages: []llm.Message{{Role: "user", Content: composeUser(res.Standalone, hits, asOf, cardsNote(res.Cards))}}},
		func(delta string) error {
			for {
				i := strings.IndexByte(delta, '\n')
				if i < 0 {
					line.WriteString(delta)
					return nil
				}
				line.WriteString(delta[:i])
				l := line.String()
				line.Reset()
				if err := handle(l); err != nil {
					return err
				}
				delta = delta[i+1:]
			}
		})
	if err != nil {
		// Claims already shown stay shown; the caller stores them with the
		// error so the reader sees where the answer stopped.
		lap("compose", t)
		return res, err
	}
	if err := handle(line.String()); err != nil {
		return nil, err
	}
	res.Provider, res.Model = resp.Provider, resp.Model
	addUsage(res, llm.RoleAnswer, resp.Usage)
	lap("compose", t)

	switch {
	case len(res.Claims) > 0:
		return res, nil
	case cardsAnswer(res, refusal != nil || len(res.Dropped) > 0):
		// The cards answer from the data; no claim was needed beside them.
		return res, nil
	case refusal != nil:
		return a.refuse(res, emit, refusal)
	}
	return a.refuse(res, emit, &Refusal{Code: "unverified",
		Reason:      "Không có nhận định nào được kiểm chứng nguyên văn với văn bản còn hiệu lực, nên CustomsVN không đưa ra câu trả lời.",
		Suggestions: []string{"Hỏi cụ thể hơn, ví dụ nêu tên thủ tục hoặc văn bản.", "Tra trực tiếp điều khoản, ví dụ: Điều 18 Luật Hải quan quy định gì?"}})
}

func (a *Answerer) refuse(res *Result, emit func(Event) error, r *Refusal) (*Result, error) {
	res.Refusal = r
	res.Claims = []Claim{}
	if err := emit(Event{Type: "refusal", Refusal: r}); err != nil {
		return nil, err
	}
	return res, nil
}

// IsNoModel reports whether err means no answer model is configured.
func IsNoModel(err error) bool { return errors.Is(err, ErrNoModel) }

func lastTurns(h []Turn, n int) []Turn {
	if len(h) > n {
		return h[len(h)-n:]
	}
	return h
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
