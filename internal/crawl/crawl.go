// Package crawl brings the corpus up to date with vbpl.vn once a week. It
// drives the crawler published with the th1nhng0 dataset (Scrapy, in
// crawler/ of the dataset repository) rather than a crawler of its own, so
// what it writes has exactly the dataset's schema and luatdo, prep and ingest
// read it unchanged.
//
// A run:
//  1. walks the newest pages of the vbpl.vn catalogue and fetches the
//     documents the latest revision does not have;
//  2. keeps those in scope: a customs or related cue in the title, field or
//     sector, or a relation to a document already in tiers 1-2;
//  3. fetches again the in-scope documents the new ones amend, replace or
//     repeal, and a quarter of the rest in turn, so a status the portal
//     changed is picked up within a month;
//  4. merges what changed into a copy of the latest revision with the
//     dataset's own upsert script, as a new revision directory with a
//     manifest, which luatdo and prep then take as the latest.
//
// The Hugging Face snapshot (prep.Revision) stays the base and is never
// modified; crawl revisions are pruned to the last few.
package crawl

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/prep"
)

// Prefix names the revision directories a crawl writes.
const Prefix = "vbpl-"

// Options configures a run. Zero values take the defaults below.
type Options struct {
	Data    string // luatdo data directory
	Crawler string // the dataset's crawler/ directory: scrapy.cfg, vbpl/, the scripts
	Python  string // the interpreter with scrapy, polars and pyarrow

	MaxPages    int     // catalogue pages walked, newest first (10)
	PageSize    int     // documents per catalogue page (100)
	Concurrency int     // requests in flight (4)
	Delay       float64 // seconds between requests (0.5)
	// RefreshParts is how many runs it takes to fetch every in-scope
	// document again (4: about a month at one run a week).
	RefreshParts int
	// NoRotation fetches again only the documents the new ones act on, for
	// a quick run by hand.
	NoRotation bool
	Keep       int // crawl revisions kept, the new one included (2)
	// CatalogAction overrides the portal's catalogue server action, which
	// changes when vbpl.vn is redeployed.
	CatalogAction string
	UserAgent     string

	Now func() time.Time
	Log io.Writer
}

// Result is what a run found and wrote.
type Result struct {
	Base      string `json:"base"`               // revision the run built on
	Revision  string `json:"revision,omitempty"` // revision written; empty when nothing changed
	Fetched   int    `json:"fetched"`            // new documents fetched
	InScope   int    `json:"in_scope"`           // of those, kept
	Rechecked int    `json:"rechecked"`          // in-scope documents fetched again
	Changed   int    `json:"changed"`            // of those, with a new status, dates or title
}

