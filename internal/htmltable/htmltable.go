// Package htmltable reads the tables of a vbpl.vn page as grids of cell text
// and renders them as Markdown.
//
// luatdo's HTMLText turns every cell into its own line, which keeps the words
// and loses the columns: a tariff line becomes four unrelated lines. Schedules,
// the HS nomenclature and fee tables are where customs questions are answered,
// so the grid has to survive. Like HTMLText this is a scanner rather than a
// parser, because the input is generated HTML of one known shape and a scanner
// cannot fail on malformed markup.
package htmltable

import (
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Table is a grid of cell texts. A cell spanning several columns or rows
// fills the first position and leaves the others empty, so every column of
// every row means the same thing.
type Table struct {
	Rows [][]string
}

// maxSpan bounds a colspan or rowspan. Generated HTML sometimes says
// colspan="1000"; a grid that wide is a defect, not a table.
const maxSpan = 64

var (
	colspanAttr = regexp.MustCompile(`(?i)\bcolspan\s*=\s*["']?\s*(\d+)`)
	rowspanAttr = regexp.MustCompile(`(?i)\browspan\s*=\s*["']?\s*(\d+)`)
)

// cellBreaks are the tags that separate words inside a cell.
var cellBreaks = map[string]bool{"br": true, "p": true, "div": true, "li": true}

// Parse returns the top-level tables of src in document order. A table nested
// in a cell becomes text of that cell. Rows with no text at all are dropped.
func Parse(src string) []Table {
	var (
		tables []Table
		depth  int
		row    []string
		inRow  bool
		cell   *strings.Builder
		span   [2]int      // colspan, rowspan of the open cell
		below  map[int]int // column -> rows still covered by a rowspan from above
	)
	occupied := func() {
		for below[len(row)] > 0 {
			below[len(row)]--
			row = append(row, "")
		}
	}
	closeCell := func() {
		if cell == nil {
			return
		}
		occupied()
		start := len(row)
		row = append(row, strings.Join(strings.Fields(html.UnescapeString(cell.String())), " "))
		for range span[0] - 1 {
			row = append(row, "")
		}
		if span[1] > 1 {
			for c := start; c < start+span[0]; c++ {
				below[c] = span[1] - 1
			}
		}
		cell = nil
	}
	closeRow := func() {
		if !inRow {
			return
		}
		closeCell()
		// Columns at the end of the row still covered from above.
		for c := len(row); ; c++ {
			if below[c] == 0 {
				break
			}
			below[c]--
			row = append(row, "")
		}
		for _, c := range row {
			if c != "" {
				tables[len(tables)-1].Rows = append(tables[len(tables)-1].Rows, row)
				break
			}
		}
		row, inRow = nil, false
	}

	for i := 0; i < len(src); {
		if src[i] != '<' {
			j := strings.IndexByte(src[i:], '<')
			if j < 0 {
				j = len(src) - i
			}
			if cell != nil {
				cell.WriteString(src[i : i+j])
			}
			i += j
			continue
		}
		name, closing, attrs, end := scanTag(src, i)
		i = end
		switch {
		case name == "table" && !closing:
			depth++
			if depth == 1 {
				tables = append(tables, Table{})
				below = map[int]int{}
			} else if cell != nil {
				cell.WriteByte(' ')
			}
		case name == "table" && closing:
			if depth == 1 {
				closeRow()
			} else if cell != nil {
				cell.WriteByte(' ')
			}
			if depth > 0 {
				depth--
			}
		case depth != 1:
			if cell != nil && depth > 1 && (name == "td" || name == "th" || name == "tr" || cellBreaks[name]) {
				cell.WriteByte(' ')
			}
		case name == "tr":
			closeRow()
			inRow = !closing
		case (name == "td" || name == "th") && !closing:
			if !inRow {
				continue
			}
			closeCell()
			cell = &strings.Builder{}
			span = [2]int{spanOf(colspanAttr, attrs), spanOf(rowspanAttr, attrs)}
		case name == "td" || name == "th":
			closeCell()
		case cellBreaks[name] && cell != nil:
			cell.WriteByte(' ')
		}
	}
	return tables
}

func spanOf(attr *regexp.Regexp, attrs string) int {
	m := attr.FindStringSubmatch(attrs)
	if m == nil {
		return 1
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 {
		return 1
	}
	return min(n, maxSpan)
}

// scanTag reads the tag that starts at src[i] and returns its lowercase name,
// whether it closes, its attribute text, and the index just past it. A comment
// or a declaration comes back with an empty name.
func scanTag(src string, i int) (name string, closing bool, attrs string, end int) {
	if strings.HasPrefix(src[i:], "<!--") {
		j := strings.Index(src[i+4:], "-->")
		if j < 0 {
			return "", false, "", len(src)
		}
		return "", false, "", i + 4 + j + 3
	}
	j := strings.IndexByte(src[i:], '>')
	if j < 0 {
		return "", false, "", len(src)
	}
	end = i + j + 1
	body := src[i+1 : i+j]
	if strings.HasPrefix(body, "/") {
		closing = true
		body = body[1:]
	}
	k := 0
	for k < len(body) && (body[k] >= 'a' && body[k] <= 'z' || body[k] >= 'A' && body[k] <= 'Z' || body[k] >= '0' && body[k] <= '9') {
		k++
	}
	return strings.ToLower(body[:k]), closing, body[k:], end
}

// Markdown renders a table as Markdown pieces of at most limit characters each,
// every piece opening with the first header rows of the table, so a piece read
// alone still says what its columns are. A row longer than limit is a piece of
// its own. With limit <= 0 the table is one piece.
func Markdown(t Table, header, limit int) []string {
	if len(t.Rows) == 0 {
		return nil
	}
	width := 0
	for _, r := range t.Rows {
		width = max(width, len(r))
	}
	header = min(header, len(t.Rows))
	var head strings.Builder
	if header == 0 {
		// Markdown needs a header row; an empty one keeps the grid valid.
		head.WriteString(line(make([]string, width), width))
	}
	for _, r := range t.Rows[:header] {
		head.WriteString(line(r, width))
	}
	head.WriteString("|" + strings.Repeat(" --- |", width) + "\n")
	h := head.String()

	var pieces []string
	var cur strings.Builder
	size := 0
	for _, r := range t.Rows[header:] {
		l := line(r, width)
		n := utf8.RuneCountInString(l)
		if limit > 0 && size > 0 && utf8.RuneCountInString(h)+size+n > limit {
			pieces = append(pieces, strings.TrimRight(h+cur.String(), "\n"))
			cur.Reset()
			size = 0
		}
		cur.WriteString(l)
		size += n
	}
	if size > 0 || len(pieces) == 0 {
		pieces = append(pieces, strings.TrimRight(h+cur.String(), "\n"))
	}
	return pieces
}

var cellEscape = strings.NewReplacer("|", `\|`, "\n", " ")

func line(r []string, width int) string {
	var b strings.Builder
	b.WriteByte('|')
	for c := range width {
		v := ""
		if c < len(r) {
			v = cellEscape.Replace(r[c])
		}
		b.WriteString(" " + v + " |")
	}
	b.WriteByte('\n')
	return b.String()
}
