package httpapi

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/chat"
	"github.com/nvtquang/CustomsVN/internal/evalx"
	"github.com/nvtquang/CustomsVN/internal/jobs"
	"github.com/nvtquang/CustomsVN/internal/llmconfig"
)

// dayZone is where a day starts for the dashboard: the readers are in
// Vietnam, and a question asked at 6 a.m. there belongs to that day, not to
// the previous one in UTC.
const dayZone = "Asia/Ho_Chi_Minh"

// utf8BOM starts the CSV reports prep writes, for Excel.
const utf8BOM = "\xef\xbb\xbf"

// --- Data ---

type corpusVersion struct {
	ID             int64     `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	AsOf           string    `json:"as_of"`
	SourceRevision string    `json:"source_revision"`
	Documents      int       `json:"documents"`
	Chunks         int       `json:"chunks"`
	Added          int       `json:"added"`
	Changed        int       `json:"changed"`
	Removed        int       `json:"removed"`
}

type countRow struct {
	Key       string `json:"key"`
	Documents int    `json:"documents"`
	Chunks    int    `json:"chunks"`
}

func (s *Server) adminCorpus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{}
	rows, err := s.Pool.Query(ctx, `SELECT id, created_at, as_of::text, source_revision, documents, chunks, added, changed, removed
		FROM corpus_versions ORDER BY id DESC LIMIT 30`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	versions := []corpusVersion{}
	for rows.Next() {
		var v corpusVersion
		if err := rows.Scan(&v.ID, &v.CreatedAt, &v.AsOf, &v.SourceRevision, &v.Documents, &v.Chunks, &v.Added, &v.Changed, &v.Removed); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		versions = append(versions, v)
	}
	rows.Close()
	out["versions"] = versions

	// What is in memory now, counted the way answers see it.
	c := s.Engine.Corpus()
	today := s.Engine.Now().Format(time.DateOnly)
	tiers, types, kinds, force := map[string]*countRow{}, map[string]*countRow{}, map[string]*countRow{}, map[string]*countRow{}
	add := func(m map[string]*countRow, key string, docs, chunks int) {
		if m[key] == nil {
			m[key] = &countRow{Key: key}
		}
		m[key].Documents += docs
		m[key].Chunks += chunks
	}
	perDoc := map[string]int{}
	for _, ch := range c.Chunks {
		perDoc[ch.DocID]++
	}
	for _, d := range c.Docs {
		n := perDoc[d.ID]
		add(tiers, strconv.Itoa(d.Tier), 1, n)
		add(types, d.DocType, 1, n)
		add(kinds, d.SourceKind, 1, n)
		add(force, c.Force(d, today, today), 1, n)
	}
	out["current"] = map[string]any{"version": c.Version, "documents": len(c.Docs), "chunks": len(c.Chunks),
		"by_tier": sorted(tiers), "by_type": sorted(types), "by_source": sorted(kinds), "by_force": sorted(force), "today": today}

	// Embeddings per model, against what there is to embed.
	type emb struct {
		Model    string `json:"model"`
		Chunks   int    `json:"chunks"`
		HS       int    `json:"hs"`
		Loaded   bool   `json:"loaded"`
		Dims     int    `json:"dims"`
		Provider string `json:"provider"`
	}
	var embs []emb
	loaded := s.Engine.Models().Embedders
	erows, err := s.Pool.Query(ctx, `SELECT m.name, m.provider, m.dims,
		(SELECT count(*) FROM chunk_embeddings e WHERE e.model = m.name),
		(SELECT count(*) FROM hs_embeddings h WHERE h.model = m.name)
		FROM embedding_models m ORDER BY m.name`)
	if err == nil {
		for erows.Next() {
			var e emb
			if erows.Scan(&e.Model, &e.Provider, &e.Dims, &e.Chunks, &e.HS) == nil {
				_, e.Loaded = loaded[e.Model]
				embs = append(embs, e)
			}
		}
		erows.Close()
	}
	if embs == nil {
		embs = []emb{}
	}
	out["embeddings"] = embs
	var registry, edges, hs int
	_ = s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM registry), (SELECT count(*) FROM edges), (SELECT count(*) FROM hs_codes)`).Scan(&registry, &edges, &hs)
	out["registry"], out["edges"], out["hs_lines"] = registry, edges, hs
	out["models"] = s.Engine.Models().EmbeddingNames()
	out["coverage"] = s.coverage()
	writeJSON(w, http.StatusOK, out)
}