func (o *Options) defaults() {
	if o.Python == "" {
		o.Python = "python3"
	}
	if o.MaxPages <= 0 {
		o.MaxPages = 10
	}
	if o.PageSize <= 0 {
		o.PageSize = 100
	}
	if o.Concurrency <= 0 {
		o.Concurrency = 4
	}
	if o.Delay <= 0 {
		o.Delay = 0.5
	}
	if o.RefreshParts <= 0 {
		o.RefreshParts = 4
	}
	if o.Keep <= 0 {
		o.Keep = 2
	}
	if o.UserAgent == "" {
		o.UserAgent = "CustomsVN (+https://github.com/nvtquang/CustomsVN)"
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Log == nil {
		o.Log = io.Discard
	}
}

// Item is one document as the dataset's spider writes it.
type Item struct {
	ID            string              `json:"id"`
	Title         string              `json:"title"`
	Number        string              `json:"so_ky_hieu"`
	DocType       string              `json:"loai_van_ban"`
	EffectiveFrom string              `json:"ngay_co_hieu_luc"`
	ExpiredOn     string              `json:"ngay_het_hieu_luc"`
	Field         string              `json:"linh_vuc"`
	Sector        string              `json:"nganh"`
	Status        string              `json:"tinh_trang_hieu_luc"`
	Relationships map[string][]string `json:"relationships"`
	raw           json.RawMessage
}

// Run crawls, filters and merges. It returns a Result with an empty Revision
// when nothing in scope is new or changed; nothing is written then.
func Run(ctx context.Context, opts Options) (*Result, error) {
	opts.defaults()
	if opts.Data == "" {
		return nil, errors.New("crawl: no data directory")
	}
	if _, err := os.Stat(filepath.Join(opts.Crawler, "scrapy.cfg")); err != nil {
		return nil, fmt.Errorf("crawl: the dataset's crawler is not at %q (set CUSTOMSVN_CRAWLER): %w", opts.Crawler, err)
	}
	logf := func(f string, a ...any) { fmt.Fprintf(opts.Log, "crawl: "+f+"\n", a...) }
	now := opts.Now()
	res := &Result{Base: prep.LatestRevision(opts.Data)}
	raw := filepath.Join(opts.Data, "raw", "th1nhng0")
	baseData := filepath.Join(raw, res.Base, "data")
	work := filepath.Join(opts.Data, "customsvn", "crawl", now.UTC().Format("20060102T150405"))
	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, err
	}
	logf("base revision %s, work directory %s", res.Base, work)

	rows, err := parquet.ReadFile[corpus.Row](filepath.Join(baseData, "metadata.parquet"))
	if err != nil {
		return nil, fmt.Errorf("crawl: read metadata: %w", err)
	}
	current := make(map[string]corpus.Row, len(rows))
	for _, r := range rows {
		current[r.ID] = r
	}
	scope, err := inScopeKnown(filepath.Join(opts.Data, "customsvn", "prep", "domain.jsonl"), rows)
	if err != nil {
		return nil, err
	}
	logf("%d documents in the base, %d of them in scope", len(current), len(scope))

	// 1. New documents from the newest catalogue pages. Every known id is
	// handed to the spider as already seen, so only new ones are fetched.
	known := filepath.Join(work, "known.jsonl")
	if err := writeIDs(known, keys(current), nil); err != nil {
		return nil, err
	}
	fresh := filepath.Join(work, "new.jsonl")
	out, err := opts.scrapy(ctx, work, "new",
		"-a", "full=1", "-a", "max_pages="+strconv.Itoa(opts.MaxPages), "-a", "page_size="+strconv.Itoa(opts.PageSize),
		"-a", "resume=1", "-a", "resume_from="+known, "-a", "output="+fresh)
	if err != nil {
		return nil, err
	}
	news, err := readItems(fresh)
	if err != nil {
		return nil, err
	}
	if len(news) == 0 && bytes.Contains(out, []byte("catalog action ID may have changed")) {
		return nil, errors.New("crawl: the vbpl.vn catalogue did not parse; its action ID has likely changed: set CUSTOMSVN_VBPL_ACTION to the new one")
	}
	res.Fetched = len(news)
	var keep []Item
	for _, it := range news {
		if _, ok := current[it.ID]; ok {
			continue
		}
		if InScope(it, scope) {
			keep = append(keep, it)
		}
	}
	res.InScope = len(keep)
	logf("%d new documents fetched, %d in scope", res.Fetched, res.InScope)

	// 2. Documents in scope fetched again for their status.
	parts := opts.RefreshParts
	if opts.NoRotation {
		parts = 0
	}
	seeds := Recheck(keep, scope, parts, now)
	if len(seeds) > 0 {
		seedFile := filepath.Join(work, "seeds.txt")
		if err := os.WriteFile(seedFile, []byte(strings.Join(seeds, "\n")+"\n"), 0o644); err != nil {
			return nil, err
		}
		// Every known id but the seeds counts as seen, so the spider does not
		// walk out from a seed into the documents it cites.
		seen := filepath.Join(work, "seen.jsonl")
		skip := map[string]bool{}
		for _, s := range seeds {
			skip[s] = true
		}
		fetchedIDs := make([]string, 0, len(news))
		for _, it := range news {
			fetchedIDs = append(fetchedIDs, it.ID)
		}
		if err := writeIDs(seen, append(keys(current), fetchedIDs...), skip); err != nil {
			return nil, err
		}
		again := filepath.Join(work, "recheck.jsonl")
		if _, err := opts.scrapy(ctx, work, "recheck",
			"-a", "seed_file="+seedFile, "-a", "resume=1", "-a", "resume_from="+seen, "-a", "output="+again,
			"-s", "DEPTH_LIMIT=1", "-s", "CLOSESPIDER_ITEMCOUNT="+strconv.Itoa(len(seeds)+200)); err != nil {
			return nil, err
		}
		items, err := readItems(again)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			old, ok := current[it.ID]
			switch {
			case !ok:
				// Linked from a seed and missing from the base: new, if in scope.
				if InScope(it, scope) && !slices.ContainsFunc(keep, func(k Item) bool { return k.ID == it.ID }) {
					keep = append(keep, it)
					res.InScope++
				}
			case scope[it.ID]:
				res.Rechecked++
				if Changed(old, it) {
					res.Changed++
					keep = append(keep, it)
				}
			}
		}
		logf("%d in-scope documents fetched again, %d changed", res.Rechecked, res.Changed)
	}
	if len(keep) == 0 {
		logf("nothing new or changed in scope; no revision written")
		return res, nil
	}

	// 3. Merge into a new revision with the dataset's own scripts.
	merge := filepath.Join(work, "merge.jsonl")
	if err := writeItems(merge, keep); err != nil {
		return nil, err
	}
	refresh := filepath.Join(work, "refresh")
	if _, err := opts.python(ctx, "build_full_dataset.py", merge, refresh); err != nil {
		return nil, err
	}
	res.Revision = Prefix + now.UTC().Format("20060102T150405")
	dir := filepath.Join(raw, res.Revision)
	if err := opts.merge(ctx, baseData, refresh, filepath.Join(dir, "data")); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if err := writeManifest(dir, res, now); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	logf("revision %s written: %d new, %d changed", res.Revision, res.InScope, res.Changed)
	prune(raw, opts.Keep, logf)
	prune(filepath.Dir(work), 8, logf)
	return res, nil
}

