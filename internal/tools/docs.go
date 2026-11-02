package tools

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/refs"
	"github.com/nvtquang/CustomsVN/internal/search"
)

// Resolved is a reference found in text and where it leads.
type Resolved struct {
	Span        string `json:"span"`
	DocID       string `json:"doc_id,omitempty"`
	Number      string `json:"number,omitempty"`
	Title       string `json:"title,omitempty"`
	ProvisionID string `json:"provision_id,omitempty"`
	ChunkID     string `json:"chunk_id,omitempty"`
	Note        string `json:"note,omitempty"`
}

func (t *Toolset) resolve(p refs.Provision, asOf string) Resolved {
	c := t.Engine.Corpus()
	today := t.Engine.Now().Format(time.DateOnly)
	r := Resolved{Span: p.Span}
	d := c.ResolveDoc(p.Number, p.Name, asOf, today)
	if d == nil {
		r.Note = "văn bản không có trong corpus"
		return r
	}
	r.DocID, r.Number, r.Title, r.ProvisionID = d.ID, d.OfficialNumber, d.Title, d.ID+":"+p.Tail()
	var ch *search.Chunk
	if d.ConsolidatedIn != "" {
		ch = c.CoveringChunk(d.ConsolidatedIn, p.Tail())
	}
	if ch == nil {
		ch = c.CoveringChunk(d.ID, p.Tail())
	}
	if ch == nil {
		r.Note = "không có điều khoản này trong corpus"
		return r
	}
	r.ChunkID = ch.ID
	return r
}

func (t *Toolset) resolveReference(text, asOf string) (*Result, error) {
	found := refs.Parse(text)
	var out []Resolved
	for _, p := range found.Provisions {
		if p.Number == "" && p.Name == "" {
			out = append(out, Resolved{Span: p.Span, Note: "không nêu văn bản"})
			continue
		}
		out = append(out, t.resolve(p, asOf))
	}
	for _, n := range found.Numbers {
		c := t.Engine.Corpus()
		if d := c.ResolveDoc(n, "", asOf, t.Engine.Now().Format(time.DateOnly)); d != nil {
			out = append(out, Resolved{Span: n, DocID: d.ID, Number: d.OfficialNumber, Title: d.Title})
		} else {
			out = append(out, Resolved{Span: n, Note: "văn bản không có trong corpus (có thể đã hết hiệu lực)"})
		}
	}
	return &Result{Text: jsonText(out), Summary: fmt.Sprintf("%d tham chiếu", len(out))}, nil
}

func (t *Toolset) getProvision(reference, asOf string) (*Result, error) {
	found := refs.Parse(reference)
	if len(found.Provisions) == 0 {
		return nil, fmt.Errorf("get_provision: %q is not a provision reference", reference)
	}
	var hits []search.Hit
	var notes []string
	for _, p := range found.Provisions {
		r := t.resolve(p, asOf)
		if r.ChunkID == "" {
			notes = append(notes, p.Span+": "+firstNonEmpty(r.Note, "không tìm thấy"))
			continue
		}
		h := t.Engine.HitFor(r.ChunkID, asOf)
		if h != nil && !h.Usable && search.Usable(h.Force) {
			// In force, but an amendment rewrote this provision: point the
			// model at the amendment instead of letting it give up.
			notes = append(notes, p.Span+": đã được văn bản sửa đổi viết lại, hãy tìm nội dung mới trong văn bản sửa đổi (get_amendments)")
			continue
		}
		if h == nil || !h.Usable {
			notes = append(notes, p.Span+": văn bản không còn hiệu lực tại "+asOf)
			continue
		}
		hits = append(hits, *h)
	}
	text := hitsText(hits)
	if len(notes) > 0 {
		text += "\n" + strings.Join(notes, "\n")
	}
	return &Result{Text: text, Summary: fmt.Sprintf("%d điều khoản", len(hits)), Sources: hits}, nil
}

// Doc is a document as the status tool reports it.
type Doc struct {
	ID            string `json:"id"`
	Number        string `json:"number"`
	Title         string `json:"title"`
	DocType       string `json:"doc_type,omitempty"`
	IssuingBody   string `json:"issuing_body,omitempty"`
	SignedOn      string `json:"signed_on,omitempty"`
	EffectiveFrom string `json:"effective_from,omitempty"`
	ExpiredOn     string `json:"expired_on,omitempty"`
	ForceStatus   string `json:"force_status,omitempty"`
	SourceURL     string `json:"source_url,omitempty"`
	InCorpus      bool   `json:"in_corpus"`
}

