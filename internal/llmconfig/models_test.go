package llmconfig

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestListModels(t *testing.T) {
	var gotAuth, gotKey, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotKey, gotVersion = r.Header.Get("Authorization"), r.Header.Get("x-api-key"), r.Header.Get("anthropic-version")
		switch r.URL.Path {
		case "/v1/models", "/models":
			_, _ = w.Write([]byte(`{"data": [{"id": "model-b"}, {"id": "model-a"}, {"id": ""}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s := &Store{}
	ctx := context.Background()

	ids, err := s.ListModels(ctx, Provider{Kind: KindOpenAI, BaseURL: srv.URL + "/", APIKey: "sk-test"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []string{"model-a", "model-b"}) || gotAuth != "Bearer sk-test" {
		t.Errorf("openai: %v auth %q", ids, gotAuth)
	}

	t.Setenv("CVN_TEST_KEY", "ant-key")
	ids, err = s.ListModels(ctx, Provider{Kind: KindAnthropic, BaseURL: srv.URL, APIKeyEnv: "CVN_TEST_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || gotKey != "ant-key" || gotVersion == "" {
		t.Errorf("anthropic: %v key %q version %q", ids, gotKey, gotVersion)
	}

	if _, err := s.ListModels(ctx, Provider{Kind: KindOpenAI, BaseURL: srv.URL + "/nowhere", APIKey: "x"}); err == nil {
		t.Error("a 404 is not an error")
	}
	if _, err := s.ListModels(ctx, Provider{Kind: KindLuatdo}); err == nil {
		t.Error("luatdo lists models")
	}
}
