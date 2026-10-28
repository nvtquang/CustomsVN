package corpus

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/luatdo/law"

	"github.com/nvtquang/CustomsVN/internal/htmltable"
)

// Source kinds, in the order a retriever should trust them.
const (
	// SourceConsolidated is a VBHN whose base is still in force.
	SourceConsolidated = "consolidated"
	// SourceOriginal is an in-force document of the store. When a VBHN
	// consolidates it, ConsolidatedIn says which, because its wording may be
	// stale.
	SourceOriginal = "original"
	// SourceRescued is an in-force document luatdo quarantined on a false
	// alarm or because it has no article structure.
	SourceRescued = "rescued"
)

const (
	// MaxArticle is the length past which an article is split at its clauses.
	MaxArticle = 4000
	// MaxPiece is the length past which a chunk that is still too long, a
	// table or a schedule, is cut at line boundaries.
	MaxPiece = 2500
	// MaxParagraphs is the size of a chunk cut from a text with no structure.
	MaxParagraphs = 1500
	// maxNotes caps the amendment footnotes carried on one chunk.
	maxNotes = 12
)

var kindVI = map[string]string{
	"chapter": "Chương", "section": "Mục", "article": "Điều", "clause": "Khoản", "point": "Điểm", "annex": "Phụ lục",
}

// Consolidates is what a consolidated chunk says about the text it replaces.
type Consolidates struct {
	Base      string   `json:"base"`
	BaseDocID string   `json:"base_doc_id"`
	AmendedBy []string `json:"amended_by"`
}

// Source is what every chunk of one document carries besides its text.
type Source struct {
	Kind           string        `json:"source_kind"`
	Tier           int           `json:"tier"`
	Force          string        `json:"force"`
	Subjects       []string      `json:"subjects"`
	Consolidates   *Consolidates `json:"consolidates,omitempty"`
	ConsolidatedIn string        `json:"consolidated_in,omitempty"`
	RescueReason   string        `json:"rescue_reason,omitempty"`
	NeedsReview    bool          `json:"needs_review,omitempty"`
	// TextSource is set when the text is not from the dataset's main
	// content config.
	TextSource string `json:"text_source,omitempty"`
}

// Chunk is one retrievable unit: an article, a clause of a long article, or a
// piece of a long table, with its heading path and its document's force.
type Chunk struct {
	ID             string   `json:"id"`
	DocID          string   `json:"doc_id"`
	OfficialNumber string   `json:"official_number"`
	DocTitle       string   `json:"doc_title"`
	DocType        string   `json:"doc_type"`
	IssuingBody    string   `json:"issuing_body"`
	EffectiveFrom  string   `json:"effective_from"`
	SignedOn       string   `json:"signed_on"`
	Path           []string `json:"path"`
	Text           string   `json:"text"`
	SourceURL      string   `json:"source_url"`
	// Kind is "table" for a table rendered as Markdown; empty for text.
	Kind string `json:"chunk_kind,omitempty"`
	Source
	AmendmentNotes []string `json:"amendment_notes,omitempty"`
}

// Chunking modes.
const (
	// ChunkGeneral is an article per chunk, split at its clauses past
	// MaxLength characters.
	ChunkGeneral = "general"
	// ChunkParentChild makes every clause of an article its own chunk, the
	// child a search matches; a search puts the article, the parent, back
	// together around it for the model to read.
	ChunkParentChild = "parent_child"
)

// Chunking is how documents are cut, set on the knowledge screen. Prep
// applies it, so a change needs a new prep and ingest. The separators are
// not a setting: a chunk follows the article and clause structure, because
// every answer cites an article, clause or point.
type Chunking struct {
	Mode string `json:"mode"`
	// MaxLength is the length past which a general article is split.
	MaxLength int `json:"max_length,omitempty"`
	// ParentLength caps the article put back together around a child, in
	// characters.
	ParentLength int `json:"parent_length,omitempty"`
}

// Chunking defaults.
const (
	DefaultParentLength = 6000
	MinChunkLength      = 1000
	MaxChunkLength      = 12000
)

