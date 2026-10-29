// Package chat stores conversations, their messages, the verified citations
// of each answer, feedback and token usage.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/answer"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/tools"
)

// Store is the conversation store. Conversations belong to the user who
// started them; an owner of 0 stands for "any", for admins reviewing and for
// callers with the API token.
type Store struct {
	Pool *pgxpool.Pool
	// Cost prices one call's usage; nil records no cost.
	Cost func(llm.Usage) float64
}

// Conversation is a conversation as the history sidebar lists it.
type Conversation struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	AsOf      string    `json:"as_of_date,omitempty"`
	Messages  int       `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message is one turn as the chat screen shows it.
type Message struct {
	ID        int64           `json:"id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	Refused   bool            `json:"refused"`
	Claims    []answer.Claim  `json:"claims,omitempty"`
	Refusal   *answer.Refusal `json:"refusal,omitempty"`
	Clarify   string          `json:"clarify,omitempty"`
	Intent    *answer.Intent  `json:"intent,omitempty"`
	Steps     []answer.Step   `json:"steps,omitempty"`
	Cards     []tools.Card    `json:"cards,omitempty"`
	Gaps      []string        `json:"gaps,omitempty"`
	Dropped   int             `json:"dropped,omitempty"`
	AsOf      string          `json:"as_of_date,omitempty"`
	Corpus    *int64          `json:"corpus_version,omitempty"`
	Error     string          `json:"error,omitempty"`
	Rating    int             `json:"rating,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

// ErrNotFound is a conversation or message that does not exist.
var ErrNotFound = errors.New("not found")

// titleLength bounds a title made from the first question.
const titleLength = 80

// Owns returns ErrNotFound unless the conversation exists and belongs to
// owner. Someone else's conversation is reported as missing, not forbidden,
// so its existence is not given away.
func (s *Store) Owns(ctx context.Context, conv, owner int64) error {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT $2::bigint = 0 OR coalesce(user_id = $2, false) FROM conversations WHERE id = $1`, conv, owner).Scan(&ok)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !ok) {
		return ErrNotFound
	}
	return err
}

// Create starts a conversation of owner titled after its first question.
func (s *Store) Create(ctx context.Context, owner int64, firstQuestion, asOf string) (int64, error) {
	title := strings.Join(strings.Fields(firstQuestion), " ")
	if r := []rune(title); len(r) > titleLength {
		title = string(r[:titleLength]) + "…"
	}
	var id int64
	err := s.Pool.QueryRow(ctx, `INSERT INTO conversations (title, as_of_date, user_id) VALUES ($1, nullif($2, '')::date, nullif($3::bigint, 0)) RETURNING id`,
		title, asOf, owner).Scan(&id)
	return id, err
}

// AddUser stores a question.
func (s *Store) AddUser(ctx context.Context, conv int64, question, asOf string) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `INSERT INTO messages (conversation_id, role, content, as_of_date) VALUES ($1, 'user', $2, nullif($3, '')::date) RETURNING id`,
		conv, question, asOf).Scan(&id)
	if err != nil {
		return 0, err
	}
	_, err = s.Pool.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, conv)
	return id, err
}

// trace is what an assistant message keeps besides its text, for review.
type trace struct {
	Standalone   string             `json:"standalone"`
	Retrieved    []string           `json:"retrieved"`
	Dropped      []answer.Dropped   `json:"dropped,omitempty"`
	Gaps         []string           `json:"gaps,omitempty"`
	Refusal      *answer.Refusal    `json:"refusal,omitempty"`
	Provider     string             `json:"provider,omitempty"`
	Model        string             `json:"model,omitempty"`
	TimingMS     map[string]float64 `json:"timing_ms"`
	FirstClaimMS float64            `json:"first_claim_ms,omitempty"`
	Error        string             `json:"error,omitempty"`
	Clarify      string             `json:"clarify,omitempty"`
	Intent       *answer.Intent     `json:"intent,omitempty"`
	Steps        []answer.Step      `json:"steps,omitempty"`
}

