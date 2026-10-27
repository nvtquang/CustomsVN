package corpus

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/tamnd/luatdo/law"
)

//go:embed mandatory.tsv
var mandatoryTSV string

// Mandatory is one document the customs corpus must hold or explain.
type Mandatory struct {
	Number string `json:"number"`
	Cue    string `json:"cue"`
	Group  string `json:"group"`
	Note   string `json:"note"`
}

// MandatoryList returns the list shipped with the binary. It is a draft for
// the customs expert to review; the coverage report says what the corpus does
// with each entry.
func MandatoryList() ([]Mandatory, error) {
	var out []Mandatory
	for i, line := range strings.Split(mandatoryTSV, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			return nil, fmt.Errorf("mandatory.tsv line %d: want 4 fields, got %d", i+1, len(f))
		}
		out = append(out, Mandatory{Number: f[0], Cue: f[1], Group: f[2], Note: f[3]})
	}
	return out, nil
}

// TitleMatches reports whether a store title carries the entry's cue, with
// case and diacritics folded so "Hoá" and "Hóa" agree.
func (m Mandatory) TitleMatches(title string) bool {
	return strings.Contains(law.Slug(title), law.Slug(m.Cue))
}
