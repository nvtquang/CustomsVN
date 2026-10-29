package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nvtquang/CustomsVN/internal/ingest"
	"github.com/nvtquang/CustomsVN/internal/prep"
)

// runBootstrap takes an empty data directory to a served corpus in one
// command: luatdo's fetch, parse, cite and subjects at the pinned revision,
// the legacy configs, prep, the schema and ingest. Each step is skipped when
// its output is already there, so the command can be rerun after a failure.
func runBootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	configPath := fs.String("config", "", "config file (default ./customsvn.yaml)")
	luatdo := fs.String("luatdo", "luatdo", "the luatdo binary")
	legacy := fs.Bool("legacy", true, "download and use the legacy_content config (see NOTICE)")
	asOf := fs.String("as-of", "", "judge force on this day, YYYY-MM-DD (default today)")
	if ok, err := parse(fs, args); !ok {
		return err
	}
	data := os.Getenv("LUATDO_DATA")
	if data == "" {
		return errors.New("set LUATDO_DATA to the data directory")
	}
	ctx, cancel := signalContext()
	defer cancel()
	say := func(f string, a ...any) { fmt.Fprintf(os.Stderr, "bootstrap: "+f+"\n", a...) }
	raw := filepath.Join(data, "raw", "th1nhng0", prep.Revision, "data")

	for _, config := range []string{"metadata", "relationships", "content"} {
		if exists(filepath.Join(raw, config+".parquet")) {
			continue
		}
		say("luatdo fetch %s", config)
		if err := runTool(ctx, *luatdo, "fetch", "--data", data, "--revision", prep.Revision, "--config", config, "th1nhng0"); err != nil {
			return err
		}
	}
	if *legacy {
		dir := filepath.Join(data, "raw", "th1nhng0", prep.Revision, "legacy")
		for _, f := range []string{"metadata.parquet", "content.parquet"} {
			if exists(filepath.Join(dir, f)) {
				continue
			}
			say("download legacy/%s", f)
			url := "https://huggingface.co/datasets/th1nhng0/vietnamese-legal-documents/resolve/" + prep.Revision + "/legacy/" + f
			if err := download(ctx, url, filepath.Join(dir, f)); err != nil {
				return err
			}
		}
	}
	// parse reads uts_vlc unless told otherwise; the corpus is th1nhng0.
	steps := []struct {
		name, done string
		args       []string
	}{
		{"parse", filepath.Join(data, "docs"), []string{"--dataset", "th1nhng0"}},
		{"cite", filepath.Join(data, "cite"), nil},
		{"subjects", filepath.Join(data, "subject", "assignments.jsonl"), nil},
	}
	for _, s := range steps {
		if nonEmpty(s.done) {
			continue
		}
		say("luatdo %s", s.name)
		if err := runTool(ctx, *luatdo, append([]string{s.name, "--data", data}, s.args...)...); err != nil {
			return err
		}
	}

	say("prep")
	if err := prep.Run(prep.Options{Data: data, AsOf: *asOf, Legacy: *legacy, Log: os.Stderr}); err != nil {
		return err
	}
	cfg, pool, err := openDB(ctx, *configPath)
	if err != nil {
		return err
	}
	defer pool.Close()
	say("ingest")
	res, err := ingest.Run(ctx, pool, ingest.Options{Prep: cfg.Prep, Data: data, Log: os.Stderr})
	if err != nil {
		return err
	}
	return printJSON(res)
}

func runTool(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", bin, args[0], err)
	}
	return nil
}

// download writes url to path through a temporary file, so an interrupted
// download is never mistaken for a finished one.
func download(ctx context.Context, url, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 2 * time.Hour}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func nonEmpty(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !st.IsDir() {
		return st.Size() > 0
	}
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) > 0
}
