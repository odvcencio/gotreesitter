package gotreesitter_test

// Tests for the opt-in impossible-pattern structural validator
// (query_impossible_pattern.go / query_impossible_pattern_analysis.go):
// ValidateQueryPatterns and the WithStrictPatternValidation NewQuery option.
//
// The "historical dead pattern" test cases below are drawn from
// grammars/tags_query_infer.go's "Impossible-pattern fixes (2026-08-11,
// cgo_harness C-oracle exhaustive sweep)" comment: eleven-plus rows that
// compiled successfully in gotreesitter but matched zero nodes against real
// source, while tree-sitter's C ts_query_new rejects the identical pattern
// outright as an "Impossible pattern". Each row here was independently
// re-verified against this repository's own compiled grammar blobs while
// writing this test (isolated single-pattern NewQuery/Execute, 0 matches).

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// historicalDeadPattern is one (language, pattern) pair that tree-sitter's C
// query compiler rejects as structurally impossible and that gotreesitter's
// pre-fix tags tables shipped anyway.
type historicalDeadPattern struct {
	lang    string
	pattern string
	// parent/child are the node type names the pattern asserts a
	// (parent (child)) relationship between; used to sanity-check the
	// reported PatternIssue fields, not just its presence.
	parent string
	child  string
}

var historicalDeadPatterns = []historicalDeadPattern{
	// javascript: every method_definition names through property_identifier,
	// computed_property_name, or private_property_identifier -- never a bare
	// identifier.
	{"javascript", "(method_definition (identifier) @name) @definition.method", "method_definition", "identifier"},

	// typescript, tsx: function/enum name through identifier only,
	// class/interface name through type_identifier only -- never the other
	// spelling, for any of the four.
	{"typescript", "(function_declaration (type_identifier) @name) @definition.function", "function_declaration", "type_identifier"},
	{"tsx", "(function_declaration (type_identifier) @name) @definition.function", "function_declaration", "type_identifier"},
	{"typescript", "(class_declaration (identifier) @name) @definition.class", "class_declaration", "identifier"},
	{"tsx", "(class_declaration (identifier) @name) @definition.class", "class_declaration", "identifier"},
	{"typescript", "(interface_declaration (identifier) @name) @definition.interface", "interface_declaration", "identifier"},
	{"tsx", "(interface_declaration (identifier) @name) @definition.interface", "interface_declaration", "identifier"},
	{"typescript", "(enum_declaration (type_identifier) @name) @definition.type", "enum_declaration", "type_identifier"},
	{"tsx", "(enum_declaration (type_identifier) @name) @definition.type", "enum_declaration", "type_identifier"},

	// rust: field_identifier for a method-call reference like "obj.method()"
	// is always nested one level inside field_expression, never a bare
	// direct child of call_expression.
	{"rust", "(call_expression (field_identifier) @name) @reference.call", "call_expression", "field_identifier"},

	// java: own-name child is (identifier) only for all three; type_identifier
	// appears solely inside type_parameters/superclass/super_interfaces/
	// extends/implements clauses, never as the declaration's own name.
	{"java", "(class_declaration (type_identifier) @name) @definition.class", "class_declaration", "type_identifier"},
	{"java", "(interface_declaration (type_identifier) @name) @definition.interface", "interface_declaration", "type_identifier"},
	{"java", "(enum_declaration (type_identifier) @name) @definition.type", "enum_declaration", "type_identifier"},

	// c: a function_definition's name is always wrapped one level inside
	// function_declarator, never a bare direct child.
	{"c", "(function_definition (field_identifier) @name) @definition.function", "function_definition", "field_identifier"},

	// cpp: a typedef's target is always (type_identifier) in this grammar
	// family, never plain identifier.
	{"cpp", "(type_definition (identifier) @name) @definition.type", "type_definition", "identifier"},
}

// historicalDeadPatternLanguage resolves a *gotreesitter.Language for a
// historicalDeadPattern's language name, skipping the test entry if the
// grammar cannot be loaded (defensive; every language above is built in).
func historicalDeadPatternLanguage(t *testing.T, name string) *gotreesitter.Language {
	t.Helper()
	entry := grammars.DetectLanguageByName(name)
	if entry == nil || entry.Language == nil {
		t.Fatalf("no registered grammar for %q", name)
	}
	lang := entry.Language()
	if lang == nil {
		t.Fatalf("grammar loader for %q returned nil", name)
	}
	return lang
}

