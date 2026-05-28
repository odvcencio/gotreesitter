// Command grammargen generates tree-sitter parser artifacts from grammar definitions.
//
// Usage:
//
//	grammargen [flags] <grammar-name-or-file>
//
// Input sources:
//
//	<name>          Built-in grammar (json, calc, glr, go, js, ts, tsx, fortran, etc.)
//	-js <path>      Import a tree-sitter grammar.js file
//	-json <path>    Import a resolved tree-sitter grammar.json file
//	-grammar <path> Parse a portable .grammar file
//
// Output formats:
//
//	-bin <path>      Write gotreesitter .bin blob
//	-c <path>        Write tree-sitter parser.c
//	-go <path>       Write grammargen Go DSL source
//	-json-out <path> Write resolved grammar.json (emit subcommand)
//
// Authoring helpers:
//
//	parse      Parse a sample and print text, S-expression, or JSON output
//	doctor     Validate, generate, test, parse samples, and report next steps
//
// Other flags:
//
//	-lr-split      Enable LR(1) state splitting before generation
//	-validate      Check grammar for issues without generating
//	-report        Show generation report with conflict diagnostics
//	-list          List available built-in grammars
package main

import (
	"flag"
	"fmt"
	"os"
	"unicode"

	"github.com/odvcencio/gotreesitter/grammargen"
)

var builtinGrammars = map[string]func() *grammargen.Grammar{
	"json":               grammargen.JSONGrammar,
	"calc":               grammargen.CalcGrammar,
	"glr":                grammargen.GLRGrammar,
	"go":                 grammargen.GoGrammar,
	"javascript":         grammargen.JavaScriptGrammar,
	"js":                 grammargen.JSGrammar,
	"jsx":                grammargen.JSXGrammar,
	"typescript":         grammargen.TypeScriptGrammar,
	"ts":                 grammargen.TSGrammar,
	"tsx":                grammargen.TSXGrammar,
	"fortran":            grammargen.FortranGrammar,
	"keyword":            grammargen.KeywordGrammar,
	"ext":                grammargen.ExtScannerGrammar,
	"alias":              grammargen.AliasSuperGrammar,
	"kotlin":             grammargen.KotlinGrammar,
	"swift":              grammargen.SwiftGrammar,
	"swift-abi-mangling": grammargen.SwiftABIManglingGrammar,
}

type cliConfig struct {
	binOut      string
	cOut        string
	goOut       string
	jsInput     string
	jsonInput   string
	grammarFile string
	pkgName     string
	funcName    string
	highlight   bool
	validate    bool
	report      bool
	list        bool
	lrSplit     bool
	args        []string
}

func main() {
	if len(os.Args) > 1 && isSubcommand(os.Args[1]) {
		runSubcommand(os.Args[1], os.Args[2:])
		return
	}

	cfg := parseCLIConfig()
	if cfg.list {
		runListMode()
		return
	}

	g, name := loadGrammar(cfg)
	if cfg.lrSplit {
		g.EnableLRSplitting = true
	}

	switch {
	case cfg.highlight:
		fmt.Print(grammargen.GenerateHighlightQuery(g))
	case cfg.validate:
		runValidateMode(name, g)
	case cfg.report:
		runReportMode(name, g)
	default:
		runGenerateMode(cfg, g)
	}
}