func sorted(m map[string]*countRow) []countRow {
	out := make([]countRow, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Documents > out[j].Documents || (out[i].Documents == out[j].Documents && out[i].Key < out[j].Key)
	})
	return out
}

// coverage reads the coverage report the last prep wrote: what the corpus
// does with each document on the mandatory list.
func (s *Server) coverage() map[string]any {
	if s.PrepDir == "" {
		return map[string]any{"available": false}
	}
	path := filepath.Join(s.PrepDir, "report", "coverage.csv")
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{"available": false, "path": path}
	}
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), utf8BOM))).ReadAll()
	if err != nil || len(recs) == 0 {
		return map[string]any{"available": false, "path": path}
	}
	head := recs[0]
	counts := map[string]int{}
	rows := []map[string]string{}
	for _, rec := range recs[1:] {
		row := map[string]string{}
		for i, h := range head {
			if i < len(rec) {
				row[h] = rec[i]
			}
		}
		counts[row["status"]]++
		rows = append(rows, row)
	}
	info, _ := os.Stat(path)
	return map[string]any{"available": true, "counts": counts, "rows": rows, "written_at": info.ModTime()}
}

// --- Jobs ---

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	list, err := jobs.List(r.Context(), s.Pool, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	j, err := jobs.Get(r.Context(), s.Pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *Server) startJob(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind   string          `json:"kind"`
		Params json.RawMessage `json:"params"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if s.Jobs == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("background jobs are not available"))
		return
	}
	id, err := s.Jobs.Enqueue(r.Context(), in.Kind, in.Params, userOf(r).ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int64{"id": id})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.Jobs.Cancel(r.Context(), id); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- Feedback review ---

// FeedbackLabels are what a reviewer can say went wrong.
var FeedbackLabels = []string{"wrong_citation", "missing_grounds", "wrong_refusal", "should_refuse", "wrong_force", "incomplete", "unclear", "data_gap", "other"}

type feedbackRow struct {
	ID           int64      `json:"id"`
	Rating       int        `json:"rating"`
	Reason       string     `json:"reason"`
	Label        string     `json:"label"`
	Status       string     `json:"status"`
	ReviewNote   string     `json:"review_note"`
	CreatedAt    time.Time  `json:"created_at"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	GoldItemID   *int64     `json:"gold_item_id,omitempty"`
	MessageID    int64      `json:"message_id"`
	Conversation int64      `json:"conversation_id"`
	Answer       string     `json:"answer"`
	Refused      bool       `json:"refused"`
	Question     string     `json:"question"`
	User         string     `json:"user"`
	Reviewer     string     `json:"reviewer"`
}

const feedbackSelect = `SELECT f.id, f.rating, f.reason, f.label, f.status, f.review_note, f.created_at, f.reviewed_at, f.gold_item_id,
	m.id, m.conversation_id, m.content, m.refused,
	coalesce((SELECT q.content FROM messages q WHERE q.conversation_id = m.conversation_id AND q.role = 'user' AND q.id < m.id ORDER BY q.id DESC LIMIT 1), ''),
	coalesce(u.email, ''), coalesce(rv.email, '')
	FROM feedback f JOIN messages m ON m.id = f.message_id
	LEFT JOIN users u ON u.id = f.user_id LEFT JOIN users rv ON rv.id = f.reviewer_id`

func scanFeedback(row pgx.Row) (*feedbackRow, error) {
	var f feedbackRow
	err := row.Scan(&f.ID, &f.Rating, &f.Reason, &f.Label, &f.Status, &f.ReviewNote, &f.CreatedAt, &f.ReviewedAt, &f.GoldItemID,
		&f.MessageID, &f.Conversation, &f.Answer, &f.Refused, &f.Question, &f.User, &f.Reviewer)
	return &f, err
}

func (s *Server) listFeedback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	status := q.Get("status")
	rating, _ := strconv.Atoi(q.Get("rating"))
	rows, err := s.Pool.Query(ctx, feedbackSelect+` WHERE ($1 = '' OR f.status = $1) AND ($2::int = 0 OR f.rating = $2)
		ORDER BY f.created_at DESC LIMIT 300`, status, rating)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	list := []feedbackRow{}
	for rows.Next() {
		f, err := scanFeedback(rows)
		if err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		list = append(list, *f)
	}
	rows.Close()
	counts := map[string]int{}
	crows, err := s.Pool.Query(ctx, `SELECT status, rating, count(*) FROM feedback GROUP BY 1, 2`)
	if err == nil {
		for crows.Next() {
			var st string
			var rt, n int
			if crows.Scan(&st, &rt, &n) == nil {
				counts[st+":"+strconv.Itoa(rt)] = n
			}
		}
		crows.Close()
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "counts": counts, "labels": FeedbackLabels})
}

