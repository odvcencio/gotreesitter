package query_test

// Focused C-parity regression tests for the tree-sitter query-engine
// parity fixes landed on this branch:
//
//   - D8: #is?/#is-not? are inert property predicates and must not filter
//     matches.
//   - D4: (MISSING) / (MISSING kind) query patterns test Node.IsMissing(),
//     instead of matching every node like an unqualified wildcard.
// Each test's expected result was cross-checked against the real C
// tree-sitter runtime (github.com/tree-sitter/go-tree-sitter) via the
// parity-audit probe harness; the numbers baked in here are the C behavior,
// not a description of the old (buggy) Go behavior.

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func parseWithLanguage(t *testing.T, lang *gotreesitter.Language, src []byte) *gotreesitter.Tree {
	t.Helper()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Parse returned nil tree/root")
	}
	return tree
}

// --- D8: #is?/#is-not? must not filter matches -----------------------------

func TestParityD8IsPredicateDoesNotFilterMatches(t *testing.T) {
	lang := grammars.GoLanguage()
	src := []byte("package main\nvar count = 1\n")
	tree := parseWithLanguage(t, lang, src)
	defer tree.Release()

	for _, q := range []string{
		`((identifier) @id (#is? local))`,
		`((identifier) @id (#is-not? local))`,
		`((identifier) @id (#is? "local"))`,
	} {
		query, err := gotreesitter.NewQuery(q, lang)
		if err != nil {
			t.Fatalf("NewQuery(%q): %v", q, err)
		}
		matches := query.Execute(tree)
		if len(matches) == 0 {
			t.Errorf("query %q: got 0 matches, want at least 1 (C treats #is?/#is-not? as inert metadata, never filtering)", q)
			continue
		}
		var sawCount bool
		for _, m := range matches {
			for _, c := range m.Captures {
				if c.Text(src) == "count" {
					sawCount = true
				}
			}
		}
		if !sawCount {
			t.Errorf("query %q: expected the \"count\" identifier to be captured (C does not drop it)", q)
		}
	}
}

func TestParityD8PropertyPredicateMetadataIsPublic(t *testing.T) {
	lang := grammars.GoLanguage()
	query, err := gotreesitter.NewQuery(`
		((identifier) @id (#is? @id local) (#is-not? local.definition @id))
	`, lang)
	if err != nil {
		t.Fatalf("NewQuery: %v", err)
	}

	predicates, ok := query.PredicatesForPattern(0)
	if !ok || len(predicates) != 2 {
		t.Fatalf("PredicatesForPattern = %d, %v; want 2, true", len(predicates), ok)
	}
	first, ok := predicates[0].PropertyPredicate()
	if !ok || first.Property != "local" || first.Capture != "id" || !first.Positive {
		t.Fatalf("first property predicate = %#v, %v", first, ok)
	}
	second, ok := predicates[1].PropertyPredicate()
	if !ok || second.Property != "local.definition" || second.Capture != "id" || second.Positive {
		t.Fatalf("second property predicate = %#v, %v", second, ok)
	}

	properties, ok := query.PropertyPredicatesForPattern(0)
	if !ok || len(properties) != 2 || properties[0] != first || properties[1] != second {
		t.Fatalf("PropertyPredicatesForPattern = %#v, %v", properties, ok)
	}
}

// --- D4: (MISSING) / (MISSING kind) -----------------------------------------

func TestParityD4MissingPatternMatchesOnlyMissingNode(t *testing.T) {
	lang := grammars.CLanguage()
	src := []byte("int a\n")
	tree := parseWithLanguage(t, lang, src)
	defer tree.Release()

	bare, err := gotreesitter.NewQuery(`(MISSING) @m`, lang)
	if err != nil {
		t.Fatalf("NewQuery((MISSING)): %v", err)
	}
	matches := bare.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf("(MISSING) @m: got %d matches, want 1 (C matches only the missing ';')", len(matches))
	}
	if got := len(matches[0].Captures); got != 1 {
		t.Fatalf("(MISSING) @m: got %d captures, want 1", got)
	}
	mc := matches[0].Captures[0]
	if !mc.Node.IsMissing() {
		t.Fatalf("(MISSING) @m: matched node is not IsMissing()")
	}
	if mc.Node.StartByte() != mc.Node.EndByte() {
		t.Fatalf("(MISSING) @m: matched node is not zero-width (start=%d end=%d)", mc.Node.StartByte(), mc.Node.EndByte())
	}

	kinded, err := gotreesitter.NewQuery(`(MISSING ";") @m`, lang)
	if err != nil {
		t.Fatalf(`NewQuery((MISSING ";")): %v`, err)
	}
	matches = kinded.Execute(tree)
	if len(matches) != 1 {
		t.Fatalf(`(MISSING ";") @m: got %d matches, want 1`, len(matches))
	}
	if got := len(matches[0].Captures); got != 1 || !matches[0].Captures[0].Node.IsMissing() {
		t.Fatalf(`(MISSING ";") @m: expected exactly one missing-node capture`)
	}

	// A pattern that treats MISSING as an unqualified wildcard (the old,
	// buggy Go behavior) would match every node in the tree, not just the
	// missing token -- guard against regressing back to that.
	allNodes, err := gotreesitter.NewQuery(`(_) @all`, lang)
	if err != nil {
		t.Fatalf("NewQuery((_)): %v", err)
	}
	if got := len(allNodes.Execute(tree)); got <= 1 {
		t.Fatalf("sanity check failed: (_) @all matched %d nodes, expected the tree to contain more than just the missing node", got)
	}

	// (MISSING (child)) must be rejected at compile time (TSQueryErrorSyntax
	// upstream): MISSING only accepts a bare form, a kind identifier, or a
	// string-literal token qualifier -- never a nested child pattern.
	if _, err := gotreesitter.NewQuery(`(MISSING (identifier))`, lang); err == nil {
		t.Fatal("(MISSING (identifier)): expected a compile error, got nil")
	}
}

