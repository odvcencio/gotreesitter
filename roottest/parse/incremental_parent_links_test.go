package parse_test

// After ParseIncremental reuses subtrees, every child of the returned tree
// must report the node it was reached from through Child() as its Parent().
// These tests use the public API only.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func plPoint(src []byte, off int) gotreesitter.Point {
	row := bytes.Count(src[:off], []byte{'\n'})
	col := off - (bytes.LastIndexByte(src[:off], '\n') + 1)
	return gotreesitter.Point{Row: uint32(row), Column: uint32(col)}
}

func plEdit(old, src []byte, start, oldEnd, newEnd int) gotreesitter.InputEdit {
	return gotreesitter.InputEdit{
		StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd),
		StartPoint: plPoint(old, start), OldEndPoint: plPoint(old, oldEnd), NewEndPoint: plPoint(src, newEnd),
	}
}

// plWrongParents walks the tree with ChildCount and Child and lists every
// child whose Parent() is not the node it was reached from.
func plWrongParents(root *gotreesitter.Node, lang *gotreesitter.Language) []string {
	var bad []string
	stack := []*gotreesitter.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for i := 0; i < n.ChildCount(); i++ {
			c := n.Child(i)
			if c == nil {
				continue
			}
			if p := c.Parent(); p != n {
				desc := "nil"
				if p != nil {
					desc = fmt.Sprintf("%s[%d,%d)", p.Type(lang), p.StartByte(), p.EndByte())
				}
				bad = append(bad, fmt.Sprintf("child %d %s[%d,%d) of %s[%d,%d) reports Parent() = %s",
					i, c.Type(lang), c.StartByte(), c.EndByte(), n.Type(lang), n.StartByte(), n.EndByte(), desc))
			}
			stack = append(stack, c)
		}
	}
	return bad
}

// plCheck fails the test when the tree has a wrong parent link, and also walks
// upward from every leaf to the root (the path Node.Edit uses).
func plCheck(t *testing.T, what string, tree *gotreesitter.Tree, lang *gotreesitter.Language) {
	t.Helper()
	root := tree.RootNode()
	if bad := plWrongParents(root, lang); len(bad) != 0 {
		t.Errorf("%s: %d children report the wrong Parent(); first: %s", what, len(bad), bad[0])
		return
	}
	stack := []*gotreesitter.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.ChildCount() == 0 {
			top := n
			for hops := 0; top.Parent() != nil; hops++ {
				if hops > 1<<20 {
					t.Errorf("%s: parent chain does not end", what)
					return
				}
				top = top.Parent()
			}
			if top != root {
				t.Errorf("%s: leaf %s[%d,%d) reaches a root other than RootNode()", what, n.Type(lang), n.StartByte(), n.EndByte())
				return
			}
			continue
		}
		for i := 0; i < n.ChildCount(); i++ {
			if c := n.Child(i); c != nil {
				stack = append(stack, c)
			}
		}
	}
}

func plLang(t *testing.T, name string) *gotreesitter.Language {
	t.Helper()
	e := grammars.DetectLanguageByName(name)
	if e == nil || e.Language() == nil {
		t.Fatalf("grammar %q unavailable", name)
	}
	return e.Language()
}

// plInsert returns old with text inserted at byte at, and the matching edit.
func plInsert(old []byte, at int, text string) ([]byte, gotreesitter.InputEdit) {
	src := append(append(append([]byte{}, old[:at]...), text...), old[at:]...)
	return src, plEdit(old, src, at, at, at+len(text))
}

func TestIncrementalParentLinksReproducer(t *testing.T) {
	cases := []struct {
		lang   string
		old    string
		at     int
		insert string
	}{
		{"markdown", "\n", 0, "x"},
		{"go", "{{l(``)}}", 0, " "},
		{"c", "#E\n#i\n#i\n", 9, " "},
		{"elixir", "%{s}", 0, "x"},
		{"json", "a", 0, "x"},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			lang := plLang(t, tc.lang)
			old := []byte(tc.old)
			src, edit := plInsert(old, tc.at, tc.insert)
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(old)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			tree.Edit(edit)
			next, err := parser.ParseIncremental(src, tree)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			plCheck(t, "incremental tree", next, lang)
		})
	}
}

// Broken sources: the parse runs recovery attempts and drops the losers.
func TestIncrementalParentLinksRecoveryAttempts(t *testing.T) {
	cases := []struct {
		lang   string
		old    string
		at     int
		insert string
	}{
		{"c", `"`, 0, "x"},
		{"go", `"`, 1, "x"},
		{"json", "a", 0, "x"},
		{"python", "and 2", 5, "x"},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			lang := plLang(t, tc.lang)
			old := []byte(tc.old)
			src, edit := plInsert(old, tc.at, tc.insert)
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(old)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			tree.Edit(edit)
			next, err := parser.ParseIncremental(src, tree)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			plCheck(t, "incremental tree", next, lang)
		})
	}
}

