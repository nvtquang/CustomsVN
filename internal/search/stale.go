package search

import (
	"regexp"
	"strings"

	"github.com/nvtquang/CustomsVN/internal/corpus"
)

// Amendment is an "amends" edge: a document that changes another one.
type Amendment struct {
	From     string // the amending document
	ToDoc    string // the amended document, when the edge names it
	ToNumber string // its official number otherwise
}

// A line that instructs a change: "8. Sửa đổi, bổ sung khoản 1, khoản 2
// Điều 20 như sau:", "b) Bãi bỏ khoản 3 Điều 5". Only the words before "như
// sau" are read, so the new wording that follows, which cites articles of
// its own, is not taken for the target.
var (
	instruction = regexp.MustCompile(`(?i)^\s*(?:\d+[a-z]?\.\s*)?(?:[a-zđ]\)\s*)?(?:sửa đổi, bổ sung|sửa đổi|bãi bỏ|thay thế|bổ sung)\b(.*?)(?:như sau|$)`)
	target      = regexp.MustCompile(`(?i)((?:(?:khoản|điểm)\s+[\p{L}\d,\s]+?)?)điều\s+(\d+[a-z]?)\b`)
	// "khoản 1, khoản 2" and "các khoản 2, 3 và 7": every number of a run.
	clauseRun   = regexp.MustCompile(`(?i)khoản\s+(\d+[a-z]?(?:\s*(?:,|và)\s*\d+[a-z]?)*)`)
	clauseNum   = regexp.MustCompile(`\d+[a-z]?`)
	officialNum = regexp.MustCompile(`\d+/\d{4}/[\p{L}\-]+`)
)

// superseded finds the provisions an amendment rewrote after a text of them
// was fixed: the original the amendment changes, and any consolidated text
// (VBHN) of it dated before the amendment took effect. The source keeps such
// a VBHN labelled in force, and preferring it over the original then quotes
// wording that no longer exists: 46/VBHN-BTC of 2019 still carries the
// articles of decree 08/2015 that decree 167/2025 rewrote. The result maps a
// chunk to the day it stopped being the law; the amending document itself is
// the source from that day.
//
// Only amending documents in the corpus count, since only their words say
// which provisions changed. One that amends several documents is read only
// in its articles headed with the number of the one at hand.
func superseded(c *Corpus, edges []Amendment) map[string]string {
	consolidations := map[string][]*Doc{}
	for _, d := range c.Docs {
		if d.SourceKind == corpus.SourceConsolidated && d.Consolidates != "" {
			consolidations[d.Consolidates] = append(consolidations[d.Consolidates], d)
		}
	}
	out := map[string]string{}
	for _, e := range edges {
		am := c.Docs[e.From]
		if am == nil || am.EffectiveFrom == "" {
			continue
		}
		var bases []*Doc
		if d := c.Docs[e.ToDoc]; d != nil {
			bases = append(bases, d)
		} else if e.ToNumber != "" {
			bases = c.numbers[corpus.NormNumber(e.ToNumber)]
		}
		for _, base := range bases {
			if base.SourceKind == corpus.SourceConsolidated {
				continue
			}
			keys := amendedProvisions(c, am, base.OfficialNumber)
			if len(keys) == 0 {
				continue
			}
			mark := func(d *Doc) {
				for _, ch := range c.byDoc[d.ID] {
					if coveredBy(plainTail(ch), keys) {
						if prev, ok := out[ch.ID]; !ok || am.EffectiveFrom < prev {
							out[ch.ID] = am.EffectiveFrom
						}
					}
				}
			}
			mark(base)
			// A VBHN signed after the amendment was signed already carries
			// it, even when the amendment takes effect later.
			for _, v := range consolidations[base.ID] {
				if dateOf(v) < dateOf(am) {
					mark(v)
				}
			}
		}
	}
	return out
}

// amendedProvisions lists what an amending document changes in the document
// numbered number, as tails: "article-20:clause-1", or "article-21" when the
// whole article is rewritten.
func amendedProvisions(c *Corpus, am *Doc, number string) []string {
	multi := len(uniq(officialNum.FindAllString(am.Title, -1))) != 1 || !strings.Contains(am.Title, number)
	seen := map[string]bool{}
	var keys []string
	for _, ch := range c.byDoc[am.ID] {
		if multi && !headedWith(ch, number) {
			continue
		}
		for _, line := range strings.Split(ch.Text, "\n") {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, `"`) || strings.HasPrefix(t, "“") {
				continue
			}
			m := instruction.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			for _, r := range target.FindAllStringSubmatch(m[1], -1) {
				art := "article-" + strings.ToLower(r[2])
				var clauses []string
				for _, run := range clauseRun.FindAllStringSubmatch(r[1], -1) {
					clauses = append(clauses, clauseNum.FindAllString(run[1], -1)...)
				}
				if len(clauses) == 0 {
					if !seen[art] {
						seen[art] = true
						keys = append(keys, art)
					}
					continue
				}
				for _, cl := range clauses {
					k := art + ":clause-" + strings.ToLower(cl)
					if !seen[k] {
						seen[k] = true
						keys = append(keys, k)
					}
				}
			}
		}
	}
	return keys
}

// headedWith reports whether a chunk is about the document numbered number:
// its article's heading names it, or its own first line does. A law
// amending several laws gives each one a clause that opens with its name and
// number ("Sửa đổi, bổ sung khoản 2 Điều 42 của Luật Hải quan số
// 54/2014/QH13…") under an article headed only "các luật có liên quan".
func headedWith(ch *Chunk, number string) bool {
	for _, p := range ch.Path {
		if strings.HasPrefix(p, "Điều") && strings.Contains(p, number) {
			return true
		}
	}
	first, _, _ := strings.Cut(ch.Text, "\n")
	return strings.Contains(first, number)
}

// coveredBy reports whether a chunk is one of the provisions, inside one, or
// an article that holds one: a whole article quoted with a rewritten clause
// in it is out of date too.
func coveredBy(tail string, keys []string) bool {
	for _, k := range keys {
		if tail == k || strings.HasPrefix(tail, k+":") || strings.HasPrefix(k, tail+":") {
			return true
		}
	}
	return false
}

// plainTail is a chunk's tail without the marks of a split: "~2" for the
// second piece of a long clause.
func plainTail(ch *Chunk) string {
	t := ch.Tail()
	parts := strings.Split(t, ":")
	for i, p := range parts {
		if j := strings.Index(p, "~"); j >= 0 {
			parts[i] = p[:j]
		}
	}
	return strings.Join(parts, ":")
}

func dateOf(d *Doc) string {
	if d.SignedOn != "" {
		return d.SignedOn
	}
	return d.EffectiveFrom
}

func uniq(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
