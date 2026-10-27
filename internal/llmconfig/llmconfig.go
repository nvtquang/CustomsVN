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
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/llm"
	"github.com/nvtquang/CustomsVN/internal/secret"
)

// Provider kinds.
const (
	KindAnthropic = "anthropic"
	KindOpenAI    = "openai" // any OpenAI-compatible /chat/completions
	KindLuatdo    = "luatdo" // a luatdo routes file; base_url is its path
)

// Provider is a provider as the settings screen sees it. APIKey is write-only:
// it is read from a save and never filled on a load.
type Provider struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	BaseURL   string   `json:"base_url"`
	APIKeyEnv string   `json:"api_key_env"`
	Models    []string `json:"models"`
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

// Settings is everything the LLM settings screen edits.
type Settings struct {
	Providers []Provider          `json:"providers"`
	Roles     map[string][]Target `json:"roles"`
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
	rows, err := s.Pool.Query(ctx, `SELECT name, kind, base_url, api_key_env, api_key_enc, models FROM llm_providers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	st := &Settings{Providers: []Provider{}, Roles: map[string][]Target{}, MasterKey: s.Box != nil}
	for rows.Next() {
		var p Provider
		var enc []byte
		var models []byte
		if err := rows.Scan(&p.Name, &p.Kind, &p.BaseURL, &p.APIKeyEnv, &enc, &models); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(models, &p.Models)
		if p.Models == nil {
			p.Models = []string{}
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
	return st, nil
}

// Save writes providers and roles. A provider left out is deleted. A key is
// replaced only when a new one is given, and removed only on ClearKey.
func (s *Store) Save(ctx context.Context, in *Settings) error {
	seen := map[string]bool{}
	for _, p := range in.Providers {
		if err := validate(p); err != nil {
			return err
		}
		if seen[p.Name] {
			return fmt.Errorf("provider %q is listed twice", p.Name)
		}
		seen[p.Name] = true
		if p.APIKey != "" && s.Box == nil {
			return secret.ErrNoMasterKey
		}
	}
	for role, targets := range in.Roles {
		if role != llm.RoleAnswer && role != llm.RoleRewrite {
			return fmt.Errorf("unknown role %q", role)
		}
		for _, t := range targets {
			if !seen[t.Provider] || strings.TrimSpace(t.Model) == "" {
				return fmt.Errorf("role %s names provider %q and model %q: both must exist", role, t.Provider, t.Model)
			}
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
		if _, err := tx.Exec(ctx, `INSERT INTO llm_providers (name, kind, base_url, api_key_env, models) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (name) DO UPDATE SET kind = EXCLUDED.kind, base_url = EXCLUDED.base_url, api_key_env = EXCLUDED.api_key_env,
			models = EXCLUDED.models, updated_at = now()`, p.Name, p.Kind, p.BaseURL, p.APIKeyEnv, models); err != nil {
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
	return tx.Commit(ctx)
}

func validate(p Provider) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("a provider needs a name")
	}
	switch p.Kind {
	case KindAnthropic, KindOpenAI, KindLuatdo:
	default:
		return fmt.Errorf("provider %s: kind must be anthropic, openai or luatdo", p.Name)
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

// Instance builds the provider client a stored provider describes.
func (s *Store) Instance(ctx context.Context, p Provider) (llm.Provider, error) {
	key, err := s.key(ctx, p.Name)
	if err != nil {
		return nil, fmt.Errorf("provider %s: %w", p.Name, err)
	}
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
	st, err := s.Load(ctx)
	if err != nil {
		return nil, nil, err
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
}

// Test sends one short request to a stored provider.
func (s *Store) Test(ctx context.Context, provider, model string) TestResult {
	st, err := s.Load(ctx)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	for _, p := range st.Providers {
		if p.Name != provider {
			continue
		}
		inst, err := s.Instance(ctx, p)
		if err != nil {
			return TestResult{Error: err.Error(), Kind: llm.FailAuth}
		}
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		start := time.Now()
		resp, err := inst.Stream(ctx, llm.Request{Model: model, MaxTokens: 16,
			Messages: []llm.Message{{Role: "user", Content: "Trả lời đúng một từ: OK"}}}, nil)
		r := TestResult{LatencyMS: float64(time.Since(start).Milliseconds())}
		if err != nil {
			r.Error, r.Kind = err.Error(), llm.Kind(err)
			return r
		}
		r.OK, r.Text, r.Model = true, strings.TrimSpace(resp.Text), resp.Model
		return r
	}
	return TestResult{Error: fmt.Sprintf("no provider %q", provider)}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