func plSample(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			Language      string `json:"language"`
			Role          string `json:"role"`
			CommittedPath string `json:"committed_path"`
			Bytes         int    `json:"bytes"`
			SHA256        string `json:"sha256"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, row := range manifest.Entries {
		if row.Language != name || row.Role != "sample" || row.CommittedPath == "" {
			continue
		}
		src, err := os.ReadFile(filepath.Join("..", "..", "internal", "benchfixtures", row.CommittedPath))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(src)
		if len(src) != row.Bytes || hex.EncodeToString(sum[:]) != row.SHA256 {
			t.Fatalf("sample for %s does not match the manifest", name)
		}
		return src
	}
	return []byte(grammars.ParseSmokeSample(name))
}

// Gate sessions of c and go include steps where the accepted-error retry runs.
// The links must be right and must not depend on pooled arena memory.
func TestIncrementalParentLinksRetryAttempts(t *testing.T) {
	retried := map[bool]int{}
	for _, name := range []string{"c", "go"} {
		lang := plLang(t, name)
		src := plSample(t, name)
		parser := gotreesitter.NewParser(lang)
		other := gotreesitter.NewParser(plLang(t, "json"))
		tree, err := parser.Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		for i, step := range benchfixtures.EditingSession(src) {
			tree.Edit(step.Edit)
			next, err := parser.ParseIncremental(step.Source, tree)
			if err != nil {
				t.Fatalf("%s step %d: %v", name, i+1, err)
			}
			rt := next.ParseRuntime()
			what := fmt.Sprintf("%s step %d", name, i+1)
			plCheck(t, what, next, lang)
			if rt.IncrementalAcceptedErrorRetryAttempts == 1 {
				retried[rt.IncrementalAcceptedErrorRetryAdopted]++
				tree.Release()
				filler, _ := other.Parse([]byte(`{"a":[1,2,3],"b":{"c":null}}`))
				plCheck(t, what+" after the old tree was released and the pool reused", next, lang)
				if filler != nil {
					filler.Release()
				}
			} else if next != tree {
				tree.Release()
			}
			tree = next
		}
		tree.Release()
	}
	t.Logf("retry steps seen: adopted=%d not adopted=%d", retried[true], retried[false])
	// These sessions never adopt a retry; the adopted case (the first attempt is
	// released, the retry's tree is returned) is checked in
	// TestGoLanguageFixtureLengthChangeIncrementalMatchesFresh in the root package.
	if retried[false] == 0 {
		t.Fatal("no session step ran a non-adopted accepted-error retry; pick other inputs")
	}
}

// Thirty alternating insert and delete steps. Variant A releases the old tree
// at each step; variant B keeps every tree and checks the newest.
func TestIncrementalParentLinksChain(t *testing.T) {
	for _, name := range []string{"go", "c", "markdown", "typescript"} {
		for _, release := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/release=%t", name, release), func(t *testing.T) {
				lang := plLang(t, name)
				src := plSample(t, name)
				parser := gotreesitter.NewParser(lang)
				tree, err := parser.Parse(src)
				if err != nil {
					t.Fatal(err)
				}
				var kept []*gotreesitter.Tree
				defer func() {
					for _, k := range kept {
						k.Release()
					}
				}()
				cur := src
				for i := 0; i < 30; i++ {
					at := (i*7919 + 13) % len(cur)
					var next []byte
					var edit gotreesitter.InputEdit
					if i%2 == 0 {
						next, edit = plInsert(cur, at, "\n")
					} else {
						next = append(append([]byte{}, cur[:at]...), cur[at+1:]...)
						edit = plEdit(cur, next, at, at+1, at)
					}
					tree.Edit(edit)
					res, err := parser.ParseIncremental(next, tree)
					if err != nil {
						t.Fatalf("step %d: %v", i+1, err)
					}
					plCheck(t, fmt.Sprintf("step %d", i+1), res, lang)
					if res != tree {
						if release {
							tree.Release()
						} else {
							kept = append(kept, tree)
						}
					}
					tree, cur = res, next
				}
				tree.Release()
			})
		}
	}
}

// Tree.Copy: the copy and the source both pass; editing and reparsing the copy
// leaves the source consistent.
func TestIncrementalParentLinksCopy(t *testing.T) {
	lang := plLang(t, "go")
	src := plSample(t, "go")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	at := len(src) / 2
	for at < len(src) && src[at] != '\n' {
		at++
	}
	src2, e := plInsert(src, at, "\n")
	tree.Edit(e)
	res, err := parser.ParseIncremental(src2, tree)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Release()
	cp := res.Copy()
	defer cp.Release()
	plCheck(t, "source", res, lang)
	plCheck(t, "copy", cp, lang)
	src3, e3 := plInsert(src2, 0, "\n")
	cp.Edit(e3)
	res3, err := parser.ParseIncremental(src3, cp)
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Release()
	plCheck(t, "reparsed copy", res3, lang)
	plCheck(t, "source after the copy was reparsed", res, lang)
}

// Every public incremental entry point returns a tree with consistent links.
// Plain entries use the markdown reproducer; token-source entries use the Go
// reproducer with the Go token source.
func TestIncrementalParentLinksEntryPoints(t *testing.T) {
	type entry struct {
		name string
		lang string
		old  string
		run  func(p *gotreesitter.Parser, src []byte, old *gotreesitter.Tree, edit gotreesitter.InputEdit) (*gotreesitter.Tree, error)
	}
	goLang := plLang(t, "go")
	goFactory := func(src []byte) (gotreesitter.TokenSource, error) { return grammars.NewGoTokenSource(src, goLang) }
	toU16 := func(b []byte) []uint16 {
		out := make([]uint16, len(b))
		for i, c := range b {
			out[i] = uint16(c)
		}
		return out
	}
	toU16Bytes := func(b []byte) []byte {
		out := make([]byte, 0, 2*len(b))
		for _, c := range b {
			out = append(out, c, 0)
		}
		return out
	}
	withResult := func(r gotreesitter.ParseResult, err error) (*gotreesitter.Tree, error) { return r.Tree, err }
	entries := []entry{
		{"ParseIncremental", "markdown", "\n", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			return p.ParseIncremental(s, o)
		}},
		{"ParseIncrementalStrict", "markdown", "\n", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			return p.ParseIncrementalStrict(s, o)
		}},
		{"ParseIncrementalProfiled", "markdown", "\n", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			tree, _, err := p.ParseIncrementalProfiled(s, o)
			return tree, err
		}},
		{"ParseWith/WithOldTree", "markdown", "\n", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			return withResult(p.ParseWith(s, gotreesitter.WithOldTree(o)))
		}},
		{"ParseWith/WithOldTree+WithProfiling", "markdown", "\n", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			return withResult(p.ParseWith(s, gotreesitter.WithOldTree(o), gotreesitter.WithProfiling()))
		}},
		{"ParseWithStrict/WithOldTree", "markdown", "\n", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			return withResult(p.ParseWithStrict(s, gotreesitter.WithOldTree(o)))
		}},
		{"ParseIncrementalUTF16", "markdown", "\n", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			return p.ParseIncrementalUTF16(toU16(s), o)
		}},
		{"ParseIncrementalUTF16Bytes", "markdown", "\n", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			return p.ParseIncrementalUTF16Bytes(toU16Bytes(s), o, gotreesitter.UTF16LittleEndian)
		}},
		{"ParseIncrementalWithTokenSource", "go", "{{l(``)}}", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			ts, err := goFactory(s)
			if err != nil {
				return nil, err
			}
			return p.ParseIncrementalWithTokenSource(s, o, ts)
		}},
		{"ParseIncrementalWithTokenSourceStrict", "go", "{{l(``)}}", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			ts, err := goFactory(s)
			if err != nil {
				return nil, err
			}
			return p.ParseIncrementalWithTokenSourceStrict(s, o, ts)
		}},
		{"ParseIncrementalWithTokenSourceFactory", "go", "{{l(``)}}", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			return p.ParseIncrementalWithTokenSourceFactory(s, o, goFactory)
		}},
		{"ParseIncrementalWithTokenSourceProfiled", "go", "{{l(``)}}", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			ts, err := goFactory(s)
			if err != nil {
				return nil, err
			}
			tree, _, err := p.ParseIncrementalWithTokenSourceProfiled(s, o, ts)
			return tree, err
		}},
		{"ParseWith/WithOldTree+WithTokenSource", "go", "{{l(``)}}", func(p *gotreesitter.Parser, s []byte, o *gotreesitter.Tree, _ gotreesitter.InputEdit) (*gotreesitter.Tree, error) {
			ts, err := goFactory(s)
			if err != nil {
				return nil, err
			}
			return withResult(p.ParseWith(s, gotreesitter.WithOldTree(o), gotreesitter.WithTokenSource(ts)))
		}},
	}
	for _, e := range entries {
		t.Run(e.name, func(t *testing.T) {
			lang := plLang(t, e.lang)
			old := []byte(e.old)
			ins := "x"
			if e.lang == "go" {
				ins = " "
			}
			src, edit := plInsert(old, 0, ins)
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(old)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			tree.Edit(edit)
			next, err := e.run(parser, src, tree, edit)
			if err != nil || next == nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer next.Release()
			plCheck(t, e.name, next, lang)
		})
	}
}

// tsx session steps 26 onward returned wrong parents under the first design
// (a retry attempt's lazy compatibility pass linked leaves of a reused subtree).
func TestIncrementalParentLinksTSXSession(t *testing.T) {
	lang := plLang(t, "tsx")
	src := plSample(t, "tsx")
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	for i, step := range benchfixtures.EditingSession(src) {
		if i >= 40 {
			break
		}
		tree.Edit(step.Edit)
		next, err := parser.ParseIncremental(step.Source, tree)
		if err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
		plCheck(t, fmt.Sprintf("tsx step %d", i+1), next, lang)
		if next != tree {
			tree.Release()
		}
		tree = next
	}
	tree.Release()
}
