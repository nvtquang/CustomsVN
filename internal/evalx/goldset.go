// Package evalx is Retrieval Testing: gold sets, batch runs and metrics.
package evalx

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/nvtquang/CustomsVN/internal/refs"
	"github.com/nvtquang/CustomsVN/internal/search"
)

// Item is one gold question, in the columns docs/PLAN.md gives the experts.
type Item struct {
	ExtID              string   `json:"id"`
	Question           string   `json:"question"`
	ExpectedProvisions string   `json:"expected_provisions"`
	ExpectedIDs        []string `json:"expected_ids,omitempty"`
	ExpectedAnswer     string   `json:"expected_answer,omitempty"`
	AsOf               string   `json:"as_of_date,omitempty"`
	Category           string   `json:"category,omitempty"`
	ShouldRefuse       bool     `json:"should_refuse"`
	Notes              string   `json:"notes,omitempty"`
}

// RowError is one problem with one row, so an import can show every bad row
// at once instead of stopping at the first.
type RowError struct {
	Row     int    `json:"row"`
	ID      string `json:"id,omitempty"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e RowError) Error() string {
	return fmt.Sprintf("row %d (%s) %s: %s", e.Row, e.ID, e.Field, e.Message)
}

// Columns of the CSV form.
var Columns = []string{"id", "question", "expected_provisions", "expected_answer", "as_of_date", "category", "should_refuse", "notes"}

// Parse reads a gold set as CSV or JSON (an array of items). An empty format
// is detected from the first character.
func Parse(format string, data []byte) ([]Item, []RowError, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if format == "" {
		if t := bytes.TrimSpace(data); len(t) > 0 && (t[0] == '[' || t[0] == '{') {
			format = "json"
		} else {
			format = "csv"
		}
	}
	var items []Item
	switch format {
	case "json":
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, nil, fmt.Errorf("JSON gold set: %w", err)
		}
	case "csv":
		var err error
		if items, err = parseCSV(data); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, fmt.Errorf("unknown gold set format %q (want csv or json)", format)
	}
	return items, validate(items), nil
}

func parseCSV(data []byte) ([]Item, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV gold set: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, need := range []string{"id", "question"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("CSV gold set has no %q column (want %s)", need, strings.Join(Columns, ","))
		}
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	var items []Item
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV gold set: %w", err)
		}
		refuse, _ := strconv.ParseBool(strings.ToLower(get(rec, "should_refuse")))
		items = append(items, Item{
			ExtID: get(rec, "id"), Question: get(rec, "question"), ExpectedProvisions: get(rec, "expected_provisions"),
			ExpectedAnswer: get(rec, "expected_answer"), AsOf: get(rec, "as_of_date"), Category: get(rec, "category"),
			ShouldRefuse: refuse, Notes: get(rec, "notes"),
		})
	}
	return items, nil
}

func validate(items []Item) []RowError {
	var errs []RowError
	seen := map[string]bool{}
	for i, it := range items {
		row := i + 2 // the header is row 1
		switch {
		case it.ExtID == "":
			errs = append(errs, RowError{Row: row, Field: "id", Message: "empty"})
		case seen[it.ExtID]:
			errs = append(errs, RowError{Row: row, ID: it.ExtID, Field: "id", Message: "repeated"})
		}
		seen[it.ExtID] = true
		if it.Question == "" {
			errs = append(errs, RowError{Row: row, ID: it.ExtID, Field: "question", Message: "empty"})
		}
		if it.AsOf != "" {
			if _, err := time.Parse(time.DateOnly, it.AsOf); err != nil {
				errs = append(errs, RowError{Row: row, ID: it.ExtID, Field: "as_of_date", Message: "not YYYY-MM-DD"})
			}
		}
		if !it.ShouldRefuse && it.ExpectedProvisions == "" && len(it.ExpectedIDs) == 0 {
			errs = append(errs, RowError{Row: row, ID: it.ExtID, Field: "expected_provisions", Message: "empty for a question that should be answered"})
		}
	}
	return errs
}

// Resolve turns each item's expected provisions, written as text ("điểm b
// khoản 1 Điều 18 Luật Hải quan 54/2014/QH13", several separated by ";"),
// into provision identifiers. An identifier written as is ("vn:law:...") is
// kept. What cannot be resolved is reported per row and left out.
func Resolve(c *search.Corpus, items []Item, today string) []RowError {
	var errs []RowError
	for i := range items {
		it := &items[i]
		if len(it.ExpectedIDs) > 0 || it.ExpectedProvisions == "" {
			continue
		}
		asOf := it.AsOf
		if asOf == "" {
			asOf = today
		}
		for _, part := range strings.Split(it.ExpectedProvisions, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if strings.HasPrefix(part, "vn:") {
				it.ExpectedIDs = append(it.ExpectedIDs, part)
				continue
			}
			found := refs.Parse(part)
			if len(found.Provisions) != 1 {
				errs = append(errs, RowError{Row: i + 2, ID: it.ExtID, Field: "expected_provisions", Message: fmt.Sprintf("%q is not one provision reference", part)})
				continue
			}
			p := found.Provisions[0]
			d := c.ResolveDoc(p.Number, p.Name, asOf, today)
			if d == nil {
				errs = append(errs, RowError{Row: i + 2, ID: it.ExtID, Field: "expected_provisions", Message: fmt.Sprintf("%q names no instrument in the corpus", part)})
				continue
			}
			if c.CoveringChunk(d.ID, p.Tail()) == nil {
				errs = append(errs, RowError{Row: i + 2, ID: it.ExtID, Field: "expected_provisions", Message: fmt.Sprintf("%q: %s has no such provision in the corpus", part, d.OfficialNumber)})
				continue
			}
			it.ExpectedIDs = append(it.ExpectedIDs, d.ID+":"+p.Tail())
		}
	}
	return errs
}