// scopeCues are the phrases that put a document in tiers 1-2 by its words;
// the field and sector the portal gives count as much as the title.
var scopeCues = append(append([]string{}, corpus.CoreCues...), corpus.NearCues...)

// InScope reports whether a new document belongs in the customs corpus: a
// customs or related cue in its title, field or sector, or a relation to a
// document already in scope (an amendment of a customs decree need not say
// "hải quan" in its title).
func InScope(it Item, scope map[string]bool) bool {
	if len(corpus.Cues(it.Title+" "+it.Field+" "+it.Sector, scopeCues)) > 0 {
		return true
	}
	for _, ids := range it.Relationships {
		for _, id := range ids {
			if scope[id] {
				return true
			}
		}
	}
	return false
}

// weakRelations say nothing about the target's force: a document citing
// another as its basis, or translating it, leaves it as it was.
var weakRelations = map[string]bool{"Căn cứ": true, "Dẫn chiếu": true, "Bản dịch": true}

// Recheck picks the in-scope documents to fetch again: those the new
// documents act on, then the share of the rest whose turn it is this week,
// one in parts; with parts 0, none of the rest.
func Recheck(news []Item, scope map[string]bool, parts int, now time.Time) []string {
	pick := map[string]bool{}
	for _, it := range news {
		for rel, ids := range it.Relationships {
			if weakRelations[rel] {
				continue
			}
			for _, id := range ids {
				if scope[id] {
					pick[id] = true
				}
			}
		}
	}
	if parts <= 0 {
		return keys(pick)
	}
	_, week := now.ISOWeek()
	for id := range scope {
		h := fnv.New32a()
		h.Write([]byte(id))
		if int(h.Sum32()%uint32(parts)) == week%parts {
			pick[id] = true
		}
	}
	return keys(pick)
}

// Changed reports whether a fetched document differs from the base in what
// the corpus reads about its force.
func Changed(old corpus.Row, it Item) bool {
	same := func(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) }
	// The spider leaves a field empty when the portal does; the upsert then
	// keeps the old value, so an empty field is no change.
	differs := func(a, b string) bool { return strings.TrimSpace(b) != "" && !same(a, b) }
	return differs(old.TinhTrang, it.Status) || differs(old.NgayHetHieu, it.ExpiredOn) ||
		differs(old.NgayHieuLuc, it.EffectiveFrom) || differs(old.Title, it.Title)
}