// AddAnswer stores an answer with its citations and usage. failure is the
// error that cut it short, if any.
func (s *Store) AddAnswer(ctx context.Context, conv int64, res *answer.Result, failure string) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	claims, _ := json.Marshal(res.Claims)
	var cards []byte
	if len(res.Cards) > 0 {
		cards, _ = json.Marshal(res.Cards)
	}
	intent := res.Intent
	tr, _ := json.Marshal(trace{Standalone: res.Standalone, Retrieved: res.Retrieved, Dropped: res.Dropped, Gaps: res.Gaps, Refusal: res.Refusal,
		Provider: res.Provider, Model: res.Model, TimingMS: res.TimingMS, FirstClaimMS: res.FirstClaimMS, Error: failure,
		Clarify: res.Clarify, Intent: &intent, Steps: res.Steps})
	var version *int64
	if res.CorpusVersion > 0 {
		version = &res.CorpusVersion
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO messages (conversation_id, role, content, refused, trace, claims, cards, corpus_version, as_of_date)
		VALUES ($1, 'assistant', $2, $3, $4, $5, $6, $7, nullif($8, '')::date) RETURNING id`,
		conv, res.Markdown(), res.Refusal != nil, tr, claims, cards, version, res.AsOf).Scan(&id)
	if err != nil {
		return 0, err
	}
	for _, c := range res.Claims {
		for _, ct := range c.Citations {
			if _, err := tx.Exec(ctx, `INSERT INTO citations (message_id, ordinal, chunk_id, quote, verified, force) VALUES ($1, $2, $3, $4, $5, $6)`,
				id, ct.N, ct.ChunkID, ct.Quote, ct.Verified, ct.Force); err != nil {
				return 0, err
			}
		}
	}
	if err := s.usage(ctx, tx, `coalesce((SELECT user_id FROM conversations WHERE id = $2), nullif($1::bigint, 0))`, 0, conv, "", res); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, conv); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// usage records the token use of an answer, one row per purpose, priced
// per call. who is the SQL for the user ($1 is the user, $2 the
// conversation); prefix marks usage that is not a reader's question, such as
// an evaluation run.
func (s *Store) usage(ctx context.Context, db execer, who string, user, conv int64, prefix string, res *answer.Result) error {
	for purpose, u := range res.Usage {
		if u.InputTokens == 0 && u.OutputTokens == 0 {
			continue
		}
		if u.Provider == "" {
			u.Provider, u.Model = res.Provider, res.Model
		}
		cost := 0.0
		if s.Cost != nil {
			cost = s.Cost(u)
		}
		if _, err := db.Exec(ctx, `INSERT INTO usage (user_id, conversation_id, provider, model, purpose, input_tokens, output_tokens, cost_usd)
			VALUES (`+who+`, nullif($2::bigint, 0), $3, $4, $5, $6, $7, $8)`,
			user, conv, u.Provider, u.Model, prefix+purpose, u.InputTokens, u.OutputTokens, cost); err != nil {
			return err
		}
	}
	return nil
}

// RecordUsage records the usage of an answer that is not stored as a
// conversation: the OpenAI-compatible API, MCP, an evaluation run.
func (s *Store) RecordUsage(ctx context.Context, user int64, prefix string, res *answer.Result) error {
	if res == nil {
		return nil
	}
	return s.usage(ctx, s.Pool, `nullif($1::bigint, 0)`, user, 0, prefix, res)
}

// History returns the turns of a conversation, oldest first.
func (s *Store) History(ctx context.Context, conv int64) ([]answer.Turn, error) {
	rows, err := s.Pool.Query(ctx, `SELECT role, content FROM messages WHERE conversation_id = $1 ORDER BY id`, conv)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []answer.Turn
	for rows.Next() {
		var t answer.Turn
		if err := rows.Scan(&t.Role, &t.Content); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// PopAnswer removes the last answer of a conversation, for regenerating it,
// and returns the question it answered.
func (s *Store) PopAnswer(ctx context.Context, conv int64) (string, string, error) {
	var lastRole string
	var lastID int64
	err := s.Pool.QueryRow(ctx, `SELECT id, role FROM messages WHERE conversation_id = $1 ORDER BY id DESC LIMIT 1`, conv).Scan(&lastID, &lastRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if lastRole == "assistant" {
		if _, err := s.Pool.Exec(ctx, `DELETE FROM messages WHERE id = $1`, lastID); err != nil {
			return "", "", err
		}
	}
	var q, asOf string
	err = s.Pool.QueryRow(ctx, `SELECT content, coalesce(as_of_date::text, '') FROM messages WHERE conversation_id = $1 AND role = 'user' ORDER BY id DESC LIMIT 1`, conv).Scan(&q, &asOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return q, asOf, err
}

// List returns conversations, newest first, optionally only those whose title
// or messages contain q.
func (s *Store) List(ctx context.Context, owner int64, q string, limit int) ([]Conversation, error) {
	sql := `SELECT c.id, c.title, coalesce(c.as_of_date::text, ''), count(m.id), c.created_at, c.updated_at
		FROM conversations c LEFT JOIN messages m ON m.conversation_id = c.id WHERE c.user_id = $2`
	args := []any{limit, owner}
	if q = strings.TrimSpace(q); q != "" {
		sql += ` AND (c.title ILIKE $3 OR EXISTS (SELECT 1 FROM messages x WHERE x.conversation_id = c.id AND x.role = 'user' AND x.content ILIKE $3))`
		args = append(args, "%"+strings.NewReplacer("%", `\%`, "_", `\_`).Replace(q)+"%")
	}
	sql += ` GROUP BY c.id ORDER BY c.updated_at DESC LIMIT $1`
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.Title, &c.AsOf, &c.Messages, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get returns a conversation and its messages.
func (s *Store) Get(ctx context.Context, id int64) (*Conversation, []Message, error) {
	c := &Conversation{ID: id}
	err := s.Pool.QueryRow(ctx, `SELECT title, coalesce(as_of_date::text, ''), created_at, updated_at FROM conversations WHERE id = $1`, id).
		Scan(&c.Title, &c.AsOf, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT m.id, m.role, m.content, m.refused, m.claims, m.cards, m.trace, coalesce(m.as_of_date::text, ''), m.corpus_version, m.created_at,
		coalesce((SELECT rating FROM feedback f WHERE f.message_id = m.id ORDER BY f.id DESC LIMIT 1), 0)
		FROM messages m WHERE m.conversation_id = $1 ORDER BY m.id`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		var claims, cards, tr []byte
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.Refused, &claims, &cards, &tr, &m.AsOf, &m.Corpus, &m.CreatedAt, &m.Rating); err != nil {
			return nil, nil, err
		}
		if len(claims) > 0 {
			_ = json.Unmarshal(claims, &m.Claims)
		}
		if len(cards) > 0 {
			_ = json.Unmarshal(cards, &m.Cards)
		}
		if len(tr) > 0 {
			var t trace
			if json.Unmarshal(tr, &t) == nil {
				m.Refusal, m.Dropped, m.Error, m.Gaps = t.Refusal, len(t.Dropped), t.Error, t.Gaps
				m.Clarify, m.Intent, m.Steps = t.Clarify, t.Intent, t.Steps
			}
		}
		msgs = append(msgs, m)
	}
	c.Messages = len(msgs)
	return c, msgs, rows.Err()
}

