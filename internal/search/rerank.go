package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nvtquang/CustomsVN/internal/config"
)

// HTTPReranker calls a /rerank endpoint. Cohere, Jina and Voyage take the
// same request; Cohere and Jina answer under "results", Voyage under "data".
type HTTPReranker struct {
	URL, Model, Key string
	Client          *http.Client
}

// NewReranker builds the reranker a config entry describes.
func NewReranker(r *config.Rerank) (*HTTPReranker, error) {
	key, err := config.APIKey(r.APIKeyEnv)
	if err != nil {
		return nil, fmt.Errorf("rerank: %w", err)
	}
	base := r.BaseURL
	if base == "" {
		switch r.Provider {
		case "cohere":
			base = "https://api.cohere.com/v2"
		case "jina":
			base = "https://api.jina.ai/v1"
		case "voyage":
			base = "https://api.voyageai.com/v1"
		default:
			return nil, fmt.Errorf("rerank: unknown provider %q (want cohere, jina or voyage, or give base_url)", r.Provider)
		}
	}
	return &HTTPReranker{URL: strings.TrimRight(base, "/") + "/rerank", Model: r.Model, Key: key,
		Client: &http.Client{Timeout: 60 * time.Second}}, nil
}

// Rerank returns one score per passage, in order.
func (r *HTTPReranker) Rerank(ctx context.Context, query string, passages []string) ([]float64, error) {
	body, _ := json.Marshal(map[string]any{"model": r.Model, "query": query, "documents": passages, "top_n": len(passages)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.Key != "" {
		req.Header.Set("Authorization", "Bearer "+r.Key)
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %.300s", resp.StatusCode, data)
	}
	var out struct {
		Results []struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		} `json:"results"`
		Data []struct {
			Index int     `json:"index"`
			Score float64 `json:"relevance_score"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	scores := make([]float64, len(passages))
	for _, x := range append(out.Results, out.Data...) {
		if x.Index >= 0 && x.Index < len(scores) {
			scores[x.Index] = x.Score
		}
	}
	return scores, nil
}
