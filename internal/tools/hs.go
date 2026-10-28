package tools

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nvtquang/CustomsVN/internal/embed"
)

// HSLine is one line of the nomenclature with its place in the tree.
type HSLine struct {
	Position      int      `json:"position"`
	Code          string   `json:"code"`
	Level         int      `json:"level"`
	DescriptionVI string   `json:"description_vi"`
	DescriptionEN string   `json:"description_en"`
	Unit          string   `json:"unit,omitempty"`
	Chapter       string   `json:"chapter"`
	Hash          string   `json:"-"`
	Path          []HSStep `json:"path,omitempty"`
	Children      []HSStep `json:"children,omitempty"`
	Score         float64  `json:"score,omitempty"`
}

// HSStep is an ancestor or child shown beside a line.
type HSStep struct {
	Code          string `json:"code,omitempty"`
	DescriptionVI string `json:"description_vi"`
	Level         int    `json:"level"`
}

// HSResult is what lookup_hs found.
type HSResult struct {
	Query    string   `json:"query"`
	ByCode   bool     `json:"by_code"`
	Lines    []HSLine `json:"lines"`
	SourceID string   `json:"source_doc_id"`
	Source   string   `json:"source"`
}

// HSSource is the document the nomenclature comes from.
const HSSource = "vn:law:2022:31-2022-tt-btc"

var hsCodeQuery = regexp.MustCompile(`^\s*(\d{2}(?:[.\s]?\d{2}){0,3})\s*$`)

