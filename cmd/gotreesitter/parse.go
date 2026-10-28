package main

import (
	"fmt"
	"os"

	"github.com/odvcencio/gotreesitter"
)

// firstError returns the first ERROR or MISSING node in document order.
func firstError(n *gotreesitter.Node, lang *gotreesitter.Language) *gotreesitter.Node {
	if n == nil || !n.HasErrorOrMissing() {
		return nil
	}
	if n.IsMissing() || n.Type(lang) == "ERROR" {
		return n
	}
	for i := 0; i < n.ChildCount(); i++ {
		if e := firstError(n.Child(i), lang); e != nil {
			return e
		}
	}
	return n
}

func (c *cli) parse(args []string) int {
	fs := c.flags("parse")
	lang := fs.String("l", "", "language name (default: detect from the file)")
	asJSON := fs.Bool("json", false, "print JSON")
	if code, stop := parseFlags(fs, args); stop {
		return code
	}
	in, code := c.loadArg(fs, *lang)
	if in == nil {
		return code
	}
	bt, err := in.parse()
	if err != nil {
		return c.fail("%v", err)
	}
	defer bt.Release()

	root, tl := bt.RootNode(), bt.Language()
	bad := root.HasErrorOrMissing() || bt.ParseStoppedEarly()
	sexp := root.SExpr(tl)
	if *asJSON {
		c.emitJSON(map[string]any{
			"file": in.name, "language": in.entry.Name, "has_error": bad,
			"stopped_early": bt.ParseStoppedEarly(), "sexp": sexp,
		})
	} else {
		fmt.Fprintln(c.stdout, sexp)
	}
	if !bad {
		return 0
	}
	if e := firstError(root, tl); e != nil {
		fmt.Fprintf(c.stderr, "%s:%s: syntax error (%s)\n", in.name, pos(e.StartPoint()), e.Type(tl))
	} else {
		fmt.Fprintf(c.stderr, "%s: parse stopped early (%v)\n", in.name, bt.ParseStopReason())
	}
	return 1
}

func (c *cli) query(args []string) int {
	fs := c.flags("query")
	lang := fs.String("l", "", "language name (default: detect from the file)")
	text := fs.String("q", "", "query source")
	qfile := fs.String("f", "", "file holding the query")
	asJSON := fs.Bool("json", false, "print JSON")
	if code, stop := parseFlags(fs, args); stop {
		return code
	}
	if (*text == "") == (*qfile == "") {
		return c.fail("query needs exactly one of -q or -f")
	}
	if *qfile != "" {
		b, err := os.ReadFile(*qfile)
		if err != nil {
			return c.fail("%v", err)
		}
		*text = string(b)
	}
	in, code := c.loadArg(fs, *lang)
	if in == nil {
		return code
	}
	bt, err := in.parse()
	if err != nil {
		return c.fail("%v", err)
	}
	defer bt.Release()

	tl := bt.Language()
	q, err := gotreesitter.NewQuery(*text, tl)
	if err != nil {
		return c.fail("query: %v", err)
	}
	type capture struct {
		Name      string `json:"name"`
		Type      string `json:"type"`
		Start     string `json:"start"`
		End       string `json:"end"`
		StartByte uint32 `json:"start_byte"`
		EndByte   uint32 `json:"end_byte"`
		Text      string `json:"text"`
	}
	type match struct {
		Pattern  int       `json:"pattern"`
		Captures []capture `json:"captures"`
	}
	var matches []match
	cur := q.Exec(bt.RootNode(), tl, in.text)
	for {
		m, ok := cur.NextMatch()
		if !ok {
			break
		}
		out := match{Pattern: m.PatternIndex}
		for _, cp := range m.Captures {
			r := cp.Range()
			out.Captures = append(out.Captures, capture{
				cp.Name, cp.Node.Type(tl), pos(r.StartPoint), pos(r.EndPoint),
				r.StartByte, r.EndByte, cp.Text(in.text),
			})
		}
		matches = append(matches, out)
	}
	if *asJSON {
		c.emitJSON(matches)
		return 0
	}
	for _, m := range matches {
		for _, cp := range m.Captures {
			fmt.Fprintf(c.stdout, "%s-%s\t@%s\t%s\t%s\n", cp.Start, cp.End, cp.Name, cp.Type, snippet(cp.Text))
		}
	}
	return 0
}
