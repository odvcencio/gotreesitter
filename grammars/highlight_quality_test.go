package grammars

import (
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// highlightColorFloor is the fewest distinct color captures that a composed
// highlight query must define.
const highlightColorFloor = 3

// highlightColorFloorExempt names the languages that stay below the floor, with
// the reason. The list may only shrink: TestHighlightQueryColorFloor fails for
// an entry that now meets the floor, so the entry must be deleted.
//
// Most of these ship the only query that their upstream repository has, or no
// query at all. The census found no MIT, Apache-2.0 or BSD replacement that
// compiles against the locked grammar. The helix queries for verilog and wat
// are MPL-2.0; copyleft sources stay out while copyleft grammars become opt-in.
var highlightColorFloorExempt = map[string]string{
	"cobol":             "upstream ships no highlight query; one-line stub",
	"jinja2":            "upstream ships no highlight query; one-line stub",
	"wolfram":           "upstream query is one line",
	"prolog":            "only candidate (helix) uses a field the locked grammar lacks",
	"crystal":           "only candidate (helix) uses a node the locked grammar lacks",
	"jq":                "only candidate (nvim) uses the node \"index\", which the locked grammar lacks",
	"verilog":           "only candidate (helix) is MPL-2.0",
	"wat":               "only candidate (helix) is MPL-2.0",
	"markdown_inline":   "the Markdown injection layer depends on this query; upgrade it with the fence tests",
	"eex":               "upstream and nvim queries are two patterns",
	"embedded_template": "upstream and nvim queries are two color captures",
}

// highlightColorCaptures counts the distinct capture names of a compiled query
// that color text. It mirrors the rule in highlight.go (isNonHighlightCapture)
// and also leaves out @none, which only resets earlier captures.
func highlightColorCaptures(q *gotreesitter.Query) int {
	colors := map[string]bool{}
	for _, name := range q.CaptureNames() {
		switch name {
		case "spell", "nospell", "conceal", "none",
			"local.scope", "local.reference", "local.definition", "local.definition-value":
			continue
		}
		if strings.HasPrefix(name, "_") || strings.HasPrefix(name, "local.definition.") {
			continue
		}
		colors[name] = true
	}
	return len(colors)
}

func TestHighlightQueryColorFloor(t *testing.T) {
	entries := AllLanguages()
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.Name] = true
		if strings.TrimSpace(entry.HighlightQuery) == "" {
			continue
		}
		q, err := gotreesitter.NewQuery(entry.HighlightQuery, entry.Language())
		UnloadEmbeddedLanguage(entry.Name + ".bin")
		if err != nil {
			t.Errorf("%s: highlight query does not compile: %v", entry.Name, err)
			continue
		}
		got := highlightColorCaptures(q)
		reason, exempt := highlightColorFloorExempt[entry.Name]
		switch {
		case exempt && got >= highlightColorFloor:
			t.Errorf("%s: has %d color captures (floor %d); remove it from highlightColorFloorExempt (%s)",
				entry.Name, got, highlightColorFloor, reason)
		case !exempt && got < highlightColorFloor:
			t.Errorf("%s: has %d color captures, want at least %d", entry.Name, got, highlightColorFloor)
		}
	}
	for name := range highlightColorFloorExempt {
		if !seen[name] && languageEnabled(name) && len(entries) > 150 {
			t.Errorf("highlightColorFloorExempt names %q, which is not a registered language", name)
		}
	}
}

// TestHighlightInheritanceEdgesResolve fails when a "; inherits:" directive
// names a language that exists but whose query did not end up ahead of the
// child's own text. Angular once looked inherited and was not.
func TestHighlightInheritanceEdgesResolve(t *testing.T) {
	for _, entry := range AllLanguages() {
		for _, parent := range highlightInheritsFromQuery(entry.rawHighlightQuery) {
			parentEntry := DetectLanguageByName(parent)
			if parentEntry == nil || strings.TrimSpace(parentEntry.HighlightQuery) == "" {
				continue
			}
			want := dropQueryPatterns(parentEntry.HighlightQuery, highlightInheritDropNodes[entry.Name])
			if !strings.Contains(entry.HighlightQuery, want) {
				t.Errorf("%s inherits %s, but the composed query lacks the %s query", entry.Name, parent, parent)
			}
		}
	}
}

