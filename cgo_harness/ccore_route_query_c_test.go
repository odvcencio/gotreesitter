//go:build cgo && treesitter_c_parity

package cgoharness

// Query field constraints on a core tree against C's query engine: a positive field
// (C reads the cursor's field) and an absent field (C calls ts_node_child_by_field_id)
// around nested hidden fields and hidden extras. The grammars have such rules:
//
//	hack      parameters: the field `name` is inherited through a hidden node
//	lua       chunk: the field `local_declaration` passes through hidden repeats and comments (hidden extras)
//	scss      arguments: `value`
//	dockerfile env_instruction: `name`
//
//	CCORE_QUERIES=1 go test -run '^TestCcoreRouteQueryFieldConstraintsEqualC$'

import (
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func ccoreQCaps(t *testing.T, lang *g.Language, tree *g.Tree, q string) []string {
	query, err := g.NewQuery(q, lang)
	if err != nil {
		t.Fatalf("Go query %q: %v", q, err)
	}
	set := map[string]bool{}
	for _, m := range query.Execute(tree) {
		for _, cp := range m.Captures {
			set[fmt.Sprintf("%s:%d-%d", cp.Name, cp.Node.StartByte(), cp.Node.EndByte())] = true
		}
	}
	return ccoreKeys(set)
}

// ccoreQCapsC returns C's captures, and false when C's query compiler rejects the pattern
// (an impossible pattern: the grammar has no such field under that node).
func ccoreQCapsC(t *testing.T, cl *sitter.Language, tree *sitter.Tree, q string, src []byte) ([]string, bool) {
	cq, qerr := sitter.NewQuery(cl, q)
	if qerr != nil {
		return nil, false
	}
	defer cq.Close()
	cur := sitter.NewQueryCursor()
	defer cur.Close()
	matches := cur.Matches(cq, tree.RootNode(), src)
	names := cq.CaptureNames()
	set := map[string]bool{}
	for m := matches.Next(); m != nil; m = matches.Next() {
		for _, cp := range m.Captures {
			set[fmt.Sprintf("%s:%d-%d", names[cp.Index], cp.Node.StartByte(), cp.Node.EndByte())] = true
		}
	}
	return ccoreKeys(set), true
}

func ccoreKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func ccoreSameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestCcoreKnownDifferenceQueryMatcherLuaPositiveField fails by design while the query
// matcher differs from C on a positive field constraint through hidden structure. It
// runs only with CCORE_KNOWN_DIFFERENCES=1 and is not part of the equality script. It
// compares the present engine (no route) with C, so a pass means the matcher was fixed.
func TestCcoreKnownDifferenceQueryMatcherLuaPositiveField(t *testing.T) {
	if os.Getenv("CCORE_KNOWN_DIFFERENCES") == "" {
		t.Skip("known difference; set CCORE_KNOWN_DIFFERENCES=1 to see it fail")
	}
	const pat = "(chunk local_declaration: (_) @c)"
	lang := grammars.DetectLanguageByName("lua").Language()
	cLang, err := ParityCLanguage("lua")
	if err != nil {
		t.Fatal(err)
	}
	src, _ := ccoreRouteSource(t, "lua")
	cTree := compactT3ParseC(t, cLang, src)
	defer cTree.Close()
	p := g.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	defer tree.Release()
	present := ccoreQCaps(t, lang, tree, pat)
	want, ok := ccoreQCapsC(t, cLang, cTree, pat, src)
	if !ok {
		t.Fatal("C rejects the pattern")
	}
	if !ccoreSameStrings(present, want) {
		t.Fatalf("the present engine's matcher gives %d captures for %s on the lua sample; C gives %d", len(present), pat, len(want))
	}
}

// ccoreQueryCases are the patterns of the query test. Patterns in rejected are those that
// C's query compiler rejects (the grammar has no such field under that node); the test
// requires exactly these to be rejected, so that a skipped comparison cannot hide.
var ccoreQueryCases = []struct {
	lang     string
	patterns []string
	rejected map[string]bool
}{
	{"hack", []string{"(parameters name: (_) @c)", "(parameters !name) @c", "(parameters (variable) @c)", "(parameter name: (_) @c)", "(parameter !name) @c"},
		map[string]bool{"(parameters name: (_) @c)": true, "(parameters (variable) @c)": true}},
	{"lua", []string{"(chunk local_declaration: (_) @c)", "(chunk !local_declaration) @c", "(chunk (comment) @c)", "(variable_declaration name: (_) @c)", "(variable_declaration !name) @c"},
		map[string]bool{"(variable_declaration name: (_) @c)": true}},
	{"scss", []string{"(arguments value: (_) @c)", "(arguments !value) @c", "(arguments name: (_) @c)", "(arguments !name) @c"}, nil},
	{"dockerfile", []string{"(env_instruction name: (_) @c)", "(env_instruction !name) @c", "(env_pair name: (_) @c)", "(env_pair !name) @c"},
		map[string]bool{"(env_instruction name: (_) @c)": true}},
	{"haskell", []string{"(quantified_variables bind: (_) @c)", "(quantified_variables !bind) @c"}, nil},
	{"nushell", []string{"(pipe_element arg: (_) @c)", "(pipe_element !arg) @c"}, map[string]bool{"(pipe_element arg: (_) @c)": true}},
}

func TestCcoreRouteQueryFieldConstraintsEqualC(t *testing.T) {
	if os.Getenv("CCORE_QUERIES") == "" {
		t.Skip("set CCORE_QUERIES")
	}
	defer ccoreRouteCertify(t)()
	// Run each grammar in its own process and suspend GC while C trees are in use.
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	total, equal, nonEmpty, rejectedN := 0, 0, 0, 0
	for _, c := range ccoreQueryCases {
		if only := os.Getenv("CCORE_QUERY_ONLY"); only != "" && only != c.lang {
			continue // a diagnostic filter; the required run sets none
		}
		lang := grammars.DetectLanguageByName(c.lang).Language()
		cLang, err := ParityCLanguage(c.lang)
		if err != nil {
			t.Fatal(err)
		}
		src, _ := ccoreRouteSource(t, c.lang)
		cTree := compactT3ParseC(t, cLang, src)
		p := g.NewParser(lang)
		p.SetAdmissionCandidateRoute(true)
		_, f0, _ := bind.RouteCounters()
		tree, err := p.Parse(src)
		if err != nil || tree == nil {
			t.Fatal(err)
		}
		_, f1, _ := bind.RouteCounters()
		ccoreExpect(t, f1 > f0, "%s: the core did not serve the parse", c.lang)
		pp := g.NewParser(lang) // the present engine, for the same input
		pp.SetAdmissionCandidateRoute(false)
		presentTree, err := pp.Parse(src)
		if err != nil || presentTree == nil {
			t.Fatal(err)
		}
		langCompared := 0
		for _, pat := range c.patterns {
			want, ok := ccoreQCapsC(t, cLang, cTree, pat, src)
			if !ok {
				fmt.Printf("CCQUERY lang=%s pattern=%q c_rejects_the_pattern=true\n", c.lang, pat)
				rejectedN++
				ccoreExpect(t, c.rejected[pat], "%s %s: C rejects a pattern that the test expects to compile", c.lang, pat)
				continue
			}
			ccoreExpect(t, !c.rejected[pat], "%s %s: C compiles a pattern that the test expects it to reject", c.lang, pat)
			total++
			langCompared++
			got := ccoreQCaps(t, lang, tree, pat)
			present := ccoreQCaps(t, lang, presentTree, pat)
			same := ccoreSameStrings(got, want)
			if same {
				equal++
			}
			if len(want) > 0 {
				nonEmpty++
			}
			fmt.Printf("CCQUERY lang=%s pattern=%q route_captures=%d present_captures=%d c_captures=%d equal=%t\n", c.lang, pat, len(got), len(present), len(want), same)
			ccoreExpect(t, same, "%s %s: route %d captures, C %d (present engine %d)", c.lang, pat, len(got), len(want), len(present))
		}
		ccoreRequirePositive(t, "queries compared for "+c.lang, langCompared)
		presentTree.Release()
		tree.Release()
		cTree.Close()
	}
	only := os.Getenv("CCORE_QUERY_ONLY")
	fmt.Printf("CCQUERYTOTAL equal=%d of=%d with_captures=%d c_rejected=%d\n", equal, total, nonEmpty, rejectedN)
	ccoreRequirePositive(t, "queries", total)
	minWithCaptures := 3 // all grammars together
	if only != "" {
		minWithCaptures = 1 // one grammar
	}
	ccoreExpect(t, equal == total && nonEmpty >= minWithCaptures, "%d of %d patterns equal C (%d with captures)", equal, total, nonEmpty)
	if as := os.Getenv("CCORE_COUNT_AS"); as != "" {
		fmt.Printf("CCCOUNT test=%s set=queries checked=%d\n", as, total)
	}
}
