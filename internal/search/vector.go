package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nvtquang/CustomsVN/internal/embed"
)

func (e *Engine) vector(ctx context.Context, emb embed.Embedder, query string, docs []string, k int) ([]ranked, error) {
	vecs, err := emb.Embed(ctx, []string{query}, embed.Query)
	if err != nil {
		return nil, err
	}
	return VectorSearch(ctx, e.Pool, emb.Name(), emb.Dims(), vecs[0], docs, k)
}

// VectorSearch ranks the chunks of docs by cosine similarity to v.
func VectorSearch(ctx context.Context, pool *pgxpool.Pool, model string, dims int, v []float32, docs []string, k int) ([]ranked, error) {
	typ, _ := embed.VectorType(dims)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The scope filter runs after the index walk; iterative scan keeps
	// walking until k rows pass it instead of returning a short list.
	// An older pgvector does not know the setting; a savepoint keeps that
	// failure from aborting the transaction.
	for _, s := range []string{`SET LOCAL hnsw.ef_search = 200`, `SET LOCAL hnsw.iterative_scan = relaxed_order`} {
		sp, err := tx.Begin(ctx)
		if err != nil {
			return nil, err
		}
		if _, err := sp.Exec(ctx, s); err != nil {
			_ = sp.Rollback(ctx)
			continue
		}
		if err := sp.Commit(ctx); err != nil {
			return nil, err
		}
	}
	dist := fmt.Sprintf("(e.embedding::%s) <=> $1::%s", typ, typ)
	q := fmt.Sprintf(`SELECT e.chunk_id, 1 - (%s) FROM chunk_embeddings e JOIN chunks c ON c.id = e.chunk_id
		WHERE e.model = %s AND c.document_id = ANY($2) ORDER BY %s LIMIT $3`, dist, embed.Literal(model), dist)
	rows, err := tx.Query(ctx, q, embed.VectorLiteral(v), docs, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ranked
	for rows.Next() {
		var r ranked
		if err := rows.Scan(&r.id, &r.score); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
