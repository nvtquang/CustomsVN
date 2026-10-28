package jobs

import (
	"fmt"
	"strings"
	"testing"
)

func TestLogWriterProgressIsTheLastLine(t *testing.T) {
	w := &logWriter{}
	fmt.Fprint(w, "chunks: 100/70654\n")
	fmt.Fprint(w, "chunks: 200/70654\n\n")
	if _, p, changed := w.snapshot(); p != "chunks: 200/70654" || !changed {
		t.Errorf("progress %q changed %v", p, changed)
	}
	if _, _, changed := w.snapshot(); changed {
		t.Error("a snapshot with nothing new reports a change")
	}
	fmt.Fprint(w, "embedding batch 3")
	if _, p, _ := w.snapshot(); p != "embedding batch 3" {
		t.Errorf("a partial line is not the progress: %q", p)
	}
}

func TestLogWriterKeepsHeadAndTail(t *testing.T) {
	w := &logWriter{}
	fmt.Fprintln(w, "FIRST")
	line := strings.Repeat("x", 1023) + "\n"
	for i := 0; i < maxLog/len(line)*2; i++ {
		fmt.Fprint(w, line)
	}
	fmt.Fprintln(w, "LAST")
	text, p, _ := w.snapshot()
	if len(text) > maxLog+64 || !strings.HasPrefix(text, "FIRST") || !strings.HasSuffix(text, "LAST\n") || !strings.Contains(text, "[…]") {
		t.Errorf("len %d head %q tail %q", len(text), text[:8], text[len(text)-8:])
	}
	if p != "LAST" {
		t.Errorf("progress %q", p)
	}
}
