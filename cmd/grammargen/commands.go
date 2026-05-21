package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
)

type sourceFlags struct {
	jsInput     string
	jsonInput   string
	grammarFile string
	lrSplit     bool
	args        []string
}

type sampleFlags struct {
	samplePath string
	text       string
	stdin      bool
}

func isSubcommand(name string) bool {
	switch name {
	case "emit", "parse", "doctor":
		return true
	default:
		return false
	}
}

func runSubcommand(name string, args []string) {
	switch name {
	case "emit":
		runEmitCommand(args)
	case "parse":
		runParseCommand(args)
	case "doctor":
		runDoctorCommand(args)
	default:
		exitf("unknown command %q", name)
	}
}

func runEmitCommand(args []string) {
	var src sourceFlags
	var binOut string
	var cOut string
	var goOut string
	var pkgName string
	var funcName string
	var highlight bool
	fs := flag.NewFlagSet("grammargen emit", flag.ExitOnError)
	registerSourceFlags(fs, &src)
	fs.StringVar(&binOut, "bin", "", "output path for gotreesitter .bin blob")
	fs.StringVar(&cOut, "c", "", "output path for tree-sitter parser.c")
	fs.StringVar(&goOut, "go", "", "output path for grammargen Go DSL source")
	fs.StringVar(&pkgName, "pkg", "grammargen", "package name for -go output")
	fs.StringVar(&funcName, "func", "", "function name for -go output (default: <GrammarName>Grammar)")
	fs.BoolVar(&highlight, "highlight", false, "write inferred highlight query to stdout")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: grammargen emit [flags] <grammar-name>")
		fmt.Fprintln(os.Stderr, "       grammargen emit -json <grammar.json> -go <out.go>")
		fmt.Fprintln(os.Stderr, "       grammargen emit -grammar <file.grammar> -go <out.go>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(normalizeSubcommandArgs(args)); err != nil {
		exitf("%v", err)
	}
	src.args = fs.Args()

	g, _ := loadCommandGrammar(src)
	if highlight {
		fmt.Print(grammargen.GenerateHighlightQuery(g))
	}
	if binOut == "" && cOut == "" && goOut == "" {
		if highlight {
			return
		}
		exitf("specify at least one output: -bin <path>, -c <path>, -go <path>, or -highlight")
	}
	runGenerateMode(cliConfig{
		binOut:   binOut,
		cOut:     cOut,
		goOut:    goOut,
		pkgName:  pkgName,
		funcName: funcName,
	}, g)
}

func registerSourceFlags(fs *flag.FlagSet, src *sourceFlags) {
	fs.StringVar(&src.jsInput, "js", "", "path to a tree-sitter grammar.js file to import")
	fs.StringVar(&src.jsonInput, "json", "", "path to a resolved tree-sitter grammar.json file to import")
	fs.StringVar(&src.grammarFile, "grammar", "", "path to a .grammar file to parse")
	fs.BoolVar(&src.lrSplit, "lr-split", false, "enable LR(1) state splitting before generation")
}

func registerSampleFlags(fs *flag.FlagSet, sample *sampleFlags) {
	fs.StringVar(&sample.samplePath, "sample", "", "path to source sample to parse")
	fs.StringVar(&sample.text, "text", "", "inline source sample to parse")
	fs.BoolVar(&sample.stdin, "stdin", false, "read source sample from stdin")
}

func loadCommandGrammar(src sourceFlags) (*grammargen.Grammar, string) {
	cfg := cliConfig{
		jsInput:     src.jsInput,
		jsonInput:   src.jsonInput,
		grammarFile: src.grammarFile,
		lrSplit:     src.lrSplit,
		args:        src.args,
	}
	g, name := loadGrammar(cfg)
	if cfg.lrSplit {
		g.EnableLRSplitting = true
	}
	return g, name
}

func readSample(sample sampleFlags, required bool) ([]byte, string, bool) {
	count := 0
	if sample.samplePath != "" {
		count++
	}
	if sample.text != "" {
		count++
	}
	if sample.stdin {
		count++
	}
	if count > 1 {
		exitf("use only one of -sample, -text, or -stdin")
	}
	if count == 0 {
		if required {
			exitf("provide a sample with -sample <path>, -text <source>, or -stdin")
		}
		return nil, "", false
	}

	switch {
	case sample.samplePath != "":
		data, err := os.ReadFile(sample.samplePath)
		if err != nil {
			exitf("read sample %s: %v", sample.samplePath, err)
		}
		return data, sample.samplePath, true
	case sample.text != "":
		return []byte(sample.text), "<text>", true
	case sample.stdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			exitf("read stdin: %v", err)
		}
		return data, "<stdin>", true
	default:
		return nil, "", false
	}
}

