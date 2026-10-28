package embed

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StoreOptions bounds one run.
type StoreOptions struct {
	// Limit stops after this many chunks; zero means all. A benchmark on a
	// slice of the corpus costs a fraction of a full embed.
	Limit int
	// Docs restricts to chunks of these documents.
	Docs []string
	Log  io.Writer
}

// StoreResult is what a run did.
type StoreResult struct {
	Model    string `json:"model"`
	Pending  int    `json:"pending"`
	Embedded int    `json:"embedded"`
}

// pageSize is how many chunks are read, embedded and written per round, so
// an interrupted run keeps what it finished.
const pageSize = 256

// EmbedChunks embeds every chunk with no embedding under this model, or whose
// text changed since it was embedded, and makes sure the model's HNSW index
// exists.
func EmbedChunks(ctx context.Context, pool *pgxpool.Pool, e Embedder, provider, modelID string, opts StoreOptions) (*StoreResult, error) {
	var dims int
	err := pool.QueryRow(ctx, `INSERT INTO embedding_models (name, provider, model, dims) VALUES ($1, $2, $3, $4)
		ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING dims`, e.Name(), provider, modelID, e.Dims()).Scan(&dims)
	if err != nil {
		return nil, err
	}
	if dims != e.Dims() {
		return nil, fmt.Errorf("model %s is stored with %d dimensions and configured with %d; give the new size a new name", e.Name(), dims, e.Dims())
	}
	typ, ops := VectorType(dims)
	if _, err := pool.Exec(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON chunk_embeddings USING hnsw ((embedding::%s) %s) WHERE model = %s`,
		IndexName(e.Name()), typ, ops, Literal(e.Name()))); err != nil {
		return nil, fmt.Errorf("create index: %w", err)
	}

	where := `(e.chunk_id IS NULL OR e.content_hash <> c.content_hash)`
	args := []any{e.Name()}
	if len(opts.Docs) > 0 {
		where += ` AND c.document_id = ANY($2)`
		args = append(args, opts.Docs)
	}
	res := &StoreResult{Model: e.Name()}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM chunks c LEFT JOIN chunk_embeddings e ON e.chunk_id = c.id AND e.model = $1 WHERE `+where, args...).Scan(&res.Pending); err != nil {
		return nil, err
	}
	for opts.Limit == 0 || res.Embedded < opts.Limit {
		n := pageSize
		if opts.Limit > 0 {
			n = min(n, opts.Limit-res.Embedded)
		}
		rows, err := pool.Query(ctx, `SELECT c.id, c.content_hash, d.official_number, d.title, c.path, c.text
			FROM chunks c JOIN documents d ON d.id = c.document_id
			LEFT JOIN chunk_embeddings e ON e.chunk_id = c.id AND e.model = $1
			WHERE `+where+fmt.Sprintf(` ORDER BY c.id LIMIT %d`, n), args...)
		if err != nil {
			return nil, err
		}
		var ids, hashes, texts []string
		for rows.Next() {
			var id, hash, number, title, text string
			var path []string
			if err := rows.Scan(&id, &hash, &number, &title, &path, &text); err != nil {
				rows.Close()
				return nil, err
			}
			ids, hashes = append(ids, id), append(hashes, hash)
			texts = append(texts, Passage(number, title, path, text))
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			break
		}
		vecs, err := e.Embed(ctx, texts, Document)
		if err != nil {
			return res, err
		}
		batch := &pgx.Batch{}
		for i := range ids {
			batch.Queue(`INSERT INTO chunk_embeddings (chunk_id, model, content_hash, embedding) VALUES ($1, $2, $3, $4::vector)
				ON CONFLICT (chunk_id, model) DO UPDATE SET content_hash = EXCLUDED.content_hash, embedding = EXCLUDED.embedding, created_at = now()`,
				ids[i], e.Name(), hashes[i], VectorLiteral(vecs[i]))
		}
		if err := pool.SendBatch(ctx, batch).Close(); err != nil {
			return res, err
		}
		res.Embedded += len(ids)
		if opts.Log != nil {
			fmt.Fprintf(opts.Log, "embed %s: %d / %d\n", e.Name(), res.Embedded, res.Pending)
		}
	}
	return res, nil
}

