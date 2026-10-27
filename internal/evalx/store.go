package evalx

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/search"
)

// GoldSet is a stored gold set. Importing a set under a name that exists adds
// a version; earlier versions stay, so an old run still says what it measured.
type GoldSet struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	Format    string    `json:"format"`
	Note      string    `json:"note"`
	Items     int       `json:"items"`
	CreatedAt time.Time `json:"created_at"`
}

// SaveGoldSet stores items as the next version of name.
func SaveGoldSet(ctx context.Context, pool *pgxpool.Pool, name, format, note string, items []Item) (*GoldSet, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	g := &GoldSet{Name: name, Format: format, Note: note, Items: len(items)}
	err = tx.QueryRow(ctx, `INSERT INTO gold_sets (name, version, format, note)
		SELECT $1, coalesce(max(version), 0) + 1, $2, $3 FROM gold_sets WHERE name = $1
		RETURNING id, version, created_at`, name, format, note).Scan(&g.ID, &g.Version, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	rows := make([][]any, len(items))
	for i, it := range items {
		var asOf *time.Time
		if t, err := time.Parse(time.DateOnly, it.AsOf); err == nil {
			asOf = &t
		}
		ids := it.ExpectedIDs
		if ids == nil {
			ids = []string{}
		}
		rows[i] = []any{g.ID, it.ExtID, it.Question, it.ExpectedProvisions, ids, it.ExpectedAnswer, asOf, it.Category, it.ShouldRefuse, it.Notes}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"gold_items"},
		[]string{"gold_set_id", "ext_id", "question", "expected_provisions", "expected_ids", "expected_answer", "as_of_date", "category", "should_refuse", "notes"},
		pgx.CopyFromRows(rows)); err != nil {
		return nil, err
	}
	return g, tx.Commit(ctx)
}

