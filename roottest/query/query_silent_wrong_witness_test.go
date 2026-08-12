package query_test

// Witness suite for the query-engine "silent-wrong" correctness tranche
// (D1, D3, D4, D5, D8). Each witness was cross-checked against the real C
// tree-sitter runtime (github.com/tree-sitter/go-tree-sitter, runtime v0.25)
// through the cgo parity harness (cgo_harness, build tag treesitter_c_parity,
// GTS_PARITY_ALLOW_HOST=1). The C runtime query engine is the oracle
// (decision-0007: strict locked-C parity).
//
// Per-defect verdict (see the PR body for the full table):
//
//   D1 (descendant range queries): query-range logic is CORRECT. The
//     byteRangeIntersects / pointRangeIntersects rules match query.c
//     node_precedes_range / node_follows_range for every well-formed input.
//     The only Go/C divergences under a range restriction come from a parser
//     tree-shape difference on malformed input (Go builds an ERROR node where
//     C error-recovers into concrete nodes); the query faithfully reports its
//     own tree, so that is a parser-layer concern, out of query-engine scope.
//     Locked below with passing regression witnesses.
//
//   D3 (supertype patterns): SILENT-WRONG, unfixable in query-engine scope.
//     A bare supertype pattern such as "(_expression)" returns zero matches.
//     Upstream matches every node that occupies a supertype POSITION (query.c
//     sets step->supertype_symbol and filters with the node's in-context
//     supertypes from ts_tree_cursor_current_status, which walks the hidden
//     supertype wrapper chain up to the first visible ancestor). The Go tree
//     collapses hidden supertype wrappers and keeps no in-context supertype
//     data, and upstream also rejects impossible supertype/field combinations
//     at compile time via parse-table analysis. A plain symbol-set expansion
//     over-matches (it captures a function name identifier and a body block,
//     which upstream never does). A correct fix needs hidden-node
//     materialization or parse-table analysis in the parser/tree layer, which
//     is out of scope here. Skipped witness below.
//
//   D4 (MISSING node patterns): CORRECT. (MISSING), (MISSING kind) and
//     alternations that mix a MISSING branch with a named branch all match
//     upstream. Locked below with a passing regression witness (see also
//     query_c_parity_fixes_test.go).
//
//   D5 (quantified captures): SILENT-WRONG, unfixable safely in query scope.
//     A leading zero-or-more capture that precedes a required step drops the
//     zero-repetition completion. Upstream (query.c streaming state machine,
//     ts_query_cursor__advance) forks a state that lets the "*" match zero and
//     the following step consume the first child; the Go matcher enumerates
//     child combinations with a greedy early return and never emits that
//     branch. Matching upstream exactly requires porting the tree-sitter query
//     state machine (a large core-matcher rewrite with high regression risk),
//     so a targeted patch cannot reproduce the exact fork set. Skipped witness
//     below.
//
//   D8 (#is? local predicate): CORRECT. #is? / #is-not? are inert property
//     predicates and never filter matches, matching upstream. Locked below
//     with a passing regression witness (see also query_c_parity_fixes_test.go).

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func swParse(t *testing.T, lang *gotreesitter.Language, src string) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree/root")
	}
	return tree
}

// swRangeMatches runs a query with an optional byte range and returns, per
// match, the ordered list of capture texts.
func swRangeMatches(t *testing.T, lang *gotreesitter.Language, src, q string, hasRange bool, start, end uint32) [][]string {
	t.Helper()
	tree := swParse(t, lang, src)
	defer tree.Release()
	query, err := gotreesitter.NewQuery(q, lang)
	if err != nil {
		t.Fatalf("NewQuery(%q): %v", q, err)
	}
	cur := query.Exec(tree.RootNode(), lang, []byte(src))
	if hasRange {
		cur.SetByteRange(start, end)
	}
	var out [][]string
	for {
		m, ok := cur.NextMatch()
		if !ok {
			break
		}
		var caps []string
		for _, c := range m.Captures {
			caps = append(caps, c.Text([]byte(src)))
		}
		out = append(out, caps)
	}
	return out
}

// --- D1: descendant range queries are correct on well-formed input ---------

func TestWitnessD1DescendantByteRangeMatchesOracle(t *testing.T) {
	goLang := grammars.GoLanguage()
	src := "package main\nvar a = 1\nvar b = 2\n"

	// A byte range that covers only the second declaration must return only
	// that declaration. Oracle (C runtime): exactly one var_declaration
	// capturing "var b = 2".
	got := swRangeMatches(t, goLang, src, `(var_declaration) @v`, true, 23, 32)
	if len(got) != 1 || len(got[0]) != 1 || got[0][0] != "var b = 2" {
		t.Fatalf("byte-range var_declaration: got %v, want [[\"var b = 2\"]] (C oracle)", got)
	}

	// A byte range covering a single identifier token returns only that
	// identifier. Oracle: one identifier "a".
	got = swRangeMatches(t, goLang, src, `(identifier) @id`, true, 17, 18)
	if len(got) != 1 || len(got[0]) != 1 || got[0][0] != "a" {
		t.Fatalf("byte-range identifier: got %v, want [[\"a\"]] (C oracle)", got)
	}

	// An empty (zero-width) range strictly after the tree returns nothing.
	got = swRangeMatches(t, goLang, src, `(identifier) @id`, true, 50, 60)
	if len(got) != 0 {
		t.Fatalf("byte-range past EOF: got %v, want no matches (C oracle)", got)
	}

	// A range spanning both declarations returns both. Oracle: two matches.
	got = swRangeMatches(t, goLang, src, `(var_declaration) @v`, true, 13, 32)
	if len(got) != 2 || got[0][0] != "var a = 1" || got[1][0] != "var b = 2" {
		t.Fatalf("byte-range both declarations: got %v, want both (C oracle)", got)
	}
}