// SetAsOf sets the day a conversation's answers apply the law of; empty
// means the day each question is asked.
func (s *Store) SetAsOf(ctx context.Context, id int64, asOf string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE conversations SET as_of_date = nullif($2, '')::date WHERE id = $1`, id, asOf)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Rename sets a conversation's title.
func (s *Store) Rename(ctx context.Context, id int64, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("empty title")
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE conversations SET title = $2 WHERE id = $1`, id, title)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Delete removes a conversation and everything in it.
func (s *Store) Delete(ctx context.Context, id int64) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// Feedback records owner's rating of an answer in owner's conversation. A
// second rating replaces the first.
func (s *Store) Feedback(ctx context.Context, owner, message int64, rating int, reason string) error {
	if rating != 1 && rating != -1 {
		return errors.New("rating must be 1 or -1")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var role string
	if err := tx.QueryRow(ctx, `SELECT m.role FROM messages m JOIN conversations c ON c.id = m.conversation_id
		WHERE m.id = $1 AND ($2::bigint = 0 OR c.user_id = $2)`, message, owner).Scan(&role); errors.Is(err, pgx.ErrNoRows) || role != "assistant" {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM feedback WHERE message_id = $1`, message); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO feedback (message_id, user_id, rating, reason) VALUES ($1, nullif($2::bigint, 0), $3, $4)`,
		message, owner, rating, strings.TrimSpace(reason)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
