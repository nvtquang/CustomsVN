package answer

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// MinSegment is the shortest piece of a quote checked on its own when a quote
// is split by an ellipsis. Shorter pieces ("và", "của") match anywhere and
// prove nothing.
const MinSegment = 12

// MaxQuote bounds a quote. A quote is evidence for one claim, not a copy of
// the source.
const MaxQuote = 600

var ellipsis = regexp.MustCompile(`\s*(?:\.\.\.|…|\[\.\.\.\]|\(\.\.\.\))\s*`)

// normalize makes two renderings of the same words compare equal: Unicode
// composed form, one kind of quote and dash, runs of white space collapsed.
// Letters, digits, diacritics and word order are untouched, so a quote that
// matches after this was copied, not paraphrased.
func normalize(s string) string {
	s = norm.NFC.String(s)
	s = strings.NewReplacer(
		"“", `"`, "”", `"`, "„", `"`, "‘", "'", "’", "'",
		"–", "-", "—", "-", " ", " ",
	).Replace(s)
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	return b.String()
}

// Verbatim reports whether quote appears word for word in source. A quote may
// leave words out with an ellipsis; each piece must then appear, in order,
// and be long enough to mean something. Surrounding quotation marks and a
// trailing full stop are not part of the evidence.
func Verbatim(source, quote string) bool {
	q := strings.TrimSpace(quote)
	q = strings.Trim(q, `"'“”‘’ `)
	if q == "" || len([]rune(q)) > MaxQuote {
		return false
	}
	src := normalize(source)
	pos := 0
	pieces := ellipsis.Split(q, -1)
	matched := 0
	for _, piece := range pieces {
		p := strings.TrimRight(normalize(piece), ".;,: ")
		if p == "" {
			continue
		}
		if len([]rune(p)) < MinSegment && len(pieces) > 1 {
			return false
		}
		i := strings.Index(src[pos:], p)
		if i < 0 {
			return false
		}
		pos += i + len(p)
		matched++
	}
	return matched > 0
}

var (
	// markup is what would make a claim render a link, an image or HTML.
	// Sources are legal text in plain words; a claim that carries markup was
	// told to by something other than its sources.
	markup = regexp.MustCompile(`(?i)!\[|\]\(|<\s*/?\s*[a-z!][^>]*>|\bdata:[a-z]+/|javascript:`)
	// url is a web address in a claim's text.
	url = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s)\]"'<>]+`)
)

// Unlinked reports whether text points nowhere its sources do not: no
// Markdown link or image, no HTML, and no web address that is not written in
// one of the sources. A prompt injection that wants the reader to click, or
// the browser to fetch an address carrying the conversation, has to get past
// this as well as past the verbatim check of the quotes.
func Unlinked(text string, sources ...string) bool {
	if markup.MatchString(text) {
		return false
	}
	for _, u := range url.FindAllString(text, -1) {
		u = strings.TrimRight(u, ".,;:")
		found := false
		for _, s := range sources {
			if strings.Contains(strings.ToLower(s), strings.ToLower(u)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