// Fill returns c with its defaults set.
func (c Chunking) Fill() Chunking {
	if c.Mode == "" {
		c.Mode = ChunkGeneral
	}
	if c.MaxLength <= 0 {
		c.MaxLength = MaxArticle
	}
	if c.ParentLength <= 0 {
		c.ParentLength = DefaultParentLength
	}
	return c
}

// Validate says what is wrong with c, if anything.
func (c Chunking) Validate() error {
	c = c.Fill()
	if c.Mode != ChunkGeneral && c.Mode != ChunkParentChild {
		return fmt.Errorf("chunking mode must be %s or %s", ChunkGeneral, ChunkParentChild)
	}
	for _, n := range []int{c.MaxLength, c.ParentLength} {
		if n < MinChunkLength || n > MaxChunkLength {
			return fmt.Errorf("chunk lengths must be %d-%d characters", MinChunkLength, MaxChunkLength)
		}
	}
	return nil
}

// Chunker turns documents into chunks and counts what it did.
type Chunker struct {
	Emit  func(Chunk) error
	Stats map[string]int
	// Chunking is how to cut; the zero value is the general mode.
	Chunking Chunking
}

// NewChunker returns a chunker that hands every chunk to emit.
func NewChunker(emit func(Chunk) error) *Chunker {
	return &Chunker{Emit: emit, Stats: map[string]int{}}
}

func (c *Chunker) emit(doc *Meta, src *Source, id string, path []string, text string, notes []string) error {
	return c.emitKind(doc, src, id, path, text, notes, "")
}

func (c *Chunker) emitKind(doc *Meta, src *Source, id string, path []string, text string, notes []string, kind string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	pieces := []string{text}
	if utf8.RuneCountInString(text) > MaxArticle {
		// A table or a schedule with no clauses to split at. Every piece keeps
		// the heading path, so a hit on any of them still says where it is.
		pieces = cutLines(text, MaxPiece)
		c.Stats["chunks_cut_long"]++
	}
	for n, piece := range pieces {
		c.Stats["chunks"]++
		c.Stats["chunks_"+src.Kind]++
		c.Stats[fmt.Sprintf("chunks_tier%d", src.Tier)]++
		cid := id
		if len(pieces) > 1 {
			cid = fmt.Sprintf("%s#part-%d", id, n+1)
		}
		if err := c.Emit(Chunk{
			ID: cid, DocID: doc.ID, OfficialNumber: doc.OfficialNumber, DocTitle: doc.Title,
			DocType: doc.DocType, IssuingBody: doc.IssuingBody,
			EffectiveFrom: ISODate(doc.EffectiveFrom), SignedOn: ISODate(doc.SignedOn),
			Path: path, Text: piece, SourceURL: doc.SourceURL, Kind: kind, Source: *src, AmendmentNotes: notes,
		}); err != nil {
			return err
		}
	}
	return nil
}

// cutLines cuts text at line boundaries into pieces of about max characters.
// A single line longer than max stays whole: cutting inside a line would cut
// inside a sentence or a table row.
func cutLines(text string, max int) []string {
	var pieces []string
	var cur strings.Builder
	n := 0
	for line := range strings.SplitSeq(text, "\n") {
		l := utf8.RuneCountInString(line)
		if n > 0 && n+l > max {
			pieces = append(pieces, cur.String())
			cur.Reset()
			n = 0
		}
		if n > 0 {
			cur.WriteByte('\n')
			n++
		}
		cur.WriteString(line)
		n += l
	}
	if n > 0 {
		pieces = append(pieces, cur.String())
	}
	return pieces
}

var noteRef = regexp.MustCompile(`\[(\d+)\]`)

