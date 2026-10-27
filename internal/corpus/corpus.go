// Package corpus decides which documents of the luatdo store make up the
// customs corpus and cuts them into retrievable chunks.
//
// Everything here is deterministic and works on values, not files: tiering,
// the force filter, the reading of consolidated texts, the quarantine verdicts,
// chunking and the HS nomenclature. The prep package does the reading and
// writing around it.
package corpus

import (
	"regexp"
	"strconv"
	"strings"
)

// Meta is a store document without its provisions. Tiering and the force
// filter run over all 128 thousand documents, and none of them needs the text.
type Meta struct {
	ID             string `json:"id"`
	OfficialNumber string `json:"official_number"`
	IssuingBody    string `json:"issuing_body,omitempty"`
	Title          string `json:"title"`
	DocType        string `json:"doc_type"`
	EffectiveFrom  string `json:"effective_from,omitempty"`
	SignedOn       string `json:"signed_on,omitempty"`
	ExpiredOn      string `json:"expired_on,omitempty"`
	ForceStatus    string `json:"force_status,omitempty"`
	SourceURL      string `json:"source_url,omitempty"`
	Status         string `json:"status"`
	Quarantine     string `json:"quarantine,omitempty"`
}

// The force statuses vbpl.vn records, in its own words.
const (
	StatusInForce      = "Còn hiệu lực"
	StatusPartly       = "Hết hiệu lực một phần"
	StatusNotYet       = "Chưa có hiệu lực"
	StatusExpired      = "Hết hiệu lực toàn bộ"
	StatusSuspended    = "Ngưng hiệu lực"
	StatusObsolete     = "Không còn phù hợp"
	StatusUndetermined = "Chưa xác định"
)

// The force a kept chunk carries.
const (
	ForceInForce = "in_force"
	ForcePartly  = "partly_expired"
	ForceNotYet  = "not_yet_in_force"
)

var forceOfStatus = map[string]string{
	StatusInForce: ForceInForce,
	StatusPartly:  ForcePartly,
	StatusNotYet:  ForceNotYet,
}

var dayMonthYear = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})/(\d{4})`)

// ISODate turns "1/7/2016" into "2016-07-01" so dates compare as strings.
//
// Unlike law.ISODate it accepts one digit days and months and hands anything
// else back unchanged rather than empty. The baseline was measured that way,
// and an ISO date in the source has to survive the trip.
func ISODate(s string) string {
	s = strings.TrimSpace(s)
	m := dayMonthYear.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	d, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	return m[3] + "-" + pad2(mo) + "-" + pad2(d)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// Force is the force a document has on asOf, or "" when it is no longer law.
//
// The expiry date wins over the status label, because the source updates the
// date before the label: a document marked "Còn hiệu lực" whose expiry date has
// passed is gone. A partly expired document is the exception, since its expiry
// date names the part that ended and not the whole.
func Force(m *Meta, asOf string) string {
	st := strings.TrimSpace(m.ForceStatus)
	if exp := ISODate(m.ExpiredOn); exp != "" && exp <= asOf && st != StatusPartly {
		return ""
	}
	if ISODate(m.EffectiveFrom) > asOf && (st == StatusInForce || st == StatusNotYet) {
		return ForceNotYet
	}
	return forceOfStatus[st]
}

// Kept reports the force of a document under the whole-store filter, which is
// Force restricted to documents the parser turned into structure, and why a
// dropped document was dropped. It feeds the in-force counts of the baseline.
func Kept(m *Meta, asOf string) (force, reason string) {
	st := strings.TrimSpace(m.ForceStatus)
	if exp := ISODate(m.ExpiredOn); exp != "" && exp <= asOf && st != StatusPartly {
		return "", "expiry date has passed"
	}
	f, ok := forceOfStatus[st]
	if !ok {
		if st == "" {
			st = "none"
		}
		return "", "force status '" + st + "'"
	}
	if m.Status != "parsed" {
		return "", "in force but not usable: " + m.Status
	}
	if eff := ISODate(m.EffectiveFrom); eff != "" && eff > asOf {
		return ForceNotYet, ""
	}
	return f, ""
}

// Live reports whether the source label alone says the document is still law.
// It is the looser test used to pick quarantined documents worth rescuing;
// Force has the final word on each of them.
func Live(m *Meta) bool {
	for _, p := range []string{"Còn", StatusPartly, "Chưa có"} {
		if strings.HasPrefix(m.ForceStatus, p) {
			return true
		}
	}
	return false
}

// NormNumber reduces an official number to the letters and digits that make
// it, so "43/VBHN-BCT" and "43/VBHN - BCT" compare equal.
func NormNumber(n string) string {
	n = strings.ReplaceAll(strings.ToUpper(n), "Đ", "D")
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '/' {
			return r
		}
		return -1
	}, n)
}

var storeNumber = regexp.MustCompile(`^(\d+[a-z]?)/(\d{4})/(.+)$`)
var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// StoreID is the store identifier of a centrally issued number, or "" when the
// number has no year. A body-relative number such as "01/2024/QĐ-UBND" gets
// the identifier without its body and so finds nothing, which is the right
// answer: the base of a customs consolidation is never a provincial act.
func StoreID(number string) string {
	m := storeNumber.FindStringSubmatch(number)
	if m == nil {
		return ""
	}
	rest := strings.ReplaceAll(strings.ToLower(m[3]), "đ", "d")
	return "vn:law:" + m[2] + ":" + strings.ToLower(m[1]) + "-" + m[2] + "-" + strings.Trim(nonSlug.ReplaceAllString(rest, "-"), "-")
}

// runes cuts s to at most n characters, not bytes.
func runes(s string, n int) string {
	i := 0
	for j := range s {
		if i == n {
			return s[:j]
		}
		i++
	}
	return s
}