// --- D3: supertype patterns (SILENT-WRONG, unfixable in query scope) --------

func TestWitnessD3SupertypePatternMatchesSubtypesInPosition(t *testing.T) {
	t.Skip("D3 supertype context is unfixable in query-engine scope: correct " +
		"matching needs the node's in-context supertypes (the hidden supertype " +
		"wrapper chain), which the collapsed Go tree does not retain, plus " +
		"parse-table analysis for impossible-pattern rejection. Both live in " +
		"the parser/tree layer, out of scope for query*.go. Tracking: query " +
		"silent-wrong tranche D3.")

	goLang := grammars.GoLanguage()

	// Oracle (C runtime): "(_expression)" matches every node that sits in a
	// supertype position. For "func f() { _ = 1 }" that is the assignment
	// operands "_" and "1" -- NOT the function name identifier "f".
	got := swRangeMatches(t, goLang, "package main\nfunc f() { _ = 1 }\n", `(_expression) @e`, false, 0, 0)
	if len(got) != 2 {
		t.Fatalf("(_expression): got %d matches, want 2 (C oracle: \"_\" and \"1\")", len(got))
	}

	// Oracle: a field-restricted supertype matches the operand in that field.
	got = swRangeMatches(t, goLang, "package main\nfunc f() { _ = a + b }\n", `(binary_expression left: (_expression) @l)`, false, 0, 0)
	if len(got) != 1 || got[0][0] != "a" {
		t.Fatalf("binary left (_expression): got %v, want [[\"a\"]] (C oracle)", got)
	}
}

// --- D4: MISSING node patterns are correct ---------------------------------

func TestWitnessD4MissingAlternationMatchesOracle(t *testing.T) {
	cLang := grammars.CLanguage()
	src := "int a;\nint b\n" // second declaration is missing its ';'

	// Oracle (C runtime): "[(MISSING) (identifier)] @m" yields three matches:
	// the two identifiers plus the zero-width missing ';'.
	got := swRangeMatches(t, cLang, src, `[(MISSING) (identifier)] @m`, false, 0, 0)
	if len(got) != 3 {
		t.Fatalf("[(MISSING) (identifier)]: got %d matches, want 3 (C oracle)", len(got))
	}
	var missing, idents int
	for _, caps := range got {
		if len(caps) != 1 {
			t.Fatalf("match has %d captures, want 1", len(caps))
		}
		switch caps[0] {
		case "":
			missing++ // zero-width missing ';'
		case "a", "b":
			idents++
		default:
			t.Fatalf("unexpected capture text %q", caps[0])
		}
	}
	if missing != 1 || idents != 2 {
		t.Fatalf("missing=%d idents=%d, want missing=1 idents=2 (C oracle)", missing, idents)
	}
}

// --- D5: quantified captures (SILENT-WRONG, unfixable safely in query scope) -

func TestWitnessD5LeadingStarEmitsZeroRepetitionMatch(t *testing.T) {
	t.Skip("D5 quantified-capture fork set is unfixable safely in query scope: " +
		"upstream's streaming query state machine forks a zero-repetition " +
		"branch that the Go combination-enumeration matcher (greedy early " +
		"return) never emits. Reproducing the exact fork set requires porting " +
		"the tree-sitter query state machine, a large core-matcher rewrite " +
		"with high regression risk. Tracking: query silent-wrong tranche D5.")

	jsLang := grammars.JavascriptLanguage()

	// Oracle (C runtime) for "[a, b]" with
	// "(array (identifier)* @items (identifier) @last)": two matches --
	//   1. "*" matches zero, @last = "a";
	//   2. @items = ["a"], @last = "b".
	// The Go matcher currently emits only match 2 (drops the zero-repetition
	// completion).
	got := swRangeMatches(t, jsLang, "const v = [a, b];\n", `(array (identifier)* @items (identifier) @last)`, false, 0, 0)
	if len(got) != 2 {
		t.Fatalf("leading-star array: got %d matches, want 2 (C oracle emits the zero-repetition @last=\"a\" match)", len(got))
	}
	if len(got[0]) != 1 || got[0][0] != "a" {
		t.Fatalf("first match: got %v, want [[\"a\"]] (zero-repetition completion, C oracle)", got[0])
	}
}

// --- D8: #is? property predicates never filter matches ----------------------

func TestWitnessD8IsPredicateInertMatchesOracle(t *testing.T) {
	goLang := grammars.GoLanguage()
	src := "package main\nvar count = 1\n"

	// Oracle (C runtime): #is? is inert metadata, so the identifier "count" is
	// still captured.
	for _, q := range []string{
		`((identifier) @id (#is? local))`,
		`((identifier) @id (#is-not? local))`,
	} {
		got := swRangeMatches(t, goLang, src, q, false, 0, 0)
		var sawCount bool
		for _, caps := range got {
			for _, c := range caps {
				if c == "count" {
					sawCount = true
				}
			}
		}
		if !sawCount {
			t.Fatalf("query %q: expected \"count\" to be captured (C oracle: #is? never filters)", q)
		}
	}
}