// TestValidateQueryPatternsFlagsHistoricalDeadPatterns asserts every
// pattern tree-sitter's C compiler already rejects as structurally
// impossible is flagged by ValidateQueryPatterns.
func TestValidateQueryPatternsFlagsHistoricalDeadPatterns(t *testing.T) {
	for _, tc := range historicalDeadPatterns {
		t.Run(tc.lang+"/"+tc.child+"_under_"+tc.parent, func(t *testing.T) {
			lang := historicalDeadPatternLanguage(t, tc.lang)
			q, err := gotreesitter.NewQuery(tc.pattern, lang)
			if err != nil {
				t.Fatalf("NewQuery (default, non-strict) unexpectedly failed to compile %q: %v", tc.pattern, err)
			}

			issues := gotreesitter.ValidateQueryPatterns(lang, q)
			if len(issues) == 0 {
				t.Fatalf("ValidateQueryPatterns found no issues for %q (lang=%s); want an impossible-child report for %s under %s",
					tc.pattern, tc.lang, tc.child, tc.parent)
			}

			found := false
			for _, issue := range issues {
				if issue.Kind != gotreesitter.PatternIssueImpossibleChild {
					continue
				}
				if issue.ParentType == tc.parent && issue.ChildType == tc.child {
					found = true
					if issue.PatternIndex != 0 {
						t.Errorf("issue.PatternIndex = %d, want 0 (single-pattern query)", issue.PatternIndex)
					}
					if issue.String() == "" {
						t.Errorf("PatternIssue.String() returned empty string")
					}
				}
			}
			if !found {
				t.Errorf("ValidateQueryPatterns issues %+v did not report %s can never be a child of %s", issues, tc.child, tc.parent)
			}
		})
	}
}

// TestQueryStrictPatternValidationRejectsHistoricalDeadPatterns asserts that
// NewQueryWithOptions(source, lang, WithStrictPatternValidation()) fails to
// compile every historically dead pattern, mirroring tree-sitter C's
// ts_query_new atomic rejection of the whole query.
func TestQueryStrictPatternValidationRejectsHistoricalDeadPatterns(t *testing.T) {
	for _, tc := range historicalDeadPatterns {
		t.Run(tc.lang+"/"+tc.child+"_under_"+tc.parent, func(t *testing.T) {
			lang := historicalDeadPatternLanguage(t, tc.lang)
			q, err := gotreesitter.NewQueryWithOptions(tc.pattern, lang, gotreesitter.WithStrictPatternValidation())
			if err == nil {
				t.Fatalf("NewQueryWithOptions with WithStrictPatternValidation() unexpectedly compiled %q successfully (query=%v)", tc.pattern, q)
			}
			if q != nil {
				t.Errorf("NewQueryWithOptions returned a non-nil *Query alongside an error: %v", q)
			}
			if !strings.Contains(err.Error(), tc.child) || !strings.Contains(err.Error(), tc.parent) {
				t.Errorf("error %q does not mention parent %q / child %q", err.Error(), tc.parent, tc.child)
			}
		})
	}
}

// currentCleanPattern is one of the exact single patterns that replaced a
// historicalDeadPatterns row in grammars/tags_query_infer.go's override
// tables -- i.e. what actually ships today for that exact relationship.
type currentCleanPattern struct {
	lang    string
	pattern string
}

var currentCleanPatterns = []currentCleanPattern{
	{"javascript", "(method_definition (property_identifier) @name) @definition.method"},
	{"typescript", "(function_declaration (identifier) @name) @definition.function"},
	{"tsx", "(function_declaration (identifier) @name) @definition.function"},
	{"typescript", "(class_declaration (type_identifier) @name) @definition.class"},
	{"tsx", "(class_declaration (type_identifier) @name) @definition.class"},
	{"typescript", "(interface_declaration (type_identifier) @name) @definition.interface"},
	{"tsx", "(interface_declaration (type_identifier) @name) @definition.interface"},
	{"typescript", "(enum_declaration (identifier) @name) @definition.type"},
	{"tsx", "(enum_declaration (identifier) @name) @definition.type"},
	{"rust", "(call_expression (field_expression (field_identifier) @name)) @reference.call"},
	{"java", "(class_declaration (identifier) @name) @definition.class"},
	{"java", "(interface_declaration (identifier) @name) @definition.interface"},
	{"java", "(enum_declaration (identifier) @name) @definition.type"},
	{"c", "(function_definition (function_declarator (field_identifier) @name)) @definition.function"},
	{"c", "(function_definition (function_declarator (identifier) @name)) @definition.function"},
	{"cpp", "(type_definition (type_identifier) @name) @definition.type"},
	{"cpp", "(function_definition (function_declarator (identifier) @name)) @definition.function"},
	// A handful of unrelated, ordinary patterns across other core languages,
	// so this table is not exclusively "patterns that replaced a dead one".
	{"go", "(function_declaration name: (identifier) @name) @definition.function"},
	{"go", "(method_declaration name: (field_identifier) @name) @definition.method"},
	{"python", "(function_definition (identifier) @name) @definition.function"},
	{"python", "(class_definition (identifier) @name) @definition.class"},
}

