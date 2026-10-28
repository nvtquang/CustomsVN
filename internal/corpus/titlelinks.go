package corpus

import (
	"regexp"
	"strings"
)

// TitleLink is a link a document's title states: "Sửa đổi, bổ sung một số
// điều của Nghị định số 08/2015/NĐ-CP" amends 08/2015/NĐ-CP.
type TitleLink struct {
	Number   string
	Relation string
	Phrase   string
}

var (
	titleVerb   = regexp.MustCompile(`(?i)(sửa đổi|bổ sung|bãi bỏ|thay thế|đính chính|ngưng hiệu lực|đình chỉ)`)
	titleNumber = regexp.MustCompile(`\d+[a-z]?/\d{4}/[A-ZĐ][A-ZĐa-zđ0-9-]*[A-ZĐa-zđ0-9]`)
	// A number after "căn cứ" or "hướng dẫn" names an instrument the
	// document relies on, not one it changes. No \b: RE2's word boundary is
	// ASCII and "cứ" ends in a letter it does not count.
	titleStop = regexp.MustCompile(`(?i)(?:^|\s)(?:hướng dẫn|quy định chi tiết|căn cứ|để thực hiện|thi hành)(?:\s|$)`)
)

var verbRelation = map[string]string{
	"sửa đổi": RelAmends, "bổ sung": RelAmends, "bãi bỏ": RelRepeals, "thay thế": RelReplaces,
	"đính chính": RelCorrects, "ngưng hiệu lực": RelSuspends, "đình chỉ": RelSuspends,
}

// TitleLinks reads the instruments a title says the document changes: the
// numbers that follow its first change verb, up to a phrase that starts
// naming the instruments it implements instead. A title that names none
// yields none.
func TitleLinks(title string) []TitleLink {
	loc := titleVerb.FindStringIndex(title)
	if loc == nil {
		return nil
	}
	// "hướng dẫn thi hành Nghị định số 49/2022 sửa đổi Nghị định 209/2013":
	// the verb belongs to the decree being implemented, not to this
	// document. Reading it would draw an edge from the wrong document, so
	// such a title yields nothing; precision is worth more than recall here.
	if titleStop.MatchString(title[:loc[0]]) {
		return nil
	}
	verb := strings.ToLower(title[loc[0]:loc[1]])
	rest := title[loc[1]:]
	if s := titleStop.FindStringIndex(rest); s != nil {
		rest = rest[:s[0]]
	}
	var out []TitleLink
	seen := map[string]bool{}
	for _, n := range titleNumber.FindAllString(rest, -1) {
		if seen[n] || StoreID(n) == "" {
			continue
		}
		seen[n] = true
		out = append(out, TitleLink{Number: n, Relation: verbRelation[verb], Phrase: title[loc[0]:loc[1]]})
	}
	return out
}
