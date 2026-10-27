package evalx

import (
	"context"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/nvtquang/CustomsVN/internal/search"
)

// Ks are the cut-offs recall is reported at.
var Ks = []int{1, 3, 5, 10}

// HitBrief is one retrieved chunk as a run stores it.
type HitBrief struct {
	Rank     int     `json:"rank"`
	ChunkID  string  `json:"chunk_id"`
	Number   string  `json:"number"`
	Path     string  `json:"path"`
	Score    float64 `json:"score"`
	Usable   bool    `json:"usable"`
	Relevant bool    `json:"relevant"`
}

// Result is one question's outcome.
type Result struct {
	Item      Item            `json:"item"`
	Ranks     []int           `json:"ranks"` // per expected provision; 0 = not in the results
	Recall    map[int]float64 `json:"recall"`
	RR        float64         `json:"rr"`
	NDCG10    float64         `json:"ndcg10"`
	Expired   int             `json:"expired_in_top10"`
	LatencyMS float64         `json:"latency_ms"`
	Hits      []HitBrief      `json:"hits"`
	Error     string          `json:"error,omitempty"`
}

// Missed reports whether no expected provision reached the top 10.
func (r Result) Missed() bool { return !r.Item.ShouldRefuse && r.Recall[10] == 0 }

// Metrics are a run's totals. Questions that should be refused have no
// expected provisions and are left out of the ranking metrics; they are
// counted so the denominator is visible.
type Metrics struct {
	Questions   int                `json:"questions"`
	Scored      int                `json:"scored"`
	Refuse      int                `json:"refuse"`
	Errors      int                `json:"errors"`
	Recall      map[string]float64 `json:"recall"` // "1", "3", "5", "10"
	HitAt10     float64            `json:"hit_at_10"`
	MRR         float64            `json:"mrr"`
	NDCG10      float64            `json:"ndcg10"`
	ExpiredRate float64            `json:"expired_rate"` // expired chunks among all top-10 hits
	LatencyP50  float64            `json:"latency_p50_ms"`
	LatencyP95  float64            `json:"latency_p95_ms"`
}

// Score evaluates one response against one item.
func Score(c *search.Corpus, it Item, res *search.Response, latency time.Duration) Result {
	r := Result{Item: it, Recall: map[int]float64{}, LatencyMS: float64(latency.Microseconds()) / 1000}
	r.Ranks = make([]int, len(it.ExpectedIDs))
	credited := make([]bool, len(it.ExpectedIDs))
	dcg := 0.0
	for _, h := range res.Hits {
		if h.Rank <= 10 && !h.Usable {
			r.Expired++
		}
		rel := false
		for i, want := range it.ExpectedIDs {
			if c.Covers(h.Chunk, want) {
				rel = true
				if r.Ranks[i] == 0 {
					r.Ranks[i] = h.Rank
				}
				if !credited[i] && h.Rank <= 10 {
					credited[i] = true
					dcg += 1 / math.Log2(float64(h.Rank)+1)
				}
			}
		}
		if rel && r.RR == 0 {
			r.RR = 1 / float64(h.Rank)
		}
		r.Hits = append(r.Hits, HitBrief{Rank: h.Rank, ChunkID: h.ChunkID, Number: h.Doc.OfficialNumber,
			Path: last(h.Chunk.Path), Score: h.Score, Usable: h.Usable, Relevant: rel})
	}
	if n := len(it.ExpectedIDs); n > 0 {
		for _, k := range Ks {
			found := 0
			for _, rank := range r.Ranks {
				if rank > 0 && rank <= k {
					found++
				}
			}
			r.Recall[k] = float64(found) / float64(n)
		}
		ideal := 0.0
		for i := 1; i <= min(n, 10); i++ {
			ideal += 1 / math.Log2(float64(i)+1)
		}
		r.NDCG10 = dcg / ideal
	}
	return r
}

func last(p []string) string {
	if len(p) == 0 {
		return ""
	}
	return p[len(p)-1]
}

// Run asks every item and scores it. base carries the filters and the
// configuration under test; each item's as-of day overrides base's. progress
// is called after each item.
func Run(ctx context.Context, e *search.Engine, items []Item, base search.Request, progress func(done int, r Result)) (Metrics, []Result) {
	c := e.Corpus()
	results := make([]Result, 0, len(items))
	k := max(base.Config.K, 10)
	for i, it := range items {
		if ctx.Err() != nil {
			break
		}
		req := base
		req.Query = it.Question
		req.Config.K = k
		if it.AsOf != "" {
			req.Filters.AsOf = it.AsOf
		}
		start := time.Now()
		res, err := e.Search(ctx, req)
		var r Result
		if err != nil {
			r = Result{Item: it, Error: err.Error(), Recall: map[int]float64{}}
		} else {
			r = Score(c, it, res, time.Since(start))
		}
		results = append(results, r)
		if progress != nil {
			progress(i+1, r)
		}
	}
	return Summarize(results), results
}

// Summarize totals results.
func Summarize(results []Result) Metrics {
	m := Metrics{Questions: len(results), Recall: map[string]float64{}}
	var lat []float64
	hits10, expired := 0, 0
	for _, r := range results {
		if r.Error != "" {
			m.Errors++
			continue
		}
		lat = append(lat, r.LatencyMS)
		for _, h := range r.Hits {
			if h.Rank <= 10 {
				hits10++
			}
		}
		expired += r.Expired
		if r.Item.ShouldRefuse || len(r.Item.ExpectedIDs) == 0 {
			m.Refuse++
			continue
		}
		m.Scored++
		for _, k := range Ks {
			m.Recall[strconv.Itoa(k)] += r.Recall[k]
		}
		if r.Recall[10] > 0 {
			m.HitAt10++
		}
		m.MRR += r.RR
		m.NDCG10 += r.NDCG10
	}
	if m.Scored > 0 {
		n := float64(m.Scored)
		for k := range m.Recall {
			m.Recall[k] = round(m.Recall[k] / n)
		}
		m.HitAt10, m.MRR, m.NDCG10 = round(m.HitAt10/n), round(m.MRR/n), round(m.NDCG10/n)
	}
	if hits10 > 0 {
		m.ExpiredRate = round(float64(expired) / float64(hits10))
	}
	slices.Sort(lat)
	m.LatencyP50, m.LatencyP95 = percentile(lat, 0.5), percentile(lat, 0.95)
	return m
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	return round(sorted[max(i, 0)])
}

func round(x float64) float64 { return math.Round(x*1000) / 1000 }
