// Package ingest loads the output of customsvn prep into PostgreSQL.
//
// Documents and chunks are compared by content hash with what the database
// already holds, and only what changed is written. A run that changes
// anything records a new corpus version; one that changes nothing records
// none, so a retrieval run's corpus version names exactly the data it saw.
// Edges and the HS nomenclature are derived wholesale and are replaced on
// every run.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tamnd/luatdo/cite"
	"github.com/tamnd/luatdo/law"
	"github.com/tamnd/luatdo/parse"
	"github.com/tamnd/luatdo/store"

	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/prep"
)

// Options says where the inputs are.
type Options struct {
	Prep string // customsvn prep output
	Data string // luatdo data directory, for provisions and citations
	Log  io.Writer

	revision string // the th1nhng0 revision prep read, from its summary
}

// Result is what one ingest did.
type Result struct {
	Version   int64 `json:"corpus_version"`
	Unchanged bool  `json:"unchanged"`
	Documents int   `json:"documents"`
	Chunks    int   `json:"chunks"`
	Added     int   `json:"added"`
	Changed   int   `json:"changed"`
	Removed   int   `json:"removed"`
	Edges     int   `json:"edges"`
	HSCodes   int   `json:"hs_codes"`
	Registry  int   `json:"registry"`
}

type docRow struct {
	ID, OfficialNumber, NumberNorm, DocType, Title, IssuingBody string
	SignedOn, EffectiveFrom, ExpiredOn                          *time.Time
	ForceStatus, Status                                         string
	Tier                                                        int
	SourceKind, SourceURL                                       string
	Subjects                                                    []string
	Consolidates, ConsolidatedIn                                *string
	AmendedBy                                                   []string
	TextSource                                                  *string
	provisions                                                  []law.Provision
	hash                                                        string
}

// Run ingests the prep output.
func Run(ctx context.Context, pool *pgxpool.Pool, opts Options) (*Result, error) {
	logf := func(f string, a ...any) {
		if opts.Log != nil {
			fmt.Fprintf(opts.Log, f+"\n", a...)
		}
	}
	// Each phase says how long it took, so a job log shows where an ingest
	// is, and a canceled job stops between phases rather than at the end.
	started := time.Now()
	phase := func(name string) error {
		logf("ingest: %s (%s)", name, time.Since(started).Round(time.Second))
		return ctx.Err()
	}
	var summary struct {
		AsOf         string `json:"as_of"`
		Revision     string `json:"revision"`
		ChunksSHA256 string `json:"chunks_sha256"`
	}
	if err := store.ReadJSON(filepath.Join(opts.Prep, "summary", "chunks.json"), &summary); err != nil {
		return nil, fmt.Errorf("read prep summary (run customsvn prep first): %w", err)
	}
	// The revision prep read, a crawl's or the pinned snapshot; a summary
	// written before prep recorded it was read from the snapshot.
	if summary.Revision == "" {
		summary.Revision = prep.Revision
	}
	opts.revision = summary.Revision

	chunks, err := readChunks(filepath.Join(opts.Prep, "customs", "chunks.jsonl"))
	if err != nil {
		return nil, err
	}
	docs, err := documents(ctx, opts, chunks)
	if err != nil {
		return nil, err
	}
	if err := phase(fmt.Sprintf("%d documents, %d chunks read", len(docs), len(chunks))); err != nil {
		return nil, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// One ingest at a time: the diff below is only right if nobody writes
	// between reading the hashes and writing the rows.
	if _, err := tx.Exec(ctx, `LOCK TABLE corpus_versions IN EXCLUSIVE MODE`); err != nil {
		return nil, err
	}

	oldDocs, err := hashes(ctx, tx, `SELECT id, content_hash FROM documents`)
	if err != nil {
		return nil, err
	}
	oldChunks, err := hashes(ctx, tx, `SELECT id, content_hash FROM chunks`)
	if err != nil {
		return nil, err
	}

	res := &Result{Documents: len(docs), Chunks: len(chunks)}
	var docWrite []*docRow
	for _, d := range docs {
		if oldDocs[d.ID] != d.hash {
			docWrite = append(docWrite, d)
		}
	}
	current := make(map[string]bool, len(docs))
	for _, d := range docs {
		current[d.ID] = true
	}
	var docRemove []string
	for id := range oldDocs {
		if !current[id] {
			docRemove = append(docRemove, id)
		}
	}
	chunkHash := make(map[string]string, len(chunks))
	var chunkWrite []*corpus.Chunk
	for i := range chunks {
		h := hashOf(chunks[i])
		chunkHash[chunks[i].ID] = h
		switch old, ok := oldChunks[chunks[i].ID]; {
		case !ok:
			res.Added++
			chunkWrite = append(chunkWrite, &chunks[i])
		case old != h:
			res.Changed++
			chunkWrite = append(chunkWrite, &chunks[i])
		}
	}
	var chunkRemove []string
	for id := range oldChunks {
		if _, ok := chunkHash[id]; !ok {
			chunkRemove = append(chunkRemove, id)
		}
	}
	res.Removed = len(chunkRemove)

	if len(docWrite) == 0 && len(docRemove) == 0 && len(chunkWrite) == 0 && len(chunkRemove) == 0 {
		res.Unchanged = true
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(id), 0) FROM corpus_versions`).Scan(&res.Version); err != nil {
			return nil, err
		}
	} else {
		err := tx.QueryRow(ctx, `INSERT INTO corpus_versions (as_of, source_revision, chunks_sha256, documents, chunks, added, changed, removed)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			summary.AsOf, summary.Revision, summary.ChunksSHA256, len(docs), len(chunks), res.Added, res.Changed, res.Removed).Scan(&res.Version)
		if err != nil {
			return nil, err
		}
		if err := writeDocuments(ctx, tx, docWrite, docRemove, res.Version); err != nil {
			return nil, err
		}
		if err := writeChunks(ctx, tx, chunkWrite, chunkRemove, chunkHash, res.Version); err != nil {
			return nil, err
		}
		logf("ingest: corpus version %d: %d documents written, %d removed; chunks +%d ~%d -%d",
			res.Version, len(docWrite), len(docRemove), res.Added, res.Changed, res.Removed)
	}

	if err := phase("documents and chunks written"); err != nil {
		return nil, err
	}
	if res.Edges, err = writeEdges(ctx, tx, opts, docs); err != nil {
		return nil, err
	}
	if err := phase(fmt.Sprintf("%d relations written", res.Edges)); err != nil {
		return nil, err
	}
	if res.HSCodes, err = writeHS(ctx, tx, opts); err != nil {
		return nil, err
	}
	if err := phase(fmt.Sprintf("%d HS lines written", res.HSCodes)); err != nil {
		return nil, err
	}
	if res.Registry, err = writeRegistry(ctx, tx, opts, docs); err != nil {
		return nil, err
	}
	if err := phase(fmt.Sprintf("%d registry documents written", res.Registry)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return res, nil
}