// Status is a document's force on a day and what stands in for it.
type Status struct {
	Doc
	AsOf           string `json:"as_of"`
	Force          string `json:"force"` // in_force, partly_expired, not_yet_in_force, or "" for not in force
	ConsolidatedIn *Doc   `json:"consolidated_in,omitempty"`
	ReplacedBy     []Link `json:"replaced_by,omitempty"`
	// Note says why the force is what it is when the dates and the label
	// disagree.
	Note string `json:"note,omitempty"`
}

// Link is a related document and how it relates.
type Link struct {
	Doc
	Relation string `json:"relation"`
	Label    string `json:"label,omitempty"`
}

var errNotFound = errors.New("không tìm thấy văn bản")

// columns lists a registry row's fields for scanDoc, from the table
// aliased as a ("" for none). A reviewed override (force_overrides) wins
// over the source's force and expiry date, as it does in the corpus.
func columns(a string) string {
	t := "registry"
	if a != "" {
		t = a
		a += "."
	}
	override := func(col string) string {
		return "(SELECT o." + col + " FROM force_overrides o WHERE o.official_number = " + t + ".official_number)"
	}
	return a + "id, " + a + "official_number, " + a + "title, " + a + "doc_type, " + a + "issuing_body, " +
		"coalesce(" + a + "signed_on::text, ''), coalesce(" + a + "effective_from::text, ''), " +
		"coalesce(" + override("expired_on") + "::text, " + a + "expired_on::text, ''), " +
		"coalesce(" + override("force_status") + ", " + a + "force_status), " + a + "source_url"
}

func scanDoc(row pgx.Row) (Doc, error) {
	var d Doc
	err := row.Scan(&d.ID, &d.Number, &d.Title, &d.DocType, &d.IssuingBody, &d.SignedOn, &d.EffectiveFrom, &d.ExpiredOn, &d.ForceStatus, &d.SourceURL)
	return d, err
}

// findDoc resolves a document named by identifier, official number or law
// name. See findDocs for what a shared number prefers.
func (t *Toolset) findDoc(ctx context.Context, ref, asOf string) (Doc, error) {
	d, _, err := t.findDocs(ctx, ref, asOf)
	return d, err
}

// candidate is a registry row that shares a number with the one asked for.
type candidate struct {
	Doc
	tier   int
	hinted bool
	usable bool
}

// findDocs is findDoc that also returns the other documents sharing the
// number, so a reader is told when the card could be about another one.
//
// A number is not an identity: a VBHN number repeats across years and
// ministries (18/VBHN-BTC is both a 2023 text on cars of privileged persons
// and a 2026 one on VAT), and a provincial number repeats in every province.
// Among the documents sharing it, the one the answer is already working from
// (WithDocHints) comes first, then one in the corpus, then one in force on
// asOf, then the lowest tier, then the most recent.
func (t *Toolset) findDocs(ctx context.Context, ref, asOf string) (Doc, []Doc, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Doc{}, nil, errors.New("cần nêu văn bản")
	}
	c := t.Engine.Corpus()
	today := t.Engine.Now().Format(time.DateOnly)
	var id string
	switch {
	case strings.HasPrefix(ref, "vn:"):
		id = ref
	default:
		if f := refs.Parse(ref); len(f.Numbers) > 0 {
			ref = f.Numbers[0]
		} else if d := c.ResolveDoc("", ref, asOf, today); d != nil {
			id = d.ID
		}
	}
	var cands []candidate
	q, arg := `SELECT `+columns("")+`, tier FROM registry WHERE number_norm = $1 LIMIT 40`, any(corpus.NormNumber(ref))
	if id != "" {
		q, arg = `SELECT `+columns("")+`, tier FROM registry WHERE id = $1`, any(id)
	}
	rows, err := t.Pool.Query(ctx, q, arg)
	if err != nil {
		return Doc{}, nil, err
	}
	defer rows.Close()
	hints := docHints(ctx)
	for rows.Next() {
		var k candidate
		d := &k.Doc
		if err := rows.Scan(&d.ID, &d.Number, &d.Title, &d.DocType, &d.IssuingBody, &d.SignedOn, &d.EffectiveFrom,
			&d.ExpiredOn, &d.ForceStatus, &d.SourceURL, &k.tier); err != nil {
			return Doc{}, nil, err
		}
		d.InCorpus = c.Docs[d.ID] != nil
		k.hinted = hints[d.ID]
		f, _ := t.forceOf(*d, asOf)
		k.usable = search.Usable(f)
		cands = append(cands, k)
	}
	if err := rows.Err(); err != nil {
		return Doc{}, nil, err
	}
	if len(cands) == 0 {
		return Doc{}, nil, fmt.Errorf("%w: %s", errNotFound, ref)
	}
	rankCandidates(cands)
	var others []Doc
	for _, k := range cands[1:] {
		others = append(others, k.Doc)
	}
	return cands[0].Doc, others, nil
}