// maxPassage bounds what is sent per chunk. Embedding APIs cap input at a few
// thousand tokens and a chunk is at most about 4,000 characters, so this only
// trims the rare outlier rather than every long chunk.
const maxPassage = 6000

// Passage is the text embedded for a chunk: the instrument and the heading
// path give the words of a clause the context they lose when cut out.
func Passage(number, title string, path []string, text string) string {
	s := number + " " + title + "\n" + strings.Join(path, " / ") + "\n" + text
	if r := []rune(s); len(r) > maxPassage {
		s = string(r[:maxPassage])
	}
	return s
}

// EmbedHS embeds the HS lines with no embedding under this model, keyed by
// their content so a re-ingest keeps the vectors.
func EmbedHS(ctx context.Context, pool *pgxpool.Pool, e Embedder, provider, modelID string, opts StoreOptions) (*StoreResult, error) {
	if _, err := pool.Exec(ctx, `INSERT INTO embedding_models (name, provider, model, dims) VALUES ($1, $2, $3, $4) ON CONFLICT (name) DO NOTHING`,
		e.Name(), provider, modelID, e.Dims()); err != nil {
		return nil, err
	}
	typ, ops := VectorType(e.Dims())
	if _, err := pool.Exec(ctx, fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON hs_embeddings USING hnsw ((embedding::%s) %s) WHERE model = %s`,
		strings.Replace(IndexName(e.Name()), "chunk_", "hs_", 1), typ, ops, Literal(e.Name()))); err != nil {
		return nil, fmt.Errorf("create index: %w", err)
	}
	where := `NOT EXISTS (SELECT 1 FROM hs_embeddings x WHERE x.content_hash = h.content_hash AND x.model = $1)`
	res := &StoreResult{Model: e.Name()}
	if err := pool.QueryRow(ctx, `SELECT count(DISTINCT content_hash) FROM hs_codes h WHERE `+where, e.Name()).Scan(&res.Pending); err != nil {
		return nil, err
	}
	for opts.Limit == 0 || res.Embedded < opts.Limit {
		rows, err := pool.Query(ctx, `SELECT DISTINCT ON (h.content_hash) h.content_hash, h.code, h.description_vi, h.description_en, coalesce(p.description_vi, '')
			FROM hs_codes h LEFT JOIN hs_codes p ON p.code = h.parent_code AND p.code <> ''
			WHERE `+where+` ORDER BY h.content_hash LIMIT 256`, e.Name())
		if err != nil {
			return nil, err
		}
		var hashes, texts []string
		for rows.Next() {
			var hash, code, vi, en, parent string
			if err := rows.Scan(&hash, &code, &vi, &en, &parent); err != nil {
				rows.Close()
				return nil, err
			}
			hashes = append(hashes, hash)
			// The parent's words give "Loại khác" a meaning of its own.
			texts = append(texts, strings.TrimSpace(code+" "+parent+" — "+vi+"\n"+en))
		}
		rows.Close()
		if len(hashes) == 0 {
			break
		}
		vecs, err := e.Embed(ctx, texts, Document)
		if err != nil {
			return res, err
		}
		batch := &pgx.Batch{}
		for i := range hashes {
			batch.Queue(`INSERT INTO hs_embeddings (content_hash, model, embedding) VALUES ($1, $2, $3::vector) ON CONFLICT DO NOTHING`,
				hashes[i], e.Name(), VectorLiteral(vecs[i]))
		}
		if err := pool.SendBatch(ctx, batch).Close(); err != nil {
			return res, err
		}
		res.Embedded += len(hashes)
		if opts.Log != nil {
			fmt.Fprintf(opts.Log, "embed hs %s: %d / %d\n", e.Name(), res.Embedded, res.Pending)
		}
	}
	return res, nil
}