func runParseCommand(args []string) {
	var src sourceFlags
	var sample sampleFlags
	var runtime bool
	fs := flag.NewFlagSet("grammargen parse", flag.ExitOnError)
	registerSourceFlags(fs, &src)
	registerSampleFlags(fs, &sample)
	fs.BoolVar(&runtime, "runtime", false, "print parser runtime summary")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: grammargen parse [flags] <grammar-name>")
		fmt.Fprintln(os.Stderr, "       grammargen parse -json <grammar.json> -sample <file>")
		fmt.Fprintln(os.Stderr, "       grammargen parse -grammar <file.grammar> -text <source>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(normalizeSubcommandArgs(args)); err != nil {
		exitf("%v", err)
	}
	src.args = fs.Args()

	g, name := loadCommandGrammar(src)
	input, sampleName, _ := readSample(sample, true)
	lang, err := grammargen.GenerateLanguage(g)
	if err != nil {
		exitf("generation failed: %v", err)
	}
	result := parseSample(lang, input)

	fmt.Printf("Grammar: %s\n", name)
	fmt.Printf("Sample:  %s (%d bytes)\n", sampleName, len(input))
	printParseResult(result, lang, runtime)
}

func runDoctorCommand(args []string) {
	var src sourceFlags
	var sample sampleFlags
	var runtime bool
	fs := flag.NewFlagSet("grammargen doctor", flag.ExitOnError)
	registerSourceFlags(fs, &src)
	registerSampleFlags(fs, &sample)
	fs.BoolVar(&runtime, "runtime", false, "print parser runtime summary when parsing a sample")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: grammargen doctor [flags] <grammar-name>")
		fmt.Fprintln(os.Stderr, "       grammargen doctor -json <grammar.json> [-sample <file>]")
		fmt.Fprintln(os.Stderr, "       grammargen doctor -grammar <file.grammar> [-text <source>]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(normalizeSubcommandArgs(args)); err != nil {
		exitf("%v", err)
	}
	src.args = fs.Args()

	g, name := loadCommandGrammar(src)
	failed := false
	fmt.Printf("Grammar: %s\n", name)
	fmt.Printf("Rules:   %d\n", len(g.Rules))

	warnings := grammargen.Validate(g)
	if len(warnings) == 0 {
		fmt.Println("Validate: OK")
	} else {
		fmt.Printf("Validate: %d warning(s)\n", len(warnings))
		for _, w := range warnings {
			fmt.Printf("  - %s\n", w)
		}
		failed = true
	}

	rpt, err := grammargen.GenerateWithReport(g)
	if err != nil {
		exitf("Generate: failed: %v", err)
	}
	glrConflicts := countGLRConflicts(rpt.Conflicts)
	fmt.Println("Generate: OK")
	fmt.Printf("Symbols:  %d\n", rpt.SymbolCount)
	fmt.Printf("States:   %d\n", rpt.StateCount)
	fmt.Printf("Tokens:   %d\n", rpt.TokenCount)
	fmt.Printf("Blob:     %d bytes\n", len(rpt.Blob))
	fmt.Printf("Conflicts: %d resolved", len(rpt.Conflicts))
	if glrConflicts > 0 {
		fmt.Printf(" (%d kept for GLR)", glrConflicts)
	}
	fmt.Println()

	if len(g.Tests) == 0 {
		fmt.Println("Embedded tests: none")
	} else if err := grammargen.RunTests(g); err != nil {
		fmt.Printf("Embedded tests: failed\n")
		fmt.Printf("%v\n", err)
		failed = true
	} else {
		fmt.Printf("Embedded tests: OK (%d)\n", len(g.Tests))
	}

	input, sampleName, ok := readSample(sample, false)
	if ok {
		result := parseSample(rpt.Language, input)
		fmt.Printf("Sample: %s (%d bytes)\n", sampleName, len(input))
		printParseResult(result, rpt.Language, runtime)
		if parseResultFailed(result) {
			failed = true
		}
	} else {
		fmt.Println("Sample: not run")
	}

	printDoctorNextSteps(name, src, ok)
	if failed {
		os.Exit(1)
	}
}

type parseResult struct {
	tree *gotreesitter.Tree
	root *gotreesitter.Node
	err  error
}

func parseSample(lang *gotreesitter.Language, input []byte) parseResult {
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(input)
	if err != nil {
		return parseResult{err: err}
	}
	return parseResult{tree: tree, root: tree.RootNode()}
}

