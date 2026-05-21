package main

import (
	"encoding/json"
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

type parseOptions struct {
	format          string
	runtime         bool
	strict          bool
	expectPath      string
	writeExpectPath string
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
	var jsonOut string
	var pkgName string
	var funcName string
	var highlight bool
	fs := flag.NewFlagSet("grammargen emit", flag.ExitOnError)
	registerSourceFlags(fs, &src)
	fs.StringVar(&binOut, "bin", "", "output path for gotreesitter .bin blob")
	fs.StringVar(&cOut, "c", "", "output path for tree-sitter parser.c")
	fs.StringVar(&goOut, "go", "", "output path for grammargen Go DSL source")
	fs.StringVar(&jsonOut, "json-out", "", "output path for resolved grammar.json")
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
	if jsonOut != "" {
		writeGrammarJSONOutput(jsonOut, g)
	}
	if binOut == "" && cOut == "" && goOut == "" && jsonOut == "" {
		if highlight {
			return
		}
		exitf("specify at least one output: -bin <path>, -c <path>, -go <path>, -json-out <path>, or -highlight")
	}
	if binOut != "" || cOut != "" || goOut != "" {
		runGenerateMode(cliConfig{
			binOut:   binOut,
			cOut:     cOut,
			goOut:    goOut,
			pkgName:  pkgName,
			funcName: funcName,
		}, g)
	}
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

func registerParseOptionFlags(fs *flag.FlagSet, opts *parseOptions) {
	fs.StringVar(&opts.format, "format", "text", "output format: text, sexpr, json")
	fs.BoolVar(&opts.runtime, "runtime", false, "print parser runtime summary")
	fs.BoolVar(&opts.strict, "strict", false, "exit non-zero if the parse has ERROR nodes or stops early")
	fs.StringVar(&opts.expectPath, "expect", "", "path to expected S-expression file")
	fs.StringVar(&opts.writeExpectPath, "write-expect", "", "write actual S-expression to this file")
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
	var opts parseOptions
	fs := flag.NewFlagSet("grammargen parse", flag.ExitOnError)
	registerSourceFlags(fs, &src)
	registerSampleFlags(fs, &sample)
	registerParseOptionFlags(fs, &opts)
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
	validateParseOptions(opts)

	g, name := loadCommandGrammar(src)
	input, sampleName, _ := readSample(sample, true)
	lang, err := grammargen.GenerateLanguage(g)
	if err != nil {
		exitf("generation failed: %v", err)
	}
	result := parseSample(lang, input)
	sexpr := resultSExpr(result, lang)
	golden := applyGoldenOptions(opts, sexpr)
	failed := golden.failed() || (opts.strict && parseResultFailed(result))

	switch opts.format {
	case "text":
		fmt.Printf("Grammar: %s\n", name)
		fmt.Printf("Sample:  %s (%d bytes)\n", sampleName, len(input))
		printParseResult(result, lang, opts.runtime)
		printGoldenResult(golden)
	case "sexpr":
		if sexpr != "" {
			fmt.Println(sexpr)
		}
	case "json":
		writeJSON(parseJSONReport(name, sampleName, len(input), result, lang, opts.runtime, golden))
	default:
		exitf("unknown -format %q (want text, sexpr, or json)", opts.format)
	}
	if failed {
		os.Exit(1)
	}
}

func runDoctorCommand(args []string) {
	var src sourceFlags
	var sample sampleFlags
	var opts parseOptions
	var conflictLimit int
	fs := flag.NewFlagSet("grammargen doctor", flag.ExitOnError)
	registerSourceFlags(fs, &src)
	registerSampleFlags(fs, &sample)
	registerParseOptionFlags(fs, &opts)
	fs.IntVar(&conflictLimit, "conflicts", 0, "print the first N conflict diagnostics")
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
	validateParseOptions(opts)
	if conflictLimit < 0 {
		exitf("-conflicts must be >= 0")
	}

	g, name := loadCommandGrammar(src)
	failed := false

	warnings := grammargen.Validate(g)
	if len(warnings) > 0 {
		failed = true
	}

	rpt, err := grammargen.GenerateWithReport(g)
	if err != nil {
		exitf("Generate: failed: %v", err)
	}
	glrConflicts := countGLRConflicts(rpt.Conflicts)

	testStatus := "none"
	var testError string
	if len(g.Tests) == 0 {
		testStatus = "none"
	} else if err := grammargen.RunTests(g); err != nil {
		testStatus = "failed"
		testError = err.Error()
		failed = true
	} else {
		testStatus = "ok"
	}

	input, sampleName, ok := readSample(sample, false)
	var sampleReport *parseJSON
	var golden goldenResult
	if ok {
		result := parseSample(rpt.Language, input)
		sexpr := resultSExpr(result, rpt.Language)
		golden = applyGoldenOptions(opts, sexpr)
		sampleReport = parseJSONReport(name, sampleName, len(input), result, rpt.Language, opts.runtime, golden)
		if parseResultFailed(result) {
			failed = true
		}
		if golden.failed() {
			failed = true
		}
	} else if opts.expectPath != "" || opts.writeExpectPath != "" {
		exitf("-expect and -write-expect require a sample from -sample, -text, or -stdin")
	} else if opts.format == "sexpr" {
		exitf("-format sexpr requires a sample from -sample, -text, or -stdin")
	}

	steps := nextSteps(name, src, ok)
	switch opts.format {
	case "text":
		printDoctorText(name, g, warnings, rpt, glrConflicts, conflictLimit, testStatus, testError, ok, sampleName, len(input), sampleReport, golden, steps, opts.runtime)
	case "json":
		writeJSON(doctorJSONReport{
			Grammar: name,
			Rules:   len(g.Rules),
			Validate: validateJSON{
				OK:       len(warnings) == 0,
				Warnings: warnings,
			},
			Generate: generateJSON{
				OK:              true,
				Symbols:         rpt.SymbolCount,
				States:          rpt.StateCount,
				Tokens:          rpt.TokenCount,
				BlobBytes:       len(rpt.Blob),
				Conflicts:       len(rpt.Conflicts),
				GLRConflicts:    glrConflicts,
				ConflictDetails: conflictDetails(rpt.Conflicts, g, conflictLimit),
			},
			EmbeddedTests: testsJSON{
				Status: testStatus,
				Count:  len(g.Tests),
				Error:  testError,
			},
			Sample: sampleReport,
			Next:   steps,
		})
	case "sexpr":
		if sampleReport != nil {
			fmt.Println(sampleReport.Parse.SExpr)
		}
	default:
		exitf("unknown -format %q (want text, sexpr, or json)", opts.format)
	}
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
	if parseResultFailed(result) {
		fmt.Println("Parse:   completed with errors")
	} else {
		fmt.Println("Parse:   OK")
	}
	fmt.Printf("Root:    %s [%d:%d]\n", root.Type(lang), root.StartByte(), root.EndByte())
	fmt.Printf("Error:   %v\n", root.HasError())
	fmt.Printf("Stop:    %s\n", result.tree.ParseStopReason())
	if runtime {
		fmt.Printf("Runtime: %s\n", result.tree.ParseRuntime().Summary())
	}
	fmt.Println("S-expression:")
	fmt.Println(resultSExpr(result, lang))
}

func parseResultFailed(result parseResult) bool {
	if result.err != nil || result.tree == nil || result.root == nil {
		return true
	}
	return result.root.HasError() || result.tree.ParseStoppedEarly()
}

func validateParseOptions(opts parseOptions) {
	switch opts.format {
	case "text", "sexpr", "json":
	default:
		exitf("unknown -format %q (want text, sexpr, or json)", opts.format)
	}
	if opts.expectPath != "" && opts.writeExpectPath != "" {
		exitf("use only one of -expect or -write-expect")
	}
}

func resultSExpr(result parseResult, lang *gotreesitter.Language) string {
	if result.err != nil || result.root == nil {
		return ""
	}
	return result.root.SExpr(lang)
}

type goldenResult struct {
	Path     string `json:"path,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Matched  bool   `json:"matched,omitempty"`
	Updated  bool   `json:"updated,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Error    string `json:"error,omitempty"`
}

func (r goldenResult) failed() bool {
	return r.Error != "" || (r.Mode == "expect" && !r.Matched)
}

func applyGoldenOptions(opts parseOptions, sexpr string) goldenResult {
	switch {
	case opts.expectPath != "":
		data, err := os.ReadFile(opts.expectPath)
		if err != nil {
			return goldenResult{Path: opts.expectPath, Mode: "expect", Actual: sexpr, Error: err.Error()}
		}
		expected := normalizeGoldenSExpr(string(data))
		actual := normalizeGoldenSExpr(sexpr)
		return goldenResult{
			Path:     opts.expectPath,
			Mode:     "expect",
			Matched:  expected == actual,
			Expected: expected,
			Actual:   actual,
		}
	case opts.writeExpectPath != "":
		err := os.WriteFile(opts.writeExpectPath, []byte(sexpr+"\n"), 0644)
		out := goldenResult{Path: opts.writeExpectPath, Mode: "write", Updated: err == nil, Actual: sexpr}
		if err != nil {
			out.Error = err.Error()
		}
		return out
	default:
		return goldenResult{}
	}
}

func normalizeGoldenSExpr(s string) string {
	s = strings.TrimSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\r")
	return s
}

func printGoldenResult(result goldenResult) {
	if result.Mode == "" {
		return
	}
	switch {
	case result.Error != "":
		fmt.Printf("Golden: failed: %s\n", result.Error)
	case result.Mode == "write":
		fmt.Printf("Golden: wrote %s\n", result.Path)
	case result.Matched:
		fmt.Printf("Golden: matched %s\n", result.Path)
	default:
		fmt.Printf("Golden: mismatch %s\n", result.Path)
		fmt.Printf("  expected: %s\n", result.Expected)
		fmt.Printf("  actual:   %s\n", result.Actual)
	}
}

type parseJSON struct {
	Grammar string        `json:"grammar"`
	Sample  string        `json:"sample"`
	Bytes   int           `json:"bytes"`
	Parse   parseStatus   `json:"parse"`
	Golden  *goldenResult `json:"golden,omitempty"`
}

type parseStatus struct {
	OK           bool   `json:"ok"`
	Root         string `json:"root,omitempty"`
	StartByte    uint32 `json:"start_byte"`
	EndByte      uint32 `json:"end_byte"`
	HasError     bool   `json:"has_error"`
	StoppedEarly bool   `json:"stopped_early"`
	StopReason   string `json:"stop_reason,omitempty"`
	SExpr        string `json:"sexpr,omitempty"`
	Runtime      string `json:"runtime,omitempty"`
	Error        string `json:"error,omitempty"`
}

func parseJSONReport(name, sampleName string, sampleBytes int, result parseResult, lang *gotreesitter.Language, runtime bool, golden goldenResult) *parseJSON {
	out := &parseJSON{
		Grammar: name,
		Sample:  sampleName,
		Bytes:   sampleBytes,
	}
	if golden.Mode != "" {
		out.Golden = &golden
	}
	if result.err != nil {
		out.Parse = parseStatus{OK: false, Error: result.err.Error()}
		return out
	}
	if result.root == nil || result.tree == nil {
		out.Parse = parseStatus{OK: false, Error: "nil parse tree or root"}
		return out
	}
	root := result.root
	out.Parse = parseStatus{
		OK:           !parseResultFailed(result),
		Root:         root.Type(lang),
		StartByte:    root.StartByte(),
		EndByte:      root.EndByte(),
		HasError:     root.HasError(),
		StoppedEarly: result.tree.ParseStoppedEarly(),
		StopReason:   string(result.tree.ParseStopReason()),
		SExpr:        resultSExpr(result, lang),
	}
	if runtime {
		out.Parse.Runtime = result.tree.ParseRuntime().Summary()
	}
	return out
}

func writeJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		exitf("marshal JSON: %v", err)
	}
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

func printDoctorText(
	name string,
	g *grammargen.Grammar,
	warnings []string,
	rpt *grammargen.GenerateReport,
	glrConflicts int,
	conflictLimit int,
	testStatus string,
	testError string,
	hasSample bool,
	sampleName string,
	sampleBytes int,
	sampleReport *parseJSON,
	golden goldenResult,
	steps []string,
	runtime bool,
) {
	fmt.Printf("Grammar: %s\n", name)
	fmt.Printf("Rules:   %d\n", len(g.Rules))
	if len(warnings) == 0 {
		fmt.Println("Validate: OK")
	} else {
		fmt.Printf("Validate: %d warning(s)\n", len(warnings))
		for _, w := range warnings {
			fmt.Printf("  - %s\n", w)
		}
	}
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
	printConflictDetails(rpt.Conflicts, g, conflictLimit)

	switch testStatus {
	case "none":
		fmt.Println("Embedded tests: none")
	case "ok":
		fmt.Printf("Embedded tests: OK (%d)\n", len(g.Tests))
	case "failed":
		fmt.Println("Embedded tests: failed")
		fmt.Println(testError)
	}

	if hasSample && sampleReport != nil {
		fmt.Printf("Sample: %s (%d bytes)\n", sampleName, sampleBytes)
		printParseStatus(sampleReport.Parse, runtime)
		printGoldenResult(golden)
	} else {
		fmt.Println("Sample: not run")
	}

	printNextSteps(steps)
}

func printParseStatus(status parseStatus, runtime bool) {
	if status.Error != "" {
		fmt.Printf("Parse:   failed: %v\n", status.Error)
		return
	}
	if status.OK {
		fmt.Println("Parse:   OK")
	} else {
		fmt.Println("Parse:   completed with errors")
	}
	fmt.Printf("Root:    %s [%d:%d]\n", status.Root, status.StartByte, status.EndByte)
	fmt.Printf("Error:   %v\n", status.HasError)
	fmt.Printf("Stop:    %s\n", status.StopReason)
	if runtime && status.Runtime != "" {
		fmt.Printf("Runtime: %s\n", status.Runtime)
	}
	fmt.Println("S-expression:")
	fmt.Println(status.SExpr)
}

func printConflictDetails(conflicts []grammargen.ConflictDiag, g *grammargen.Grammar, limit int) {
	for _, detail := range conflictDetails(conflicts, g, limit) {
		fmt.Println()
		fmt.Println(detail)
	}
}

func conflictDetails(conflicts []grammargen.ConflictDiag, g *grammargen.Grammar, limit int) []string {
	if limit <= 0 || len(conflicts) == 0 {
		return nil
	}
	if limit > len(conflicts) {
		limit = len(conflicts)
	}
	ng, err := grammargen.Normalize(g)
	if err != nil {
		return []string{"conflict detail unavailable: " + err.Error()}
	}
	details := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		details = append(details, fmt.Sprintf("[%d] %s", i+1, conflicts[i].String(ng)))
	}
	return details
}

