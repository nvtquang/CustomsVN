package evalx

import (
	"errors"
	"testing"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/search"
)

func TestScoreAnswers(t *testing.T) {
	c := engine().Corpus()
	art18 := luat + ":article-18"
	items := []Item{
		{ExtID: "a", ExpectedIDs: []string{art18, luat + ":article-28"}},
		{ExtID: "b", ExpectedIDs: []string{art18}},
		{ExtID: "c", ShouldRefuse: true},
		{ExtID: "d", ShouldRefuse: true},
		{ExtID: "e", ExpectedIDs: []string{art18}},
	}
	cite := func(chunk string) answer.Claim {
		return answer.Claim{Citations: []answer.Citation{{ChunkID: chunk, Verified: true}}}
	}
	price := func(u llm.Usage) float64 { return float64(u.InputTokens+u.OutputTokens) / 1e6 }
	results := []*answer.Result{
		// a: two claims, one on an expected provision and one elsewhere; one dropped.
		{Claims: []answer.Claim{cite(art18), cite(luat + ":nowhere")}, Dropped: []answer.Dropped{{Reason: "quote_not_found"}},
			TimingMS: map[string]float64{"total": 900}, FirstClaimMS: 400,
			Usage: map[string]llm.Usage{"answer": {InputTokens: 1000, OutputTokens: 200, Provider: "p", Model: "m"}}},
		// b: refused although it should have been answered.
		{Refusal: &answer.Refusal{Code: "unverified"}, TimingMS: map[string]float64{"total": 500}},
		// c: refused, rightly.
		{Refusal: &answer.Refusal{Code: "out_of_scope"}, TimingMS: map[string]float64{"total": 100}},
		// d: answered although it should have been refused.
		{Claims: []answer.Claim{cite(art18)}, TimingMS: map[string]float64{"total": 700}, FirstClaimMS: 600},
		nil,
	}
	errs := []error{nil, nil, nil, nil, errors.New("quota")}
	var scores []AnswerScore
	for i, it := range items {
		scores = append(scores, ScoreAnswer(c, it, results[i], errs[i], price))
	}
	a := scores[0]
	if a.Outcome != OutcomeAnswered || a.Citations != 2 || a.CitationsCorrect != 1 || a.ExpectedCited != 0.5 || !a.Grounded || !a.RefusalCorrect || a.CostUSD != 0.0012 {
		t.Errorf("a %+v", a)
	}
	if scores[1].RefusalCorrect || !scores[2].RefusalCorrect || scores[3].RefusalCorrect || scores[4].Outcome != OutcomeError {
		t.Errorf("refusals %+v", scores[1:])
	}
	m := SummarizeAnswers(items, scores)
	want := AnswerMetrics{Questions: 5, Answerable: 3, Refuse: 2, Answered: 2, Refused: 2, Errors: 1,
		RefusalAccuracy: 0.5, FalseRefusal: 0.333, MissedRefusal: 0.5, Supported: 0.75,
		CitationPrecision: 0.5, ExpectedCited: 0.25, Grounded: 0.5,
		FirstClaimP50: 400, FirstClaimP95: 600, TotalP50: 500, TotalP95: 900,
		InputTokens: 1000, OutputTokens: 200, CostUSD: 0.0012}
	if m != want {
		t.Errorf("metrics\n got %+v\nwant %+v", m, want)
	}
}

// An answer that cites a provision no longer law on its day is counted, per
// citation and per answer: the rule is that the count stays 0.
func TestStaleCitationsAreCounted(t *testing.T) {
	const old, now = "vn:law:2013:219-2013-tt-btc", "vn:law:2025:69-2025-tt-btc"
	c := search.NewCorpus(1, []search.Doc{
		{ID: old, OfficialNumber: "219/2013/TT-BTC", ForceStatus: "Hết hiệu lực toàn bộ", EffectiveFrom: "2014-01-01", ExpiredOn: "2025-07-01", Status: "parsed", Tier: 2, SourceKind: "original"},
		{ID: now, OfficialNumber: "69/2025/TT-BTC", ForceStatus: "Còn hiệu lực", EffectiveFrom: "2025-07-01", Status: "parsed", Tier: 2, SourceKind: "original"},
	}, []search.Chunk{
		{ID: old + ":article-12", DocID: old, Text: "cũ", Tier: 2, SourceKind: "original"},
		{ID: now + ":article-5", DocID: now, Text: "mới", Tier: 2, SourceKind: "original"},
	})
	cite := func(chunk ...string) answer.Claim {
		var cl answer.Claim
		for _, id := range chunk {
			cl.Citations = append(cl.Citations, answer.Citation{ChunkID: id, Verified: true})
		}
		return cl
	}
	items := []Item{{ExtID: "a"}, {ExtID: "b"}}
	res := []*answer.Result{
		{AsOf: "2026-10-31", Claims: []answer.Claim{cite(now+":article-5", old+":article-12"), cite(old + ":article-12")}},
		{AsOf: "2026-10-31", Claims: []answer.Claim{cite(now + ":article-5")}},
	}
	var scores []AnswerScore
	for i, it := range items {
		scores = append(scores, ScoreAnswer(c, it, res[i], nil, nil))
	}
	if scores[0].StaleCitations != 2 || scores[1].StaleCitations != 0 {
		t.Errorf("per answer %d, %d", scores[0].StaleCitations, scores[1].StaleCitations)
	}
	m := SummarizeAnswers(items, scores)
	if m.StaleCitations != 2 || m.StaleAnswers != 1 {
		t.Errorf("metrics %d citations in %d answers", m.StaleCitations, m.StaleAnswers)
	}
	// On a day the old text was law, it is not stale, and the new one, not in
	// force yet, is.
	res[0].AsOf = "2025-01-01"
	if s := ScoreAnswer(c, items[0], res[0], nil, nil); s.StaleCitations != 1 {
		t.Errorf("a day before the expiry: %d stale, want only the text not in force yet", s.StaleCitations)
	}
}
