package llmconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ListModels asks a provider which models it serves, so the settings screen
// offers a list instead of a box to type an identifier into. The key is the
// one typed in the request, else the named environment variable's, else the
// key stored for a provider of the same name.
func (s *Store) ListModels(ctx context.Context, p Provider) ([]string, error) {
	switch p.Kind {
	case KindLuatdo:
		return nil, errors.New("a luatdo routes file names its models itself")
	case KindVoyage, KindJina, KindCohere:
		return nil, fmt.Errorf("%s does not list its models; type the model name", p.Kind)
	}
	key := s.resolveKey(ctx, p)
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var req *http.Request
	var err error
	switch p.Kind {
	case KindAnthropic:
		base := strings.TrimRight(p.BaseURL, "/")
		if base == "" {
			base = "https://api.anthropic.com"
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models?limit=100", nil)
		if err == nil {
			req.Header.Set("x-api-key", key)
			req.Header.Set("anthropic-version", "2023-06-01")
		}
	case KindOpenAI:
		base := strings.TrimRight(p.BaseURL, "/")
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
		if err == nil && key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	default:
		return nil, fmt.Errorf("unknown kind %q", p.Kind)
	}
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("%s: %s", resp.Status, msg)
	}
	// Both wires answer {"data": [{"id": ...}]}; Ollama's OpenAI endpoint too.
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("unexpected answer: %w", err)
	}
	ids := make([]string, 0, len(out.Data))
	for _, d := range out.Data {
		if d.ID != "" {
			ids = append(ids, d.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