func parseCLIConfig() cliConfig {
	var cfg cliConfig
	flag.Usage = func() {
		printUsageError()
		flag.PrintDefaults()
	}
	flag.StringVar(&cfg.binOut, "bin", "", "output path for gotreesitter .bin blob")
	flag.StringVar(&cfg.cOut, "c", "", "output path for tree-sitter parser.c")
	flag.StringVar(&cfg.goOut, "go", "", "output path for grammargen Go DSL source")
	flag.StringVar(&cfg.jsInput, "js", "", "path to a tree-sitter grammar.js file to import")
	flag.StringVar(&cfg.jsonInput, "json", "", "path to a resolved tree-sitter grammar.json file to import")
	flag.StringVar(&cfg.grammarFile, "grammar", "", "path to a .grammar file to parse")
	flag.StringVar(&cfg.pkgName, "pkg", "grammargen", "package name for -go output")
	flag.StringVar(&cfg.funcName, "func", "", "function name for -go output (default: <GrammarName>Grammar)")
	flag.BoolVar(&cfg.highlight, "highlight", false, "generate a highlight query for the grammar")
	flag.BoolVar(&cfg.validate, "validate", false, "validate grammar without generating")
	flag.BoolVar(&cfg.report, "report", false, "show generation report with conflict diagnostics")
	flag.BoolVar(&cfg.list, "list", false, "list available built-in grammars")
	flag.BoolVar(&cfg.lrSplit, "lr-split", false, "enable LR(1) state splitting before generation")
	flag.Parse()
	cfg.args = flag.Args()
	return cfg
}

func runListMode() {
	fmt.Println("Available built-in grammars:")
	for name := range builtinGrammars {
		fmt.Printf("  %s\n", name)
	}
}

func loadGrammar(cfg cliConfig) (*grammargen.Grammar, string) {
	switch {
	case cfg.jsInput != "" && cfg.jsonInput != "":
		exitf("use only one of -js or -json")
	case cfg.jsonInput != "":
		return loadImportedGrammar(cfg.jsonInput, grammargen.ImportGrammarJSON)
	case cfg.jsInput != "":
		return loadImportedGrammar(cfg.jsInput, grammargen.ImportGrammarJS)
	case cfg.grammarFile != "":
		return loadGrammarFile(cfg.grammarFile)
	default:
		return loadBuiltinGrammar(cfg.args)
	}
	return nil, ""
}

func loadImportedGrammar(path string, importGrammar func([]byte) (*grammargen.Grammar, error)) (*grammargen.Grammar, string) {
	source, err := os.ReadFile(path)
	if err != nil {
		exitf("read %s: %v", path, err)
	}
	g, err := importGrammar(source)
	if err != nil {
		exitf("import %s: %v", path, err)
	}
	return g, grammarDisplayName(g, path)
}

func loadGrammarFile(path string) (*grammargen.Grammar, string) {
	source, err := os.ReadFile(path)
	if err != nil {
		exitf("read %s: %v", path, err)
	}
	g, err := grammargen.ParseGrammarFile(string(source))
	if err != nil {
		exitf("parse %s: %v", path, err)
	}
	return g, grammarDisplayName(g, path)
}

func loadBuiltinGrammar(args []string) (*grammargen.Grammar, string) {
	if len(args) == 0 {
		printUsageError()
		os.Exit(1)
	}
	name := args[0]
	fn, ok := builtinGrammars[name]
	if !ok {
		exitf("unknown grammar %q (use -list, -js, -json, or -grammar)", name)
	}
	return fn(), name
}

func grammarDisplayName(g *grammargen.Grammar, fallback string) string {
	if g != nil && g.Name != "" {
		return g.Name
	}
	return fallback
}

func runValidateMode(name string, g *grammargen.Grammar) {
	warnings := grammargen.Validate(g)
	if len(warnings) == 0 {
		fmt.Printf("grammar %q: OK (no warnings)\n", name)
	} else {
		fmt.Printf("grammar %q: %d warning(s):\n", name, len(warnings))
		for _, w := range warnings {
			fmt.Printf("  - %s\n", w)
		}
		os.Exit(1)
	}

	if len(g.Tests) == 0 {
		return
	}
	fmt.Printf("running %d embedded test(s)...\n", len(g.Tests))
	if err := grammargen.RunTests(g); err != nil {
		exitf("tests failed: %v", err)
	}
	fmt.Println("all tests passed")
}

