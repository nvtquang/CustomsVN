package embed

import (
	"fmt"
	"strconv"
	"strings"
)

// maxVectorDims is pgvector's limit for an HNSW index on vector; past it the
// index is built on a halfvec cast, which allows twice as many.
const maxVectorDims = 2000

// VectorType is the type an embedding of dims dimensions is cast to, for
// its index and for every query against it. The index and the query must use
// the same expression, or the planner falls back to a scan.
func VectorType(dims int) (typ, ops string) {
	if dims > maxVectorDims {
		return fmt.Sprintf("halfvec(%d)", dims), "halfvec_cosine_ops"
	}
	return fmt.Sprintf("vector(%d)", dims), "vector_cosine_ops"
}

// IndexName is the HNSW index of one model.
func IndexName(model string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(model) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return "chunk_embeddings_hnsw_" + b.String()
}

// Literal quotes a string for SQL. Model names go into the query text, not a
// parameter, so the planner can match the partial index on them.
func Literal(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// VectorLiteral writes a vector the way pgvector reads it.
func VectorLiteral(v []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