// rankCandidates puts the document a bare number most likely means first.
func rankCandidates(cands []candidate) {
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		switch {
		case a.hinted != b.hinted:
			return a.hinted
		case a.InCorpus != b.InCorpus:
			return a.InCorpus
		case a.usable != b.usable:
			return a.usable
		case a.tier != b.tier:
			return a.tier < b.tier
		}
		return a.SignedOn > b.SignedOn
	})
}

// forceOf is a document's force on asOf, and a sentence saying where that
// force comes from when it is not the document's own label.
//
// A consolidated text (VBHN) has the force of the instrument it consolidates:
// vbpl.vn labels almost every VBHN "Chưa xác định", and reading that label as
// "not in force" showed 18/VBHN-BTC as ended under citations the corpus holds
// as in force. This is the rule the corpus applies to its chunks (Corpus.Force).
func (t *Toolset) forceOf(d Doc, asOf string) (force, note string) {
	c := t.Engine.Corpus()
	today := t.Engine.Now().Format(time.DateOnly)
	if cd := c.Docs[d.ID]; cd != nil {
		force = c.Force(cd, asOf, today)
		if cd.SourceKind != corpus.SourceConsolidated {
			if e, gone := c.EndedBy(cd.ID, asOf); gone {
				return force, endedNote(c, e)
			}
			return force, ""
		}
		base := c.Docs[cd.Consolidates]
		if base == nil {
			return force, "Văn bản hợp nhất; văn bản gốc không có trong kho nên chưa xác định được hiệu lực."
		}
		if e, gone := c.EndedBy(base.ID, asOf); gone {
			return force, "Văn bản hợp nhất của " + base.OfficialNumber + ". " + endedNote(c, e)
		}
		return force, "Văn bản hợp nhất: hiệu lực theo văn bản gốc " + base.OfficialNumber + " (" + forceVI[force] + ")."
	}
	m := corpus.Meta{ForceStatus: d.ForceStatus, EffectiveFrom: d.EffectiveFrom, ExpiredOn: d.ExpiredOn}
	return search.ForceAt(m, asOf, today), ""
}

// endedNote says which text replaced a document as a whole and from when.
func endedNote(c *search.Corpus, e search.Ended) string {
	by := e.By
	if d := c.Docs[e.By]; d != nil {
		by = d.OfficialNumber
	}
	return "Đã hết hiệu lực toàn bộ từ " + e.On + ": bị " + by + " thay thế hoặc bãi bỏ, dù nguồn vẫn ghi nhãn cũ."
}

// sameNumberNote lists the other documents that carry the number the card is
// about, so the reader can see it might be a different one.
func sameNumberNote(others []Doc) string {
	if len(others) == 0 {
		return ""
	}
	var parts []string
	for i, o := range others {
		if i == 3 {
			parts = append(parts, fmt.Sprintf("và %d văn bản khác", len(others)-i))
			break
		}
		year := o.SignedOn
		if len(year) >= 4 {
			year = year[:4]
		}
		parts = append(parts, "ký năm "+year+": "+truncate(o.Title, 90))
	}
	return "Còn văn bản khác cùng số hiệu " + others[0].Number + " (" + strings.Join(parts, "; ") + ")."
}