// ListGoldSets lists every stored version, newest first.
func ListGoldSets(ctx context.Context, pool *pgxpool.Pool) ([]GoldSet, error) {
	rows, err := pool.Query(ctx, `SELECT g.id, g.name, g.version, g.format, g.note, g.created_at, count(i.id)
		FROM gold_sets g LEFT JOIN gold_items i ON i.gold_set_id = g.id GROUP BY g.id ORDER BY g.created_at DESC, g.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GoldSet{}
	for rows.Next() {
		var g GoldSet
		if err := rows.Scan(&g.ID, &g.Name, &g.Version, &g.Format, &g.Note, &g.CreatedAt, &g.Items); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// StoredItem is an item with its database identifier.
type StoredItem struct {
	DBID int64 `json:"db_id"`
	Item
}

// LoadGoldSet returns a set and its items in import order.
func LoadGoldSet(ctx context.Context, pool *pgxpool.Pool, id int64) (*GoldSet, []StoredItem, error) {
	g := &GoldSet{ID: id}
	if err := pool.QueryRow(ctx, `SELECT name, version, format, note, created_at FROM gold_sets WHERE id = $1`, id).
		Scan(&g.Name, &g.Version, &g.Format, &g.Note, &g.CreatedAt); err != nil {
		return nil, nil, err
	}
	rows, err := pool.Query(ctx, `SELECT id, ext_id, question, expected_provisions, expected_ids, expected_answer,
		coalesce(as_of_date::text, ''), category, should_refuse, notes FROM gold_items WHERE gold_set_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var items []StoredItem
	for rows.Next() {
		var s StoredItem
		if err := rows.Scan(&s.DBID, &s.ExtID, &s.Question, &s.ExpectedProvisions, &s.ExpectedIDs, &s.ExpectedAnswer,
			&s.AsOf, &s.Category, &s.ShouldRefuse, &s.Notes); err != nil {
			return nil, nil, err
		}
		items = append(items, s)
	}
	g.Items = len(items)
	return g, items, rows.Err()
}

// RunInfo is a stored batch run.
type RunInfo struct {
	ID            int64          `json:"id"`
	Label         string         `json:"label"`
	GoldSetID     *int64         `json:"gold_set_id"`
	GoldSetName   string         `json:"gold_set_name,omitempty"`
	CorpusVersion int64          `json:"corpus_version"`
	Request       search.Request `json:"request"`
	Status        string         `json:"status"`
	Progress      int            `json:"progress"`
	Total         int            `json:"total"`
	Metrics       *Metrics       `json:"metrics,omitempty"`
	Error         string         `json:"error,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
}

// StartRun asks every item of a stored gold set in the background and
// records each result as it comes. It returns the run's identifier at once.
func StartRun(ctx context.Context, pool *pgxpool.Pool, e *search.Engine, goldSetID int64, label string, req search.Request) (int64, error) {
	_, items, err := LoadGoldSet(ctx, pool, goldSetID)
	if err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, fmt.Errorf("gold set %d is empty", goldSetID)
	}
	cfg, _ := json.Marshal(req)
	var id int64
	if err := pool.QueryRow(ctx, `INSERT INTO retrieval_runs (label, gold_set_id, corpus_version, config, status, total)
		VALUES ($1, $2, $3, $4, 'running', $5) RETURNING id`, label, goldSetID, e.Corpus().Version, cfg, len(items)).Scan(&id); err != nil {
		return 0, err
	}
	plain := make([]Item, len(items))
	for i := range items {
		plain[i] = items[i].Item
	}
	go func() {
		var failure error
		metrics, _ := Run(ctx, e, plain, req, func(done int, r Result) {
			if failure != nil {
				return
			}
			hits, _ := json.Marshal(r.Hits)
			m, _ := json.Marshal(map[string]any{"recall": r.Recall, "rr": r.RR, "ndcg10": r.NDCG10, "expired_in_top10": r.Expired, "error": r.Error})
			ranks := r.Ranks
			if ranks == nil {
				ranks = []int{}
			}
			_, failure = pool.Exec(ctx, `INSERT INTO retrieval_results (run_id, gold_item_id, hits, ranks, metrics, latency_ms)
				VALUES ($1, $2, $3, $4, $5, $6)`, id, items[done-1].DBID, hits, ranks, m, r.LatencyMS)
			if failure == nil {
				_, failure = pool.Exec(ctx, `UPDATE retrieval_runs SET progress = $2 WHERE id = $1`, id, done)
			}
		})
		status, msg := "done", ""
		if failure != nil {
			status, msg = "failed", failure.Error()
		} else if ctx.Err() != nil {
			status, msg = "failed", "interrupted"
		}
		mj, _ := json.Marshal(metrics)
		// The server may be shutting down; the final write gets its own deadline.
		fctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(fctx, `UPDATE retrieval_runs SET status = $2, metrics = $3, error = nullif($4, ''), finished_at = now() WHERE id = $1`,
			id, status, mj, msg)
	}()
	return id, nil
}

// ListRuns lists runs, newest first.
func ListRuns(ctx context.Context, pool *pgxpool.Pool, limit int) ([]RunInfo, error) {
	rows, err := pool.Query(ctx, runSelect+` ORDER BY r.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RunInfo{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

// RunResult is one stored question result.
type RunResult struct {
	Item      StoredItem      `json:"item"`
	Ranks     []int           `json:"ranks"`
	Metrics   json.RawMessage `json:"metrics"`
	Hits      json.RawMessage `json:"hits"`
	LatencyMS float64         `json:"latency_ms"`
}

// GetRun returns a run and the results stored so far.
func GetRun(ctx context.Context, pool *pgxpool.Pool, id int64) (*RunInfo, []RunResult, error) {
	r, err := scanRun(pool.QueryRow(ctx, runSelect+` WHERE r.id = $1`, id))
	if err != nil {
		return nil, nil, err
	}
	rows, err := pool.Query(ctx, `SELECT i.id, i.ext_id, i.question, i.expected_provisions, i.expected_ids, i.category, i.should_refuse,
		coalesce(i.as_of_date::text, ''), x.ranks, x.metrics, x.hits, x.latency_ms
		FROM retrieval_results x JOIN gold_items i ON i.id = x.gold_item_id WHERE x.run_id = $1 ORDER BY i.id`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var out []RunResult
	for rows.Next() {
		var x RunResult
		if err := rows.Scan(&x.Item.DBID, &x.Item.ExtID, &x.Item.Question, &x.Item.ExpectedProvisions, &x.Item.ExpectedIDs,
			&x.Item.Category, &x.Item.ShouldRefuse, &x.Item.AsOf, &x.Ranks, &x.Metrics, &x.Hits, &x.LatencyMS); err != nil {
			return nil, nil, err
		}
		out = append(out, x)
	}
	return r, out, rows.Err()
}

const runSelect = `SELECT r.id, r.label, r.gold_set_id, coalesce(g.name || ' v' || g.version, ''), coalesce(r.corpus_version, 0),
	r.config, r.status, r.progress, r.total, r.metrics, coalesce(r.error, ''), r.created_at, r.finished_at
	FROM retrieval_runs r LEFT JOIN gold_sets g ON g.id = r.gold_set_id`

func scanRun(row pgx.Row) (*RunInfo, error) {
	var r RunInfo
	var cfg, metrics []byte
	if err := row.Scan(&r.ID, &r.Label, &r.GoldSetID, &r.GoldSetName, &r.CorpusVersion, &cfg, &r.Status, &r.Progress, &r.Total,
		&metrics, &r.Error, &r.CreatedAt, &r.FinishedAt); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(cfg, &r.Request)
	if len(metrics) > 0 {
		r.Metrics = &Metrics{}
		_ = json.Unmarshal(metrics, r.Metrics)
	}
	return &r, nil
}