// TestValidateQueryPatternsPassesCurrentCleanPatterns asserts the exact
// patterns that replaced a historically dead one -- what ships today --
// report no issues, whether or not the offending sibling relationship
// (identifier vs type_identifier, field_identifier vs nested
// field_expression) exists elsewhere in the same grammar.
func TestValidateQueryPatternsPassesCurrentCleanPatterns(t *testing.T) {
	for _, tc := range currentCleanPatterns {
		t.Run(tc.lang+"/"+tc.pattern, func(t *testing.T) {
			lang := historicalDeadPatternLanguage(t, tc.lang)
			q, err := gotreesitter.NewQuery(tc.pattern, lang)
			if err != nil {
				t.Fatalf("NewQuery failed to compile %q: %v", tc.pattern, err)
			}
			issues := gotreesitter.ValidateQueryPatterns(lang, q)
			if len(issues) != 0 {
				t.Errorf("ValidateQueryPatterns(%q) = %+v, want no issues", tc.pattern, issues)
			}

			// WithStrictPatternValidation must accept the exact same source.
			if _, err := gotreesitter.NewQueryWithOptions(tc.pattern, lang, gotreesitter.WithStrictPatternValidation()); err != nil {
				t.Errorf("NewQueryWithOptions with WithStrictPatternValidation() rejected a clean pattern %q: %v", tc.pattern, err)
			}
		})
	}
}

// TestValidateQueryPatternsAuditedLanguagesPassClean resolves every
// historically-audited language's real, currently-shipping tags query (the
// same query ResolveTagsQuery hands to NewTagger/NewOutliner in production)
// and asserts ValidateQueryPatterns reports nothing for it. This is the
// broadest available regression guard against the analysis flagging a
// pattern that is actually fine, scoped to the languages
// grammars/tags_query_infer.go's "Impossible-pattern fixes" comment
// documents as already audited and confirmed to carry no dead pattern: the
// core nine from the CoreNine C-oracle differential run (go, python,
// javascript, typescript, tsx, rust, java, c, cpp) plus c_sharp and lua,
// checked by the same method separately per that comment.
//
// This deliberately does NOT sweep the full fleet: running the same check
// against every registered language finds real, previously-unaudited dead
// patterns in languages nobody has fixed yet (for example css and scss ship
// "(call_expression (identifier) @name)", but both grammars name a call's
// callee "function_name", not "identifier" -- confirmed against a clean,
// error-free real parse while writing this test). Finding those is this
// analysis working as intended, not a false positive, and fixing 24+
// languages' tags tables is a separate, larger effort outside this change's
// scope. See TestValidateQueryPatternsFleetSweepSmoke for a non-fatal report
// covering the rest of the fleet.
func TestValidateQueryPatternsAuditedLanguagesPassClean(t *testing.T) {
	audited := []string{
		"go", "python", "javascript", "typescript", "tsx",
		"rust", "java", "c", "cpp", "c_sharp", "lua",
	}
	for _, name := range audited {
		t.Run(name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(name)
			if entry == nil {
				t.Fatalf("no registered grammar for %q", name)
			}
			lang := entry.Language()
			if lang == nil {
				t.Fatalf("grammar loader for %q returned nil", name)
			}
			query := grammars.ResolveTagsQuery(*entry)
			if strings.TrimSpace(query) == "" {
				t.Fatalf("%q resolved to an empty tags query", name)
			}
			q, err := gotreesitter.NewQuery(query, lang)
			if err != nil {
				t.Fatalf("%q's shipping tags query failed to compile: %v", name, err)
			}
			issues := gotreesitter.ValidateQueryPatterns(lang, q)
			if len(issues) != 0 {
				t.Errorf("ValidateQueryPatterns flagged %d issue(s) in %s's shipping tags query, want zero: %+v",
					len(issues), name, issues)
			}
		})
	}
}