func (s *Server) getFeedback(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	f, err := scanFeedback(s.Pool.QueryRow(ctx, feedbackSelect+` WHERE f.id = $1`, id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	conv, msgs, err := s.Chat.Get(ctx, f.Conversation)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if msgs == nil {
		msgs = []chat.Message{}
	}
	// The whole trace of the rated answer: what was retrieved, what the
	// verifier dropped and why, which model, how long each stage took.
	var trace json.RawMessage
	_ = s.Pool.QueryRow(ctx, `SELECT coalesce(trace, '{}') FROM messages WHERE id = $1`, f.MessageID).Scan(&trace)
	var gold *evalx.StoredItem
	if f.GoldItemID != nil {
		var g evalx.StoredItem
		if s.Pool.QueryRow(ctx, `SELECT id, ext_id, question, expected_provisions, expected_answer, category, should_refuse, coalesce(as_of_date::text, '')
			FROM gold_items WHERE id = $1`, *f.GoldItemID).Scan(&g.DBID, &g.ExtID, &g.Question, &g.ExpectedProvisions, &g.ExpectedAnswer,
			&g.Category, &g.ShouldRefuse, &g.AsOf) == nil {
			gold = &g
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"feedback": f, "conversation": conv, "messages": msgs, "trace": trace, "gold": gold, "labels": FeedbackLabels})
}

func (s *Server) reviewFeedback(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Status     *string `json:"status"`
		Label      *string `json:"label"`
		ReviewNote *string `json:"review_note"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.Status != nil && *in.Status != "open" && *in.Status != "resolved" && *in.Status != "dismissed" {
		writeError(w, http.StatusBadRequest, errors.New("status must be open, resolved or dismissed"))
		return
	}
	tag, err := s.Pool.Exec(r.Context(), `UPDATE feedback SET status = coalesce($2, status), label = coalesce($3, label),
		review_note = coalesce($4, review_note), reviewer_id = $5, reviewed_at = now() WHERE id = $1`,
		id, in.Status, in.Label, in.ReviewNote, userOf(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, errors.New("no such feedback"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// feedbackToGold turns a reviewed question into a gold item, so the mistake
// is measured on every later run.
func (s *Server) feedbackToGold(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		GoldSetID int64 `json:"gold_set_id"`
		evalx.Item
	}
	if !readJSON(w, r, &in) {
		return
	}
	in.Question = strings.TrimSpace(in.Question)
	if in.Question == "" {
		writeError(w, http.StatusBadRequest, errors.New("the question is empty"))
		return
	}
	if in.ShouldRefuse == (strings.TrimSpace(in.ExpectedProvisions) != "") {
		writeError(w, http.StatusBadRequest, errors.New("give the expected provisions, or mark the question as one to refuse, not both"))
		return
	}
	if in.AsOf != "" {
		if _, err := time.Parse(time.DateOnly, in.AsOf); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("as_of_date must be YYYY-MM-DD"))
			return
		}
	}
	in.ExtID = "fb-" + strconv.FormatInt(id, 10)
	items := []evalx.Item{in.Item}
	if errs := evalx.Resolve(s.Engine.Corpus(), items, s.Engine.Now().Format(time.DateOnly)); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": errs[0].Message, "errors": errs})
		return
	}
	setID, itemID, err := evalx.AddGoldItem(r.Context(), s.Pool, in.GoldSetID, items[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.Pool.Exec(r.Context(), `UPDATE feedback SET gold_item_id = $2, status = 'resolved', reviewer_id = $3, reviewed_at = now() WHERE id = $1`,
		id, itemID, userOf(r).ID); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"gold_set_id": setID, "gold_item_id": itemID, "expected_ids": items[0].ExpectedIDs})
}

// --- Dashboard ---

type dayStats struct {
	Day           string  `json:"day"`
	Questions     int     `json:"questions"`
	Answers       int     `json:"answers"`
	Refused       int     `json:"refused"`
	Clarified     int     `json:"clarified"`
	Errors        int     `json:"errors"`
	Users         int     `json:"users"`
	FirstClaimP50 float64 `json:"first_claim_p50_ms"`
	FirstClaimP95 float64 `json:"first_claim_p95_ms"`
	TotalP50      float64 `json:"total_p50_ms"`
	TotalP95      float64 `json:"total_p95_ms"`
	CostUSD       float64 `json:"cost_usd"`
	EvalCostUSD   float64 `json:"eval_cost_usd"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	Up            int     `json:"up"`
	Down          int     `json:"down"`
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days < 1 || days > 366 {
		days = 30
	}
	loc, err := time.LoadLocation(dayZone)
	if err != nil {
		loc = time.FixedZone("ICT", 7*3600)
	}
	today := s.Engine.Now().In(loc)
	byDay := map[string]*dayStats{}
	var order []string
	for i := days - 1; i >= 0; i-- {
		d := today.AddDate(0, 0, -i).Format(time.DateOnly)
		byDay[d] = &dayStats{Day: d}
		order = append(order, d)
	}
	since := order[0]
	day := `(%s AT TIME ZONE '` + dayZone + `')::date::text`
	type query struct {
		sql  string
		scan func(pgx.Rows, *dayStats) error
	}
	queries := []query{
		{`SELECT ` + fmt.Sprintf(day, "m.created_at") + `, count(*), count(DISTINCT c.user_id)
			FROM messages m JOIN conversations c ON c.id = m.conversation_id
			WHERE m.role = 'user' AND m.created_at >= ($1::date AT TIME ZONE '` + dayZone + `') GROUP BY 1`,
			func(rows pgx.Rows, d *dayStats) error { var k string; return rows.Scan(&k, &d.Questions, &d.Users) }},
		{`SELECT ` + fmt.Sprintf(day, "created_at") + `, count(*), count(*) FILTER (WHERE refused),
			count(*) FILTER (WHERE coalesce(trace->>'clarify', '') <> ''), count(*) FILTER (WHERE coalesce(trace->>'error', '') <> ''),
			coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY (trace->>'first_claim_ms')::float8) FILTER (WHERE (trace->>'first_claim_ms')::float8 > 0), 0),
			coalesce(percentile_cont(0.95) WITHIN GROUP (ORDER BY (trace->>'first_claim_ms')::float8) FILTER (WHERE (trace->>'first_claim_ms')::float8 > 0), 0),
			coalesce(percentile_cont(0.5) WITHIN GROUP (ORDER BY (trace->'timing_ms'->>'total')::float8), 0),
			coalesce(percentile_cont(0.95) WITHIN GROUP (ORDER BY (trace->'timing_ms'->>'total')::float8), 0)
			FROM messages WHERE role = 'assistant' AND created_at >= ($1::date AT TIME ZONE '` + dayZone + `') GROUP BY 1`,
			func(rows pgx.Rows, d *dayStats) error {
				var k string
				return rows.Scan(&k, &d.Answers, &d.Refused, &d.Clarified, &d.Errors, &d.FirstClaimP50, &d.FirstClaimP95, &d.TotalP50, &d.TotalP95)
			}},
		{`SELECT ` + fmt.Sprintf(day, "at") + `, coalesce(sum(cost_usd) FILTER (WHERE purpose NOT LIKE 'eval:%'), 0)::float8,
			coalesce(sum(cost_usd) FILTER (WHERE purpose LIKE 'eval:%'), 0)::float8, coalesce(sum(input_tokens), 0), coalesce(sum(output_tokens), 0)
			FROM usage WHERE at >= ($1::date AT TIME ZONE '` + dayZone + `') GROUP BY 1`,
			func(rows pgx.Rows, d *dayStats) error {
				var k string
				return rows.Scan(&k, &d.CostUSD, &d.EvalCostUSD, &d.InputTokens, &d.OutputTokens)
			}},
		{`SELECT ` + fmt.Sprintf(day, "created_at") + `, count(*) FILTER (WHERE rating = 1), count(*) FILTER (WHERE rating = -1)
			FROM feedback WHERE created_at >= ($1::date AT TIME ZONE '` + dayZone + `') GROUP BY 1`,
			func(rows pgx.Rows, d *dayStats) error { var k string; return rows.Scan(&k, &d.Up, &d.Down) }},
	}
	for _, q := range queries {
		rows, err := s.Pool.Query(ctx, q.sql, since)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		for rows.Next() {
			// The first column is the day; read it alone, then the row
			// into that day's stats.
			key := rows.RawValues()[0]
			d := byDay[string(key)]
			if d == nil {
				continue
			}
			if err := q.scan(rows, d); err != nil {
				rows.Close()
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	series := make([]dayStats, 0, len(order))
	var total dayStats
	for _, k := range order {
		d := byDay[k]
		series = append(series, *d)
		total.Questions += d.Questions
		total.Answers += d.Answers
		total.Refused += d.Refused
		total.Clarified += d.Clarified
		total.Errors += d.Errors
		total.CostUSD += d.CostUSD
		total.EvalCostUSD += d.EvalCostUSD
		total.InputTokens += d.InputTokens
		total.OutputTokens += d.OutputTokens
		total.Up += d.Up
		total.Down += d.Down
	}
	// Latency over the whole window, not an average of daily percentiles.
	_ = s.Pool.QueryRow(ctx, `SELECT count(DISTINCT c.user_id),
		coalesce((SELECT percentile_cont(0.5) WITHIN GROUP (ORDER BY (trace->>'first_claim_ms')::float8) FROM messages WHERE role = 'assistant' AND (trace->>'first_claim_ms')::float8 > 0 AND created_at >= ($1::date AT TIME ZONE '`+dayZone+`')), 0),
		coalesce((SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY (trace->>'first_claim_ms')::float8) FROM messages WHERE role = 'assistant' AND (trace->>'first_claim_ms')::float8 > 0 AND created_at >= ($1::date AT TIME ZONE '`+dayZone+`')), 0),
		coalesce((SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY (trace->'timing_ms'->>'total')::float8) FROM messages WHERE role = 'assistant' AND created_at >= ($1::date AT TIME ZONE '`+dayZone+`')), 0)
		FROM messages m JOIN conversations c ON c.id = m.conversation_id WHERE m.role = 'user' AND m.created_at >= ($1::date AT TIME ZONE '`+dayZone+`')`, since).
		Scan(&total.Users, &total.FirstClaimP50, &total.FirstClaimP95, &total.TotalP95)
	// Models used without a price: their cost shows as 0 and should not be
	// read as free.
	unpriced := []string{}
	if rows, err := s.Pool.Query(ctx, `SELECT DISTINCT provider || '/' || model FROM usage
		WHERE at >= ($1::date AT TIME ZONE '`+dayZone+`') AND cost_usd = 0 AND input_tokens + output_tokens > 0 ORDER BY 1`, since); err == nil {
		for rows.Next() {
			var m string
			if rows.Scan(&m) == nil {
				unpriced = append(unpriced, m)
			}
		}
		rows.Close()
	}
	var open int
	_ = s.Pool.QueryRow(ctx, `SELECT count(*) FROM feedback WHERE status = 'open' AND rating = -1`).Scan(&open)
	writeJSON(w, http.StatusOK, map[string]any{"days": series, "total": total, "unpriced": unpriced, "open_down": open, "zone": dayZone})
}

// --- Configuration export and import ---

// ConfigFormat names the export file.
const ConfigFormat = "customsvn-config"

type exportedProvider struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	BaseURL   string   `json:"base_url"`
	APIKeyEnv string   `json:"api_key_env"`
	Models    []string `json:"models"`
	// Embeddings and Rerankers are left out of files written before model
	// types existed.
	Embeddings []llmconfig.EmbeddingModel `json:"embeddings,omitempty"`
	Rerankers  []string                   `json:"rerankers,omitempty"`
}

// ConfigFile is an exported configuration. It never holds a key: a provider
// carries the name of the environment variable it reads, or nothing, and the
// keys are typed again on the server it is imported into.
type ConfigFile struct {
	Format     string             `json:"format"`
	Version    int                `json:"version"`
	ExportedAt time.Time          `json:"exported_at"`
	Retrieval  *RetrievalSettings `json:"retrieval,omitempty"`
	LLM        *struct {
		Providers []exportedProvider            `json:"providers"`
		Roles     map[string][]llmconfig.Target `json:"roles"`
		Prices    llmconfig.Prices              `json:"prices"`
	} `json:"llm,omitempty"`
	Limits  *Limits         `json:"limits,omitempty"`
	Prompts *answer.Prompts `json:"prompts,omitempty"`
}

func (s *Server) exportConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, err := s.LLM.Load(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ret := s.settings(ctx)
	lim := s.limits(ctx)
	prompts := s.Prompts(ctx)
	out := ConfigFile{Format: ConfigFormat, Version: 1, ExportedAt: time.Now().UTC(), Retrieval: &ret, Limits: &lim, Prompts: &prompts}
	out.LLM = &struct {
		Providers []exportedProvider            `json:"providers"`
		Roles     map[string][]llmconfig.Target `json:"roles"`
		Prices    llmconfig.Prices              `json:"prices"`
	}{Roles: st.Roles, Prices: st.Prices}
	for _, p := range st.Providers {
		out.LLM.Providers = append(out.LLM.Providers, exportedProvider{Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, APIKeyEnv: p.APIKeyEnv, Models: p.Models,
			Embeddings: p.Embeddings, Rerankers: p.Rerankers})
	}
	w.Header().Set("Content-Disposition", `attachment; filename="customsvn-config-`+time.Now().Format("20060102")+`.json"`)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) importConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in ConfigFile
	if !readJSON(w, r, &in) {
		return
	}
	if in.Format != ConfigFormat || in.Version != 1 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("not a %s version 1 file", ConfigFormat))
		return
	}
	applied := []string{}
	var needKeys []string
	if in.LLM != nil {
		st := &llmconfig.Settings{Roles: in.LLM.Roles, Prices: in.LLM.Prices}
		if st.Roles == nil {
			st.Roles = map[string][]llmconfig.Target{}
		}
		cur, err := s.LLM.Load(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		has := map[string]bool{}
		for _, p := range cur.Providers {
			has[p.Name] = p.HasKey
		}
		for _, p := range in.LLM.Providers {
			// A provider of the same name keeps the key this server holds.
			st.Providers = append(st.Providers, llmconfig.Provider{Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, APIKeyEnv: p.APIKeyEnv, Models: p.Models,
				Embeddings: p.Embeddings, Rerankers: p.Rerankers})
			if !has[p.Name] && p.APIKeyEnv == "" && p.Kind != llmconfig.KindLuatdo {
				needKeys = append(needKeys, p.Name)
			}
		}
		if err := s.LLM.Save(ctx, st); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("llm: %w", err))
			return
		}
		if _, err := s.ReloadLLM(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		applied = append(applied, "llm")
	}
	if in.Retrieval != nil {
		if err := in.Retrieval.Chunking.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		in.Retrieval.Prepared, in.Retrieval.Config.ParentContext = nil, 0
		raw, _ := json.Marshal(in.Retrieval)
		if _, err := s.Pool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('retrieval', $1)
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, raw); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		applied = append(applied, "retrieval")
	}
	if in.Limits != nil {
		if err := s.saveLimits(ctx, *in.Limits); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		applied = append(applied, "limits")
	}
	if in.Prompts != nil {
		if err := s.savePrompts(ctx, *in.Prompts); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		applied = append(applied, "prompts")
	}
	if needKeys == nil {
		needKeys = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"applied": applied, "providers_without_key": needKeys})
}

// --- Answer evaluation ---

func (s *Server) startAnswerRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		GoldSetID int64              `json:"gold_set_id"`
		Label     string             `json:"label"`
		Config    evalx.AnswerConfig `json:"config"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	id, err := s.Answers.Start(s.ctx, in.GoldSetID, strings.TrimSpace(in.Label), in.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]int64{"id": id})
}

func (s *Server) listAnswerRuns(w http.ResponseWriter, r *http.Request) {
	list, err := evalx.ListAnswerRuns(r.Context(), s.Pool, 100)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getAnswerRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	run, results, err := evalx.GetAnswerRun(r.Context(), s.Pool, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "results": results})
}

func (s *Server) cancelAnswerRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if !s.Answers.Cancel(id) {
		writeError(w, http.StatusConflict, errors.New("the run is not running"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
