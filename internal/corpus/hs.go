package corpus

import (
	"regexp"
	"strings"
)

// HSRow is one line of the Vietnamese HS nomenclature: a heading, a
// subheading, an 8-digit tariff line, or an unnumbered grouping line such as
// "Ngựa:" that only exists to give the lines under it a parent.
type HSRow struct {
	Code              string `json:"code"`
	Level             int    `json:"level"`
	DescriptionVI     string `json:"description_vi"`
	DescriptionEN     string `json:"description_en"`
	Unit              string `json:"unit"`
	Chapter           string `json:"chapter"`
	ParentCode        string `json:"parent_code"`
	ParentDescription string `json:"parent_description"`
}

var (
	hsCode   = regexp.MustCompile(`^\d{2}(?:\.\d{2})?$|^\d{4}(?:\.\d{2}){0,2}$`)
	hsDashes = regexp.MustCompile(`^((?:-\s*)*)`)
	hsEight  = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}$`)
)

// HSRows reads the nomenclature out of the tables of TT 31/2022/TT-BTC. The
// columns are code, Vietnamese description, unit, then the English
// description in the fifth column of the six-column layout. The depth of a
// line is its leading dashes, and its parent is the nearest line above it that
// is less deep, which is how the printed nomenclature says that "Loại thuần
// chủng để nhân giống" sits under "Ngựa".
func HSRows(tables [][][]string) []HSRow {
	var rows, stack []HSRow
	for _, t := range tables {
		for _, r := range t {
			if len(r) < 3 || r[1] == "" {
				continue
			}
			code, vi, unit := r[0], r[1], r[2]
			en := ""
			if len(r) >= 6 {
				en = r[4]
			}
			if code != "" && !hsCode.MatchString(code) {
				continue // header rows, chapter titles, notes
			}
			level := strings.Count(hsDashes.FindString(vi), "-")
			if level == 0 && len(strings.ReplaceAll(code, ".", "")) > 4 {
				level = 1
			}
			for len(stack) > 0 && stack[len(stack)-1].Level >= level {
				stack = stack[:len(stack)-1]
			}
			var parent HSRow
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			chapter := code
			if chapter == "" {
				chapter = parent.Code
			}
			row := HSRow{
				Code: code, Level: level, Unit: unit,
				DescriptionVI: strings.TrimSpace(strings.TrimLeft(vi, "- ")),
				DescriptionEN: strings.TrimSpace(strings.TrimLeft(en, "- ")),
				Chapter:       runes(chapter, 2),
				ParentCode:    parent.Code, ParentDescription: parent.DescriptionVI,
			}
			rows = append(rows, row)
			stack = append(stack, row)
		}
	}
	return rows
}

// IsTariffLine reports whether a code is a full 8-digit line.
func IsTariffLine(code string) bool { return hsEight.MatchString(code) }
