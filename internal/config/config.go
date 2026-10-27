// Package config reads customsvn.yaml.
//
// The file holds no secrets. A provider's API key is named by the environment
// variable that holds it (api_key_env), and the database URL, which carries a
// password, comes only from CUSTOMSVN_DATABASE_URL. Unknown keys are an error,
// so an "api_key:" pasted into the file is refused rather than silently read.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DatabaseEnv is the variable that holds the PostgreSQL URL.
const DatabaseEnv = "CUSTOMSVN_DATABASE_URL"

// DefaultFile is read from the working directory when no path is given.
const DefaultFile = "customsvn.yaml"

// Config is the whole configuration.
type Config struct {
	// Listen is the HTTP address of serve.
	Listen string `yaml:"listen"`
	// Data is the luatdo data directory. Empty means LUATDO_DATA.
	Data string `yaml:"data"`
	// Prep is the output of customsvn prep. Empty means <data>/customsvn/prep.
	Prep string `yaml:"prep"`
	// Embedding lists the embedding models that can be computed and searched.
	Embedding []Embedding `yaml:"embedding"`
	// Rerank is the optional reranker. Off until it earns its place on the gold set.
	Rerank *Rerank `yaml:"rerank"`

	// DatabaseURL comes from the environment, never from the file.
	DatabaseURL string `yaml:"-"`
}

// Embedding is one embedding model.
type Embedding struct {
	// Name is how runs and settings refer to the model, e.g. "openai-3-large".
	Name string `yaml:"name"`
	// Provider is openai (any OpenAI-compatible /embeddings), voyage or gemini.
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	Dims     int    `yaml:"dims"`
	BaseURL  string `yaml:"base_url"`
	// APIKeyEnv names the environment variable holding the key.
	APIKeyEnv string `yaml:"api_key_env"`
	// Batch is how many texts go in one request. Zero means 64.
	Batch int `yaml:"batch"`
	// RPM caps requests per minute. Zero means 60.
	RPM int `yaml:"rpm"`
}

// Rerank is a cross-encoder reranker behind a /rerank endpoint (Cohere, Jina
// and Voyage share the shape).
type Rerank struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	BaseURL   string `yaml:"base_url"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// Load reads path, or DefaultFile when path is empty. A missing default file
// is not an error: everything has a default.
func Load(path string) (*Config, error) {
	c := &Config{}
	explicit := path != ""
	if !explicit {
		path = DefaultFile
	}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist) && !explicit:
	case err != nil:
		return nil, fmt.Errorf("read config: %w", err)
	default:
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(c); err != nil && !errors.Is(err, io.EOF) { // an empty file is fine
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:8080"
	}
	if c.Data == "" {
		c.Data = os.Getenv("LUATDO_DATA")
	}
	if c.Prep == "" && c.Data != "" {
		c.Prep = filepath.Join(c.Data, "customsvn", "prep")
	}
	c.DatabaseURL = os.Getenv(DatabaseEnv)
	seen := map[string]bool{}
	for i := range c.Embedding {
		e := &c.Embedding[i]
		if e.Name == "" || e.Model == "" || e.Dims <= 0 {
			return nil, fmt.Errorf("%s: embedding %d needs name, model and dims", path, i+1)
		}
		if seen[e.Name] {
			return nil, fmt.Errorf("%s: embedding %q listed twice", path, e.Name)
		}
		seen[e.Name] = true
		if e.Batch <= 0 {
			e.Batch = 64
		}
		if e.RPM <= 0 {
			e.RPM = 60
		}
	}
	return c, nil
}

// EmbeddingModel returns the named model.
func (c *Config) EmbeddingModel(name string) (*Embedding, error) {
	for i := range c.Embedding {
		if c.Embedding[i].Name == name {
			return &c.Embedding[i], nil
		}
	}
	return nil, fmt.Errorf("no embedding model %q in the config", name)
}

// RequireDatabase returns the database URL or says where to set it.
func (c *Config) RequireDatabase() (string, error) {
	if c.DatabaseURL == "" {
		return "", fmt.Errorf("set %s, e.g. postgres://customsvn:customsvn@localhost:55432/customsvn", DatabaseEnv)
	}
	return c.DatabaseURL, nil
}

// APIKey reads a key from the variable an entry names.
func APIKey(env string) (string, error) {
	if env == "" {
		return "", nil
	}
	k := os.Getenv(env)
	if k == "" {
		return "", fmt.Errorf("environment variable %s is empty", env)
	}
	return k, nil
}
