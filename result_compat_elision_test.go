package gotreesitter

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"testing"
)

// TestResultCompatibilityElisionSetMatchesRegistry independently re-reads
// testdata/result_compat_ownership_v1.json (compat_ownership_test.go already
// proves this file matches runLanguageResultCompatibility's switch and
// isCobolLanguage's predicate byte-for-byte) and computes the ineligible
// language set by the schema's plain reading, then checks
// resultCompatibilityElisionEligible against every language name the
// registry mentions. This is a second, independent computation from the
// production parser in result_compat_elision.go: it guards against a bug in
// THIS package's own JSON consumption, not against registry/switch drift
// (compat_ownership_test.go's job).
func TestResultCompatibilityElisionSetMatchesRegistry(t *testing.T) {
	raw, err := os.ReadFile("testdata/result_compat_ownership_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var registry resultCompatElisionRegistry
	if err := json.Unmarshal(raw, &registry); err != nil {
		t.Fatal(err)
	}

	wantIneligible := map[string]bool{}
	blockedAll := false
	allLanguages := map[string]bool{}
	for _, entry := range registry.Entries {
		for _, lang := range entry.Languages {
			allLanguages[lang] = true
		}
		if entry.Status != "live" {
			continue
		}
		switch entry.Kind {
		case "dispatcher_arm", "dispatcher_predicate", "second_pass_fixpoint":
			for _, lang := range entry.Languages {
				wantIneligible[lang] = true
			}
		case "generic_pass":
			for _, lang := range entry.Languages {
				if lang == "*" {
					blockedAll = true
					continue
				}
				wantIneligible[lang] = true
			}
		}
	}
	if blockedAll {
		t.Fatal("registry currently has a live generic_pass naming \"*\" -- update this test's expectations before landing that change")
	}
	if len(allLanguages) == 0 {
		t.Fatal("registry produced no language names to check")
	}

	for lang := range allLanguages {
		lang := lang
		t.Run(lang, func(t *testing.T) {
			got := resultCompatibilityElisionEligible(&Language{Name: lang})
			want := !wantIneligible[lang]
			if got != want {
				t.Fatalf("resultCompatibilityElisionEligible(%q) = %t, want %t (registry ineligible=%t)", lang, got, want, wantIneligible[lang])
			}
		})
	}
}

// TestResultCompatibilityElisionEligibleSpotChecks pins a handful of
// human-legible cases so a reader can see the eligibility rule's shape
// without cross-referencing the registry: languages with a live dispatcher
// arm (or the cobol predicate) are ineligible; languages with only retired
// arms, or that never had an arm at all, are eligible.
func TestResultCompatibilityElisionEligibleSpotChecks(t *testing.T) {
	cases := []struct {
		name     string
		eligible bool
	}{
		// Live dispatcher arms (parser_result_compat.go's switch).
		{"go", false},
		{"python", false},
		{"rust", false},
		{"javascript", false},
		{"typescript", false},
		{"tsx", false},
		{"c", false},
		{"cpp", false},
		{"yaml", false},
		// The cobol predicate (isCobolLanguage), not a switch case at all.
		{"cobol", false},
		{"COBOL", false},
		// Retired dispatcher arms: an arm existed and was fully retired.
		{"html", true},
		{"linkerscript", true},
		{"erlang", true},
		{"ruby", true},
		{"ocaml", true},
		{"zig", true},
		{"d", true},
		// Never had a dispatcher arm at all.
		{"java", true},
		{"css", true},
		{"json", true},
		// Unknown language name: eligible by construction (no registry entry
		// can name it), matching "no arm exists" for a hypothetical grammar.
		{"not-a-real-language", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resultCompatibilityElisionEligible(&Language{Name: c.name}); got != c.eligible {
				t.Fatalf("resultCompatibilityElisionEligible(%q) = %t, want %t", c.name, got, c.eligible)
			}
		})
	}
}

func TestResultCompatibilityElisionEligibleNilLanguage(t *testing.T) {
	if resultCompatibilityElisionEligible(nil) {
		t.Fatal("resultCompatibilityElisionEligible(nil) = true, want false")
	}
}

