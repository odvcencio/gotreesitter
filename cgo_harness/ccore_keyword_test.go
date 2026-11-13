//go:build cgo && treesitter_c_parity

package cgoharness

// Heap keyword leaf: a grammar with keyword extraction where the
// keyword tokens are heap leaves (300 spaces of padding put a token past the
// inline limit of 255 bytes) and several versions are alive at one byte, so
// that the cached token is changed from keyword to word token while other
// versions still hold it (internal/ccore/lifetime.go, step 2; C/parser.c:1760-1780). The set is
// checked fresh and through an edit chain against pinned C: trace, digest,
// subtree records.

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ccoreKeywordSource builds n lines of the construct that the scan found for
// the keyword switch: a type assertion followed by the word "extends", which is
// padded by pad spaces so that the token is a heap leaf. The grammar lexes the
// keyword, the parse state has no action for it, and the parser changes the
// token to the word token while other versions are alive at the same byte.
func ccoreKeywordSource(n, pad int) []byte {
	sp := strings.Repeat(" ", pad)
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "x%d = <T>%sextends;\nlet y%d = %d;\n", i, sp, i, i)
	}
	return []byte(b.String())
}

var ccoreKeywordWords = []string{"abstract", "accessor", "any", "as", "asserts", "async", "await", "boolean", "break", "case", "catch", "class",
	"const", "constructor", "continue", "debugger", "declare", "default", "delete", "do", "else", "enum", "export", "extends", "false",
	"finally", "for", "from", "function", "get", "global", "if", "implements", "import", "in", "infer", "instanceof", "interface", "is",
	"keyof", "let", "module", "namespace", "never", "new", "null", "number", "object", "of", "override", "package", "private", "protected",
	"public", "readonly", "require", "return", "satisfies", "set", "static", "string", "super", "switch", "symbol", "target", "this",
	"throw", "true", "try", "type", "typeof", "undefined", "unique", "unknown", "using", "var", "void", "while", "with", "yield"}

var ccoreKeywordTemplates = []string{
	"a.%s;\n", "a?.%s;\n", "({ %s: 1 });\n", "({ %s });\n", "({ %s() {} });\n", "class C { %s() {} }\n", "class C { %s = 1 }\n", "class C { static %s = 1 }\n",
	"let x: a.%s;\n", "function f(%s) {}\n", "function %s() {}\n", "import { %s } from \"m\";\n", "import %s from \"m\";\n", "type X = { %s: number };\n",
	"interface I { %s: number }\n", "enum E { %s }\n", "for (%s of xs) {}\n", "%s: for (;;) {}\n", "new %s();\n", "f(%s => 1);\n", "(%s) => 1;\n",
	"async %s => 1;\n", "x = %s;\n", "let %s = 1;\n", "%s = 1;\n", "%s(1);\n", "%s`t`;\n", "a[%s];\n", "type T = %s;\n", "let y: %s;\n",
	"x = <T>%s;\n", "x = a < %s > b;\n", "x = a as %s;\n", "export { %s };\n", "export default %s;\n", "namespace N { %s }\n", "x = { get %s() { return 1 } };\n",
	"@%s class C {}\n", "x = y ? %s : z;\n", "x = y ? (%s) : z => 1;\n",
}

// TestCcoreKeywordScan finds which (template, word) pairs make pinned C log
// "switch from_keyword" with the word padded by 300 spaces.
func TestCcoreKeywordScan(t *testing.T) {
	if os.Getenv("CCORE_KW") != "scan" {
		t.Skip("set CCORE_KW=scan")
	}
	lang := os.Getenv("CCORE_KW_LANG")
	if lang == "" {
		lang = "typescript"
	}
	gl, cl := ccoreLanguages(t, lang)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	var clog []string
	cp.SetLogger(func(kind sitter.LogType, msg string) {
		if kind == sitter.LogTypeParse {
			clog = append(clog, msg)
		}
	})
	core := ccoreNew(gl)
	pad := strings.Repeat(" ", 300)
	hits, total := 0, 0
	for ti, tmpl := range ccoreKeywordTemplates {
		per := 0
		for _, kw := range ccoreKeywordWords {
			src := []byte(fmt.Sprintf(tmpl, pad+kw))
			clog = clog[:0]
			ct := cp.Parse(src, nil)
			cerr := ct.RootNode().HasError()
			ct.Close()
			total++
			sw := ccoreCountLines(clog, "switch from_keyword")
			if sw == 0 {
				continue
			}
			var slog []string
			core.Parse(src, func(s string) { slog = append(slog, s) })
			_, _, copies := core.RecordSources()
			core.ReleaseLast()
			hits++
			per++
			fmt.Printf("CCKWSCAN template=%d word=%s switch=%d leaf_copies=%d trace_eq=%t c_error=%t\n", ti, kw, sw, copies, ccoreFirstDiff(clog, slog) < 0, cerr)
		}
		fmt.Printf("CCKWSCANT template=%d %q hits=%d\n", ti, tmpl, per)
	}
	fmt.Printf("CCKWSCANDONE pairs=%d hits=%d\n", total, hits)
}

