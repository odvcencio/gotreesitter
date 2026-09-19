package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// RunReservedWordsOnlyManifest clones each requested grammar repo in the
// manifest and (re)generates only its *_reserved_words_gen.go sidecar under
// grammars/runtime. It never touches grammar_blobs/*.bin, *_register.go, or
// embedded_grammars_gen.go, so it is safe to run against grammars whose
// checked-in blob predates cmd/ts2go's ABI 15 reserved-word extraction
// (extractReservedWords).
//
// If only is non-empty, generation is restricted to the named languages
// (matching ManifestEntry.Name exactly). Entries whose extracted grammar
// carries no reserved words (no ts_reserved_words table found, or the table
// exists but every set is empty) are skipped with a log line: reserved
// words are an ABI 15 feature and most grammars have none.
func RunReservedWordsOnlyManifest(manifestPath, outDir, pkg string, only []string) error {
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

	tmpRoot, err := os.MkdirTemp("", "ts2go-reservedwords-*")
	if err != nil {
		return fmt.Errorf("mktemp: %w", err)
	}
	defer os.RemoveAll(tmpRoot)

	grp, grpCtx := errgroup.WithContext(context.Background())
	grp.SetLimit(runtime.GOMAXPROCS(-1))
	var mu sync.Mutex
	var written []string
	var skippedNoWords []string

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

			if len(grammar.ReservedWords) == 0 || grammar.MaxReservedWordSetSize == 0 {
				mu.Lock()
				skippedNoWords = append(skippedNoWords, entry.Name)
				mu.Unlock()
				fmt.Printf("%s: no ts_reserved_words table found; skipping\n", entry.Name)
				return nil
			}

			sourceComment := externalLexStatesSourceComment(entry)
			if err := writeReservedWordsSidecar(outDir, pkg, entry.Name, sourceComment, grammar); err != nil {
				return fmt.Errorf("%s: write reserved words: %w", entry.Name, err)
			}

			mu.Lock()
			written = append(written, entry.Name)
			mu.Unlock()
			fmt.Printf("generated %s_reserved_words_gen.go (%d symbol slots, stride %d)\n",
				safeFileBase(entry.Name), len(grammar.ReservedWords), grammar.MaxReservedWordSetSize)
			return nil
		})
	}
	if err := grp.Wait(); err != nil {
		return err
	}

	fmt.Printf("reservedwords-only: wrote %d file(s), skipped %d (no reserved words)\n", len(written), len(skippedNoWords))
	return nil
}

// writeReservedWordsSidecar writes grammars/runtime/<name>_reserved_words_gen.go,
// registering an ABI 15 reserved-word table (RegisterReservedWords) that the
// gotreesitter/grammars/runtime attach guard (attachRegisteredReservedWords)
// later confirms and merges onto the decoded embedded Language for name.
func writeReservedWordsSidecar(outDir, pkg, name, sourceComment string, grammar *ExtractedGrammar) error {
	if grammar == nil || len(grammar.ReservedWords) == 0 || grammar.MaxReservedWordSetSize == 0 {
		return nil
	}
	// Keep sidecars beside the shared runtime when generating this catalog,
	// matching writeExternalLexStatesSidecar.
	if pkg == "grammars" {
		if info, err := os.Stat(filepath.Join(outDir, "runtime")); err == nil && info.IsDir() {
			outDir = filepath.Join(outDir, "runtime")
			pkg = "grammarruntime"
		}
	}

	fileBase := safeFileBase(name)
	ident := languageRegisterIdentifier(name)
	wordsVar := ident + "ReservedWords"
	namesVar := ident + "ReservedWordsSymbolNames"
	buildTag := "grammar_subset_" + fileBase
	stride := grammar.MaxReservedWordSetSize

	// Collect the distinct non-zero symbols this table references, with
	// each one's C ts_symbol_names display name at generation time. The
	// runtime attach guard uses this to confirm the embedded language's
	// decoded symbol table has not shifted since this sidecar was written.
	symbolNames := map[int]string{}
	var symbolIDs []int
	for _, sym := range grammar.ReservedWords {
		if sym == 0 {
			continue
		}
		id := int(sym)
		if _, seen := symbolNames[id]; seen {
			continue
		}
		displayName := ""
		if id < len(grammar.SymbolNames) {
			displayName = grammar.SymbolNames[id]
		}
		symbolNames[id] = displayName
		symbolIDs = append(symbolIDs, id)
	}
	sort.Ints(symbolIDs)

	var b strings.Builder
	fmt.Fprintf(&b, "//go:build !grammar_subset || %s\n\n", buildTag)
	b.WriteString("// Code generated by cmd/ts2go; DO NOT EDIT.\n")
	if strings.TrimSpace(sourceComment) != "" {
		fmt.Fprintf(&b, "// Source: %s\n", sourceComment)
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	b.WriteString("import \"github.com/odvcencio/gotreesitter\"\n\n")

	fmt.Fprintf(&b, "// %s mirrors C tree-sitter ts_reserved_words (ABI 15),\n", wordsVar)
	fmt.Fprintf(&b, "// flattened with stride %d.\n", stride)
	fmt.Fprintf(&b, "var %s = []gotreesitter.Symbol{", wordsVar)
	for i, sym := range grammar.ReservedWords {
		if i%stride == 0 {
			b.WriteString("\n\t")
		}
		fmt.Fprintf(&b, "%d, ", sym)
	}
	b.WriteString("\n}\n\n")

	fmt.Fprintf(&b, "// %s records, for every symbol ID %s\n", namesVar, wordsVar)
	b.WriteString("// references, that symbol's C ts_symbol_names display name at generation\n")
	b.WriteString("// time.\n")
	fmt.Fprintf(&b, "var %s = map[gotreesitter.Symbol]string{\n", namesVar)
	for _, id := range symbolIDs {
		fmt.Fprintf(&b, "\t%d: %q,\n", id, symbolNames[id])
	}
	b.WriteString("}\n\n")

	b.WriteString("func init() {\n")
	fmt.Fprintf(&b, "\tRegisterReservedWords(%q, ReservedWordTable{\n", name)
	fmt.Fprintf(&b, "\t\tMaxSetSize:  %d,\n", stride)
	fmt.Fprintf(&b, "\t\tWords:       %s,\n", wordsVar)
	fmt.Fprintf(&b, "\t\tSymbolCount: %d,\n", len(grammar.SymbolNames))
	fmt.Fprintf(&b, "\t\tSymbolNames: %s,\n", namesVar)
	b.WriteString("\t})\n")
	b.WriteString("}\n")

	outFile := filepath.Join(outDir, fileBase+"_reserved_words_gen.go")
	return os.WriteFile(outFile, []byte(b.String()), 0644)
}