// TestValidateQueryPatternsFleetSweepSmoke runs ValidateQueryPatterns over
// every registered language's shipping tags query. It never fails on a
// reported PatternIssue -- most of the fleet has never been audited for this
// bug class, so genuine, unfixed dead patterns are expected here (see
// TestValidateQueryPatternsAuditedLanguagesPassClean) -- it only asserts the
// analysis completes without panicking or hanging for every language the
// registry knows about, and logs what it finds for visibility.
func TestValidateQueryPatternsFleetSweepSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping full-fleet smoke sweep in -short mode")
	}
	checked := 0
	flagged := 0
	for _, entry := range grammars.AllLanguages() {
		entry := entry
		if entry.Language == nil {
			continue
		}
		query := grammars.ResolveTagsQuery(entry)
		if strings.TrimSpace(query) == "" {
			continue
		}
		t.Run(entry.Name, func(t *testing.T) {
			lang := entry.Language()
			if lang == nil {
				t.Skip("language loader returned nil")
			}
			q, err := gotreesitter.NewQuery(query, lang)
			if err != nil {
				t.Skipf("tags query does not compile: %v", err)
			}
			issues := gotreesitter.ValidateQueryPatterns(lang, q)
			if len(issues) != 0 {
				t.Logf("ValidateQueryPatterns found %d issue(s) in %s's shipping tags query (informational, not a failure): %+v",
					len(issues), entry.Name, issues)
				flagged++
			}
		})
		checked++
	}
	t.Logf("fleet smoke sweep: checked %d languages, %d had at least one reported issue", checked, flagged)
	if checked < 30 {
		t.Fatalf("only checked %d languages with a non-empty tags query; expected the fleet sweep to cover far more (registry regression?)", checked)
	}
}

// TestNewQueryDefaultBehaviorUnchanged is the byte-identical-default-behavior
// gate for this change: NewQuery keeps its exact pre-existing signature (a
// compile-time check, not just a call-site one -- Go func-value assignment
// requires identical function types, so a variadic NewQuery would fail this
// even though every existing two-argument call site still compiles), and
// NewQuery(source, lang) must still compile a structurally-impossible
// pattern successfully and execute it as a normal (silently zero-match)
// query, exactly as before WithStrictPatternValidation existed.
func TestNewQueryDefaultBehaviorUnchanged(t *testing.T) {
	var _ func(string, *gotreesitter.Language) (*gotreesitter.Query, error) = gotreesitter.NewQuery

	lang := historicalDeadPatternLanguage(t, "javascript")
	const src = "class Widget {\n  render() {\n    return null\n  }\n}\n"
	const deadPattern = "(method_definition (identifier) @name) @definition.method"

	tree, err := gotreesitter.NewParser(lang).Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	q, err := gotreesitter.NewQuery(deadPattern, lang)
	if err != nil {
		t.Fatalf("NewQuery(source, lang) [2-arg form] must still compile a structurally impossible pattern by default, got error: %v", err)
	}
	matches := q.Execute(tree)
	if len(matches) != 0 {
		t.Fatalf("expected the historically dead pattern to keep matching nothing by default, got %d matches", len(matches))
	}

	// The same Query, hand to ValidateQueryPatterns explicitly, must still
	// find the issue -- confirming Execute's zero matches is really the
	// known dead-pattern bug and not an unrelated parse/query problem.
	issues := gotreesitter.ValidateQueryPatterns(lang, q)
	if len(issues) == 0 {
		t.Fatalf("ValidateQueryPatterns found no issues for the query that produced zero matches; test fixture may be stale")
	}
}

// TestValidateQueryPatternsNilSafety checks the exported entry points do not
// panic on nil inputs and fail closed (no issues, no crash).
func TestValidateQueryPatternsNilSafety(t *testing.T) {
	if issues := gotreesitter.ValidateQueryPatterns(nil, nil); issues != nil {
		t.Errorf("ValidateQueryPatterns(nil, nil) = %+v, want nil", issues)
	}
	lang := historicalDeadPatternLanguage(t, "go")
	if issues := gotreesitter.ValidateQueryPatterns(lang, nil); issues != nil {
		t.Errorf("ValidateQueryPatterns(lang, nil) = %+v, want nil", issues)
	}
	if issues := gotreesitter.ValidateQueryPatterns(nil, &gotreesitter.Query{}); issues != nil {
		t.Errorf("ValidateQueryPatterns(nil, query) = %+v, want nil", issues)
	}
}

// TestValidateQueryPatternsSkipsFieldConstrainedSteps documents and locks in
// the field-constraint scope limit from ValidateQueryPatterns' doc comment:
// a field-qualified child step is never flagged, even for a combination
// (Go's function_declaration "name" field can never hold a block) that a
// position-aware analysis with real field/child ground truth would catch.
func TestValidateQueryPatternsSkipsFieldConstrainedSteps(t *testing.T) {
	lang := historicalDeadPatternLanguage(t, "go")
	q, err := gotreesitter.NewQuery(`(function_declaration name: (block) @x)`, lang)
	if err != nil {
		t.Fatalf("query did not compile: %v", err)
	}
	if issues := gotreesitter.ValidateQueryPatterns(lang, q); len(issues) != 0 {
		t.Errorf("ValidateQueryPatterns flagged a field-constrained step, want it always skipped: %+v", issues)
	}
}
