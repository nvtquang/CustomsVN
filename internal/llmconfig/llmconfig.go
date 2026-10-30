// Package llmconfig keeps the LLM providers and the model chosen for each
// role in the database, and builds the router serve uses from them.
//
// A provider's key comes from one of two places: an environment variable
// named on the provider, or a key typed in the settings screen, which is
// stored encrypted (internal/secret) and never sent back to the browser.
package llmconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/config"
	"github.com/nvtquang/CustomsVN/internal/embed"
	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/search"
	"github.com/nvtquang/CustomsVN/internal/secret"
)

// Provider kinds.
const (
	KindAnthropic = "anthropic"
	KindOpenAI    = "openai" // any OpenAI-compatible /chat/completions and /embeddings
	KindLuatdo    = "luatdo" // a luatdo routes file; base_url is its path
	// Embedding and rerank only.
	KindVoyage = "voyage"
	KindJina   = "jina"
	KindCohere = "cohere"
)

// Model types, as the settings screen groups them.
const (
	TypeChat      = "chat"
	TypeEmbedding = "embedding"
	TypeRerank    = "rerank"
)

// Supports says whether a provider kind serves a model type. An
// OpenAI-compatible provider reranks only at a base URL of its own, a local
// server with a /rerank endpoint.
func Supports(kind, typ, baseURL string) bool {
	switch typ {
	case TypeChat:
		return kind == KindAnthropic || kind == KindOpenAI || kind == KindLuatdo
	case TypeEmbedding:
		return kind == KindOpenAI || kind == KindVoyage || kind == KindJina
	case TypeRerank:
		return kind == KindVoyage || kind == KindJina || kind == KindCohere || kind == KindOpenAI && baseURL != ""
	}
	return false
}

// EmbeddingModel is an embedding model on a provider. Its vectors are stored
// under "provider/model".
type EmbeddingModel struct {
	Model string `json:"model"`
	Dims  int    `json:"dims"`
}

// MaxDims is the largest embedding pgvector can index (as halfvec).
const MaxDims = 4000

// Provider is a provider as the settings screen sees it. APIKey is write-only:
// it is read from a save and never filled on a load.
type Provider struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	BaseURL   string `json:"base_url"`
	APIKeyEnv string `json:"api_key_env"`
	// Models are its chat models.
	Models     []string         `json:"models"`
	Embeddings []EmbeddingModel `json:"embeddings"`
	Rerankers  []string         `json:"rerankers"`
	// HasKey and KeyHint describe a stored key without revealing it.
	HasKey  bool   `json:"has_key"`
	KeyHint string `json:"key_hint,omitempty"`
	// EnvKeySet says whether the named environment variable holds a value.
	EnvKeySet bool `json:"env_key_set"`

	APIKey   string `json:"api_key,omitempty"`
	ClearKey bool   `json:"clear_key,omitempty"`
}

// Target is a model on a provider, by name.
type Target struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// Price is what a model costs, in US dollars per million tokens.
type Price struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// Prices are keyed by "provider/model".
type Prices map[string]Price

// Cost prices one call's usage; a model with no price costs nothing, which
// the dashboard shows as an unpriced model rather than a free one.
func (p Prices) Cost(u llm.Usage) float64 {
	pr, ok := p[u.Provider+"/"+u.Model]
	if !ok {
		return 0
	}
	return (float64(u.InputTokens)*pr.Input + float64(u.OutputTokens)*pr.Output) / 1e6
}

// Settings is everything the LLM settings screen edits.
type Settings struct {
	Providers []Provider          `json:"providers"`
	Roles     map[string][]Target `json:"roles"`
	Prices    Prices              `json:"prices"`
	// MasterKey says whether keys can be stored encrypted.
	MasterKey bool `json:"master_key"`
}

// Store reads and writes the settings.
type Store struct {
	Pool *pgxpool.Pool
	Box  *secret.Box // nil when no master key is set
}