// lines loads the nomenclature once; 18 thousand lines are cheaper to walk in
// memory than to climb with recursive queries. A failed load is tried again
// on the next call.
func (t *Toolset) lines(ctx context.Context) ([]HSLine, error) {
	t.hsMu.Lock()
	defer t.hsMu.Unlock()
	if t.hs != nil {
		return t.hs, nil
	}
	rows, err := t.Pool.Query(ctx, `SELECT position, code, level, description_vi, description_en, unit, chapter, content_hash FROM hs_codes ORDER BY position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all := []HSLine{}
	for rows.Next() {
		var l HSLine
		if err := rows.Scan(&l.Position, &l.Code, &l.Level, &l.DescriptionVI, &l.DescriptionEN, &l.Unit, &l.Chapter, &l.Hash); err != nil {
			return nil, err
		}
		all = append(all, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	t.hs = all
	return all, nil
}

func digits(code string) string { return strings.NewReplacer(".", "", " ", "").Replace(code) }

// with fills a line's ancestors and, for a heading, the lines under it.
//
// A coded line is placed by its code: 8703.23.61 sits under 8703.23 and
// 87.03. The dashes that mark depth in the printed nomenclature are not
// reliable in the source (whole runs of lines lost them in the conversion to
// HTML), so they are used only for a line with no code of its own.
func with(all []HSLine, i int, children bool) HSLine {
	l := all[i]
	d := digits(l.Code)
	if d != "" {
		for _, n := range []int{4, 6} {
			if len(d) <= n {
				break
			}
			for j := i - 1; j >= 0; j-- {
				if digits(all[j].Code) == d[:n] {
					l.Path = append(l.Path, HSStep{Code: all[j].Code, DescriptionVI: all[j].DescriptionVI, Level: all[j].Level})
					break
				}
			}
		}
	} else {
		level := l.Level
		for j := i - 1; j >= 0 && level > 0; j-- {
			if all[j].Level < level {
				l.Path = append([]HSStep{{Code: all[j].Code, DescriptionVI: all[j].DescriptionVI, Level: all[j].Level}}, l.Path...)
				level = all[j].Level
			}
		}
	}
	if children && d != "" && len(d) < 8 {
		for j := i + 1; j < len(all) && len(l.Children) < 40; j++ {
			cd := digits(all[j].Code)
			if cd != "" && !strings.HasPrefix(cd, d) {
				break
			}
			l.Children = append(l.Children, HSStep{Code: all[j].Code, DescriptionVI: all[j].DescriptionVI, Level: l.Level + 1})
		}
	}
	return l
}

func (t *Toolset) lookupHS(ctx context.Context, query string) (*Result, error) {
	res, err := t.HS(ctx, query, 12)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	for _, l := range res.Lines {
		var path []string
		for _, p := range l.Path {
			path = append(path, strings.TrimSpace(p.Code+" "+p.DescriptionVI))
		}
		fmt.Fprintf(&b, "- %s %s (%s)", firstNonEmpty(l.Code, "—"), l.DescriptionVI, l.DescriptionEN)
		if len(path) > 0 {
			fmt.Fprintf(&b, " ← %s", strings.Join(path, " › "))
		}
		b.WriteByte('\n')
	}
	if len(res.Lines) == 0 {
		b.WriteString("Không tìm thấy dòng nào trong Danh mục.")
	}
	return &Result{Text: "Nguồn: Thông tư 31/2022/TT-BTC (Danh mục hàng hóa XNK Việt Nam)\n" + b.String(),
		Summary: fmt.Sprintf("%d dòng HS", len(res.Lines)), Card: &Card{Type: "hs", Data: res}}, nil
}

// HS looks the nomenclature up by code prefix, or by description with trigram
// similarity fused with the HS embeddings when a model has them.
func (t *Toolset) HS(ctx context.Context, query string, limit int) (*HSResult, error) {
	all, err := t.lines(ctx)
	if err != nil {
		return nil, err
	}
	res := &HSResult{Query: strings.TrimSpace(query), SourceID: HSSource, Source: "31/2022/TT-BTC", Lines: []HSLine{}}
	if res.Query == "" {
		return res, nil
	}
	if m := hsCodeQuery.FindStringSubmatch(res.Query); m != nil {
		res.ByCode = true
		want := digits(m[1])
		for i, l := range all {
			if l.Code != "" && strings.HasPrefix(digits(l.Code), want) {
				res.Lines = append(res.Lines, with(all, i, len(res.Lines) == 0))
				if len(res.Lines) == limit {
					break
				}
			}
		}
		return res, nil
	}

	byPos := map[int]int{}
	for i, l := range all {
		byPos[l.Position] = i
	}
	scores := map[int]float64{}
	rows, err := t.Pool.Query(ctx, `SELECT position, word_similarity($1, description_vi) AS s FROM hs_codes
		WHERE $1 <% description_vi OR description_vi ILIKE '%' || $1 || '%' ORDER BY s DESC LIMIT 50`, res.Query)
	if err != nil {
		return nil, err
	}
	rank := 0
	for rows.Next() {
		var pos int
		var s float64
		if err := rows.Scan(&pos, &s); err != nil {
			rows.Close()
			return nil, err
		}
		rank++
		scores[pos] += 1 / (60 + float64(rank))
	}
	rows.Close()
	emb := t.HSEmbedder
	if emb == nil && t.Engine != nil {
		emb = t.Engine.Models().Embedder("")
	}
	if emb != nil {
		vecs, err := emb.Embed(ctx, []string{res.Query}, embed.Query)
		if err == nil {
			typ, _ := embed.VectorType(emb.Dims())
			q := fmt.Sprintf(`SELECT h.position FROM hs_embeddings e JOIN hs_codes h ON h.content_hash = e.content_hash
				WHERE e.model = %s ORDER BY (e.embedding::%s) <=> $1::%s LIMIT 50`, embed.Literal(emb.Name()), typ, typ)
			if vr, err := t.Pool.Query(ctx, q, embed.VectorLiteral(vecs[0])); err == nil {
				rank = 0
				for vr.Next() {
					var pos int
					if vr.Scan(&pos) == nil {
						rank++
						scores[pos] += 1 / (60 + float64(rank))
					}
				}
				vr.Close()
			}
		}
	}
	type scored struct {
		pos int
		s   float64
	}
	var order []scored
	for p, s := range scores {
		order = append(order, scored{p, s})
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].s != order[j].s {
			return order[i].s > order[j].s
		}
		return order[i].pos < order[j].pos
	})
	for _, o := range order {
		i, ok := byPos[o.pos]
		if !ok {
			continue
		}
		l := with(all, i, false)
		l.Score = o.s
		res.Lines = append(res.Lines, l)
		if len(res.Lines) == limit {
			break
		}
	}
	return res, nil
}
