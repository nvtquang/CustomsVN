// Package refs finds what a Vietnamese legal question points at by itself:
// a provision ("điểm b khoản 1 Điều 18 Luật Hải quan"), an instrument by its
// number ("Thông tư 38/2015/TT-BTC"), an HS code ("8703.23.61") and a day
// ("ngày 1/7/2025").
//
// Everything here is pattern matching on the words of the question. A
// reference found this way is looked up directly; nothing is inferred.
package refs

import (
	"regexp"
	"strings"

	"github.com/tamnd/luatdo/law"
)

// Provision is a reference to an article, a clause or a point, and to the
// instrument it belongs to when the text names one.
type Provision struct {
	Point   string `json:"point,omitempty"`
	Clause  string `json:"clause,omitempty"`
	Article string `json:"article"`
	// Number is the instrument's official number when the text gives it.
	Number string `json:"number,omitempty"`
	// Name is the instrument's name when the text gives it instead, e.g.
	// "Luật Hải quan". It runs to the end of the clause it appears in, since
	// a law's name can hold a comma; the resolver matches the longest known
	// name it starts with.
	Name string `json:"name,omitempty"`
	Span string `json:"span"`
}

// Tail is the identifier segments below the document: "article-18:clause-1:point-b".
func (p Provision) Tail() string {
	t := "article-" + law.NumberSegment(p.Article)
	if p.Clause != "" {
		t += ":clause-" + law.NumberSegment(p.Clause)
		if p.Point != "" {
			t += ":point-" + law.NumberSegment(p.Point)
		}
	}
	return t
}

// Found is everything Parse recognised in one text.
type Found struct {
	Provisions []Provision `json:"provisions,omitempty"`
	Numbers    []string    `json:"numbers,omitempty"`
	HSCodes    []string    `json:"hs_codes,omitempty"`
	Dates      []string    `json:"dates,omitempty"` // ISO
}

// Empty reports whether nothing was recognised.
func (f Found) Empty() bool {
	return len(f.Provisions) == 0 && len(f.Numbers) == 0 && len(f.HSCodes) == 0 && len(f.Dates) == 0
}

const docTypes = `Bộ luật|Luật|Pháp lệnh|Nghị quyết|Nghị định|Thông tư liên tịch|Thông tư|Quyết định|Chỉ thị|Văn bản hợp nhất`

var (
	// An official number: 54/2014/QH13, 08/2015/NĐ-CP, 80/2016/TTLT-BTC-BTTTT, 47/VBHN-BCT.
	numberPat = `\d+[a-zA-Z]?/(?:\d{4}/)?[A-ZĐa-zđ][A-ZĐa-zđ0-9&-]*[A-ZĐa-zđ0-9]`
	number    = regexp.MustCompile(`(?:^|[^\w/])(` + numberPat + `)`)

	provision = regexp.MustCompile(`(?i)(?:điểm\s+([a-zđ])\s*,?\s+)?(?:khoản\s+(\d+[a-zđ]?)\s*,?\s+)?điều\s+(\d+[a-zđ]?)\b`)

	// What may follow a provision to name its instrument.
	ofNumber = regexp.MustCompile(`^\s*,?\s*(?:của\s+)?(?:(?i:` + docTypes + `)\s*)?(?:số\s*)?(` + numberPat + `)`)
	ofName   = regexp.MustCompile(`^\s*,?\s*(?:của\s+)?((?:Bộ luật|Luật|BỘ LUẬT|LUẬT|bộ luật|luật)\s+[^\d.;:()?!\n]+)`)

	hsDotted = regexp.MustCompile(`\b(\d{4})\.(\d{2})(?:\.(\d{2}))?\b`)
	hsPlain  = regexp.MustCompile(`(?i)(?:mã(?:\s+số)?(?:\s+hs)?|hs|nhóm|phân nhóm)\s*:?\s*(\d{4}(?:\d{2}){0,2})(?:$|[^\d.])`)

	dateWords = regexp.MustCompile(`(?i)ngày\s+(\d{1,2})\s+tháng\s+(\d{1,2})\s+năm\s+(\d{4})`)
	dateSlash = regexp.MustCompile(`\b(\d{1,2})[/.-](\d{1,2})[/.-](\d{4})\b`)
)

