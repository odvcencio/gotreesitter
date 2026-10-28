package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/odvcencio/gotreesitter/grep"
)

// grep matches a code pattern ($NAME, $$$ARGS, $_ are metavariables) in each
// file. With -r it rewrites matches: -w writes files in place, otherwise it
// prints the rewritten text of the one file.
func (c *cli) grep(args []string) int {
	fs := c.flags("grep")
	lang := fs.String("l", "", "language name (default: detect from each file)")
	repl := fs.String("r", "", "replacement template; may use the pattern's metavariables")
	write := fs.Bool("w", false, "with -r, write changes to the files")
	if code, stop := parseFlags(fs, args); stop {
		return code
	}
	if fs.NArg() < 2 {
		return c.fail("grep needs a pattern and at least one file")
	}
	pattern, files := fs.Arg(0), fs.Args()[1:]
	rewrite := false
	fs.Visit(func(f *flag.Flag) { rewrite = rewrite || f.Name == "r" })
	if rewrite && !*write && len(files) != 1 {
		return c.fail("grep -r without -w takes exactly one file")
	}
	for _, file := range files {
		in, err := c.load(file, *lang)
		if err != nil {
			return c.fail("%v", err)
		}
		tl := in.entry.Language()
		if !rewrite {
			results, err := grep.Match(tl, pattern, in.text)
			if err != nil {
				return c.fail("%s: %v", file, err)
			}
			for _, r := range results {
				line, col := lineCol(in.text, r.StartByte)
				fmt.Fprintf(c.stdout, "%s:%d:%d: %s\n", file, line, col, snippet(string(in.text[r.StartByte:r.EndByte])))
			}
			continue
		}
		res, err := grep.Replace(tl, pattern, *repl, in.text)
		if err != nil {
			return c.fail("%s: %v", file, err)
		}
		for _, d := range res.Diagnostics {
			fmt.Fprintf(c.stderr, "%s: %s\n", file, d.Message)
		}
		out := grep.ApplyEdits(in.text, res.Edits)
		if !*write {
			c.stdout.Write(out)
		} else if err := os.WriteFile(file, out, 0o644); err != nil {
			return c.fail("%v", err)
		} else {
			fmt.Fprintf(c.stdout, "%s: %d replacement(s)\n", file, len(res.Edits))
		}
	}
	return 0
}

// lineCol converts a byte offset to a 1-based line and column.
func lineCol(src []byte, off uint32) (line, col int) {
	line, col = 1, 1
	for _, b := range src[:off] {
		if b == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return line, col
}
