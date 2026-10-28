package evalx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/llmconfig"
	"github.com/nvtquang/CustomsVN/internal/search"
)

// End-to-end answer evaluation: every gold question goes through the whole
// answer (route, tools, compose, verify) and is scored on what the reader
// would have seen. Nothing here asks a model to judge: the shown claims are
// verified verbatim by construction, so faithfulness is measured as the share
// of the model's claims that survived verification, and correctness as
// whether the citations land on the provisions the experts expect.

// Outcomes of one question.
const (
	OutcomeAnswered  = "answered"  // verified claims
	OutcomeCards     = "cards"     // data cards only (status, timeline, HS)
	OutcomeRefused   = "refused"   // a refusal
	OutcomeClarified = "clarified" // a question back
	OutcomeError     = "error"
)

// AnswerConfig is what an answer run tries. Roles replace the stored ones
// for the run only; a role left out keeps its stored targets.
type AnswerConfig struct {
	Roles    map[string][]llmconfig.Target `json:"roles,omitempty"`
	ToolLoop *bool                         `json:"tool_loop,omitempty"`
}

// AnswerScore is one question's outcome.
type AnswerScore struct {
	Outcome     string `json:"outcome"`
	RefusalCode string `json:"refusal_code,omitempty"`
	// RefusalCorrect: a question that should be refused was, and one that
	// should be answered was not.
	RefusalCorrect bool `json:"refusal_correct"`
	Claims         int  `json:"claims"`
	Dropped        int  `json:"dropped"`
	Citations      int  `json:"citations"`
	// CitationsCorrect counts citations of a chunk that covers an expected
	// provision.
	CitationsCorrect int `json:"citations_correct"`
	// ExpectedCited is the share of expected provisions some citation covers.
	ExpectedCited float64 `json:"expected_cited"`
	// Grounded: answered with at least one citation of an expected provision.
	Grounded     bool    `json:"grounded"`
	FirstClaimMS float64 `json:"first_claim_ms,omitempty"`
	TotalMS      float64 `json:"total_ms"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	Error        string  `json:"error,omitempty"`
}

// ScoreAnswer scores one answer against its gold item.
func ScoreAnswer(c *search.Corpus, it Item, res *answer.Result, err error, cost func(llm.Usage) float64) AnswerScore {
	var s AnswerScore
	if res != nil {
		s.TotalMS, s.FirstClaimMS = res.TimingMS["total"], res.FirstClaimMS
		for _, u := range res.Usage {
			s.InputTokens += u.InputTokens
			s.OutputTokens += u.OutputTokens
			if cost != nil {
				s.CostUSD += cost(u)
			}
		}
		s.Claims, s.Dropped = len(res.Claims), len(res.Dropped)
	}
	switch {
	case err != nil:
		s.Outcome, s.Error = OutcomeError, err.Error()
		return s
	case res.Refusal != nil:
		s.Outcome, s.RefusalCode = OutcomeRefused, res.Refusal.Code
	case res.Clarify != "":
		s.Outcome = OutcomeClarified
	case len(res.Claims) > 0:
		s.Outcome = OutcomeAnswered
	default:
		s.Outcome = OutcomeCards
	}
	s.RefusalCorrect = (s.Outcome == OutcomeRefused) == it.ShouldRefuse
	covered := make([]bool, len(it.ExpectedIDs))
	for _, cl := range res.Claims {
		for _, ct := range cl.Citations {
			s.Citations++
			ch := c.Chunks[ct.ChunkID]
			if ch == nil {
				continue
			}
			hit := false
			for i, want := range it.ExpectedIDs {
				if c.Covers(ch, want) {
					covered[i], hit = true, true
				}
			}
			if hit {
				s.CitationsCorrect++
			}
		}
	}
	if n := len(covered); n > 0 {
		k := 0
		for _, ok := range covered {
			if ok {
				k++
			}
		}
		s.ExpectedCited = float64(k) / float64(n)
		s.Grounded = k > 0
	}
	return s
}

// AnswerMetrics are a run's totals.
type AnswerMetrics struct {
	Questions  int `json:"questions"`
	Answerable int `json:"answerable"` // should not be refused
	Refuse     int `json:"refuse"`     // should be refused
	Answered   int `json:"answered"`
	Cards      int `json:"cards"`
	Refused    int `json:"refused"`
	Clarified  int `json:"clarified"`
	Errors     int `json:"errors"`
	// RefusalAccuracy is the share of questions refused exactly when they
	// should be. FalseRefusal is the share of answerable questions refused;
	// MissedRefusal the share of should-refuse questions not refused.
	RefusalAccuracy float64 `json:"refusal_accuracy"`
	FalseRefusal    float64 `json:"false_refusal"`
	MissedRefusal   float64 `json:"missed_refusal"`
	// Supported is verified claims over all claims the model made: how
	// faithful the model is to its sources before the verifier steps in.
	Supported float64 `json:"supported"`
	// CitationPrecision is correct citations over citations, on questions
	// with expected provisions. ExpectedCited is the mean share of expected
	// provisions cited, and Grounded the share of those questions answered
	// with at least one of them cited.
	CitationPrecision float64 `json:"citation_precision"`
	ExpectedCited     float64 `json:"expected_cited"`
	Grounded          float64 `json:"grounded"`
	FirstClaimP50     float64 `json:"first_claim_p50_ms"`
	FirstClaimP95     float64 `json:"first_claim_p95_ms"`
	TotalP50          float64 `json:"total_p50_ms"`
	TotalP95          float64 `json:"total_p95_ms"`
	InputTokens       int     `json:"input_tokens"`
	OutputTokens      int     `json:"output_tokens"`
	CostUSD           float64 `json:"cost_usd"`
}

// SummarizeAnswers totals scored questions.
func SummarizeAnswers(items []Item, scores []AnswerScore) AnswerMetrics {
	var m AnswerMetrics
	var right, falseRef, missed, claims, made, cites, correct, withExpected, grounded int
	var expected float64
	var first, total []float64
	for i, s := range scores {
		it := items[i]
		m.Questions++
		m.InputTokens += s.InputTokens
		m.OutputTokens += s.OutputTokens
		m.CostUSD += s.CostUSD
		if it.ShouldRefuse {
			m.Refuse++
		} else {
			m.Answerable++
		}
		switch s.Outcome {
		case OutcomeError:
			m.Errors++
			continue
		case OutcomeAnswered:
			m.Answered++
		case OutcomeCards:
			m.Cards++
		case OutcomeRefused:
			m.Refused++
		case OutcomeClarified:
			m.Clarified++
		}
		if s.RefusalCorrect {
			right++
		}
		refused := s.Outcome == OutcomeRefused
		if refused && !it.ShouldRefuse {
			falseRef++
		}
		if !refused && it.ShouldRefuse {
			missed++
		}
		claims += s.Claims
		made += s.Claims + s.Dropped
		if !it.ShouldRefuse && len(it.ExpectedIDs) > 0 {
			withExpected++
			cites += s.Citations
			correct += s.CitationsCorrect
			expected += s.ExpectedCited
			if s.Grounded {
				grounded++
			}
		}
		if s.FirstClaimMS > 0 {
			first = append(first, s.FirstClaimMS)
		}
		total = append(total, s.TotalMS)
	}
	scored := m.Questions - m.Errors
	m.RefusalAccuracy = ratio(right, scored)
	m.FalseRefusal = ratio(falseRef, m.Answerable)
	m.MissedRefusal = ratio(missed, m.Refuse)
	m.Supported = ratio(claims, made)
	m.CitationPrecision = ratio(correct, cites)
	if withExpected > 0 {
		m.ExpectedCited = round(expected / float64(withExpected))
	}
	m.Grounded = ratio(grounded, withExpected)
	slices.Sort(first)
	slices.Sort(total)
	m.FirstClaimP50, m.FirstClaimP95 = percentile(first, 0.5), percentile(first, 0.95)
	m.TotalP50, m.TotalP95 = percentile(total, 0.5), percentile(total, 0.95)
	m.CostUSD = math.Round(m.CostUSD*1e6) / 1e6
	return m
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return round(float64(a) / float64(b))
}

// AnswerRunner runs answer evaluations in the background.
type AnswerRunner struct {
	Pool *pgxpool.Pool
	// Answerer is the template; each run gets a copy with its own router.
	Answerer *answer.Answerer
	LLM      *llmconfig.Store
	// Cost prices usage; Record stores it so the dashboard counts the run.
	Cost   func(llm.Usage) float64
	Record func(ctx context.Context, res *answer.Result) error

	mu     sync.Mutex
	cancel map[int64]context.CancelFunc
}

// AnswerRunInfo is a stored answer run.
type AnswerRunInfo struct {
	ID            int64          `json:"id"`
	Label         string         `json:"label"`
	GoldSetID     *int64         `json:"gold_set_id"`
	GoldSetName   string         `json:"gold_set_name,omitempty"`
	CorpusVersion int64          `json:"corpus_version"`
	Config        AnswerConfig   `json:"config"`
	Status        string         `json:"status"`
	Progress      int            `json:"progress"`
	Total         int            `json:"total"`
	Metrics       *AnswerMetrics `json:"metrics,omitempty"`
	Error         string         `json:"error,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
}

