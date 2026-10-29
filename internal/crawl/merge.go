package crawl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/parquet-go/parquet-go"
)

// upsertSmall runs the dataset's upsert for the metadata and the relations,
// the two small tables, with its own functions, so the merge rules (a field
// the portal left empty keeps its old value; new relation labels replace the
// old ones of a pair) are the dataset's.
const upsertSmall = `import sys
from pathlib import Path
from upsert_dataset import upsert_metadata, upsert_relationships
current, refresh, out = map(Path, sys.argv[1:])
out.mkdir(parents=True)
upsert_metadata(current, refresh, out)
upsert_relationships(current, refresh, out)
`

// merge writes the new revision's tables. The dataset's upsert_dataset.py
// would do all three, but it holds the whole content table (3.5 GB of HTML)
// in memory and is killed in a container of a few GB, so the content table
// is merged here, row by row, by the same rule: a fetched document replaces
// the old row of its id, every other row is kept.
func (o *Options) merge(ctx context.Context, base, refresh, out string) error {
	if _, err := o.run(ctx, o.Crawler, "-c", upsertSmall, base, refresh, out); err != nil {
		return fmt.Errorf("crawl: upsert metadata and relationships: %w", err)
	}
	fmt.Fprintln(o.Log, "crawl: merge content")
	return mergeContent(filepath.Join(base, "content.parquet"), filepath.Join(refresh, "content.parquet"), filepath.Join(out, "content.parquet"))
}

type contentRow struct {
	ID          string `parquet:"id,optional"`
	ContentHTML string `parquet:"content_html,optional"`
}

func mergeContent(base, refresh, out string) error {
	fresh, err := parquet.ReadFile[contentRow](refresh)
	if err != nil {
		return fmt.Errorf("read %s: %w", refresh, err)
	}
	replaced := make(map[string]bool, len(fresh))
	for _, r := range fresh {
		replaced[r.ID] = true
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	w := parquet.NewGenericWriter[contentRow](f, parquet.Compression(&parquet.Zstd), parquet.MaxRowsPerRowGroup(5000))
	if _, err := w.Write(fresh); err != nil {
		return err
	}
	in, err := os.Open(base)
	if err != nil {
		return err
	}
	defer in.Close()
	rd := parquet.NewGenericReader[contentRow](in)
	defer rd.Close()
	buf := make([]contentRow, 256)
	for {
		n, err := rd.Read(buf)
		keep := buf[:0]
		for _, r := range buf[:n] {
			if !replaced[r.ID] {
				keep = append(keep, r)
			}
		}
		if _, werr := w.Write(keep); werr != nil {
			return werr
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", base, err)
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	return f.Close()
}
