// Command ts2go reads a tree-sitter generated parser.c file and outputs
// a Go source file containing a function that returns a populated
// *gotreesitter.Language with all extracted parse tables.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	input := flag.String("input", "", "path to parser.c")
	output := flag.String("output", "", "output Go file path")
	pkg := flag.String("package", "grammars", "Go package name")
	name := flag.String("name", "", "language name (auto-detected from parser.c if empty)")
	manifest := flag.String("manifest", "", "batch mode: path to manifest file")
	outdir := flag.String("outdir", "", "batch mode: output directory for generated files")
	compact := flag.Bool("compact", true, "compact and intern repeated tables before encoding")
	lexStatesOnly := flag.Bool("lexstates-only", false, "batch mode: only (re)generate *_external_lex_states_gen.go sidecars; never touch blobs, register stubs, or the embedded loader aggregate")
	reservedWordsOnly := flag.Bool("reservedwords-only", false, "batch mode: only (re)generate *_reserved_words_gen.go sidecars; never touch blobs, register stubs, or the embedded loader aggregate")
	only := flag.String("only", "", "-lexstates-only/-reservedwords-only mode: comma-separated list of manifest language names to restrict to (ignored by plain batch mode, which always processes every manifest entry)")
	flag.Parse()

	if *manifest != "" {
		if *outdir == "" {
			fmt.Fprintln(os.Stderr, "batch mode requires -outdir")
			os.Exit(1)
		}
		if *lexStatesOnly {
			var names []string
			if *only != "" {
				names = strings.Split(*only, ",")
			}
			if err := RunLexStatesOnlyManifest(*manifest, *outdir, *pkg, names); err != nil {
				fmt.Fprintf(os.Stderr, "lexstates-only: %v\n", err)
				os.Exit(1)
			}
			return
		}
		if *reservedWordsOnly {
			var names []string
			if *only != "" {
				names = strings.Split(*only, ",")
			}
			if err := RunReservedWordsOnlyManifest(*manifest, *outdir, *pkg, names); err != nil {
				fmt.Fprintf(os.Stderr, "reservedwords-only: %v\n", err)
				os.Exit(1)
			}
			return
		}
		if err := RunBatchManifest(*manifest, *outdir, *pkg, *compact); err != nil {
			fmt.Fprintf(os.Stderr, "batch: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "usage: ts2go -input parser.c -output grammar.go [-package grammars] [-name go]")
		fmt.Fprintln(os.Stderr, "   or: ts2go -manifest languages.txt -outdir ./grammars [-package grammars]")
		os.Exit(1)
	}

	source, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *input, err)
		os.Exit(1)
	}

	grammar, err := ExtractGrammar(string(source))
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract: %v\n", err)
		os.Exit(1)
	}

	if *name != "" {
		grammar.Name = *name
	}

	blobBase := safeFileBase(grammar.Name)
	blobDir := filepath.Join(filepath.Dir(*output), "grammar_blobs")
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", blobDir, err)
		os.Exit(1)
	}
	blobName := blobBase + ".bin"
	blobPath := filepath.Join(blobDir, blobName)

	lang := BuildLanguage(grammar)
	if err := applyCertifiedConflictPolicyProfiles(source, grammar, lang); err != nil {
		fmt.Fprintf(os.Stderr, "certified conflict policy: %v\n", err)
		os.Exit(1)
	}
	if *compact {
		NewLanguageCompactor().CompactLanguage(lang)
	}
	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode blob: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(blobPath, blob, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", blobPath, err)
		os.Exit(1)
	}

	code := GenerateEmbeddedGo(grammar, *pkg, blobName)
	if err := os.WriteFile(*output, []byte(code), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *output, err)
		os.Exit(1)
	}
	if err := writeExternalLexStatesSidecar(filepath.Dir(*output), *pkg, grammar.Name, filepath.Base(*input), grammar.ExternalLexStates); err != nil {
		fmt.Fprintf(os.Stderr, "write external lex states: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s and %s (%s language, %d states, %d symbols)\n",
		*output, blobPath, grammar.Name, grammar.StateCount, grammar.SymbolCount)
}
