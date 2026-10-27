package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExampleLoads(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "customsvn.example.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Embedding) != 3 || c.Embedding[0].APIKeyEnv != "OPENAI_API_KEY" || c.Embedding[1].Batch != 64 {
		t.Errorf("got %+v", c.Embedding)
	}
}

func TestAKeyInTheFileIsRefused(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.yaml")
	data := "embedding:\n  - name: x\n    model: m\n    dims: 8\n    api_key: sk-secret\n"
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Errorf("got %v", err)
	}
}

func TestDatabaseURLComesFromTheEnvironment(t *testing.T) {
	t.Setenv(DatabaseEnv, "postgres://x")
	t.Chdir(t.TempDir()) // no customsvn.yaml here
	c, err := Load("")
	if err != nil || c.DatabaseURL != "postgres://x" || c.Listen == "" {
		t.Errorf("%+v %v", c, err)
	}
}
