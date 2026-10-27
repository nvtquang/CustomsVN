package prep

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/tamnd/luatdo/store"
)

// jsonl collects rows in memory and writes them in one atomic write, so a
// crashed run never leaves half a file that a later step would trust.
type jsonl struct {
	buf bytes.Buffer
	enc *json.Encoder
	n   int
}

func newJSONL() *jsonl {
	j := &jsonl{}
	j.enc = json.NewEncoder(&j.buf)
	// Legal text is full of "<", ">" and "&"; escaping them would make the
	// file unreadable to a person and change nothing for a machine.
	j.enc.SetEscapeHTML(false)
	return j
}

func (j *jsonl) add(v any) error {
	j.n++
	return j.enc.Encode(v)
}

// write saves the rows and returns their SHA-256.
func (j *jsonl) write(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := store.WriteFile(path, j.buf.Bytes()); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return store.HashBytes(j.buf.Bytes()), nil
}

// writeJSON writes v indented, without HTML escaping.
func writeJSON(path string, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return store.WriteFile(path, buf.Bytes())
}

// readJSONL reads one JSON value per line, with room for a whole document on
// a line.
func readJSONL[T any](path string, visit func(T) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<28)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var v T
		if err := json.Unmarshal(line, &v); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := visit(v); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// jsonFiles lists the .json files of a directory in name order, so every run
// visits them in the same order whatever the file system returns.
func jsonFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, filepath.Join(dir, e.Name()))
		}
	}
	slices.Sort(names)
	return names, nil
}

// readAll decodes every file into a T, in parallel, and returns them in the
// order of paths.
func readAll[T any](paths []string) ([]T, error) {
	out := make([]T, len(paths))
	errs := make([]error, len(paths))
	next := make(chan int)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for i := range next {
				errs[i] = store.ReadJSON(paths[i], &out[i])
				if errs[i] != nil {
					errs[i] = fmt.Errorf("%s: %w", paths[i], errs[i])
				}
			}
		})
	}
	for i := range paths {
		next <- i
	}
	close(next)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// resetDir empties a directory a step owns, so a file from an earlier run
// whose document is no longer selected does not survive into this one.
func resetDir(dir string) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o755)
}