func readChunks(path string) ([]corpus.Chunk, error) {
	rows, err := store.ReadJSONL[corpus.Chunk](path)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, fmt.Errorf("%s is missing: run customsvn prep first", path)
	}
	return rows, nil
}

// documents gathers every document a chunk belongs to, with its metadata and
// provision tree.
func documents(ctx context.Context, opts Options, chunks []corpus.Chunk) ([]*docRow, error) {
	first := map[string]*corpus.Chunk{}
	var order []string
	for i := range chunks {
		if _, ok := first[chunks[i].DocID]; !ok {
			first[chunks[i].DocID] = &chunks[i]
			order = append(order, chunks[i].DocID)
		}
	}
	domain := map[string]*prep.Entry{}
	if err := eachJSONL(filepath.Join(opts.Prep, "domain.jsonl"), func(e prep.Entry) {
		if _, ok := first[e.ID]; ok {
			domain[e.ID] = &e
		}
	}); err != nil {
		return nil, err
	}
	parsed := map[string]*prep.Parsed{}
	for _, dir := range []string{prep.StepVBHN, prep.StepRescue} {
		paths, err := filepath.Glob(filepath.Join(opts.Prep, dir, "*.json"))
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			var x prep.Parsed
			if err := store.ReadJSON(p, &x); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
			if _, ok := first[x.Doc.ID]; ok {
				parsed[x.Doc.ID] = &x
			}
		}
	}

	out := make([]*docRow, 0, len(order))
	for _, id := range order {
		// Reading every store document takes most of a minute; a canceled
		// job should not wait for it.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c := first[id]
		d := &docRow{ID: id, Tier: c.Tier, SourceKind: c.Source.Kind, Subjects: nonNil(c.Subjects), AmendedBy: []string{}}
		var m corpus.Meta
		switch p, e := parsed[id], domain[id]; {
		case p != nil && c.Source.Kind != corpus.SourceOriginal:
			m = metaOf(p.Doc)
			d.Status = p.Doc.Status
			d.provisions = p.Doc.Provisions
			if p.TextSource != "" {
				d.TextSource = &p.TextSource
			}
		case e != nil:
			m = e.Meta
			d.Status = e.Status
			var full law.Document
			if err := store.ReadJSON(filepath.Join(opts.Data, "docs", law.FileName(id)), &full); err != nil {
				return nil, fmt.Errorf("read store document %s: %w", id, err)
			}
			d.provisions = full.Provisions
		default:
			return nil, fmt.Errorf("chunk document %s is in neither domain.jsonl nor the parsed outputs", id)
		}
		if c.Source.Consolidates != nil && c.Source.Consolidates.BaseDocID != "" {
			b := c.Source.Consolidates.BaseDocID
			d.Consolidates = &b
			d.AmendedBy = nonNil(c.Source.Consolidates.AmendedBy)
		}
		if c.Source.ConsolidatedIn != "" {
			ci := c.Source.ConsolidatedIn
			d.ConsolidatedIn = &ci
		}
		d.OfficialNumber, d.NumberNorm, d.DocType, d.Title = m.OfficialNumber, corpus.NormNumber(m.OfficialNumber), m.DocType, m.Title
		d.IssuingBody, d.ForceStatus, d.SourceURL = m.IssuingBody, m.ForceStatus, m.SourceURL
		d.SignedOn, d.EffectiveFrom, d.ExpiredOn = date(m.SignedOn), date(m.EffectiveFrom), date(m.ExpiredOn)
		d.hash = hashOf(struct {
			Doc  *docRow
			Prov []law.Provision
		}{d, d.provisions})
		out = append(out, d)
	}
	return out, nil
}

