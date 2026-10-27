// Package prep builds the customs corpus from a luatdo data directory.
//
// It runs after luatdo's own fetch, parse, cite and subjects stages and reads
// what they left on disk. Each step writes plain files under one output
// directory and the next step reads them back, so any step can be rerun alone
// and two runs over the same inputs write the same bytes.
package prep

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/luatdo/store"
)

// Steps, in the order they run.
const (
	StepTier   = "tier"   // tier every store document, pick quarantined ones worth rescuing
	StepVBHN   = "vbhn"   // parse the customs consolidated texts luatdo leaves out
	StepRescue = "rescue" // reparse quarantined in-force customs documents
	StepHS     = "hs"     // the HS nomenclature of TT 31/2022 from its HTML tables
	StepTables = "tables" // the data tables of every corpus document, as grids
	StepChunks = "chunks" // merge consolidated, original and rescued texts into chunks
	StepReport = "report" // coverage of the mandatory list, old documents for expert review
)

// Steps lists every step in run order.
var Steps = []string{StepTier, StepVBHN, StepRescue, StepHS, StepTables, StepChunks, StepReport}

// Revision is the th1nhng0 revision the baseline was measured on. It is the
// one named in NOTICE; moving it means remeasuring the baseline.
const Revision = "8977887f17be2defae4c5171d55562e1cde7d695"

// Options configures a run.
type Options struct {
	// Data is the luatdo data directory. Empty means LUATDO_DATA.
	Data string
	// Out is where prep writes. Empty means <Data>/customsvn/prep.
	Out string
	// AsOf is the day force is judged on, YYYY-MM-DD. Empty means today. It is
	// an input like any other: two runs agree only if they agree on it.
	AsOf string
	// Revision of the th1nhng0 dataset under <Data>/raw. Empty means Revision.
	Revision string
	// Legacy fills consolidated texts that are empty at the source from the
	// dataset's legacy_content config (see NOTICE). The CLI turns it on; a
	// data directory without the legacy files is noted and skipped.
	Legacy bool
	// Log receives progress lines. Nil means none.
	Log io.Writer
}

type run struct {
	Options
	store *store.Store
}

// Run executes the named steps in run order, or all of them when none are named.
func Run(opts Options, steps ...string) error {
	if len(steps) == 0 {
		steps = Steps
	}
	for _, s := range steps {
		if !slices.Contains(Steps, s) {
			return fmt.Errorf("unknown step %q (want one of %s)", s, strings.Join(Steps, ", "))
		}
	}
	if opts.Data == "" {
		opts.Data = os.Getenv("LUATDO_DATA")
	}
	if opts.Data == "" {
		return fmt.Errorf("no data directory: set LUATDO_DATA or pass -data")
	}
	if _, err := os.Stat(filepath.Join(opts.Data, "docs")); err != nil {
		return fmt.Errorf("data directory %s has no luatdo store: %w", opts.Data, err)
	}
	st, err := store.Open(opts.Data)
	if err != nil {
		return err
	}
	if opts.Out == "" {
		opts.Out = filepath.Join(opts.Data, "customsvn", "prep")
	}
	if opts.AsOf == "" {
		opts.AsOf = time.Now().Format(time.DateOnly)
	}
	if _, err := time.Parse(time.DateOnly, opts.AsOf); err != nil {
		return fmt.Errorf("as-of date %q is not YYYY-MM-DD", opts.AsOf)
	}
	if opts.Revision == "" {
		opts.Revision = Revision
	}
	if opts.Log == nil {
		opts.Log = io.Discard
	}
	r := &run{Options: opts, store: st}

	if slices.Contains(steps, StepTier) {
		if err := r.tier(); err != nil {
			return fmt.Errorf("tier: %w", err)
		}
	}
	// The content steps share one pass over the 800 MB content file.
	var content []string
	for _, s := range []string{StepVBHN, StepRescue, StepHS, StepTables} {
		if slices.Contains(steps, s) {
			content = append(content, s)
		}
	}
	if len(content) > 0 {
		if err := r.content(content); err != nil {
			return fmt.Errorf("%s: %w", strings.Join(content, "+"), err)
		}
	}
	if slices.Contains(steps, StepChunks) {
		if err := r.chunks(); err != nil {
			return fmt.Errorf("chunks: %w", err)
		}
	}
	if slices.Contains(steps, StepReport) {
		if err := r.report(); err != nil {
			return fmt.Errorf("report: %w", err)
		}
	}
	return nil
}

func (r *run) logf(format string, args ...any) {
	fmt.Fprintf(r.Log, format+"\n", args...)
}

func (r *run) out(parts ...string) string {
	return filepath.Join(append([]string{r.Out}, parts...)...)
}

func (r *run) rawData() string {
	return filepath.Join(r.store.Raw(), "th1nhng0", r.Revision, "data")
}
