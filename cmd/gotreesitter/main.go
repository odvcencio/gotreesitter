// Command gotreesitter parses, queries, highlights, tags and outlines source
// files with the pure-Go gotreesitter runtime. It needs no cgo.
//
// Run "gotreesitter help" for the subcommands. Exit codes: 0 success,
// 1 syntax errors found (parse, test), 2 usage or input error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const usage = `usage: gotreesitter <command> [flags] [file]

commands:
  parse      print the syntax tree as an S-expression
  query      run a tree-sitter query (-q 'text' or -f file.scm)
  highlight  print highlight captures
  tags       print symbol definitions and references
  outline    print a nested symbol outline
  grep       match a code pattern, optionally rewrite (-r)
  test       run an upstream tree-sitter corpus (--root, --langs)
  languages  list supported languages

file is a path, or "-" for standard input (then pass -l). Flags go before file.
Positions are 1-based line:column. Add -json for machine-readable output.
Exit codes: 0 ok, 1 syntax errors found, 2 usage or input error.
`

type cli struct {
	stdin          io.Reader
	stdout, stderr io.Writer
}

func main() {
	c := &cli{os.Stdin, os.Stdout, os.Stderr}
	os.Exit(c.run(os.Args[1:]))
}

func (c *cli) run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(c.stderr, usage)
		return 2
	}
	cmds := map[string]func([]string) int{
		"parse": c.parse, "query": c.query, "highlight": c.highlight,
		"tags": c.tags, "outline": c.outline, "grep": c.grep, "test": c.test,
		"languages": c.languages,
	}
	switch args[0] {
	case "help", "-h", "-help", "--help":
		fmt.Fprint(c.stdout, usage)
		return 0
	}
	fn, ok := cmds[args[0]]
	if !ok {
		fmt.Fprintf(c.stderr, "gotreesitter: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	return fn(args[1:])
}

// flags builds a flag set whose errors go to stderr.
func (c *cli) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("gotreesitter "+name, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	return fs
}

// parseFlags returns an exit code and whether the caller should stop.
func parseFlags(fs *flag.FlagSet, args []string) (int, bool) {
	switch err := fs.Parse(args); err {
	case nil:
		return 0, false
	case flag.ErrHelp:
		return 0, true
	}
	return 2, true
}

func (c *cli) fail(format string, args ...any) int {
	fmt.Fprintf(c.stderr, "gotreesitter: "+format+"\n", args...)
	return 2
}

func (c *cli) emitJSON(v any) {
	enc := json.NewEncoder(c.stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

// input is one source file with its resolved language.
type input struct {
	name  string
	text  []byte
	entry *grammars.LangEntry
}

// load reads file ("-" is stdin) and resolves its language from langName,
// then the file name, then a shebang line.
func (c *cli) load(file, langName string) (*input, error) {
	var text []byte
	var err error
	if file == "-" {
		text, err = io.ReadAll(c.stdin)
	} else {
		text, err = os.ReadFile(file)
	}
	if err != nil {
		return nil, err
	}
	in := &input{name: file, text: text}
	switch {
	case langName != "":
		if in.entry = grammars.DetectLanguageByName(langName); in.entry == nil {
			return nil, fmt.Errorf("unknown language %q (see: gotreesitter languages)", langName)
		}
	case file != "-":
		in.entry = grammars.DetectLanguage(file)
	}
	if in.entry == nil {
		first, _, _ := strings.Cut(string(text), "\n")
		in.entry = grammars.DetectLanguageByShebang(first)
	}
	if in.entry == nil {
		return nil, fmt.Errorf("cannot detect the language of %s; pass -l <language>", file)
	}
	return in, nil
}

// loadArg loads the single file argument that remains after flag parsing.
func (c *cli) loadArg(fs *flag.FlagSet, langName string) (*input, int) {
	if fs.NArg() != 1 {
		return nil, c.fail("%s needs exactly one file (or -)", fs.Name())
	}
	in, err := c.load(fs.Arg(0), langName)
	if err != nil {
		return nil, c.fail("%v", err)
	}
	return in, 0
}

// parse parses the input. The caller must Release the tree.
func (in *input) parse() (*gotreesitter.BoundTree, error) {
	if d := grammars.DetectLanguage(in.name); d != nil && d.Name == in.entry.Name {
		return grammars.ParseFile(in.name, in.text)
	}
	lang := in.entry.Language()
	parser := gotreesitter.NewParser(lang)
	var tree *gotreesitter.Tree
	var err error
	if f := in.entry.TokenSourceFactory; f != nil {
		tree, err = parser.ParseWithTokenSource(in.text, f(in.text, lang))
	} else {
		tree, err = parser.Parse(in.text)
	}
	if err != nil {
		return nil, err
	}
	return gotreesitter.Bind(tree), nil
}

func pos(p gotreesitter.Point) string { return fmt.Sprintf("%d:%d", p.Row+1, p.Column+1) }

// snippet returns the first line of s, cut to 80 bytes.
func snippet(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	if len(s) > 80 {
		s = s[:80] + "..."
	}
	return s
}

func (c *cli) languages(args []string) int {
	fs := c.flags("languages")
	if code, stop := parseFlags(fs, args); stop {
		return code
	}
	for _, e := range grammars.AllLanguages() {
		fmt.Fprintf(c.stdout, "%s\t%s\n", e.Name, strings.Join(e.Extensions, " "))
	}
	return 0
}