func (t *Toolset) documentStatus(ctx context.Context, ref, asOf string) (*Result, error) {
	d, others, err := t.findDocs(ctx, ref, asOf)
	if err != nil {
		return nil, err
	}
	st := Status{Doc: d, AsOf: asOf}
	st.Force, st.Note = t.forceOf(d, asOf)
	if label := strings.TrimSpace(d.ForceStatus); label == corpus.StatusInForce && st.Force == "" && d.ExpiredOn != "" {
		st.Note = "Nguồn vẫn ghi \"Còn hiệu lực\" nhưng ngày hết hiệu lực " + d.ExpiredOn + " đã qua."
	}
	if n := sameNumberNote(others); n != "" {
		st.Note = strings.TrimSpace(st.Note + " " + n)
	}
	if cd := t.Engine.Corpus().Docs[d.ID]; cd != nil && cd.ConsolidatedIn != "" {
		if v, err := scanDoc(t.Pool.QueryRow(ctx, `SELECT `+columns("")+` FROM registry WHERE id = $1`, cd.ConsolidatedIn)); err == nil {
			v.InCorpus = true
			st.ConsolidatedIn = &v
		}
	}
	links, err := t.links(ctx, `e.to_doc = $1 AND e.relation IN ('replaces', 'repeals', 'expires')`, "e.from_doc", d.ID, 20)
	if err != nil {
		return nil, err
	}
	st.ReplacedBy = links
	summary := d.Number + ": " + firstNonEmpty(forceVI[st.Force], "không còn hiệu lực")
	return &Result{Text: jsonText(st), Summary: summary, Card: &Card{Type: "status", Data: st}}, nil
}

// links lists the documents related to id by the edges where cond holds,
// joined on the other end (other is e.from_doc or e.to_doc).
func (t *Toolset) links(ctx context.Context, cond, other, id string, limit int) ([]Link, error) {
	q := `SELECT DISTINCT ON (r.id) ` + columns("r") + `, e.relation, e.label
		FROM edges e JOIN registry r ON r.id = ` + other + `
		WHERE ` + cond + ` ORDER BY r.id LIMIT ` + fmt.Sprint(limit*4)
	rows, err := t.Pool.Query(ctx, q, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	c := t.Engine.Corpus()
	var out []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ID, &l.Number, &l.Title, &l.DocType, &l.IssuingBody, &l.SignedOn, &l.EffectiveFrom, &l.ExpiredOn,
			&l.ForceStatus, &l.SourceURL, &l.Relation, &l.Label); err != nil {
			return nil, err
		}
		l.InCorpus = c.Docs[l.ID] != nil
		out = append(out, l)
	}
	sort.SliceStable(out, func(i, j int) bool { return dateOf(out[i].Doc) < dateOf(out[j].Doc) })
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, rows.Err()
}

func dateOf(d Doc) string { return firstNonEmpty(d.EffectiveFrom, d.SignedOn) }

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// Event is one point on a document's timeline.
type Event struct {
	Date    string `json:"date"`
	Kind    string `json:"kind"` // issued, effective, amends, replaces, repeals, expires, suspends, corrects, consolidates, ended
	Doc     Doc    `json:"doc"`
	Label   string `json:"label,omitempty"`
	Undated bool   `json:"undated,omitempty"`
}

// Timeline is a document's amendment history.
type Timeline struct {
	Doc    Doc     `json:"doc"`
	AsOf   string  `json:"as_of"`
	Force  string  `json:"force"`
	Events []Event `json:"events"`
}

func (t *Toolset) amendments(ctx context.Context, ref, asOf string) (*Result, error) {
	d, err := t.findDoc(ctx, ref, asOf)
	if err != nil {
		return nil, err
	}
	tl := Timeline{Doc: d, AsOf: asOf}
	tl.Force, _ = t.forceOf(d, asOf)
	if d.SignedOn != "" {
		tl.Events = append(tl.Events, Event{Date: d.SignedOn, Kind: "issued", Doc: d})
	}
	if d.EffectiveFrom != "" {
		tl.Events = append(tl.Events, Event{Date: d.EffectiveFrom, Kind: "effective", Doc: d})
	}
	links, err := t.links(ctx, `e.to_doc = $1 AND e.relation IN ('amends', 'replaces', 'repeals', 'expires', 'suspends', 'corrects', 'consolidates')`, "e.from_doc", d.ID, 60)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		date := dateOf(l.Doc)
		tl.Events = append(tl.Events, Event{Date: date, Kind: l.Relation, Doc: l.Doc, Label: l.Label, Undated: date == ""})
	}
	if d.ExpiredOn != "" {
		tl.Events = append(tl.Events, Event{Date: d.ExpiredOn, Kind: "ended", Doc: d})
	}
	sort.SliceStable(tl.Events, func(i, j int) bool { return tl.Events[i].Date < tl.Events[j].Date })
	summary := fmt.Sprintf("%s: %d mốc", d.Number, len(tl.Events))
	return &Result{Text: jsonText(tl), Summary: summary, Card: &Card{Type: "amendments", Data: tl}}, nil
}

