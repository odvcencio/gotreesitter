package gotreesitter

import (
	_ "embed"
	"encoding/json"
	"sync"
	"sync/atomic"
)

// This file computes compat-tail elision eligibility (spec.campaign.v7
// tranche A5/C1) for the compact fresh path (admission_switch_candidate.go's
// tryCompactFullParseRoute). See the compat-tail elision record (Hyphae space m31labs/gotreesitter) for the full
// eligibility mechanism and the equivalence proof for each elided step.

// resultCompatOwnershipRegistryForElision embeds the exact registry file
// TestResultCompatibilityOwnershipRegistry (compat_ownership_test.go) proves,
// via go/ast, matches runLanguageResultCompatibility's dispatcher switch and
// isCobolLanguage's predicate byte-for-byte: every switch case's language set
// and function set must equal some "live" dispatcher_arm/dispatcher_predicate
// registry entry, and every "live" entry must name an existing switch case.
// Deriving eligibility from this file -- rather than a second, hand-
// maintained language list -- means a future dispatcher arm cannot silently
// escape elision: adding a live switch case without updating this registry
// entry fails that ratchet test, so shipping the two out of sync is not
// possible; updating both keeps this computed set correct automatically.
//
//go:embed testdata/result_compat_ownership_v1.json
var resultCompatOwnershipRegistryForElision []byte

// resultCompatElisionRegistryEntry decodes only the fields the eligibility
// computation needs from testdata/result_compat_ownership_v1.json. It is
// deliberately narrower than compat_ownership_test.go's full
// resultCompatOwnershipEntry: this reads the shipped registry at runtime, so
// it must tolerate the file's full schema without decoding (or requiring)
// every field the test-time ratchet also checks.
type resultCompatElisionRegistryEntry struct {
	Kind      string   `json:"kind"`
	Languages []string `json:"languages"`
	Status    string   `json:"status"`
}

type resultCompatElisionRegistry struct {
	Entries []resultCompatElisionRegistryEntry `json:"entries"`
}

var (
	resultCompatibilityElisionOnce       sync.Once
	resultCompatibilityElisionIneligible map[string]struct{}
	// resultCompatibilityElisionBlockedAll is set when the registry cannot be
	// parsed (fail closed: elision is off for every language) or when it
	// records a live generic_pass entry naming "*" (a pass that runs for
	// every language regardless of dispatcher arm -- none exist today, but a
	// future one must disable elision everywhere, not just for the
	// languages it happens to list explicitly).
	resultCompatibilityElisionBlockedAll bool
)

// computeResultCompatibilityElisionSet parses the embedded ownership
// registry exactly once and records, per language name, whether a live
// dispatcher arm, dispatcher predicate, or generic pass covers it.
func computeResultCompatibilityElisionSet() {
	resultCompatibilityElisionOnce.Do(func() {
		resultCompatibilityElisionIneligible = make(map[string]struct{}, 64)
		var registry resultCompatElisionRegistry
		if err := json.Unmarshal(resultCompatOwnershipRegistryForElision, &registry); err != nil {
			// The embedded file is a committed, test-verified artifact, so this
			// should never happen outside a corrupted build. Fail closed rather
			// than silently eliding nothing is safe (it just costs the recovered
			// CPU this tranche targets) but silently eliding everything on a
			// parse failure would not be, so fail closed all the way: treat every
			// language as ineligible.
			resultCompatibilityElisionBlockedAll = true
			return
		}
		for _, entry := range registry.Entries {
			if entry.Status != "live" {
				continue
			}
			switch entry.Kind {
			case "dispatcher_arm", "dispatcher_predicate", "second_pass_fixpoint":
				for _, lang := range entry.Languages {
					resultCompatibilityElisionIneligible[lang] = struct{}{}
				}
			case "generic_pass":
				for _, lang := range entry.Languages {
					if lang == "*" {
						resultCompatibilityElisionBlockedAll = true
						continue
					}
					resultCompatibilityElisionIneligible[lang] = struct{}{}
				}
			}
		}
	})
}

// resultCompatibilityElisionEligible reports whether lang's compact
// fresh-path parse may elide the compat-tail's three no-op steps (see
// tryCompactFullParseRoute, admission_switch_candidate.go): true only when
// the registry records no live dispatcher arm, dispatcher predicate, or
// generic pass naming it. This is a computed set, not a maintained list: see
// computeResultCompatibilityElisionSet and the compat-tail elision record (Hyphae space m31labs/gotreesitter).
func resultCompatibilityElisionEligible(lang *Language) bool {
	if lang == nil {
		return false
	}
	computeResultCompatibilityElisionSet()
	if resultCompatibilityElisionBlockedAll {
		return false
	}
	_, ineligible := resultCompatibilityElisionIneligible[lang.Name]
	return !ineligible
}

// resultCompatibilityElisionForceDisabledForTest lets the elision-equivalence
// regression tests force every language ineligible, so they can drive the
// SAME source through the SAME compact route with elision on and off and
// diff the two resulting trees. Never set outside a test: the setter,
// SetResultCompatibilityElisionForceDisabledForTest, lives in export_test.go,
// which this package's shipped build never compiles.
var resultCompatibilityElisionForceDisabledForTest atomic.Bool

// resultCompatibilityElisionActive is the single call site
// tryCompactFullParseRoute consults. It is registry eligibility gated by the
// test-only kill switch above.
func resultCompatibilityElisionActive(lang *Language) bool {
	if resultCompatibilityElisionForceDisabledForTest.Load() {
		return false
	}
	return resultCompatibilityElisionEligible(lang)
}
