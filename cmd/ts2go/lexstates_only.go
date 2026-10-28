package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// sidecarOnlyMode describes one "regenerate a single sidecar file" mode of
// the batch manifest runner: which entries it skips, what it writes and what
// it logs.
type sidecarOnlyMode struct {
	// name prefixes the final summary line, e.g. "lexstates-only".
	name string
	// tmpPrefix is the os.MkdirTemp pattern for the clone directory.
	tmpPrefix string
	// skipNoun completes "skipped N (no <skipNoun>)" in the summary line.
	skipNoun string
	// skipMessage is logged, after "<entry>: ", when skip reports true.
	skipMessage string
	// skip reports whether the extracted grammar has nothing to write.
	skip func(*ExtractedGrammar) bool
	// write writes the sidecar for one grammar.
	write func(outDir, pkg string, entry ManifestEntry, sourceComment string, grammar *ExtractedGrammar) error
	// writeErrLabel completes "<entry>: write <writeErrLabel>: <err>".
	writeErrLabel string
	// generated formats the per-entry "generated ..." log line.
	generated func(entry ManifestEntry, grammar *ExtractedGrammar) string
}

// runSidecarOnlyManifest clones each requested grammar repo in the manifest
// and runs mode on its extracted grammar. If only is non-empty, it restricts
// generation to the named languages (matching ManifestEntry.Name exactly).
func runSidecarOnlyManifest(mode sidecarOnlyMode, manifestPath, outDir, pkg string, only []string) error {
	entries, err := ParseManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("manifest is empty: %s", manifestPath)
	}

	wanted := map[string]bool{}
	for _, n := range only {
		n = strings.TrimSpace(n)
		if n != "" {
			wanted[n] = true
		}
	}
	if len(wanted) > 0 {
		filtered := entries[:0]
		for _, e := range entries {
			if wanted[e.Name] {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}
	if len(entries) == 0 {
		return fmt.Errorf("no manifest entries matched -only filter")
	}

	tmpRoot, err := os.MkdirTemp("", mode.tmpPrefix)
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(tmpRoot)

	grp, grpCtx := errgroup.WithContext(context.Background())
	grp.SetLimit(runtime.GOMAXPROCS(-1))
	var mu sync.Mutex
	var written []string
	var skipped []string

	for _, entry := range entries {
		entry := entry
		if err := grpCtx.Err(); err != nil {
			break
		}
		grp.Go(func() error {
			if err := grpCtx.Err(); err != nil {
				return err
			}
			repoDir := filepath.Join(tmpRoot, safeFileBase(entry.Name))
			if err := cloneRepo(entry.RepoURL, entry.Commit, repoDir); err != nil {
				return fmt.Errorf("%s: clone: %w", entry.Name, err)
			}

			parserPath := filepath.Join(repoDir, entry.Subdir, "parser.c")
			if _, err := os.Stat(parserPath); err != nil {
				// findParserC's fallback walk is subdir-agnostic and prefers
				// any .../src/parser.c it finds first; in a multi-grammar
				// repo (several parser.c files under different subdirs) it
				// can pick the wrong one. The entry.Subdir path above is
				// what makes selection safe today — this fallback only
				// fires when that exact path is missing.
				detected, derr := findParserC(repoDir)
				if derr != nil {
					return fmt.Errorf("%s: parser.c not found under %s", entry.Name, repoDir)
				}
				parserPath = detected
			}
			source, err := os.ReadFile(parserPath)
			if err != nil {
				return fmt.Errorf("%s: read %s: %w", entry.Name, parserPath, err)
			}

			grammar, err := ExtractGrammar(string(source))
			if err != nil {
				return fmt.Errorf("%s: extract: %w", entry.Name, err)
			}
			grammar.Name = entry.Name

			if mode.skip(grammar) {
				mu.Lock()
				skipped = append(skipped, entry.Name)
				mu.Unlock()
				fmt.Printf("%s: %s\n", entry.Name, mode.skipMessage)
				return nil
			}

			sourceComment := externalLexStatesSourceComment(entry)
			if err := mode.write(outDir, pkg, entry, sourceComment, grammar); err != nil {
				return fmt.Errorf("%s: write %s: %w", entry.Name, mode.writeErrLabel, err)
			}

			mu.Lock()
			written = append(written, entry.Name)
			mu.Unlock()
			fmt.Print(mode.generated(entry, grammar))
			return nil
		})
	}
	if err := grp.Wait(); err != nil {
		return err
	}

	fmt.Printf("%s: wrote %d file(s), skipped %d (no %s)\n", mode.name, len(written), len(skipped), mode.skipNoun)
	return nil
}

// RunLexStatesOnlyManifest clones each requested grammar repo in the manifest
// and (re)generates only its *_external_lex_states_gen.go sidecar file. It
// never touches grammar_blobs/*.bin, *_register.go, or
// embedded_grammars_gen.go, so it is safe to run against grammars whose
// embedded blob and registration were produced by a different pipeline (e.g.
// hand-written external scanners registered via zzz_scanner_attachments.go).
//
// If only is non-empty, generation is restricted to the named languages
// (matching ManifestEntry.Name exactly).
func RunLexStatesOnlyManifest(manifestPath, outDir, pkg string, only []string) error {
	return runSidecarOnlyManifest(sidecarOnlyMode{
		name:        "lexstates-only",
		tmpPrefix:   "ts2go-lexstates-*",
		skipNoun:    "external lex states",
		skipMessage: "no ts_external_scanner_states table found; skipping",
		skip:        func(g *ExtractedGrammar) bool { return len(g.ExternalLexStates) == 0 },
		write: func(outDir, pkg string, entry ManifestEntry, sourceComment string, g *ExtractedGrammar) error {
			return writeExternalLexStatesSidecar(outDir, pkg, entry.Name, sourceComment, g.ExternalLexStates)
		},
		writeErrLabel: "external lex states",
		generated: func(entry ManifestEntry, g *ExtractedGrammar) string {
			return fmt.Sprintf("generated %s_external_lex_states_gen.go (%d states, %d external tokens)\n",
				safeFileBase(entry.Name), len(g.ExternalLexStates), g.ExternalTokenCount)
		},
	}, manifestPath, outDir, pkg, only)
}
