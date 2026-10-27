package corpus

import (
	"regexp"
	"slices"
	"strings"
)

// Quarantine verdicts. luatdo quarantines a document whose body names another
// number than its metadata, and one whose text has no article structure. Most
// customs documents caught that way are false alarms.
const (
	// VerdictFormat: the two numbers differ only in form (09 and 9, a dropped
	// suffix). The same instrument.
	VerdictFormat = "format"
	// VerdictYearTypo: same number and suffix, another year. Most likely a typo
	// at the source; kept, and flagged for review.
	VerdictYearTypo = "year-typo"
	// VerdictNumberDiffers: same year and suffix, another number. Perhaps the
	// text of another instrument; not rescued.
	VerdictNumberDiffers = "number-differs"
	// VerdictOtherDocument: nothing in common. Not rescued.
	VerdictOtherDocument = "other-document"
	// VerdictNoStructure: no "Điều" in the text. Rescued as text only.
	VerdictNoStructure = "no-structure"
)

var numberMismatch = regexp.MustCompile(`^body carries official number (.+), metadata says (.+)$`)

// Verdict classifies a quarantine reason.
func Verdict(quarantine string) string {
	m := numberMismatch.FindStringSubmatch(quarantine)
	if m == nil {
		return VerdictNoStructure
	}
	body, mine := looseNumber(m[1]), looseNumber(m[2])
	bp, mp := strings.Split(body, "/"), strings.Split(mine, "/")
	switch {
	case body == mine || strings.HasPrefix(mine, body) || strings.HasPrefix(body, mine):
		return VerdictFormat
	case len(bp) >= 3 && len(mp) >= 3 && bp[0] == mp[0] && slices.Equal(bp[2:], mp[2:]):
		return VerdictYearTypo
	case len(bp) >= 3 && len(mp) >= 3 && slices.Equal(bp[1:], mp[1:]) && bp[0] != mp[0]:
		return VerdictNumberDiffers
	}
	return VerdictOtherDocument
}

// Rescued reports whether a verdict lets the document back into the corpus.
func Rescued(verdict string) bool {
	return verdict != VerdictNumberDiffers && verdict != VerdictOtherDocument
}

// looseNumber is NormNumber with leading zeros dropped from every part.
func looseNumber(n string) string {
	parts := strings.Split(NormNumber(strings.ReplaceAll(n, " ", "")), "/")
	for i, p := range parts {
		parts[i] = strings.TrimLeft(p, "0")
	}
	return strings.Join(parts, "/")
}