func writeDocuments(ctx context.Context, tx pgx.Tx, rows []*docRow, remove []string, version int64) error {
	if len(remove) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM documents WHERE id = ANY($1)`, remove); err != nil {
			return err
		}
	}
	if len(rows) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE documents_in (LIKE documents INCLUDING DEFAULTS) ON COMMIT DROP`); err != nil {
		return err
	}
	cols := []string{"id", "official_number", "number_norm", "doc_type", "title", "issuing_body", "signed_on", "effective_from",
		"expired_on", "force_status", "status", "tier", "source_kind", "source_url", "subjects", "consolidates", "consolidated_in",
		"amended_by", "text_source", "content_hash", "corpus_version"}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"documents_in"}, cols, pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
		d := rows[i]
		return []any{d.ID, d.OfficialNumber, d.NumberNorm, d.DocType, d.Title, d.IssuingBody, d.SignedOn, d.EffectiveFrom,
			d.ExpiredOn, d.ForceStatus, d.Status, d.Tier, d.SourceKind, d.SourceURL, d.Subjects, d.Consolidates, d.ConsolidatedIn,
			d.AmendedBy, d.TextSource, d.hash, version}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy documents: %w", err)
	}
	if _, err := tx.Exec(ctx, upsert("documents", cols)); err != nil {
		return err
	}

	ids := make([]string, len(rows))
	n := 0
	for i, d := range rows {
		ids[i] = d.ID
		n += len(d.provisions)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM provisions WHERE document_id = ANY($1)`, ids); err != nil {
		return err
	}
	flat := make([][]any, 0, n)
	seen := map[string]bool{}
	for _, d := range rows {
		for _, p := range d.provisions {
			if seen[p.ID] {
				continue // a provision identifier repeated across two documents' trees
			}
			seen[p.ID] = true
			flat = append(flat, []any{p.ID, d.ID, nullable(p.ParentID), p.Kind, p.Number, p.Heading, p.Text, p.TextHash, p.Position})
		}
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"provisions"},
		[]string{"id", "document_id", "parent_id", "kind", "number", "heading", "text", "text_hash", "position"}, pgx.CopyFromRows(flat))
	if err != nil {
		return fmt.Errorf("copy provisions: %w", err)
	}
	return nil
}

func writeChunks(ctx context.Context, tx pgx.Tx, rows []*corpus.Chunk, remove []string, hash map[string]string, version int64) error {
	if len(remove) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM chunks WHERE id = ANY($1)`, remove); err != nil {
			return err
		}
	}
	if len(rows) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE chunks_in (LIKE chunks INCLUDING DEFAULTS) ON COMMIT DROP`); err != nil {
		return err
	}
	cols := []string{"id", "document_id", "path", "text", "chunk_kind", "tier", "source_kind", "force", "subjects",
		"amendment_notes", "consolidated_in", "rescue_reason", "needs_review", "text_source", "content_hash", "corpus_version"}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"chunks_in"}, cols, pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
		c := rows[i]
		kind := c.Kind
		if kind == "" {
			kind = "text"
		}
		return []any{c.ID, c.DocID, nonNil(c.Path), c.Text, kind, c.Tier, c.Source.Kind, c.Force, nonNil(c.Subjects),
			nonNil(c.AmendmentNotes), nullable(c.ConsolidatedIn), nullable(c.RescueReason), c.NeedsReview,
			nullable(c.TextSource), hash[c.ID], version}, nil
	}))
	if err != nil {
		return fmt.Errorf("copy chunks: %w", err)
	}
	_, err = tx.Exec(ctx, upsert("chunks", cols))
	return err
}

// upsert moves rows from <table>_in into table, replacing rows by id.
func upsert(table string, cols []string) string {
	q := "INSERT INTO " + table + " SELECT * FROM " + table + "_in ON CONFLICT (id) DO UPDATE SET "
	for i, c := range cols[1:] {
		if i > 0 {
			q += ", "
		}
		q += c + " = EXCLUDED." + c
	}
	return q
}

// writeEdges replaces the link graph: vbpl.vn's official relations between
// all documents (so an amending act outside the corpus still appears on the
// timeline of the act it amended), each consolidated text's link to its base,
// and the citations luatdo found in the text of corpus documents.
func writeEdges(ctx context.Context, tx pgx.Tx, opts Options, docs []*docRow) (int, error) {
	if _, err := tx.Exec(ctx, `TRUNCATE edges`); err != nil {
		return 0, err
	}
	var rows [][]any
	numbers := map[string]string{}
	for _, d := range docs {
		numbers[d.ID] = d.OfficialNumber
		if d.Consolidates != nil {
			rows = append(rows, []any{d.ID, nil, *d.Consolidates, d.OfficialNumber, "cites", "vbhn", corpus.RelConsolidates, "Hợp nhất"})
		}
	}
	rels, _, err := parse.Th1nhng0Relations(filepath.Join(opts.Data, "raw", "th1nhng0", opts.revision))
	if err != nil {
		return 0, fmt.Errorf("official relations: %w", err)
	}
	have := map[string]bool{} // from|to of every acting edge already known
	for _, r := range rels {
		rel := corpus.Relation(r.Label)
		rows = append(rows, []any{r.FromDoc, nil, r.ToDoc, numbers[r.ToDoc], r.Kind, "vbpl", rel, r.Label})
		if corpus.Acts(rel) {
			have[r.FromDoc+"|"+r.ToDoc] = true
		}
	}
	// vbpl.vn's relation table misses links its own titles state: a decree
	// titled "Sửa đổi, bổ sung một số điều của Nghị định số 08/2015/NĐ-CP"
	// amends 08/2015 whether or not the table says so. Those links are read
	// from the titles, and from the "được sửa đổi, bổ sung bởi" list of each
	// consolidated text, and added only where vbpl.vn has none.
	var entries []prep.Entry
	if err := eachJSONL(filepath.Join(opts.Prep, "domain.jsonl"), func(e prep.Entry) { entries = append(entries, e) }); err != nil {
		return 0, err
	}
	known := make(map[string]bool, len(entries))
	for _, e := range entries {
		known[e.ID] = true
	}
	add := func(from, to, number, rel, method, label string) {
		if from == to || !known[to] && !isDoc(docs, to) || have[from+"|"+to] {
			return
		}
		have[from+"|"+to] = true
		rows = append(rows, []any{from, nil, to, number, "amends", method, rel, label})
	}
	for _, e := range entries {
		for _, l := range corpus.TitleLinks(e.Title) {
			add(e.ID, corpus.StoreID(l.Number), l.Number, l.Relation, "title", l.Phrase)
		}
	}
	for _, d := range docs {
		if d.Consolidates == nil {
			continue
		}
		for _, n := range d.AmendedBy {
			add(corpus.StoreID(n), *d.Consolidates, numbers[*d.Consolidates], corpus.RelAmends, "vbhn", "được sửa đổi, bổ sung bởi")
		}
	}
	for _, d := range docs {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if d.SourceKind == corpus.SourceConsolidated {
			continue
		}
		var links []cite.Link
		data, err := os.ReadFile(filepath.Join(opts.Data, "cite", law.FileName(d.ID)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return 0, err
		}
		if err := json.Unmarshal(data, &links); err != nil {
			return 0, fmt.Errorf("citations of %s: %w", d.ID, err)
		}
		for _, l := range links {
			// Official links are already in, from the relations above.
			if l.Method == "official" {
				continue
			}
			rows = append(rows, []any{l.FromDoc, nullable(l.FromProvision), nullable(l.ToDoc), l.ToNumber, l.Kind, l.Method, "", ""})
		}
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"edges"}, []string{"from_doc", "from_provision", "to_doc", "to_number", "kind", "method", "relation", "label"},
		pgx.CopyFromRows(rows))
	return len(rows), err
}

func isDoc(docs []*docRow, id string) bool {
	for _, d := range docs {
		if d.ID == id {
			return true
		}
	}
	return false
}

// writeRegistry replaces the status table with every store document and
// every consolidated text in the corpus.
func writeRegistry(ctx context.Context, tx pgx.Tx, opts Options, docs []*docRow) (int, error) {
	if _, err := tx.Exec(ctx, `TRUNCATE registry`); err != nil {
		return 0, err
	}
	var rows [][]any
	seen := map[string]bool{}
	if err := eachJSONL(filepath.Join(opts.Prep, "domain.jsonl"), func(e prep.Entry) {
		seen[e.ID] = true
		rows = append(rows, []any{e.ID, e.OfficialNumber, corpus.NormNumber(e.OfficialNumber), e.DocType, e.Title, e.IssuingBody,
			date(e.SignedOn), date(e.EffectiveFrom), date(e.ExpiredOn), e.ForceStatus, e.Status, e.Tier, e.SourceURL})
	}); err != nil {
		return 0, err
	}
	for _, d := range docs {
		if seen[d.ID] {
			continue
		}
		rows = append(rows, []any{d.ID, d.OfficialNumber, d.NumberNorm, d.DocType, d.Title, d.IssuingBody,
			d.SignedOn, d.EffectiveFrom, d.ExpiredOn, d.ForceStatus, d.Status, d.Tier, d.SourceURL})
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"registry"}, []string{"id", "official_number", "number_norm", "doc_type", "title", "issuing_body",
		"signed_on", "effective_from", "expired_on", "force_status", "status", "tier", "source_url"}, pgx.CopyFromRows(rows))
	return len(rows), err
}

// HSHash identifies an HS line by what it says, so its embedding survives a
// re-ingest that renumbers the rows.
func HSHash(r corpus.HSRow) string {
	return hashOf([]any{r.Code, r.Level, r.DescriptionVI, r.DescriptionEN})
}

func writeHS(ctx context.Context, tx pgx.Tx, opts Options) (int, error) {
	rows, err := store.ReadJSONL[corpus.HSRow](filepath.Join(opts.Prep, "hs", "hs_31-2022.jsonl"))
	if err != nil || rows == nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `TRUNCATE hs_codes`); err != nil {
		return 0, err
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"hs_codes"},
		[]string{"code", "level", "description_vi", "description_en", "unit", "chapter", "parent_code", "parent_description", "source_doc", "position", "content_hash"},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			r := rows[i]
			return []any{r.Code, r.Level, r.DescriptionVI, r.DescriptionEN, r.Unit, r.Chapter, r.ParentCode, r.ParentDescription, prep.HSDocID, i, HSHash(r)}, nil
		}))
	return len(rows), err
}

func hashes(ctx context.Context, tx pgx.Tx, q string) (map[string]string, error) {
	rows, err := tx.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for rows.Next() {
		var id, h string
		if err := rows.Scan(&id, &h); err != nil {
			return nil, err
		}
		out[id] = h
	}
	return out, rows.Err()
}

func hashOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // only plain structs reach here
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func eachJSONL[T any](path string, visit func(T)) error {
	rows, err := store.ReadJSONL[T](path)
	if err != nil {
		return err
	}
	if rows == nil {
		return fmt.Errorf("%s is missing: run customsvn prep first", path)
	}
	for _, r := range rows {
		visit(r)
	}
	return nil
}

func metaOf(d *law.Document) corpus.Meta {
	return corpus.Meta{
		ID: d.ID, OfficialNumber: d.OfficialNumber, IssuingBody: d.IssuingBody, Title: d.Title, DocType: d.DocType,
		EffectiveFrom: d.EffectiveFrom, SignedOn: d.SignedOn, ExpiredOn: d.ExpiredOn, ForceStatus: d.ForceStatus,
		SourceURL: d.SourceURL, Status: d.Status,
	}
}

func date(s string) *time.Time {
	t, err := time.Parse(time.DateOnly, corpus.ISODate(s))
	if err != nil {
		return nil
	}
	return &t
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return slices.Clip(s)
}