func TestParityD4MissingAlternationPreservesIdentity(t *testing.T) {
	lang := grammars.CLanguage()
	src := []byte("int a;\nint b\n")
	tree := parseWithLanguage(t, lang, src)
	defer tree.Release()

	for _, querySource := range []string{
		`[(MISSING) (identifier)] @match`,
		`[(MISSING ";") (identifier)] @match`,
	} {
		t.Run(querySource, func(t *testing.T) {
			query, err := gotreesitter.NewQuery(querySource, lang)
			if err != nil {
				t.Fatalf("NewQuery(%q): %v", querySource, err)
			}
			matches := query.Execute(tree)
			if len(matches) != 3 {
				t.Fatalf("%s: got %d matches, want the two identifiers and one missing semicolon", querySource, len(matches))
			}

			var missing int
			identifiers := map[string]int{}
			for _, match := range matches {
				if len(match.Captures) != 1 {
					t.Fatalf("%s: got %d captures in match, want 1", querySource, len(match.Captures))
				}
				capture := match.Captures[0]
				if capture.Node.IsMissing() {
					missing++
					if capture.Node.Type(lang) != ";" {
						t.Fatalf("%s: missing capture type = %q, want semicolon", querySource, capture.Node.Type(lang))
					}
					continue
				}
				if capture.Node.Type(lang) != "identifier" {
					t.Fatalf("%s: non-missing capture type = %q, want identifier", querySource, capture.Node.Type(lang))
				}
				identifiers[capture.Text(src)]++
			}
			if missing != 1 || identifiers["a"] != 1 || identifiers["b"] != 1 || len(identifiers) != 2 {
				t.Fatalf("%s: missing=%d identifiers=%v, want missing=1 identifiers={a:1 b:1}", querySource, missing, identifiers)
			}
		})
	}
}

func TestParityD4QualifiedMissingNamedAlternationPreservesIdentity(t *testing.T) {
	lang := grammars.TypescriptLanguage()
	src := []byte("const { value: [dirPath, { dirName, options, fileNames }] } = result;\nswitch (x) { case: }\n")
	tree := parseWithLanguage(t, lang, src)
	defer tree.Release()

	querySource := `[(MISSING identifier) (switch_case)] @match`
	query, err := gotreesitter.NewQuery(querySource, lang)
	if err != nil {
		t.Fatalf("NewQuery(%q): %v", querySource, err)
	}
	matches := query.Execute(tree)
	if len(matches) != 2 {
		t.Fatalf("%s: got %d matches, want one missing identifier and one switch_case", querySource, len(matches))
	}

	var missingIdentifier, switchCase int
	for _, match := range matches {
		if len(match.Captures) != 1 {
			t.Fatalf("%s: got %d captures in match, want 1", querySource, len(match.Captures))
		}
		capture := match.Captures[0]
		switch {
		case capture.Node.IsMissing() && capture.Node.Type(lang) == "identifier":
			missingIdentifier++
		case !capture.Node.IsMissing() && capture.Node.Type(lang) == "switch_case":
			switchCase++
		default:
			t.Fatalf("%s: unexpected capture type=%q missing=%v", querySource, capture.Node.Type(lang), capture.Node.IsMissing())
		}
	}
	if missingIdentifier != 1 || switchCase != 1 {
		t.Fatalf("%s: missing identifiers=%d switch cases=%d, want 1 each", querySource, missingIdentifier, switchCase)
	}
}
