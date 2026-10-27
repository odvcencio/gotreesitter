// Command scannercert reports whether a grammar's external scanner gives the
// same answers when it resumes from its serialized state. It writes one
// deterministic JSON report per grammar.
//
// It is report-only. It exits zero when it wrote every report, whatever the
// scanner findings, and it changes no grant. It exits nonzero for a report
// with the verdict not-transparent, because the parse with the recorder then
// differed from the parse without it. Run it from the repository root with the
// GOT_, GTS_ and GOTREESITTER_ variables unset; each report lists any that are
// set:
//
//	go run ./cmd/scannercert -language lua,yaml -out /tmp/scannercert
//	go run ./cmd/scannercert -language lua
//	go run ./cmd/scannercert -list
//
// The committed samples are small. To also parse each grammar's median and
// largest R4 inputs, fetch them first and name the directory:
//
//	python3 scripts/benchfixture_corpus.py fetch --language lua --output /tmp/r4
//	go run ./cmd/scannercert -language lua -fetched /tmp/r4 -out /tmp/scannercert
//
// The internal/scannercert package describes what a report records.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/odvcencio/gotreesitter/cmd/scannercert/internal/scannercert"
	"github.com/odvcencio/gotreesitter/grammars"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// certifier turns one grammar name into its report.
type certifier func(root, name string, opts scannercert.CorpusOptions) (*scannercert.Report, error)

func run(args []string, stdout, stderr io.Writer) error {
	return runWith(args, stdout, stderr, certify)
}

func runWith(args []string, stdout, stderr io.Writer, makeReport certifier) error {
	flags := flag.NewFlagSet("scannercert", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "repository root")
	languages := flags.String("language", "", "comma-separated grammar names")
	out := flags.String("out", "", "directory for the <language>.json reports; required for several languages")
	session := flags.Bool("session", true, "also parse the 72 sources of the sample's editing session")
	fetched := flags.String("fetched", "", "directory written by scripts/benchfixture_corpus.py fetch; also parse each grammar's median and largest inputs from it")
	list := flags.Bool("list", false, "list the grammars that have an external scanner and a committed sample")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if *list {
		return listCertifiable(*root, stdout)
	}
	names := splitNames(*languages)
	switch {
	case len(names) == 0:
		return errors.New("name at least one grammar with -language, or use -list")
	case len(names) > 1 && *out == "":
		return errors.New("several grammars need -out, a directory for their reports")
	}
	var failures []string
	for _, name := range names {
		report, err := makeReport(*root, name, scannercert.CorpusOptions{Session: *session, Fetched: *fetched})
		if err == nil {
			err = write(report, *out, stdout, stderr)
		}
		if err == nil {
			err = unreliable(report)
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
		}
		// Hold one decoded grammar at a time, however long the list.
		grammars.PurgeEmbeddedLanguageCache()
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "\n"))
	}
	return nil
}

func splitNames(list string) []string {
	var names []string
	seen := map[string]bool{}
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func certify(root, name string, opts scannercert.CorpusOptions) (*scannercert.Report, error) {
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		return nil, errors.New("unknown grammar")
	}
	corpus, err := scannercert.LoadCorpus(root, entry.Name, opts)
	if err != nil {
		return nil, err
	}
	return scannercert.Certify(entry.Language(), corpus)
}

// unreliable reports a report that records a parse the recorder changed. The
// command writes such a report so a reader can see which sources changed, but
// it counts as a failure: the recorded calls are not the production parser's.
func unreliable(report *scannercert.Report) error {
	if report.Verdict != scannercert.VerdictNotTransparent {
		return nil
	}
	return fmt.Errorf("the recorded parse differs from the production parse in %d of %d sources, so the report says nothing about the scanner",
		report.Parses.Divergent, report.Parses.Sources)
}

// write puts the report in dir, or on stdout when dir is empty.
func write(report *scannercert.Report, dir string, stdout, stderr io.Writer) error {
	data, err := report.JSON()
	if err != nil {
		return err
	}
	if dir == "" {
		_, err = stdout.Write(data)
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, report.Language+".json"), data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "%s: %s (%d distinct calls, %d accepted)\n",
		report.Language, report.Verdict, report.Calls.Distinct, report.Calls.Accepted)
	return nil
}

func listCertifiable(root string, stdout io.Writer) error {
	var names []string
	for _, entry := range grammars.AllLanguages() {
		hasScanner := entry.Language().ExternalScanner != nil
		grammars.PurgeEmbeddedLanguageCache()
		if !hasScanner {
			continue
		}
		if _, err := scannercert.LoadCorpus(root, entry.Name, scannercert.CorpusOptions{}); err == nil {
			names = append(names, entry.Name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintln(stdout, name)
	}
	return nil
}
