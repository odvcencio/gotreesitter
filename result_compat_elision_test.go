package gotreesitter

import (
	"encoding/json"
	"os"
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
