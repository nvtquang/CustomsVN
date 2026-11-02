package search

import (
	"regexp"
	"sort"
	"strings"
)

// A provision that says when another one does not apply: "Những trường hợp
// không xử phạt", in its heading or its first lines, and a reference back to
// an article of the same instrument ("khai sai theo quy định tại Điều 10
// Nghị định này"). "trừ trường hợp" is left out: it is everywhere, and it
// points the other way, from the rule to its carve-outs.
var (
	exceptionCue  = regexp.MustCompile(`(?i)không xử phạt|không bị xử phạt|không áp dụng|miễn trách nhiệm`)
	transitionCue = regexp.MustCompile(`(?i)chuyển tiếp`)
	sameDocRef    = regexp.MustCompile(`(?i)điều\s+(\d+[a-z]?)(?:\s+của)?\s+(?:nghị định|thông tư|luật|quyết định|bộ luật)\s+này`)
)

// indexExceptions maps each article to the provisions of its own instrument
// that carve exceptions out of it.
func (c *Corpus) indexExceptions() {
	c.exceptions = map[string][]string{}
	c.transitions = map[string][]string{}
	for id, ch := range c.Chunks {
		heading := strings.Join(ch.Path, " ")
		kind, index := "", c.exceptions
		switch {
		case transitionCue.MatchString(heading):
			kind, index = "transition", c.transitions
		case exceptionCue.MatchString(heading + " " + firstRunes(ch.Text, 300)):
			kind = "exception"
		}
		if kind == "" {
			continue
		}
		own := parentKey(id)
		for _, m := range sameDocRef.FindAllStringSubmatch(ch.Text, -1) {
			art := ch.DocID + ":article-" + strings.ToLower(m[1])
			if art == own || containsStr(index[art], id) {
				continue
			}
			index[art] = append(index[art], id)
		}
	}
	for _, ids := range c.exceptions {
		sort.Strings(ids)
	}
	for _, ids := range c.transitions {
		sort.Strings(ids)
	}
}

// Transitions lists the transition provisions ("Điều khoản chuyển tiếp") of
// an instrument that refer to the articles the given chunks belong to, in the
// order the chunks come, each once. "Dự án đã được đầu tư trước ngày
// 01/7/2025 ... được áp dụng quy định hoàn thuế tại Điều 30 Nghị định này"
// is the answer to what happens to a project begun before the decree, and an
// answer that quotes Article 30 alone misses it.
func (c *Corpus) Transitions(chunkIDs []string) []string {
	have := map[string]bool{}
	for _, id := range chunkIDs {
		have[id] = true
	}
	var out []string
	for _, id := range chunkIDs {
		for _, e := range c.transitions[parentKey(id)] {
			if !have[e] {
				have[e] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// Exceptions lists the provisions that carve exceptions out of the articles
// the given chunks belong to, in the order the chunks come, each once. A
// penalty quoted without its "no penalty when" clause reads as the whole
// rule: the composer concluded a fine was due for a correction made within
// the 60 days in which decree 169/2026 says there is none.
func (c *Corpus) Exceptions(chunkIDs []string) []string {
	have := map[string]bool{}
	for _, id := range chunkIDs {
		have[id] = true
	}
	var out []string
	for _, id := range chunkIDs {
		for _, e := range c.exceptions[parentKey(id)] {
			if !have[e] {
				have[e] = true
				out = append(out, e)
			}
		}
	}
	return out
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