func TestCcoreKeyword(t *testing.T) {
	if os.Getenv("CCORE_KW") != "1" {
		t.Skip("set CCORE_KW=1")
	}
	lang := os.Getenv("CCORE_KW_LANG")
	if lang == "" {
		lang = "typescript"
	}
	gl, cl := ccoreLanguages(t, lang)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	var clog []string
	cp.SetLogger(func(kind sitter.LogType, msg string) {
		if kind == sitter.LogTypeParse {
			clog = append(clog, msg)
		}
	})
	src := ccoreKeywordSource(120, 300)
	core := ccoreNew(gl)
	var slog []string
	clog = clog[:0]
	ctree := cp.Parse(src, nil)
	stop := core.Parse(src, func(s string) { slog = append(slog, s) })
	first := -1
	for i := 0; i < len(clog) || i < len(slog); i++ {
		if i >= len(clog) || i >= len(slog) || ccoreNormLine(clog[i]) != ccoreNormLine(slog[i]) {
			first = i
			break
		}
	}
	tree := core.Tree()
	fromKw := ccoreCountLines(clog, "switch from_keyword")
	_, _, copies := core.RecordSources()
	ch, cn, rc := corecords.Hash(ctree, "")
	gh, gn := core.RecordsHash("")
	fmt.Printf("CCKW input=fresh bytes=%d stop=%q trace_eq=%t first_diff=%d c_lines=%d switch_from_keyword=%d leaf_copies=%d digest_eq=%t rec_eq=%t rec_lines=%d c_error=%t\n",
		len(src), stop, first < 0, first, len(clog), fromKw, copies, core.Digest() == ccoreCDigest(ctree), rc == 0 && ch == gh && cn == gn, gn, ctree.RootNode().HasError())
	if stop != "" || first >= 0 || core.Digest() != ccoreCDigest(ctree) || rc != 0 || ch != gh || cn != gn {
		t.Errorf("keyword test failed on the fresh parse: stop=%q first_diff=%d", stop, first)
	}
	checks := 1

	// The chain: at 60 places change the T of "<T>" to U and back, and take one
	// padding space away and put it back (the token stays a heap leaf).
	cur := src
	places := 0
	for at := 0; at+3 < len(src) && places < 60; at++ {
		if string(src[at:at+3]) != "<T>" {
			continue
		}
		places++
		for k := 0; k < 4; k++ {
			next := append([]byte(nil), cur...)
			switch k {
			case 0:
				next[at+1] = 'U'
			case 1:
				next[at+1] = 'T'
			case 2:
				next = append(next[:at+3], next[at+4:]...)
			default:
				next = append(next[:at+3], append([]byte{' '}, next[at+3:]...)...)
			}
			e := ccoreDiffEdit(cur, next)
			ctree.Edit(ccoreSitterEdit(e))
			clog = clog[:0]
			cnext := cp.Parse(next, ctree)
			ctree.Close()
			ctree = cnext
			edited := core.Edit(tree, *e)
			slog = slog[:0]
			ntree, stop := core.ParseIncremental(next, edited, func(s string) { slog = append(slog, s) })
			first = -1
			for i := 0; i < len(clog) || i < len(slog); i++ {
				if i >= len(clog) || i >= len(slog) || ccoreNormLine(clog[i]) != ccoreNormLine(slog[i]) {
					first = i
					break
				}
			}
			ch, cn, rc = corecords.Hash(ctree, "")
			gh, gn = core.RecordsHash("")
			_, _, copies = core.RecordSources()
			checks++
			fmt.Printf("CCKWINC place=%d k=%d stop=%q trace_eq=%t first_diff=%d switch_from_keyword=%d leaf_copies=%d digest_eq=%t rec_eq=%t\n",
				places, k, stop, first < 0, first, ccoreCountLines(clog, "switch from_keyword"), copies,
				core.DigestOf(ntree, false) == ccoreCDigest(ctree), rc == 0 && ch == gh && cn == gn)
			if stop != "" || first >= 0 || core.DigestOf(ntree, false) != ccoreCDigest(ctree) || rc != 0 || ch != gh || cn != gn {
				t.Errorf("keyword test failed at place %d k %d: stop=%q first_diff=%d", places, k, stop, first)
			}
			core.Release(edited)
			core.Release(tree)
			tree = ntree
			cur = next
		}
	}
	core.Release(tree)
	fmt.Printf("CCKWDONE places=%d shared_in_tree=%d double_free=%d bad_retain=%d\n", places, core.SharedInTree(), core.DoubleFree(), core.BadRetain())
	if core.SharedInTree() != 0 || core.DoubleFree() != 0 || core.BadRetain() != 0 {
		t.Errorf("keyword test: shared_in_tree=%d double_free=%d bad_retain=%d", core.SharedInTree(), core.DoubleFree(), core.BadRetain())
	}
	ctree.Close()
	ccoreCount("TestCcoreKeyword", lang, checks)
}