// Start begins a run over a stored gold set and returns its identifier. The
// stored config is the one in effect: the overrides merged into the stored
// roles, so two runs can be compared on what they actually used.
func (r *AnswerRunner) Start(ctx context.Context, goldSetID int64, label string, cfg AnswerConfig) (int64, error) {
	_, items, err := LoadGoldSet(ctx, r.Pool, goldSetID)
	if err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, fmt.Errorf("gold set %d is empty", goldSetID)
	}
	router, problems, err := r.LLM.RouterWith(ctx, cfg.Roles)
	if err != nil {
		return 0, err
	}
	if !router.Has(llm.RoleAnswer) {
		return 0, errors.Join(append([]error{errors.New("no answer model for this run")}, problems...)...)
	}
	st, err := r.LLM.Load(ctx)
	if err != nil {
		return 0, err
	}
	eff := AnswerConfig{Roles: st.Roles, ToolLoop: cfg.ToolLoop}
	for role, ts := range cfg.Roles {
		if len(ts) == 0 {
			delete(eff.Roles, role)
		} else {
			eff.Roles[role] = ts
		}
	}
	if eff.ToolLoop == nil {
		loop := r.Answerer.ToolLoop
		eff.ToolLoop = &loop
	}
	raw, _ := json.Marshal(eff)
	var id int64
	if err := r.Pool.QueryRow(ctx, `INSERT INTO answer_runs (label, gold_set_id, corpus_version, config, status, total)
		VALUES ($1, $2, $3, $4, 'running', $5) RETURNING id`, label, goldSetID, r.Answerer.Engine.Corpus().Version, raw, len(items)).Scan(&id); err != nil {
		return 0, err
	}
	a := *r.Answerer
	a.Router = func() *llm.Router { return router }
	a.ToolLoop = *eff.ToolLoop
	rctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.mu.Lock()
	if r.cancel == nil {
		r.cancel = map[int64]context.CancelFunc{}
	}
	r.cancel[id] = cancel
	r.mu.Unlock()
	go r.run(rctx, id, &a, items)
	return id, nil
}

