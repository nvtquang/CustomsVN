// Package embed turns text into vectors through cloud embedding APIs.
//
// Three wire formats cover the providers worth benchmarking: the OpenAI
// /embeddings shape (OpenAI, Jina, Together, Mistral, a local Ollama or vLLM,
// and most others), Voyage, which adds an input type, and Gemini. Every client
// is rate limited and retries the failures that are worth retrying, because a
// corpus embed is tens of thousands of requests and one 429 must not lose it.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nvtquang/CustomsVN/internal/config"
)

// Kind says what a text is for. Some models embed a query and a passage
// differently, and getting it backwards costs recall.
type Kind int

const (
	Document Kind = iota
	Query
)

// Embedder turns texts into vectors.
type Embedder interface {
	// Name is the configured model name, the key embeddings are stored under.
	Name() string
	Dims() int
	// Embed returns one vector per text, in order.
	Embed(ctx context.Context, texts []string, kind Kind) ([][]float32, error)
}

// TokenCounter is an embedder that reports the input tokens its provider
// billed, so a job can record what embedding the corpus cost.
type TokenCounter interface {
	Tokens() int64
}

// New builds the embedder a config entry describes.
func New(e *config.Embedding) (Embedder, error) {
	key, err := config.APIKey(e.APIKeyEnv)
	if err != nil {
		return nil, fmt.Errorf("embedding %s: %w", e.Name, err)
	}
	return NewWithKey(e, key)
}

// NewWithKey builds an embedder with a key given directly, for a model set
// up in the settings screen, whose key is stored encrypted.
func NewWithKey(e *config.Embedding, key string) (Embedder, error) {
	if e.Batch <= 0 {
		e.Batch = 64
	}
	if e.RPM <= 0 {
		e.RPM = 60
	}
	c := &client{cfg: *e, key: key, http: &http.Client{Timeout: 120 * time.Second}, limit: newLimiter(e.RPM)}
	switch e.Provider {
	case "openai", "":
		if c.cfg.BaseURL == "" {
			c.cfg.BaseURL = "https://api.openai.com/v1"
		}
		c.request = c.openAI
	case "voyage":
		if c.cfg.BaseURL == "" {
			c.cfg.BaseURL = "https://api.voyageai.com/v1"
		}
		c.request = c.voyage
	case "gemini":
		if c.cfg.BaseURL == "" {
			c.cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
		}
		c.request = c.gemini
	default:
		return nil, fmt.Errorf("embedding %s: unknown provider %q (want openai, voyage or gemini)", e.Name, e.Provider)
	}
	return c, nil
}

type client struct {
	cfg     config.Embedding
	key     string
	http    *http.Client
	limit   *limiter
	request func(ctx context.Context, texts []string, kind Kind) ([][]float32, error)
	// tokens adds up the usage the provider reports; Gemini reports none.
	tokens atomic.Int64
}

// Tokens is the input tokens billed so far.
func (c *client) Tokens() int64 { return c.tokens.Load() }

func (c *client) Name() string { return c.cfg.Name }
func (c *client) Dims() int    { return c.cfg.Dims }

// Embed sends texts in batches of the configured size.
func (c *client) Embed(ctx context.Context, texts []string, kind Kind) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += c.cfg.Batch {
		batch := texts[start:min(start+c.cfg.Batch, len(texts))]
		vecs, err := c.withRetry(ctx, func() ([][]float32, error) { return c.request(ctx, batch, kind) })
		if err != nil {
			return nil, err
		}
		if len(vecs) != len(batch) {
			return nil, fmt.Errorf("%s returned %d vectors for %d texts", c.cfg.Name, len(vecs), len(batch))
		}
		for _, v := range vecs {
			if c.cfg.Dims > 0 && len(v) != c.cfg.Dims { // zero only in a connection test
				return nil, fmt.Errorf("%s returned %d dimensions, config says %d", c.cfg.Name, len(v), c.cfg.Dims)
			}
		}
		out = append(out, vecs...)
	}
	return out, nil
}

// statusError is a response worth reading the status of.
type statusError struct {
	code int
	body string
}

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.code, e.body) }

