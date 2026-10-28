package llmconfig

import "testing"

func TestValidateModelTypes(t *testing.T) {
	for _, c := range []struct {
		p  Provider
		ok bool
	}{
		{Provider{Name: "openai", Kind: KindOpenAI, Models: []string{"gpt"}, Embeddings: []EmbeddingModel{{"text-embedding-3-large", 3072}}}, true},
		{Provider{Name: "voyage", Kind: KindVoyage, Embeddings: []EmbeddingModel{{"voyage-3-large", 1024}}, Rerankers: []string{"rerank-2"}}, true},
		{Provider{Name: "cohere", Kind: KindCohere, Rerankers: []string{"rerank-v3.5"}}, true},
		{Provider{Name: "local", Kind: KindOpenAI, BaseURL: "http://tei:8080", Rerankers: []string{"bge-reranker"}}, true},
		{Provider{Name: "openai", Kind: KindOpenAI, Rerankers: []string{"x"}}, false}, // no rerank at OpenAI itself
		{Provider{Name: "anthropic", Kind: KindAnthropic, Embeddings: []EmbeddingModel{{"x", 8}}}, false},
		{Provider{Name: "voyage", Kind: KindVoyage, Models: []string{"chat"}}, false},
		{Provider{Name: "openai", Kind: KindOpenAI, Embeddings: []EmbeddingModel{{"big", 8000}}}, false}, // too big to index
		{Provider{Name: "a/b", Kind: KindOpenAI}, false},
	} {
		if err := validate(c.p); (err == nil) != c.ok {
			t.Errorf("%+v: %v", c.p, err)
		}
	}
}
