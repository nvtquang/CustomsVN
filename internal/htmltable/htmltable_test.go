package htmltable

import (
	"slices"
	"strings"
	"testing"
)

func TestParseAlignsColspanAndRowspan(t *testing.T) {
	src := `<p>Biểu</p><table>
<tr><th rowspan="2">Mã hàng</th><th colspan="2">Thuế suất (%)</th></tr>
<tr><td>2026</td><td>2027</td></tr>
<tr><td>0101.21.00</td><td>5</td><td>0</td></tr>
<tr><td colspan=3>&nbsp;</td></tr>
</table>`
	tables := Parse(src)
	if len(tables) != 1 {
		t.Fatalf("got %d tables", len(tables))
	}
	want := [][]string{
		{"Mã hàng", "Thuế suất (%)", ""},
		{"", "2026", "2027"},
		{"0101.21.00", "5", "0"},
	}
	if len(tables[0].Rows) != len(want) {
		t.Fatalf("rows %q", tables[0].Rows)
	}
	for i := range want {
		if !slices.Equal(tables[0].Rows[i], want[i]) {
			t.Errorf("row %d = %q, want %q", i, tables[0].Rows[i], want[i])
		}
	}
}

func TestParseRowspanInTheLastColumn(t *testing.T) {
	src := `<table><tr><td>a</td><td rowspan="2">x</td></tr><tr><td>b</td></tr><tr><td>c</td><td>y</td></tr></table>`
	rows := Parse(src)[0].Rows
	want := [][]string{{"a", "x"}, {"b", ""}, {"c", "y"}}
	for i := range want {
		if !slices.Equal(rows[i], want[i]) {
			t.Errorf("row %d = %q, want %q", i, rows[i], want[i])
		}
	}
}

func TestParseCellText(t *testing.T) {
	src := `<table><tr><td><p>Ngựa,<br/>lừa</p>  <span>sống</span> &amp; khác</td><td>- - Loại<!-- x --> khác</td></tr></table>`
	got := Parse(src)[0].Rows[0]
	if !slices.Equal(got, []string{"Ngựa, lừa sống & khác", "- - Loại khác"}) {
		t.Errorf("got %q", got)
	}
}

func TestParseKeepsANestedTableInItsCell(t *testing.T) {
	src := `<table><tr><td>a</td><td><table><tr><td>b</td><td>c</td></tr></table></td></tr><tr><td>d</td><td>e</td></tr></table>`
	tables := Parse(src)
	if len(tables) != 1 || len(tables[0].Rows) != 2 {
		t.Fatalf("got %q", tables)
	}
	if !slices.Equal(tables[0].Rows[0], []string{"a", "b c"}) {
		t.Errorf("row 0 = %q", tables[0].Rows[0])
	}
}

func TestParseClosesAnUnclosedCell(t *testing.T) {
	src := `<table><tr><td>a<td>b</tr></table>`
	if got := Parse(src)[0].Rows[0]; !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("got %q", got)
	}
}

func TestMarkdownRepeatsTheHeaderOnEveryPiece(t *testing.T) {
	tbl := Table{Rows: [][]string{{"Mã", "Mô tả"}, {"01", "Ngựa | lừa"}, {"02", "Thịt"}, {"03", "Cá"}}}
	one := Markdown(tbl, 1, 0)
	if len(one) != 1 || !strings.Contains(one[0], `Ngựa \| lừa`) {
		t.Fatalf("got %q", one)
	}
	pieces := Markdown(tbl, 1, 50)
	if len(pieces) < 2 {
		t.Fatalf("got %d pieces", len(pieces))
	}
	for _, p := range pieces {
		if !strings.HasPrefix(p, "| Mã | Mô tả |\n| --- | --- |\n") {
			t.Errorf("piece without header: %q", p)
		}
	}
}
