package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/nvtquang/CustomsVN/internal/config"
)

func TestOpenAIBatchesAndRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		if r.URL.Path != "/embeddings" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var data []item
		// Answer out of order: the client must place vectors by index.
		for i := len(body.Input) - 1; i >= 0; i-- {
			data = append(data, item{Index: i, Embedding: []float32{float32(len(body.Input[i])), 0}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer srv.Close()
	t.Setenv("TEST_KEY", "k")
	e, err := New(&config.Embedding{Name: "t", Provider: "openai", Model: "m", Dims: 2, BaseURL: srv.URL, APIKeyEnv: "TEST_KEY", Batch: 2, RPM: 6000})
	if err != nil {
		t.Fatal(err)
	}
	vecs, err := e.Embed(context.Background(), []string{"a", "bb", "ccc"}, Document)
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 3 || vecs[0][0] != 1 || vecs[1][0] != 2 || vecs[2][0] != 3 {
		t.Errorf("got %v", vecs)
	}
	if calls.Load() != 3 { // one 429, then two batches
		t.Errorf("calls %d", calls.Load())
	}
}

func TestDimensionMismatchIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[1,2,3]}]}`))
	}))
	defer srv.Close()
	e, _ := New(&config.Embedding{Name: "t", Provider: "openai", Model: "m", Dims: 2, BaseURL: srv.URL, Batch: 8, RPM: 6000})
	if _, err := e.Embed(context.Background(), []string{"a"}, Query); err == nil {
		t.Error("want an error")
	}
}

func TestClientErrorIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "bad key", http.StatusUnauthorized)
	}))
	defer srv.Close()
	e, _ := New(&config.Embedding{Name: "t", Provider: "voyage", Model: "m", Dims: 2, BaseURL: srv.URL, Batch: 8, RPM: 6000})
	if _, err := e.Embed(context.Background(), []string{"a"}, Query); err == nil || calls.Load() != 1 {
		t.Errorf("err %v, calls %d", err, calls.Load())
	}
}

func TestGeminiShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/gemini-embedding-001:batchEmbedContents" || r.Header.Get("x-goog-api-key") != "g" {
			t.Errorf("request %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"embeddings":[{"values":[0.5,0.5]}]}`))
	}))
	defer srv.Close()
	t.Setenv("G_KEY", "g")
	e, _ := New(&config.Embedding{Name: "g", Provider: "gemini", Model: "gemini-embedding-001", Dims: 2, BaseURL: srv.URL, APIKeyEnv: "G_KEY", Batch: 8, RPM: 6000})
	vecs, err := e.Embed(context.Background(), []string{"x"}, Query)
	if err != nil || len(vecs) != 1 {
		t.Fatalf("%v %v", vecs, err)
	}
}
