// Command ts2go reads a tree-sitter generated parser.c file and outputs
// a Go source file containing a function that returns a populated
// *gotreesitter.Language with all extracted parse tables.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	input := flag.String("input", "", "path to parser.c")
	output := flag.String("output", "", "output Go file path")
	pkg := flag.String("package", "grammars", "Go package name")
	name := flag.String("name", "", "language name (auto-detected from parser.c if empty)")
	manifest := flag.String("manifest", "", "batch mode: path to manifest file")
	outdir := flag.String("outdir", "", "batch mode: output directory for generated files")
	flag.Parse()

	if *manifest != "" {
		if *outdir == "" {
			fmt.Fprintln(os.Stderr, "batch mode requires -outdir")
			os.Exit(1)
		}
		if err := RunBatchManifest(*manifest, *outdir, *pkg); err != nil {
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

	code := GenerateGo(grammar, *pkg)
	if err := os.WriteFile(*output, []byte(code), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", *output, err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s (%s language, %d states, %d symbols)\n",
		*output, grammar.Name, grammar.StateCount, grammar.SymbolCount)
}