// TestHighlightCompositionFixes pins the three defects of the old name table:
// angular never got its html parent, cuda was blocked from its cpp parent, and
// glsl composed the C query a second time on top of its embedded copy.
func TestHighlightCompositionFixes(t *testing.T) {
	get := func(name string) *LangEntry {
		entry := DetectLanguageByName(name)
		if entry == nil {
			t.Skipf("%s is not registered in this build", name)
		}
		return entry
	}
	if html, angular := get("html"), get("angular"); !strings.HasPrefix(angular.HighlightQuery, html.HighlightQuery) {
		t.Error("angular must compose the html query ahead of its own")
	}
	cpp, cuda := get("cpp"), get("cuda")
	if strings.Contains(cuda.HighlightQuery, "module_name") || strings.Contains(cuda.HighlightQuery, `"import"`) {
		t.Error("cuda must drop the C++ module syntax its grammar lacks")
	}
	if want := dropQueryPatterns(cpp.HighlightQuery, highlightInheritDropNodes["cuda"]); !strings.HasPrefix(cuda.HighlightQuery, want) {
		t.Error("cuda must compose the rest of the cpp query ahead of its own")
	}
	if glsl := get("glsl"); glsl.HighlightQuery != glsl.rawHighlightQuery {
		t.Error("glsl already embeds the C query; composing C again duplicates every pattern")
	}
}

func TestHighlightInheritsFromQuery(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"first line", "; inherits: html\n\n(a) @b", "html"},
		{"leading blank lines", "\n\n; inherits: c\n(a) @b", "c"},
		{"list with optional parent", ";; inherits: (html_tags),ecma\n", "html_tags,ecma"},
		{"spaces in list", "; inherits: c, cpp\n", "c,cpp"},
		{"mid-file is ignored", "(a) @b\n; inherits: c\n", ""},
		{"other comment first", "; header\n; inherits: c\n", ""},
		{"no directive", "(a) @b\n", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		got := strings.Join(highlightInheritsFromQuery(tc.query), ",")
		if got != tc.want {
			t.Errorf("%s: parents = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDropQueryPatterns(t *testing.T) {
	query := strings.Join([]string{
		"; keep this comment",
		"(identifier) @variable",
		"(module_name",
		"  (identifier) @module)",
		"",
		`"module_name_keyword" @keyword`,
		"[",
		`  "a" ; ) in a comment`,
		`  (module_name) ; trailing`,
		"] @drop",
		"(call function: (identifier) @function)",
		`((identifier) @constant (#match? @constant "^[A-Z)]"))`,
		"(type) @type",
		"[",
		`  "import"`,
		`  "keep"`,
		"] @keyword",
	}, "\n")
	got := dropQueryPatterns(query, []string{"module_name", `"import"`})
	for _, want := range []string{"; keep this comment", "(identifier) @variable", `"module_name_keyword" @keyword`,
		"(call function: (identifier) @function)", `"^[A-Z)]"`, "(type) @type", `"keep"`} {
		if !strings.Contains(got, want) {
			t.Errorf("dropQueryPatterns removed %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"@module", "@drop", `"import"`} {
		if strings.Contains(got, gone) {
			t.Errorf("dropQueryPatterns kept %q:\n%s", gone, got)
		}
	}
	if same := dropQueryPatterns(query, nil); same != query {
		t.Error("dropQueryPatterns with no nodes must return the query unchanged")
	}
}

// TestHighlightQueryFilesAreRegistered keeps embedded query files from going
// stale: each file must name a registered language, unless the build leaves
// that language out (grammar_subset, gotreesitter_no_copyleft).
func TestHighlightQueryFilesAreRegistered(t *testing.T) {
	files, err := highlightQueryFiles.ReadDir("highlight_queries")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(f.Name(), ".scm"))
	}
	sort.Strings(names)
	if len(AllLanguages()) < 150 {
		t.Skip("grammar_subset build")
	}
	for _, name := range names {
		if !languageEnabled(name) {
			continue // for example jq under gotreesitter_no_copyleft
		}
		entry := DetectLanguageByName(name)
		if entry == nil {
			t.Errorf("highlight_queries/%s.scm names a language that is not registered", name)
			continue
		}
		data, _ := highlightQueryFiles.ReadFile("highlight_queries/" + name + ".scm")
		if !strings.Contains(entry.rawHighlightQuery, strings.TrimSpace(string(data))) {
			t.Errorf("%s: registered raw query is not the embedded file", name)
		}
	}
}