// retryable: rate limits, server errors and network failures. A 400 or a 401
// will fail the same way every time.
func retryable(err error) bool {
	var se *statusError
	if errors.As(err, &se) {
		return se.code == http.StatusTooManyRequests || se.code >= 500
	}
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// MaxAttempts bounds the retries of one batch.
const MaxAttempts = 6

func (c *client) withRetry(ctx context.Context, call func() ([][]float32, error)) ([][]float32, error) {
	var err error
	for attempt := range MaxAttempts {
		if err := c.limit.wait(ctx); err != nil {
			return nil, err
		}
		var out [][]float32
		if out, err = call(); err == nil {
			return out, nil
		}
		if !retryable(err) {
			return nil, err
		}
		backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return nil, fmt.Errorf("%s: gave up after %d attempts: %w", c.cfg.Name, MaxAttempts, err)
}

func (c *client) post(ctx context.Context, url string, header map[string]string, body, into any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &statusError{code: resp.StatusCode, body: truncate(string(data), 300)}
	}
	return json.Unmarshal(data, into)
}

func (c *client) openAI(ctx context.Context, texts []string, _ Kind) ([][]float32, error) {
	body := map[string]any{"model": c.cfg.Model, "input": texts}
	// Models that can shorten their output (text-embedding-3-*) are asked for
	// the configured size; others ignore the field or reject it, so it is sent
	// only when the model name says it is understood.
	if strings.HasPrefix(c.cfg.Model, "text-embedding-3") {
		body["dimensions"] = c.cfg.Dims
	}
	var resp struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int64 `json:"prompt_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := c.post(ctx, strings.TrimRight(c.cfg.BaseURL, "/")+"/embeddings", c.bearer(), body, &resp); err != nil {
		return nil, err
	}
	c.tokens.Add(max(resp.Usage.PromptTokens, resp.Usage.TotalTokens))
	out := make([][]float32, len(texts))
	for _, d := range resp.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, fmt.Errorf("embedding index %d out of range", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	return out, nil
}

func (c *client) voyage(ctx context.Context, texts []string, kind Kind) ([][]float32, error) {
	inputType := "document"
	if kind == Query {
		inputType = "query"
	}
	body := map[string]any{"model": c.cfg.Model, "input": texts, "input_type": inputType, "output_dimension": c.cfg.Dims}
	var resp struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int64 `json:"prompt_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := c.post(ctx, strings.TrimRight(c.cfg.BaseURL, "/")+"/embeddings", c.bearer(), body, &resp); err != nil {
		return nil, err
	}
	c.tokens.Add(max(resp.Usage.PromptTokens, resp.Usage.TotalTokens))
	out := make([][]float32, len(texts))
	for _, d := range resp.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, fmt.Errorf("embedding index %d out of range", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	return out, nil
}

func (c *client) gemini(ctx context.Context, texts []string, kind Kind) ([][]float32, error) {
	task := "RETRIEVAL_DOCUMENT"
	if kind == Query {
		task = "RETRIEVAL_QUERY"
	}
	model := "models/" + strings.TrimPrefix(c.cfg.Model, "models/")
	reqs := make([]map[string]any, len(texts))
	for i, t := range texts {
		reqs[i] = map[string]any{
			"model": model, "taskType": task, "outputDimensionality": c.cfg.Dims,
			"content": map[string]any{"parts": []map[string]string{{"text": t}}},
		}
	}
	var resp struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/" + model + ":batchEmbedContents"
	if err := c.post(ctx, url, map[string]string{"x-goog-api-key": c.key}, map[string]any{"requests": reqs}, &resp); err != nil {
		return nil, err
	}
	out := make([][]float32, len(resp.Embeddings))
	for i, e := range resp.Embeddings {
		out[i] = e.Values
	}
	return out, nil
}

func (c *client) bearer() map[string]string {
	if c.key == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + c.key}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// limiter spaces requests evenly to stay under a per-minute cap.
type limiter struct {
	mu    sync.Mutex
	every time.Duration
	next  time.Time
}

func newLimiter(rpm int) *limiter {
	if rpm <= 0 {
		rpm = 60
	}
	return &limiter{every: time.Minute / time.Duration(rpm)}
}

func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	at := l.next
	if at.Before(now) {
		at = now
	}
	l.next = at.Add(l.every)
	l.mu.Unlock()
	if d := time.Until(at); d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	return nil
}
