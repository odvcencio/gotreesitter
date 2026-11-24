// Command pgo_repdriver is a representative multi-grammar parsing workload
// used to (a) collect a CPU profile for Go build-time PGO and (b) produce a
// stable digest of parsed trees for before/after correctness comparisons.
// It parses a fixed spread of real-world source files across five grammars
// (go, c_sharp, bash, python, cmake) pulled from cgo_harness/corpus_real,
// so the resulting profile reflects real parsing hot paths rather than one
// grammar's quirks.
//
// The profile collected by this command is the production-corpus input to the
// checked-in cmd/parity_report/default.pgo composite. See cmd/pgo_repdriver/README.md for the composition
// recipe and where the result is wired into the build. From the repo root:
//
//	cd cgo_harness && go run ./cmd/build_real_corpus -out corpus_real -langs go,c_sharp,bash,python,cmake && cd ..
//	go run ./cmd/pgo_repdriver -mode=profile -cpuprofile=cmd/pgo_repdriver/inputs/production-v1.pgo -iterations=1500
//
// cgo_harness/corpus_real is gitignored and rebuilt from
// grammars/languages.lock via cgo_harness/cmd/build_real_corpus (see
// cgo_harness/README.md, "Build Real Corpus (Lock-Pinned)") — it is not
// checked in, so this driver's -corpus flag (or its auto-detection) will
// fail with a clear error until that corpus has been materialized locally.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// corpusFile pairs a source file with the language used to parse it.
type corpusFile struct {
	path string
	lang string
	src  []byte
}

var languageCtors = map[string]func() *gotreesitter.Language{
	"go":      grammars.GoLanguage,
	"c_sharp": grammars.CSharpLanguage,
	"bash":    grammars.BashLanguage,
	"python":  grammars.PythonLanguage,
	"cmake":   grammars.CmakeLanguage,
}

// corpusLanguages is the fixed, ordered grammar spread for the
// representative workload. Order matters for reproducibility of the digest
// output but not for profiling.
var corpusLanguages = []string{"go", "c_sharp", "bash", "python", "cmake"}

func loadCorpus(root string) ([]corpusFile, error) {
	var files []corpusFile
	for _, lang := range corpusLanguages {
		dir := filepath.Join(root, lang)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("read corpus dir %s: %w", dir, err)
		}
		var names []string
		for _, e := range entries {
			if e.Type().IsRegular() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			p := filepath.Join(dir, name)
			src, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", p, err)
			}
			files = append(files, corpusFile{path: p, lang: lang, src: src})
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no corpus files found under %s", root)
	}
	return files, nil
}

// findCorpusRoot walks upward from the working directory looking for
// cgo_harness/corpus_real so the driver works regardless of which directory
// `go run` is invoked from.
func findCorpusRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "cgo_harness", "corpus_real")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("cgo_harness/corpus_real not found above %s", dir)
		}
		dir = parent
	}
}

// parseOnce parses every corpus file once with a fresh parser per language
// and returns the number of bytes parsed. Parsers are cached per language
// across the whole run (mirrors realistic reuse of a Parser value).
func parseOnce(parsers map[string]*gotreesitter.Parser, files []corpusFile) (int64, error) {
	var total int64
	for _, f := range files {
		p := parsers[f.lang]
		tree, err := p.Parse(f.src)
		if err != nil {
			return total, fmt.Errorf("parse %s (%s): %w", f.path, f.lang, err)
		}
		total += int64(len(f.src))
		tree.Release()
	}
	return total, nil
}

func buildParsers() map[string]*gotreesitter.Parser {
	parsers := make(map[string]*gotreesitter.Parser, len(languageCtors))
	for lang, ctor := range languageCtors {
		parsers[lang] = gotreesitter.NewParser(ctor())
	}
	return parsers
}

func runProfile(corpusRoot, cpuProfilePath string, iterations int) error {
	files, err := loadCorpus(corpusRoot)
	if err != nil {
		return err
	}
	parsers := buildParsers()

	// Warm up (grammar table construction, JIT-ish caches) before profiling
	// so the profile reflects steady-state parsing, not one-time setup.
	if _, err := parseOnce(parsers, files); err != nil {
		return fmt.Errorf("warmup: %w", err)
	}

	f, err := os.Create(cpuProfilePath)
	if err != nil {
		return fmt.Errorf("create cpu profile %s: %w", cpuProfilePath, err)
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		return fmt.Errorf("start cpu profile: %w", err)
	}
	defer pprof.StopCPUProfile()

	var totalBytes int64
	for i := 0; i < iterations; i++ {
		n, err := parseOnce(parsers, files)
		if err != nil {
			return fmt.Errorf("iteration %d: %w", i, err)
		}
		totalBytes += n
	}
	fmt.Fprintf(os.Stderr, "profiled %d iterations over %d files, %d bytes/iteration, %d total bytes -> %s\n",
		iterations, len(files), totalBytes/int64(iterations), totalBytes, cpuProfilePath)
	return nil
}

// runDigest parses every corpus file once and prints a stable per-file
// digest line: path, language, byte length, HasError, and the S-expression
// of the parsed tree. This output must be byte-identical whether the driver
// binary was built with or without PGO -- PGO only changes codegen, never
// parse semantics.
func runDigest(corpusRoot string) error {
	files, err := loadCorpus(corpusRoot)
	if err != nil {
		return err
	}
	parsers := buildParsers()
	for _, f := range files {
		lang := languageCtors[f.lang]()
		p := parsers[f.lang]
		tree, err := p.Parse(f.src)
		if err != nil {
			return fmt.Errorf("parse %s (%s): %w", f.path, f.lang, err)
		}
		root := tree.RootNode()
		rel := f.path
		if r, err := filepath.Rel(corpusRoot, f.path); err == nil {
			rel = filepath.ToSlash(r)
		}
		fmt.Printf("%s\t%s\t%d\t%v\t%s\n", rel, f.lang, len(f.src), root.HasError(), root.SExpr(lang))
		tree.Release()
	}
	return nil
}

func main() {
	mode := flag.String("mode", "profile", "profile | digest")
	corpusRoot := flag.String("corpus", "", "path to cgo_harness/corpus_real (auto-detected if empty)")
	cpuProfile := flag.String("cpuprofile", "default.pgo", "output path for the CPU profile (profile mode); this driver supplies the production input to cmd/parity_report/default.pgo")
	iterations := flag.Int("iterations", 400, "number of full-corpus passes to profile (profile mode)")
	flag.Parse()

	root := *corpusRoot
	if root == "" {
		r, err := findCorpusRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		root = r
	}
	if _, err := os.Stat(root); err != nil {
		fmt.Fprintf(os.Stderr, "error: corpus root %s: %v\n", root, err)
		os.Exit(1)
	}

	var err error
	switch *mode {
	case "profile":
		err = runProfile(root, *cpuProfile, *iterations)
	case "digest":
		err = runDigest(root)
	default:
		err = fmt.Errorf("unknown -mode=%s (want profile or digest)", *mode)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