// TestResultCompatibilityElisionForceDisabledForTest proves the
// differential-test-only kill switch actually forces
// resultCompatibilityElisionActive off for an otherwise-eligible language,
// and that the restore function it returns puts eligibility back.
func TestResultCompatibilityElisionForceDisabledForTest(t *testing.T) {
	lang := &Language{Name: "html"}
	if !resultCompatibilityElisionActive(lang) {
		t.Fatal("precondition: html must be elision-active before forcing it off")
	}
	restore := SetResultCompatibilityElisionForceDisabledForTest(true)
	if resultCompatibilityElisionActive(lang) {
		t.Fatal("resultCompatibilityElisionActive stayed true after forcing elision off")
	}
	restore()
	if !resultCompatibilityElisionActive(lang) {
		t.Fatal("resultCompatibilityElisionActive did not restore after the toggle's restore func ran")
	}
}

// TestResultCompatibilityElisionExcludesDeferredLanguages ratchets a
// precondition finalizeCompactReturnedTreeForParse's own deferred-branch
// reasoning depends on (admission_switch_candidate.go): no elision-eligible
// language may ever appear in shouldDeferResultCompatibility's deferred set
// (parser_result_root_build.go). finalizeCompactReturnedTreeForParse DOES
// still check tree.hasDeferredResultCompatibility() -- it is not assumed
// unreachable -- but the doc comment's claim that no eligible language can
// ever set it is a claim about today's deferred set (currently
// typescript/tsx, both ineligible), not a structural guarantee. This test
// parses shouldDeferResultCompatibility's own switch with go/ast (the same
// technique compat_ownership_test.go's dispatcher-arm ratchet uses) so a
// future language added to that deferred set while also being elision-
// eligible fails the build here instead of silently changing this route's
// behavior for that language.
func TestResultCompatibilityElisionExcludesDeferredLanguages(t *testing.T) {
	deferred := deferredResultCompatibilityLanguagesForTest(t)
	if len(deferred) == 0 {
		t.Fatal("shouldDeferResultCompatibility's switch named zero languages; update this ratchet's expectations")
	}
	for _, lang := range deferred {
		if resultCompatibilityElisionEligible(&Language{Name: lang}) {
			t.Errorf(
				"%q is both elision-eligible and named in shouldDeferResultCompatibility's deferred set; "+
					"finalizeCompactReturnedTreeForParse's doc comment (admission_switch_candidate.go) claims "+
					"this never happens for an eligible language",
				lang,
			)
		}
	}
}

// deferredResultCompatibilityLanguagesForTest parses
// shouldDeferResultCompatibility (parser_result_root_build.go) with go/ast
// and returns every language name literal in a case clause that returns
// true. It does not assume the current two-language set; it reads whatever
// the switch says today.
func deferredResultCompatibilityLanguagesForTest(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "parser_result_root_build.go", nil, 0)
	if err != nil {
		t.Fatalf("parse parser_result_root_build.go: %v", err)
	}
	var fn *ast.FuncDecl
	for _, decl := range file.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == "shouldDeferResultCompatibility" {
			fn = f
			break
		}
	}
	if fn == nil {
		t.Fatal("shouldDeferResultCompatibility not found in parser_result_root_build.go")
	}
	var sw *ast.SwitchStmt
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if sw != nil {
			return false
		}
		if s, ok := node.(*ast.SwitchStmt); ok {
			sw = s
			return false
		}
		return true
	})
	if sw == nil {
		t.Fatal("shouldDeferResultCompatibility has no switch statement")
	}
	var names []string
	for _, stmt := range sw.Body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok || len(clause.List) == 0 {
			continue // skips the default clause, which has an empty List
		}
		returnsTrue := false
		for _, bodyStmt := range clause.Body {
			ret, ok := bodyStmt.(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				continue
			}
			if ident, ok := ret.Results[0].(*ast.Ident); ok && ident.Name == "true" {
				returnsTrue = true
			}
		}
		if !returnsTrue {
			continue
		}
		for _, expr := range clause.List {
			lit, ok := expr.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Fatalf("shouldDeferResultCompatibility case has non-string expression %T", expr)
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatal(err)
			}
			names = append(names, value)
		}
	}
	return names
}