func printParseResult(result parseResult, lang *gotreesitter.Language, runtime bool) {
	if result.err != nil {
		fmt.Printf("Parse:   failed: %v\n", result.err)
		return
	}
	root := result.root
	if root == nil {
		fmt.Println("Parse:   failed: nil root")
		return
	}
	fmt.Println("Parse:   OK")
	fmt.Printf("Root:    %s [%d:%d]\n", root.Type(lang), root.StartByte(), root.EndByte())
	fmt.Printf("Error:   %v\n", root.HasError())
	fmt.Printf("Stop:    %s\n", result.tree.ParseStopReason())
	if runtime {
		fmt.Printf("Runtime: %s\n", result.tree.ParseRuntime().Summary())
	}
	fmt.Println("S-expression:")
	fmt.Println(root.SExpr(lang))
}

func parseResultFailed(result parseResult) bool {
	if result.err != nil || result.tree == nil || result.root == nil {
		return true
	}
	return result.root.HasError() || result.tree.ParseStoppedEarly()
}

func countGLRConflicts(conflicts []grammargen.ConflictDiag) int {
	count := 0
	for _, c := range conflicts {
		if strings.Contains(c.Resolution, "GLR") {
			count++
		}
	}
	return count
}

func printDoctorNextSteps(name string, src sourceFlags, parsedSample bool) {
	var steps []string
	if !parsedSample {
		steps = append(steps, "add -sample <file>, -text <source>, or -stdin to inspect a parse tree")
	}
	if source := sourceSpecifier(name, src); source != "" {
		steps = append(steps,
			fmt.Sprintf("emit Go DSL with: go run ./cmd/grammargen emit %s -go <path> -pkg grammargen", source),
			fmt.Sprintf("emit a blob with: go run ./cmd/grammargen emit %s -bin <path>", source),
		)
	}
	if isKnownParityGrammar(name) {
		steps = append(steps, fmt.Sprintf("run focused parity in Docker when ready: bash cgo_harness/docker/run_single_grammar_parity.sh %s", canonicalParityName(name)))
	}
	if len(steps) == 0 {
		return
	}
	fmt.Println("Next:")
	for _, step := range steps {
		fmt.Printf("  %s\n", step)
	}
}

func sourceSpecifier(name string, src sourceFlags) string {
	switch {
	case src.grammarFile != "":
		return "-grammar " + commandArg(src.grammarFile)
	case src.jsonInput != "":
		return "-json " + commandArg(src.jsonInput)
	case src.jsInput != "":
		return "-js " + commandArg(src.jsInput)
	case name != "":
		return commandArg(name)
	default:
		return ""
	}
}

func commandArg(s string) string {
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\n'\"\\$`") {
		return strconv.Quote(s)
	}
	return s
}

func isKnownParityGrammar(name string) bool {
	switch canonicalParityName(name) {
	case "bash", "c_lang", "comment", "cpon", "css", "csv", "diff", "dockerfile",
		"dot", "eds", "eex", "elixir", "forth", "git_config", "git_rebase",
		"gitattributes", "gitcommit", "go_lang", "gomod", "graphql", "haskell",
		"hcl", "html", "ini", "javascript", "jsdoc", "json", "json5", "lua",
		"make", "nix", "ocaml", "pem", "php", "promql", "properties", "proto",
		"python", "regex", "requirements", "ron", "scala", "scheme", "sql",
		"ssh_config", "swift", "todotxt", "toml", "yaml", "rust", "c_sharp",
		"java", "ruby", "cpp", "kotlin", "cuda", "typescript", "tsx", "cobol",
		"fortran", "perl", "erlang", "d":
		return true
	default:
		return false
	}
}

func canonicalParityName(name string) string {
	switch name {
	case "go":
		return "go_lang"
	case "c":
		return "c_lang"
	default:
		return name
	}
}

func normalizeSubcommandArgs(args []string) []string {
	valueFlags := map[string]bool{
		"bin":     true,
		"c":       true,
		"func":    true,
		"go":      true,
		"js":      true,
		"json":    true,
		"grammar": true,
		"pkg":     true,
		"sample":  true,
		"text":    true,
	}
	var flags []string
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		flags = append(flags, arg)
		name, hasValue := splitFlagName(arg)
		if valueFlags[name] && !hasValue && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positionals...)
}

func splitFlagName(arg string) (name string, hasValue bool) {
	name = strings.TrimLeft(arg, "-")
	if idx := strings.IndexByte(name, '='); idx >= 0 {
		return name[:idx], true
	}
	return name, false
}