// Tree chunks a parsed document: one chunk per article with its clauses and
// points, split at clauses when the article is too long. notes are the
// footnotes of a consolidated text, attached to the chunks that cite them.
func (c *Chunker) Tree(doc *Meta, provisions []law.Provision, src *Source, notes map[string]string) error {
	pathOf := headingPaths(doc.ID, provisions)
	refs := func(text string) []string {
		if len(notes) == 0 {
			return nil
		}
		var found []string
		for _, m := range noteRef.FindAllStringSubmatch(text, -1) {
			if n, ok := notes[m[1]]; ok {
				found = append(found, n)
			}
		}
		return head(found, maxNotes)
	}

	var tops []*law.Provision
	for i := range provisions {
		if k := provisions[i].Kind; k == "article" || k == "annex" {
			tops = append(tops, &provisions[i])
		}
	}
	if len(tops) == 0 {
		// Chapters with no articles, or clauses with no article above them.
		for i := range provisions {
			if p := provisions[i].ParentID; p == "" || p == doc.ID {
				tops = append(tops, &provisions[i])
			}
		}
	}
	opts := c.Chunking.Fill()
	for _, top := range tops {
		kids := under(provisions, top.ID, tops)
		whole := joinText(top.Text, "", kids)
		if len(kids) == 0 || opts.Mode != ChunkParentChild && utf8.RuneCountInString(whole) <= opts.MaxLength {
			if err := c.emit(doc, src, top.ID, pathOf(top.ID), whole, refs(whole)); err != nil {
				return err
			}
			continue
		}
		if top.Text != "" {
			if err := c.emit(doc, src, top.ID, pathOf(top.ID), top.Text, refs(top.Text)); err != nil {
				return err
			}
		}
		var clauses []*law.Provision
		for _, k := range kids {
			if k.Kind == "clause" {
				clauses = append(clauses, k)
			}
		}
		if len(clauses) == 0 {
			clauses = kids
		}
		for _, cl := range clauses {
			sub := joinText(cl.Text, cl.ID, kids)
			if err := c.emit(doc, src, cl.ID, pathOf(cl.ID), sub, refs(sub)); err != nil {
				return err
			}
		}
	}
	return nil
}

// headingPaths returns a function giving the heading path of a provision:
// "Chương II. ...", "Điều 18. ...", down to the provision itself.
func headingPaths(docID string, provisions []law.Provision) func(id string) []string {
	byID := make(map[string]*law.Provision, len(provisions))
	for i := range provisions {
		byID[provisions[i].ID] = &provisions[i]
	}
	return func(id string) []string {
		parts := strings.Split(strings.TrimPrefix(id, docID+":"), ":")
		labels := []string{}
		for i := 1; i <= len(parts); i++ {
			p := byID[docID+":"+strings.Join(parts[:i], ":")]
			if p == nil || kindVI[p.Kind] == "" {
				continue
			}
			label := strings.TrimSpace(kindVI[p.Kind] + " " + p.Number)
			if p.Heading != "" {
				label += ". " + p.Heading
			}
			labels = append(labels, label)
		}
		return labels
	}
}

// under returns the descendants of id in document order, leaving out any other
// top and everything below it. An annex that carries a regulation has
// articles of its own; each of them is its own chunk, and counting them again
// inside the annex would give two chunks one identifier.
func under(provisions []law.Provision, id string, tops []*law.Provision) []*law.Provision {
	var nested []string
	for _, t := range tops {
		if strings.HasPrefix(t.ID, id+":") {
			nested = append(nested, t.ID)
		}
	}
	var out []*law.Provision
next:
	for i := range provisions {
		p := provisions[i].ID
		if !strings.HasPrefix(p, id+":") {
			continue
		}
		for _, n := range nested {
			if p == n || strings.HasPrefix(p, n+":") {
				continue next
			}
		}
		out = append(out, &provisions[i])
	}
	return out
}