// Load returns the settings with keys masked.
func (s *Store) Load(ctx context.Context) (*Settings, error) {
	rows, err := s.Pool.Query(ctx, `SELECT name, kind, base_url, api_key_env, api_key_enc, models, embeddings, rerankers FROM llm_providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	st := &Settings{Providers: []Provider{}, Roles: map[string][]Target{}, Prices: Prices{}, MasterKey: s.Box != nil}
	for rows.Next() {
		var p Provider
		var enc []byte
		var models, embeddings, rerankers []byte
		if err := rows.Scan(&p.Name, &p.Kind, &p.BaseURL, &p.APIKeyEnv, &enc, &models, &embeddings, &rerankers); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(models, &p.Models)
		_ = json.Unmarshal(embeddings, &p.Embeddings)
		_ = json.Unmarshal(rerankers, &p.Rerankers)
		p.Models, p.Rerankers = nonNil(p.Models), nonNil(p.Rerankers)
		if p.Embeddings == nil {
			p.Embeddings = []EmbeddingModel{}
		}
		p.HasKey = len(enc) > 0
		if p.HasKey && s.Box != nil {
			if k, err := s.Box.Open(enc); err == nil {
				p.KeyHint = secret.Hint(k)
			}
		}
		p.EnvKeySet = p.APIKeyEnv != "" && os.Getenv(p.APIKeyEnv) != ""
		st.Providers = append(st.Providers, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var raw []byte
	err = s.Pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'llm_roles'`).Scan(&raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if raw != nil {
		_ = json.Unmarshal(raw, &st.Roles)
	}
	raw = nil
	err = s.Pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = 'llm_prices'`).Scan(&raw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if raw != nil {
		_ = json.Unmarshal(raw, &st.Prices)
	}
	return st, nil
}

// Save writes providers and roles. A provider left out is deleted. A key is
// replaced only when a new one is given, and removed only on ClearKey.
func (s *Store) Save(ctx context.Context, in *Settings) error {
	seen := map[string]bool{}
	kinds := map[string]string{}
	for _, p := range in.Providers {
		if err := validate(p); err != nil {
			return err
		}
		kinds[p.Name] = p.Kind
		if seen[p.Name] {
			return fmt.Errorf("provider %q is listed twice", p.Name)
		}
		seen[p.Name] = true
		if p.APIKey != "" && s.Box == nil {
			return secret.ErrNoMasterKey
		}
	}
	for role, targets := range in.Roles {
		if role != llm.RoleAnswer && role != llm.RoleAgent && role != llm.RoleRewrite && role != llm.RoleRouter {
			return fmt.Errorf("unknown role %q", role)
		}
		for _, t := range targets {
			if !seen[t.Provider] || strings.TrimSpace(t.Model) == "" {
				return fmt.Errorf("role %s names provider %q and model %q: both must exist", role, t.Provider, t.Model)
			}
			if !Supports(kinds[t.Provider], TypeChat, "") {
				return fmt.Errorf("role %s: provider %s has no chat models", role, t.Provider)
			}
		}
	}
	for key, p := range in.Prices {
		if !strings.Contains(key, "/") || p.Input < 0 || p.Output < 0 {
			return fmt.Errorf("price %q: the key is provider/model and prices are not negative", key)
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	names := make([]string, 0, len(in.Providers))
	for _, p := range in.Providers {
		names = append(names, p.Name)
		models, _ := json.Marshal(nonNil(p.Models))
		embeddings, _ := json.Marshal(p.Embeddings)
		if p.Embeddings == nil {
			embeddings = []byte("[]")
		}
		rerankers, _ := json.Marshal(nonNil(p.Rerankers))
		if _, err := tx.Exec(ctx, `INSERT INTO llm_providers (name, kind, base_url, api_key_env, models, embeddings, rerankers)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (name) DO UPDATE SET kind = EXCLUDED.kind, base_url = EXCLUDED.base_url, api_key_env = EXCLUDED.api_key_env,
			models = EXCLUDED.models, embeddings = EXCLUDED.embeddings, rerankers = EXCLUDED.rerankers, updated_at = now()`,
			p.Name, p.Kind, p.BaseURL, p.APIKeyEnv, models, embeddings, rerankers); err != nil {
			return err
		}
		switch {
		case p.APIKey != "":
			sealed, err := s.Box.Seal(strings.TrimSpace(p.APIKey))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE llm_providers SET api_key_enc = $2 WHERE name = $1`, p.Name, sealed); err != nil {
				return err
			}
		case p.ClearKey:
			if _, err := tx.Exec(ctx, `UPDATE llm_providers SET api_key_enc = NULL WHERE name = $1`, p.Name); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM llm_providers WHERE NOT (name = ANY($1))`, names); err != nil {
		return err
	}
	roles, _ := json.Marshal(in.Roles)
	if _, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('llm_roles', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, roles); err != nil {
		return err
	}
	if in.Prices == nil {
		in.Prices = Prices{}
	}
	prices, _ := json.Marshal(in.Prices)
	if _, err := tx.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('llm_prices', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, prices); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validate(p Provider) error {
	if strings.TrimSpace(p.Name) == "" || strings.Contains(p.Name, "/") {
		return errors.New("a provider needs a name without a slash")
	}
	switch p.Kind {
	case KindAnthropic, KindOpenAI, KindLuatdo, KindVoyage, KindJina, KindCohere:
	default:
		return fmt.Errorf("provider %s: kind must be anthropic, openai, luatdo, voyage, jina or cohere", p.Name)
	}
	if len(p.Models) > 0 && !Supports(p.Kind, TypeChat, p.BaseURL) {
		return fmt.Errorf("provider %s: %s has no chat models", p.Name, p.Kind)
	}
	if len(p.Embeddings) > 0 && !Supports(p.Kind, TypeEmbedding, p.BaseURL) {
		return fmt.Errorf("provider %s: %s has no embedding models", p.Name, p.Kind)
	}
	if len(p.Rerankers) > 0 && !Supports(p.Kind, TypeRerank, p.BaseURL) {
		return fmt.Errorf("provider %s: %s reranks only at a base URL of its own", p.Name, p.Kind)
	}
	seen := map[string]bool{}
	for _, m := range p.Embeddings {
		if strings.TrimSpace(m.Model) == "" || m.Dims < 1 || m.Dims > MaxDims {
			return fmt.Errorf("provider %s: embedding model %q needs a name and 1-%d dimensions", p.Name, m.Model, MaxDims)
		}
		if seen[m.Model] {
			return fmt.Errorf("provider %s: embedding model %q listed twice", p.Name, m.Model)
		}
		seen[m.Model] = true
	}
	return nil
}

// key returns a provider's key: the stored one, else the environment's.
func (s *Store) key(ctx context.Context, name string) (string, error) {
	var enc []byte
	var env string
	if err := s.Pool.QueryRow(ctx, `SELECT api_key_enc, api_key_env FROM llm_providers WHERE name = $1`, name).Scan(&enc, &env); err != nil {
		return "", err
	}
	if len(enc) > 0 {
		if s.Box == nil {
			return "", secret.ErrNoMasterKey
		}
		return s.Box.Open(enc)
	}
	if env != "" {
		return os.Getenv(env), nil
	}
	return "", nil
}

// resolveKey is the key for a provider being set up or tested: the one typed
// in the request, else the named environment variable's, else the key stored
// for a provider of the same name.
func (s *Store) resolveKey(ctx context.Context, p Provider) string {
	key := strings.TrimSpace(p.APIKey)
	if key == "" && p.APIKeyEnv != "" {
		key = os.Getenv(p.APIKeyEnv)
	}
	if key == "" && p.Name != "" {
		if k, err := s.key(ctx, p.Name); err == nil {
			key = k
		}
	}
	return key
}

// Instance builds the provider client a stored provider describes.
func (s *Store) Instance(ctx context.Context, p Provider) (llm.Provider, error) {
	key, err := s.key(ctx, p.Name)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", p.Name, err)
	}
	return instance(p, key)
}

func instance(p Provider, key string) (llm.Provider, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	switch p.Kind {
	case KindAnthropic:
		return &llm.Anthropic{ProviderName: p.Name, BaseURL: p.BaseURL, Key: key, HTTP: client}, nil
	case KindOpenAI:
		return &llm.OpenAI{ProviderName: p.Name, BaseURL: p.BaseURL, Key: key, HTTP: client}, nil
	case KindLuatdo:
		return llm.NewLuatdo(p.Name, p.BaseURL)
	}
	return nil, fmt.Errorf("provider %s: unknown kind %q", p.Name, p.Kind)
}

// Router builds a router from the stored settings. A provider that cannot be
// built is reported and left out; the roles keep their other targets.
func (s *Store) Router(ctx context.Context) (*llm.Router, []error, error) {
	return s.RouterWith(ctx, nil)
}

// RouterWith builds a router from the stored providers with some roles
// replaced, so an answer evaluation can try a configuration before it is
// saved. A role given with no targets is left out.
func (s *Store) RouterWith(ctx context.Context, roles map[string][]Target) (*llm.Router, []error, error) {
	st, err := s.Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	for role, ts := range roles {
		if len(ts) == 0 {
			delete(st.Roles, role)
		} else {
			st.Roles[role] = ts
		}
	}
	byName := map[string]Provider{}
	for _, p := range st.Providers {
		byName[p.Name] = p
	}
	instances := map[string]llm.Provider{}
	var problems []error
	r := llm.NewRouter()
	for role, targets := range st.Roles {
		var ts []llm.Target
		for _, t := range targets {
			inst, ok := instances[t.Provider]
			if !ok {
				p, found := byName[t.Provider]
				if !found {
					problems = append(problems, fmt.Errorf("role %s: no provider %q", role, t.Provider))
					continue
				}
				if inst, err = s.Instance(ctx, p); err != nil {
					problems = append(problems, err)
					continue
				}
				instances[t.Provider] = inst
			}
			ts = append(ts, llm.Target{Provider: inst, Model: t.Model})
		}
		r.Set(role, ts...)
	}
	return r, problems, nil
}

// TestResult is what a connection test found.
type TestResult struct {
	OK        bool    `json:"ok"`
	Text      string  `json:"text,omitempty"`
	Model     string  `json:"model,omitempty"`
	LatencyMS float64 `json:"latency_ms"`
	Kind      string  `json:"kind,omitempty"`
	Error     string  `json:"error,omitempty"`
	// Dims is the size of the vector an embedding model returned.
	Dims int `json:"dims,omitempty"`
	// Scores are what a reranker gave a relevant and an unrelated passage.
	Scores []float64 `json:"scores,omitempty"`
}

// TestRequest names a model to try. The provider may be one being set up and
// not saved yet; a provider given by name only is the stored one.
type TestRequest struct {
	Provider Provider `json:"provider"`
	Type     string   `json:"type"`
	Model    string   `json:"model"`
}

// TestModel sends one short request: a one-word chat, an embedding of one
// sentence, or a rerank of two passages.
func (s *Store) TestModel(ctx context.Context, in TestRequest) TestResult {
	p := in.Provider
	if p.Kind == "" {
		st, err := s.Load(ctx)
		if err != nil {
			return TestResult{Error: err.Error()}
		}
		i := slices.IndexFunc(st.Providers, func(x Provider) bool { return x.Name == p.Name })
		if i < 0 {
			return TestResult{Error: fmt.Sprintf("no provider %q", p.Name)}
		}
		p = st.Providers[i]
	}
	if in.Type == "" {
		in.Type = TypeChat
	}
	if !Supports(p.Kind, in.Type, p.BaseURL) {
		return TestResult{Error: fmt.Sprintf("%s has no %s models", p.Kind, in.Type)}
	}
	if strings.TrimSpace(in.Model) == "" {
		return TestResult{Error: "no model named"}
	}
	key := s.resolveKey(ctx, p)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	start := time.Now()
	r := TestResult{Model: in.Model}
	done := func(err error) TestResult {
		r.LatencyMS = float64(time.Since(start).Milliseconds())
		if err != nil {
			r.Error, r.Kind = err.Error(), llm.Kind(err)
			return r
		}
		r.OK = true
		return r
	}
	switch in.Type {
	case TypeEmbedding:
		// No size yet: the answer says what it is.
		emb, err := embedder(p, EmbeddingModel{Model: in.Model}, key)
		if err != nil {
			return done(err)
		}
		vecs, err := emb.Embed(ctx, []string{"Thủ tục hải quan đối với hàng hóa xuất khẩu"}, embed.Document)
		if err == nil && len(vecs) == 1 {
			r.Dims = len(vecs[0])
		}
		return done(err)
	case TypeRerank:
		rr, err := reranker(p, in.Model, key)
		if err != nil {
			return done(err)
		}
		r.Scores, err = rr.Rerank(ctx, "thủ tục hải quan hàng xuất khẩu", []string{
			"Điều 1. Thủ tục hải quan đối với hàng hóa xuất khẩu, nhập khẩu", "Dự báo thời tiết ngày mai có mưa rào"})
		return done(err)
	}
	inst, err := instance(p, key)
	if err != nil {
		r.Kind = llm.FailAuth
		return done(err)
	}
	resp, err := inst.Stream(ctx, llm.Request{Model: in.Model, MaxTokens: 16,
		Messages: []llm.Message{{Role: "user", Content: "Trả lời đúng một từ: OK"}}}, nil)
	if err == nil {
		r.Text, r.Model = strings.TrimSpace(resp.Text), resp.Model
	}
	return done(err)
}

// embedder builds the client for an embedding model of p. Dims of zero skips
// the size check, for a test.
func embedder(p Provider, m EmbeddingModel, key string) (embed.Embedder, error) {
	e := &config.Embedding{Name: p.Name + "/" + m.Model, Model: m.Model, Dims: m.Dims, BaseURL: p.BaseURL, Provider: "openai"}
	switch p.Kind {
	case KindVoyage:
		e.Provider = "voyage"
	case KindJina:
		if e.BaseURL == "" {
			e.BaseURL = "https://api.jina.ai/v1"
		}
	case KindOpenAI:
	default:
		return nil, fmt.Errorf("provider %s: %s has no embedding models", p.Name, p.Kind)
	}
	return embed.NewWithKey(e, key)
}

func reranker(p Provider, model, key string) (search.Reranker, error) {
	provider := p.Kind
	if p.Kind == KindOpenAI {
		provider = ""
	}
	return search.RerankerFor(provider, p.BaseURL, model, key)
}

// Models builds the embedding models and rerankers of the stored providers,
// named "provider/model". A provider whose key cannot be read is reported and
// left out.
func (s *Store) Models(ctx context.Context) (*search.Models, []error, error) {
	st, err := s.Load(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := search.NewModels()
	var problems []error
	for _, p := range st.Providers {
		if len(p.Embeddings) == 0 && len(p.Rerankers) == 0 {
			continue
		}
		key, err := s.key(ctx, p.Name)
		if err != nil {
			problems = append(problems, fmt.Errorf("provider %s: %w", p.Name, err))
			continue
		}
		for _, m := range p.Embeddings {
			emb, err := embedder(p, m, key)
			if err != nil {
				problems = append(problems, err)
				continue
			}
			out.AddEmbedder(emb, search.Source{Provider: p.Name, Model: m.Model})
		}
		for _, m := range p.Rerankers {
			rr, err := reranker(p, m, key)
			if err != nil {
				problems = append(problems, fmt.Errorf("provider %s: %w", p.Name, err))
				continue
			}
			out.AddReranker(p.Name+"/"+m, rr)
		}
	}
	return out, problems, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