// graphQueries maps a query name to the edge condition and the end to join.
var graphQueries = map[string]struct{ cond, other string }{
	"amended_by":      {`e.to_doc = $1 AND e.relation IN ('amends', 'corrects', 'suspends')`, "e.from_doc"},
	"amends":          {`e.from_doc = $1 AND e.relation IN ('amends', 'corrects', 'suspends', 'replaces', 'repeals', 'expires')`, "e.to_doc"},
	"detailed_by":     {`e.to_doc = $1 AND e.relation = 'details'`, "e.from_doc"},
	"details":         {`e.from_doc = $1 AND e.relation = 'details'`, "e.to_doc"},
	"cited_by":        {`e.to_doc = $1 AND e.kind = 'cites' AND e.relation IN ('cites', '')`, "e.from_doc"},
	"cites":           {`e.from_doc = $1 AND e.kind = 'cites' AND e.relation IN ('cites', '')`, "e.to_doc"},
	"replaced_by":     {`e.to_doc = $1 AND e.relation IN ('replaces', 'repeals', 'expires')`, "e.from_doc"},
	"consolidated_in": {`e.to_doc = $1 AND e.relation = 'consolidates'`, "e.from_doc"},
}

// Graph is the answer to a graph query.
type Graph struct {
	Query string `json:"query"`
	Doc   Doc    `json:"doc"`
	Rows  []Link `json:"rows"`
}

func (t *Toolset) graphQuery(ctx context.Context, query, ref string) (*Result, error) {
	g, ok := graphQueries[query]
	if !ok {
		return nil, fmt.Errorf("graph_query: unknown query %q", query)
	}
	d, err := t.findDoc(ctx, ref, "")
	if err != nil {
		return nil, err
	}
	rows, err := t.links(ctx, g.cond, g.other, d.ID, 50)
	if err != nil {
		return nil, err
	}
	out := Graph{Query: query, Doc: d, Rows: rows}
	return &Result{Text: jsonText(out), Summary: fmt.Sprintf("%s %s: %d", query, d.Number, len(rows)), Card: &Card{Type: "graph", Data: out}}, nil
}

// MissingAmendment is an amendment of a document whose own text is not in
// the corpus, so the amended wording cannot be quoted.
type MissingAmendment struct {
	Amended       string // official number of the amended document
	By            string // official number of the amending one
	EffectiveFrom string
}

// MissingAmendments lists, for the given corpus documents, the amendments in
// force on asOf that the corpus has no text for.
func (t *Toolset) MissingAmendments(ctx context.Context, docIDs []string, asOf string) ([]MissingAmendment, error) {
	if len(docIDs) == 0 {
		return nil, nil
	}
	rows, err := t.Pool.Query(ctx, `SELECT DISTINCT d.official_number, r.official_number, r.effective_from::text
		FROM edges e
		JOIN documents d ON d.id = e.to_doc
		JOIN registry r ON r.id = e.from_doc
		WHERE e.to_doc = ANY($1) AND e.kind = 'amends' AND e.relation = 'amends'
		  AND r.effective_from IS NOT NULL AND r.effective_from <= $2::date
		  AND r.force_status = 'Còn hiệu lực'
		  AND NOT EXISTS (SELECT 1 FROM documents x WHERE x.id = r.id)
		ORDER BY 1, 3`, docIDs, asOf)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MissingAmendment
	for rows.Next() {
		var m MissingAmendment
		if err := rows.Scan(&m.Amended, &m.By, &m.EffectiveFrom); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