// inScopeKnown is the ids of the base documents in tiers 1-2: those prep
// tiered, and the consolidated texts, which prep tiers from the metadata.
func inScopeKnown(domainPath string, rows []corpus.Row) (map[string]bool, error) {
	scope := map[string]bool{}
	f, err := os.Open(domainPath)
	if err != nil {
		return nil, fmt.Errorf("crawl: %w (run customsvn prep once first)", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var e struct {
			SourceURL string `json:"source_url"`
			Tier      int    `json:"tier"`
		}
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Tier >= corpus.Tier1 && e.Tier <= corpus.Tier2 {
			if id := corpus.ItemID(e.SourceURL); id != "" {
				scope[id] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	for i := range rows {
		if corpus.IsVBHN(&rows[i]) && corpus.TierVBHN(&rows[i]) <= corpus.Tier2 {
			scope[rows[i].ID] = true
		}
	}
	return scope, nil
}

func (o *Options) scrapy(ctx context.Context, work, name string, args ...string) ([]byte, error) {
	base := []string{"-m", "scrapy", "crawl", "vbpl",
		"-s", "CONCURRENT_REQUESTS=" + strconv.Itoa(o.Concurrency),
		"-s", "CONCURRENT_REQUESTS_PER_DOMAIN=" + strconv.Itoa(o.Concurrency),
		"-s", "DOWNLOAD_DELAY=" + strconv.FormatFloat(o.Delay, 'f', -1, 64),
		"-s", "AUTOTHROTTLE_ENABLED=True",
		"-s", "USER_AGENT=" + o.UserAgent,
		"-s", "LOG_LEVEL=INFO",
		"-s", "LOG_FILE=" + filepath.Join(work, name+".log"),
	}
	if o.CatalogAction != "" {
		base = append(base, "-a", "catalog_action_id="+o.CatalogAction)
	}
	fmt.Fprintf(o.Log, "crawl: scrapy %s\n", name)
	if _, err := o.run(ctx, o.Crawler, append(base, args...)...); err != nil {
		return nil, fmt.Errorf("crawl: scrapy %s: %w", name, err)
	}
	// Scrapy logs to the file; the tail goes to the job log.
	logText, _ := os.ReadFile(filepath.Join(work, name+".log"))
	fmt.Fprintln(o.Log, tail(string(logText), 15))
	return logText, nil
}

func (o *Options) python(ctx context.Context, script string, args ...string) ([]byte, error) {
	fmt.Fprintf(o.Log, "crawl: %s\n", script)
	out, err := o.run(ctx, o.Crawler, append([]string{script}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("crawl: %s: %w", script, err)
	}
	return out, nil
}

func (o *Options) run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, o.Python, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(o.Log, &buf)
	cmd.Stderr = io.MultiWriter(o.Log, &buf)
	err := cmd.Run()
	return buf.Bytes(), err
}

func readItems(path string) ([]Item, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil // the spider writes no file when it finds nothing
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Item
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 256<<20) // a document's HTML can run to megabytes
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var it Item
		if err := json.Unmarshal(line, &it); err != nil || it.ID == "" || seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		it.raw = append(json.RawMessage(nil), line...)
		out = append(out, it)
	}
	return out, sc.Err()
}

// writeItems writes the spider's lines back as they came, for the dataset's
// build script.
func writeItems(path string, items []Item) error {
	var b bytes.Buffer
	for _, it := range items {
		b.Write(it.raw)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// writeIDs writes ids in the form the spider's resume option reads.
func writeIDs(path string, ids []string, skip map[string]bool) error {
	var b bytes.Buffer
	for _, id := range ids {
		if !skip[id] {
			fmt.Fprintf(&b, "{\"id\": %q}\n", id)
		}
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// writeManifest records the revision as luatdo's fetch does, so luatdo takes
// it as the latest: fetched_at decides, and every file carries its hash.
func writeManifest(dir string, res *Result, now time.Time) error {
	m := map[string]any{
		"dataset":    "th1nhng0",
		"repo":       "th1nhng0/vietnamese-legal-documents",
		"revision":   res.Revision,
		"fetched_at": now.UTC(),
		"extra": map[string]string{
			"source": "vbpl.vn crawl by the dataset's crawler; metadata and relations merged by its upsert functions, content by CustomsVN",
			"base":   res.Base, "new": strconv.Itoa(res.InScope), "changed": strconv.Itoa(res.Changed),
		},
	}
	files := map[string]any{}
	for _, name := range []string{"metadata.parquet", "relationships.parquet", "content.parquet"} {
		f, err := os.Open(filepath.Join(dir, "data", name))
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		files["data/"+name] = map[string]any{"size": n, "sha256": hex.EncodeToString(h.Sum(nil))}
	}
	m["files"] = files
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644)
}

// prune removes all but the newest keep crawl directories under dir. The
// Hugging Face snapshot is never touched: its name has no crawl prefix, and
// the work directories have none either but sit in a directory of their own.
func prune(dir string, keep int, logf func(string, ...any)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && (strings.HasPrefix(e.Name(), Prefix) || filepath.Base(dir) == "crawl") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // timestamps sort in time order
	for len(names) > keep {
		if err := os.RemoveAll(filepath.Join(dir, names[0])); err == nil {
			logf("removed %s", filepath.Join(dir, names[0]))
		}
		names = names[1:]
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
