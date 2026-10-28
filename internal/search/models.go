package search

import (
	"slices"

	"github.com/nvtquang/CustomsVN/internal/embed"
)

// Models are the embedding models and rerankers searches can use. They come
// from the config file and from the model settings screen, and are replaced
// as a whole when the settings change, while searches run.
type Models struct {
	Embedders map[string]embed.Embedder
	// Sources says which provider and model id each embedder calls, for the
	// embedding_models table.
	Sources   map[string]Source
	Rerankers map[string]Reranker
	// DefaultModel and DefaultReranker are used when a search names none:
	// the first of each, config file first.
	DefaultModel, DefaultReranker string
}

// Source is the provider and model id behind a configured name.
type Source struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// NewModels returns an empty set.
func NewModels() *Models {
	return &Models{Embedders: map[string]embed.Embedder{}, Sources: map[string]Source{}, Rerankers: map[string]Reranker{}}
}

// AddEmbedder adds an embedder unless its name is taken; the first added is
// the default.
func (m *Models) AddEmbedder(e embed.Embedder, src Source) bool {
	if _, ok := m.Embedders[e.Name()]; ok {
		return false
	}
	m.Embedders[e.Name()], m.Sources[e.Name()] = e, src
	if m.DefaultModel == "" {
		m.DefaultModel = e.Name()
	}
	return true
}

// AddReranker adds a reranker unless its name is taken.
func (m *Models) AddReranker(name string, r Reranker) bool {
	if _, ok := m.Rerankers[name]; ok {
		return false
	}
	m.Rerankers[name] = r
	if m.DefaultReranker == "" {
		m.DefaultReranker = name
	}
	return true
}

// Merge returns m with other's models added after its own.
func (m *Models) Merge(other *Models) *Models {
	out := NewModels()
	for _, src := range []*Models{m, other} {
		if src == nil {
			continue
		}
		for _, name := range src.EmbeddingNames() {
			out.AddEmbedder(src.Embedders[name], src.Sources[name])
		}
		for _, name := range src.RerankNames() {
			out.AddReranker(name, src.Rerankers[name])
		}
	}
	return out
}

// Embedder returns the named embedder, or the default for "".
func (m *Models) Embedder(name string) embed.Embedder {
	if name == "" {
		name = m.DefaultModel
	}
	return m.Embedders[name]
}

// Reranker returns the named reranker, or the default for "".
func (m *Models) Reranker(name string) Reranker {
	if name == "" {
		name = m.DefaultReranker
	}
	return m.Rerankers[name]
}

// EmbeddingNames lists the embedders, the default first.
func (m *Models) EmbeddingNames() []string { return names(m.Embedders, m.DefaultModel) }

// RerankNames lists the rerankers, the default first.
func (m *Models) RerankNames() []string { return names(m.Rerankers, m.DefaultReranker) }

func names[V any](set map[string]V, first string) []string {
	out := make([]string, 0, len(set))
	for n := range set {
		if n != first {
			out = append(out, n)
		}
	}
	slices.Sort(out)
	if _, ok := set[first]; ok {
		out = append([]string{first}, out...)
	}
	return out
}