// Cancel stops a running run; what was scored stays.
func (r *AnswerRunner) Cancel(id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.cancel[id]; ok {
		c()
		return true
	}
	return false
}

func (r *AnswerRunner) run(ctx context.Context, id int64, a *answer.Answerer, items []StoredItem) {
	defer func() {
		r.mu.Lock()
		if c, ok := r.cancel[id]; ok {
			c()
			delete(r.cancel, id)
		}
		r.mu.Unlock()
	}()
	var plain []Item
	var scores []AnswerScore
	var failure error
	for i, it := range items {
		if ctx.Err() != nil {
			break
		}
		res, err := a.Answer(ctx, answer.Input{Question: it.Question, AsOf: it.AsOf}, func(answer.Event) error { return nil })
		if ctx.Err() != nil {
			break
		}
		s := ScoreAnswer(a.Engine.Corpus(), it.Item, res, err, r.Cost)
		plain, scores = append(plain, it.Item), append(scores, s)
		if r.Record != nil && res != nil {
			_ = r.Record(ctx, res)
		}
		stored := res
		if stored == nil {
			stored = &answer.Result{Question: it.Question}
		}
		rj, _ := json.Marshal(stored)
		sj, _ := json.Marshal(s)
		if _, failure = r.Pool.Exec(ctx, `INSERT INTO answer_results (run_id, gold_item_id, result, metrics) VALUES ($1, $2, $3, $4)`,
			id, it.DBID, rj, sj); failure != nil {
			break
		}
		if _, failure = r.Pool.Exec(ctx, `UPDATE answer_runs SET progress = $2 WHERE id = $1`, id, i+1); failure != nil {
			break
		}
	}
	status, msg := "done", ""
	switch {
	case failure != nil:
		status, msg = "failed", failure.Error()
	case ctx.Err() != nil:
		status, msg = "canceled", "canceled"
	}
	mj, _ := json.Marshal(SummarizeAnswers(plain, scores))
	fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, _ = r.Pool.Exec(fctx, `UPDATE answer_runs SET status = $2, metrics = $3, error = nullif($4, ''), finished_at = now() WHERE id = $1`,
		id, status, mj, msg)
}