func runReportMode(name string, g *grammargen.Grammar) {
	rpt, err := grammargen.GenerateWithReport(g)
	if err != nil {
		exitf("generation failed: %v", err)
	}

	fmt.Printf("Grammar: %s\n", name)
	fmt.Printf("Symbols: %d\n", rpt.SymbolCount)
	fmt.Printf("States:  %d\n", rpt.StateCount)
	fmt.Printf("Tokens:  %d\n", rpt.TokenCount)
	fmt.Printf("Blob:    %d bytes\n", len(rpt.Blob))

	if len(rpt.Warnings) > 0 {
		fmt.Printf("\nWarnings (%d):\n", len(rpt.Warnings))
		for _, w := range rpt.Warnings {
			fmt.Printf("  - %s\n", w)
		}
	}

	if len(rpt.Conflicts) == 0 {
		fmt.Println("\nNo conflicts")
		return
	}
	ng, _ := grammargen.Normalize(g)
	fmt.Printf("\nConflicts resolved (%d):\n", len(rpt.Conflicts))
	for i, c := range rpt.Conflicts {
		fmt.Printf("\n[%d] %s\n", i+1, c.String(ng))
	}
}

func runGenerateMode(cfg cliConfig, g *grammargen.Grammar) {
	if cfg.binOut == "" && cfg.cOut == "" && cfg.goOut == "" {
		exitf("specify at least one output: -bin <path>, -c <path>, or -go <path>")
	}
	if cfg.binOut != "" {
		writeBinOutput(cfg.binOut, g)
	}
	if cfg.goOut != "" {
		writeGoOutput(cfg.goOut, cfg.pkgName, cfg.funcName, g)
	}
	if cfg.cOut != "" {
		writeCOutput(cfg.cOut, g)
	}
}

func writeBinOutput(path string, g *grammargen.Grammar) {
	blob, err := grammargen.Generate(g)
	if err != nil {
		exitf("generation failed: %v", err)
	}
	if err := os.WriteFile(path, blob, 0644); err != nil {
		exitf("write %s: %v", path, err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, len(blob))
}

func writeGoOutput(path, pkgName, funcName string, g *grammargen.Grammar) {
	outFunc := funcName
	if outFunc == "" {
		outFunc = defaultGrammarFuncName(g.Name)
	}
	source, err := grammargen.EmitGrammarGo(g, pkgName, outFunc)
	if err != nil {
		exitf("Go source generation failed: %v", err)
	}
	if err := os.WriteFile(path, source, 0644); err != nil {
		exitf("write %s: %v", path, err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, len(source))
}

func writeCOutput(path string, g *grammargen.Grammar) {
	code, err := grammargen.GenerateC(g)
	if err != nil {
		exitf("C generation failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(code), 0644); err != nil {
		exitf("write %s: %v", path, err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, len(code))
}

func printUsageError() {
	fmt.Fprintln(os.Stderr, "usage: grammargen [flags] <grammar-name>")
	fmt.Fprintln(os.Stderr, "       grammargen -js <grammar.js> [flags]")
	fmt.Fprintln(os.Stderr, "       grammargen -json <grammar.json> [flags]")
	fmt.Fprintln(os.Stderr, "       grammargen -grammar <file.grammar> [flags]")
	fmt.Fprintln(os.Stderr, "       grammargen emit [flags] <grammar-name>")
	fmt.Fprintln(os.Stderr, "       grammargen parse [flags] <grammar-name>")
	fmt.Fprintln(os.Stderr, "       grammargen doctor [flags] <grammar-name>")
	fmt.Fprintln(os.Stderr, "run with -list to see available built-in grammars")
}

func exitf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	if format == "" || format[len(format)-1] != '\n' {
		fmt.Fprintln(os.Stderr)
	}
	os.Exit(1)
}

func defaultGrammarFuncName(name string) string {
	var out []rune
	upperNext := true
	for _, r := range name {
		if !isIdentRune(r) {
			upperNext = true
			continue
		}
		if len(out) == 0 && unicode.IsDigit(r) {
			out = append(out, '_')
		}
		if upperNext {
			r = unicode.ToUpper(r)
			upperNext = false
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return "Grammar"
	}
	return string(out) + "Grammar"
}

func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
