package main

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func (c *cli) highlight(args []string) int {
	fs := c.flags("highlight")
	lang := fs.String("l", "", "language name (default: detect from the file)")
	asJSON := fs.Bool("json", false, "print JSON")
	if code, stop := parseFlags(fs, args); stop {
		return code
	}
	in, code := c.loadArg(fs, *lang)
	if in == nil {
		return code
	}
	tl := in.entry.Language()
	var opts []gotreesitter.HighlighterOption
	if f := in.entry.TokenSourceFactory; f != nil {
		opts = append(opts, gotreesitter.WithTokenSourceFactory(func(src []byte) gotreesitter.TokenSource { return f(src, tl) }))
	}
	h, err := gotreesitter.NewHighlighter(tl, in.entry.HighlightQuery, opts...)
	if err != nil {
		return c.fail("%s: highlight query: %v", in.entry.Name, err)
	}
	ranges := h.Highlight(in.text)
	if *asJSON {
		c.emitJSON(ranges)
		return 0
	}
	for _, r := range ranges {
		fmt.Fprintf(c.stdout, "%d-%d\t@%s\t%s\n", r.StartByte, r.EndByte, r.Capture, snippet(string(in.text[r.StartByte:r.EndByte])))
	}
	return 0
}

func (c *cli) tags(args []string) int {
	fs := c.flags("tags")
	lang := fs.String("l", "", "language name (default: detect from the file)")
	asJSON := fs.Bool("json", false, "print JSON")
	if code, stop := parseFlags(fs, args); stop {
		return code
	}
	in, code := c.loadArg(fs, *lang)
	if in == nil {
		return code
	}
	query := grammars.ResolveTagsQuery(*in.entry)
	if query == "" {
		return c.fail("%s has no tags query", in.entry.Name)
	}
	tl := in.entry.Language()
	var opts []gotreesitter.TaggerOption
	if f := in.entry.TokenSourceFactory; f != nil {
		opts = append(opts, gotreesitter.WithTaggerTokenSourceFactory(func(src []byte) gotreesitter.TokenSource { return f(src, tl) }))
	}
	tg, err := gotreesitter.NewTagger(tl, query, opts...)
	if err != nil {
		return c.fail("%s: tags query: %v", in.entry.Name, err)
	}
	tags := tg.Tag(in.text)
	if *asJSON {
		c.emitJSON(tags)
		return 0
	}
	for _, t := range tags {
		fmt.Fprintf(c.stdout, "%s\t%s\t%s\n", pos(t.NameRange.StartPoint), t.Kind, t.Name)
	}
	return 0
}

func (c *cli) outline(args []string) int {
	fs := c.flags("outline")
	lang := fs.String("l", "", "language name (default: detect from the file)")
	asJSON := fs.Bool("json", false, "print JSON")
	if code, stop := parseFlags(fs, args); stop {
		return code
	}
	in, code := c.loadArg(fs, *lang)
	if in == nil {
		return code
	}
	query := grammars.ResolveTagsQuery(*in.entry)
	if query == "" {
		return c.fail("%s has no tags query, so no outline", in.entry.Name)
	}
	o, err := gotreesitter.NewOutliner(in.entry.Language(), query,
		gotreesitter.WithOutlineOwnerRules(grammars.OutlineOwnerRules(*in.entry)))
	if err != nil {
		return c.fail("%s: outline: %v", in.entry.Name, err)
	}
	bt, err := in.parse()
	if err != nil {
		return c.fail("%v", err)
	}
	defer bt.Release()
	symbols, report := o.OutlineBound(bt)
	if *asJSON {
		c.emitJSON(map[string]any{"symbols": symbols, "report": report})
		return 0
	}
	var walk func([]gotreesitter.OutlineSymbol, int)
	walk = func(syms []gotreesitter.OutlineSymbol, depth int) {
		for _, s := range syms {
			name := s.Name
			if s.Owner != "" {
				name = s.Owner + "." + name
			}
			fmt.Fprintf(c.stdout, "%s%s %s\t%s\n", strings.Repeat("  ", depth), s.Kind, name, pos(s.Range.StartPoint))
			walk(s.Children, depth+1)
		}
	}
	walk(symbols, 0)
	return 0
}