// MarkInterrupted fails the runs a previous process left running.
func MarkInterrupted(ctx context.Context, pool *pgxpool.Pool) error {
	for _, table := range []string{"retrieval_runs", "answer_runs"} {
		if _, err := pool.Exec(ctx, `UPDATE `+table+` SET status = 'failed', error = 'interrupted: the server stopped', finished_at = now()
			WHERE status = 'running'`); err != nil {
			return err
		}
	}
	return nil
}

const answerRunSelect = `SELECT r.id, r.label, r.gold_set_id, coalesce(g.name || ' v' || g.version, ''), coalesce(r.corpus_version, 0),
	r.config, r.status, r.progress, r.total, r.metrics, coalesce(r.error, ''), r.created_at, r.finished_at
	FROM answer_runs r LEFT JOIN gold_sets g ON g.id = r.gold_set_id`

func scanAnswerRun(row pgx.Row) (*AnswerRunInfo, error) {
	var r AnswerRunInfo
	var cfg, metrics []byte
	if err := row.Scan(&r.ID, &r.Label, &r.GoldSetID, &r.GoldSetName, &r.CorpusVersion, &cfg, &r.Status, &r.Progress, &r.Total,
		&metrics, &r.Error, &r.CreatedAt, &r.FinishedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(cfg, &r.Config)
	if len(metrics) > 0 {
		r.Metrics = &AnswerMetrics{}
		_ = json.Unmarshal(metrics, r.Metrics)
	}
	return &r, nil
}

// ListAnswerRuns lists runs, newest first.
func ListAnswerRuns(ctx context.Context, pool *pgxpool.Pool, limit int) ([]AnswerRunInfo, error) {
	rows, err := pool.Query(ctx, answerRunSelect+` ORDER BY r.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AnswerRunInfo{}
	for rows.Next() {
		r, err := scanAnswerRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// AnswerResult is one stored question result.
type AnswerResult struct {
	Item   StoredItem      `json:"item"`
	Score  AnswerScore     `json:"score"`
	Result json.RawMessage `json:"result"`
}

// GetAnswerRun returns a run and the results stored so far.
func GetAnswerRun(ctx context.Context, pool *pgxpool.Pool, id int64) (*AnswerRunInfo, []AnswerResult, error) {
	r, err := scanAnswerRun(pool.QueryRow(ctx, answerRunSelect+` WHERE r.id = $1`, id))
	if err != nil {
		return nil, nil, err
	}
	rows, err := pool.Query(ctx, `SELECT i.id, i.ext_id, i.question, i.expected_provisions, i.expected_ids, i.category, i.should_refuse,
		coalesce(i.as_of_date::text, ''), x.metrics, x.result
		FROM answer_results x JOIN gold_items i ON i.id = x.gold_item_id WHERE x.run_id = $1 ORDER BY i.id`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []AnswerResult{}
	for rows.Next() {
		var x AnswerResult
		var score []byte
		if err := rows.Scan(&x.Item.DBID, &x.Item.ExtID, &x.Item.Question, &x.Item.ExpectedProvisions, &x.Item.ExpectedIDs,
			&x.Item.Category, &x.Item.ShouldRefuse, &x.Item.AsOf, &score, &x.Result); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal(score, &x.Score)
		out = append(out, x)
	}
	return r, out, rows.Err()
}