// nameStop ends a law's name at the words that follow it in a question. It
// has no : RE2's word boundary is ASCII and "số" ends in a letter it does
// not count as one.
var nameStop = regexp.MustCompile(`(?i)\s+(?:số|năm|ngày|quy định|thì|có|được|là|về việc|hiện hành|sửa đổi|mới|cũ|không|nào|gì|bao)(?:\s.*)?$`)

// Parse finds every reference in text.
func Parse(text string) Found {
	var f Found
	seen := map[string]bool{}
	add := func(list *[]string, v string) {
		if v != "" && !seen[v] {
			seen[v] = true
			*list = append(*list, v)
		}
	}
	for _, m := range provision.FindAllStringSubmatchIndex(text, -1) {
		p := Provision{Article: text[m[6]:m[7]]}
		if m[2] >= 0 {
			p.Point = strings.ToLower(text[m[2]:m[3]])
		}
		if m[4] >= 0 {
			p.Clause = strings.ToLower(text[m[4]:m[5]])
		}
		end := m[1]
		rest := text[end:]
		if n := ofNumber.FindStringSubmatchIndex(rest); n != nil {
			p.Number = rest[n[2]:n[3]]
			end += n[1]
		} else if n := ofName.FindStringSubmatchIndex(rest); n != nil {
			name := nameStop.ReplaceAllString(strings.TrimSpace(rest[n[2]:n[3]]), "")
			p.Name = strings.TrimSpace(name)
			end += n[3]
			// "Luật Hải quan số 54/2014/QH13": the number settles which one.
			if k := ofNumber.FindStringSubmatchIndex(text[end:]); k != nil && k[0] == 0 {
				p.Number = text[end+k[2] : end+k[3]]
				end += k[1]
			}
		}
		p.Span = strings.TrimSpace(text[m[0]:end])
		f.Provisions = append(f.Provisions, p)
	}
	for _, m := range number.FindAllStringSubmatch(text, -1) {
		if strings.Count(m[1], "/") == 1 && !strings.Contains(strings.ToUpper(m[1]), "VBHN") {
			continue // "1/7" is a day, not an instrument
		}
		add(&f.Numbers, m[1])
	}
	for _, m := range hsDotted.FindAllStringSubmatch(text, -1) {
		add(&f.HSCodes, strings.TrimSuffix(m[1]+"."+m[2]+"."+m[3], "."))
	}
	for _, m := range hsPlain.FindAllStringSubmatch(text, -1) {
		add(&f.HSCodes, dotted(m[1]))
	}
	for _, m := range dateWords.FindAllStringSubmatch(text, -1) {
		add(&f.Dates, iso(m[3], m[2], m[1]))
	}
	for _, m := range dateSlash.FindAllStringSubmatch(text, -1) {
		add(&f.Dates, iso(m[3], m[2], m[1]))
	}
	return f
}

// dotted writes "87032361" the way the nomenclature does, "8703.23.61".
func dotted(code string) string {
	switch len(code) {
	case 6:
		return code[:4] + "." + code[4:]
	case 8:
		return code[:4] + "." + code[4:6] + "." + code[6:]
	}
	return code
}

func iso(y, m, d string) string {
	pad := func(s string) string {
		if len(s) == 1 {
			return "0" + s
		}
		return s
	}
	mm, dd := pad(m), pad(d)
	if mm < "01" || mm > "12" || dd < "01" || dd > "31" {
		return ""
	}
	return y + "-" + mm + "-" + dd
}

// NameKey folds an instrument's name for lookup: "LUẬT HẢI QUAN" and "Luật
// Hải quan" are one key, "luat-hai-quan".
func NameKey(name string) string {
	return law.Slug(nameStop.ReplaceAllString(strings.TrimSpace(name), ""))
}