type doctorJSONReport struct {
	Grammar       string       `json:"grammar"`
	Rules         int          `json:"rules"`
	Validate      validateJSON `json:"validate"`
	Generate      generateJSON `json:"generate"`
	EmbeddedTests testsJSON    `json:"embedded_tests"`
	Sample        *parseJSON   `json:"sample,omitempty"`
	Next          []string     `json:"next,omitempty"`
}

type validateJSON struct {
	OK       bool     `json:"ok"`
	Warnings []string `json:"warnings,omitempty"`
}

type generateJSON struct {
	OK              bool     `json:"ok"`
	Symbols         int      `json:"symbols"`
	States          int      `json:"states"`
	Tokens          int      `json:"tokens"`
	BlobBytes       int      `json:"blob_bytes"`
	Conflicts       int      `json:"conflicts"`
	GLRConflicts    int      `json:"glr_conflicts"`
	ConflictDetails []string `json:"conflict_details,omitempty"`
}

type testsJSON struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
	Error  string `json:"error,omitempty"`
}

func nextSteps(name string, src sourceFlags, parsedSample bool) []string {
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
	return steps
}

func printNextSteps(steps []string) {
	if len(steps) == 0 {
		return
	}
	fmt.Println("Next:")
	for _, step := range steps {
		fmt.Printf("  %s\n", step)
	}
}

func writeGrammarJSONOutput(path string, g *grammargen.Grammar) {
	data, err := grammargen.ExportGrammarJSON(g)
	if err != nil {
		exitf("grammar.json generation failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		exitf("write %s: %v", path, err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, len(data))
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
		"bin":          true,
		"c":            true,
		"conflicts":    true,
		"expect":       true,
		"format":       true,
		"func":         true,
		"go":           true,
		"js":           true,
		"json":         true,
		"json-out":     true,
		"grammar":      true,
		"pkg":          true,
		"sample":       true,
		"text":         true,
		"write-expect": true,
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