// joinText is a provision's own text followed by its descendants under within,
// each clause and point opening with its marker as the instrument prints it.
func joinText(own, within string, kids []*law.Provision) string {
	lines := []string{}
	if own != "" {
		lines = append(lines, own)
	}
	for _, p := range kids {
		if within != "" && !strings.HasPrefix(p.ID, within+":") {
			continue
		}
		line := marker(p) + p.Text
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func marker(p *law.Provision) string {
	switch p.Kind {
	case "clause":
		return p.Number + ". "
	case "point":
		return p.Number + ") "
	}
	return ""
}

var signature = regexp.MustCompile(`\n(?:Nơi nhận:|TM\. |KT\. )`)

// Text chunks a document with no article structure: paragraphs gathered into
// chunks of about MaxParagraphs characters, the signature block dropped.
func (c *Chunker) Text(doc *Meta, text string, src *Source) error {
	body := signature.Split(text, 2)[0]
	path := []string{runes(doc.Title, 120)}
	var cur strings.Builder
	size, n := 0, 0
	flush := func() error {
		n++
		err := c.emit(doc, src, fmt.Sprintf("%s:para-%d", doc.ID, n), path, cur.String(), nil)
		cur.Reset()
		size = 0
		return err
	}
	for line := range strings.SplitSeq(body, "\n") {
		p := strings.TrimSpace(line)
		if p == "" {
			continue
		}
		l := utf8.RuneCountInString(p)
		if size > 0 && size+l > MaxParagraphs {
			if err := flush(); err != nil {
				return err
			}
		}
		if size > 0 {
			cur.WriteByte('\n')
			size++
		}
		cur.WriteString(p)
		size += l
	}
	return flush()
}

// MinTableRows and MinTableColumns are the smallest grid worth a table chunk.
// A one-row table is a layout device (the national header, a signature
// block), not data.
const (
	MinTableRows    = 2
	MinTableColumns = 2
)

// Tables adds one chunk per table, rendered as Markdown with its header row
// repeated on every piece. The plain-text chunks of the same document already
// carry the words, one cell per line; what these add is the grid, so a tariff
// line or a fee schedule reads as a row and not as unrelated lines. A table is
// placed under the provision whose text holds its first cells, or under the
// document title when none does.
func (c *Chunker) Tables(doc *Meta, tables [][][]string, provisions []law.Provision, src *Source) error {
	pathOf := headingPaths(doc.ID, provisions)
	for i, rows := range tables {
		if !IsDataTable(rows) {
			continue
		}
		path := []string{runes(doc.Title, 120)}
		if p := tableHome(rows, provisions); p != "" {
			path = pathOf(p)
		}
		pieces := htmltable.Markdown(htmltable.Table{Rows: rows}, 1, MaxPiece)
		for n, piece := range pieces {
			id := fmt.Sprintf("%s:table-%d", doc.ID, i+1)
			if len(pieces) > 1 {
				id = fmt.Sprintf("%s#part-%d", id, n+1)
			}
			c.Stats["chunks_table"]++
			if err := c.emitKind(doc, src, id, path, piece, nil, "table"); err != nil {
				return err
			}
		}
	}
	return nil
}

// layoutCues mark the tables vbpl.vn uses for page layout: the national
// motto beside the issuer, and the recipients beside the signature.
var layoutCues = []string{"cong-hoa-xa-hoi-chu-nghia-viet-nam", "noi-nhan"}

// IsDataTable reports whether a grid has enough rows and filled columns to be
// data rather than layout.
func IsDataTable(rows [][]string) bool {
	if len(rows) < MinTableRows {
		return false
	}
	first := law.Slug(strings.Join(rows[0], " ")) + "-" + law.Slug(strings.Join(rows[len(rows)-1], " "))
	for _, cue := range layoutCues {
		if strings.Contains(first, cue) {
			return false
		}
	}
	filled := map[int]bool{}
	for _, r := range rows {
		for j, v := range r {
			if v != "" {
				filled[j] = true
			}
		}
	}
	return len(filled) >= MinTableColumns
}

// tableHome finds the provision whose text holds the first two non-empty
// cells of the table's first data row, in order.
func tableHome(rows [][]string, provisions []law.Provision) string {
	var probe []string
	for _, r := range rows[1:] {
		probe = probe[:0]
		for _, v := range r {
			if utf8.RuneCountInString(v) >= 3 {
				probe = append(probe, v)
			}
			if len(probe) == 2 {
				break
			}
		}
		if len(probe) == 2 {
			break
		}
	}
	if len(probe) < 2 {
		return ""
	}
	for i := range provisions {
		t := provisions[i].Text
		if j := strings.Index(t, probe[0]); j >= 0 && strings.Contains(t[j:], probe[1]) {
			return provisions[i].ID
		}
	}
	return ""
}
