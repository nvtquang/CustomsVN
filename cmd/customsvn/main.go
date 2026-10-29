// Command customsvn is the CustomsVN binary: the data pipeline, the server that
// serves the API and the Web UI, and the evaluation tools, in one executable.
//
// The subcommands in docs/PLAN.md arrive one phase at a time; Phase 0 brings
// prep, which builds the customs corpus from a luatdo data directory.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/tamnd/luatdo/law"

	"github.com/nvtquang/CustomsVN/internal/corpus"
	"github.com/nvtquang/CustomsVN/internal/prep"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "customsvn:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "version":
		fmt.Println("customsvn", version, "luatdo", luatdoVersion())
		return nil
	case "docid":
		// A smoke check that the pinned luatdo is linked: the stable identifier
		// for an official number, e.g. "54/2014/QH13" -> vn:law:2014:54-2014-qh13.
		if len(args) != 2 {
			return fmt.Errorf("usage: customsvn docid <official number>")
		}
		id, err := law.DocID(args[1])
		if err != nil {
			return err
		}
		fmt.Println(id)
		return nil
	case "prep":
		return runPrep(args[1:])
	case "migrate":
		return runMigrate(args[1:])
	case "ingest":
		return runIngest(args[1:])
	case "embed":
		return runEmbed(args[1:])
	case "serve":
		return runServe(args[1:])
	case "eval":
		return runEval(args[1:])
	case "bootstrap":
		return runBootstrap(args[1:])
	case "mcp":
		return runMCP(args[1:])
	case "user":
		return runUser(args[1:])
	case "crawl":
		return runCrawl(args[1:])
	}
	return usage()
}

func usage() error {
	return errors.New(`usage: customsvn <command> [flags]

  version            print versions
  docid <number>     the luatdo identifier of an official number
  prep [step...]     build the customs corpus from a luatdo data directory
  migrate            bring the database schema up to date
  ingest             load the prep output into the database
  embed -model NAME  embed new and changed chunks
  serve              serve the API and the Web UI
  eval -goldset FILE run a gold set against the retriever and print metrics
  bootstrap          first data load: luatdo, prep, migrate, ingest
  mcp                serve MCP over stdin/stdout (the server also serves it at /mcp)
  user list|add|passwd  manage accounts of the Web UI
  crawl              fetch what is new on vbpl.vn into a new dataset revision (the server runs it weekly)`)
}

// parse parses flags and treats -h as success.
func parse(fs *flag.FlagSet, args []string) (bool, error) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func runPrep(args []string) error {
	fs := flag.NewFlagSet("prep", flag.ContinueOnError)
	var opts prep.Options
	fs.StringVar(&opts.Data, "data", "", "luatdo data directory (default $LUATDO_DATA)")
	fs.StringVar(&opts.Out, "out", "", "output directory (default <data>/customsvn/prep)")
	fs.StringVar(&opts.AsOf, "as-of", "", "judge force on this day, YYYY-MM-DD (default today)")
	fs.StringVar(&opts.Revision, "revision", "", "th1nhng0 revision under <data>/raw (default "+prep.Revision[:12]+")")
	fs.BoolVar(&opts.Legacy, "legacy", true, "fill empty consolidated texts from the legacy_content config (see NOTICE); -legacy=false to skip")
	fs.StringVar(&opts.Chunking.Mode, "chunking", corpus.ChunkGeneral, "general (an article per chunk) or parent_child (a chunk per clause)")
	fs.IntVar(&opts.Chunking.MaxLength, "max-length", corpus.MaxArticle, "general: split an article at its clauses past this many characters")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: customsvn prep [flags] [%s]\n", strings.Join(prep.Steps, " | "))
		fs.PrintDefaults()
	}
	if ok, err := parse(fs, args); !ok {
		return err
	}
	if err := opts.Chunking.Validate(); err != nil {
		return err
	}
	opts.Log = os.Stderr
	return prep.Run(opts, fs.Args()...)
}

func luatdoVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, m := range info.Deps {
		if m.Path == "github.com/tamnd/luatdo" {
			return m.Version
		}
	}
	return "unknown"
}
