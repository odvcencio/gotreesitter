package gotreesitter

import (
	"os"
	"strings"
	"sync/atomic"

	"github.com/odvcencio/gotreesitter/internal/sched"
)

// Phase-3 dual-route admission switch.
//
// The switch selects, per full parse, between two engines:
//
//   - the production route: the mature GLR engine that Parser.Parse has always
//     used and that every reuse-consuming path (ParseIncremental and the
//     token-invariant leaf fast path) uses unconditionally; and
//   - the compact candidate route: the parsercorephase0 engine measured by
//     BenchmarkParserCoreFreshFullCanonical, which streams a compact derivation
//     into a materialized public tree.
//
// The switch only ever changes which engine serves a FRESH FULL parse. A
// compact result may later feed incremental reuse only when materialization
// attached a complete table-replay proof and scanner quiescence is proven;
// every other compact tree retains the hard fail-closed reuse bar. See
// admissionCandidateFullParseEligible and the materialization proof gate.
//
// Precedence, highest to lowest:
//
//  1. a per-Parser override set with (*Parser).SetAdmissionCandidateRoute;
//  2. an explicit process-wide setting or recognized environment value;
//  3. the per-language allowlist, which widens only the implicit OFF default;
//  4. OFF.
//
// The compact route is OFF by default: buildbox measurements (tamarack,
// interleaved Go/C ratio harness) found the compact route 1.1x to 2.2x
// SLOWER than production on Python, Rust, Markdown, Lua, CSS, Bash, and Go,
// and TypeScript/YAML paid for both routes on every parse. Set
// GTS_ADMISSION_CANDIDATE=1 (or true/on/yes) to opt back in -- the escape
// hatch now runs in the opposite direction from Phase-3 admission's original
// default. An emergency build (-tags gts_no_parsercorephase0) compiles the
// compact engine out entirely and every eligible full parse then falls back to
// production regardless of this switch.

// admissionRouteMode is the per-Parser override state. The zero value follows
// the process-wide default.
type admissionRouteMode uint8

const (
	admissionRouteFollowDefault admissionRouteMode = iota
	admissionRouteCandidateForced
	admissionRouteProductionForced
)

// admissionCandidateRouteDefault is the process-wide default the switch applies
// when a Parser sets no explicit override. init seeds it from the environment;
// SetAdmissionCandidateRouteDefault changes it at runtime.
// 0 means implicit OFF, 1 means explicitly OFF, and 2 means explicitly ON.
var admissionCandidateRouteDefault atomic.Uint32

// admissionCandidateRouted counts full parses served by the compact candidate
// route. admissionCandidateFallback counts eligible full parses that attempted
// the compact route, were declined, and fell back to production. A moving
// fallback counter is the loud, fail-closed signal that the candidate route
// declined an input rather than silently diverging.
var (
	admissionCandidateRouted   atomic.Uint64
	admissionCandidateFallback atomic.Uint64
)

// admissionCandidateLastFallbackReason records the most recent decline detail
// so an operator can triage why a full parse fell back to production.
var admissionCandidateLastFallbackReason atomic.Value // string

func init() {
	admissionCandidateRouteDefault.Store(admissionCandidateEnvMode())
}

// admissionCandidateEnvMode resolves the process-wide default the switch
// seeds from GTS_ADMISSION_CANDIDATE at package initialization.
//
// The compact route is OFF by default (buildbox's tamarack measurements: 1.1x
// to 2.2x slower than production on most languages). Only an explicit on
// value ("1", "true", "on", "yes", any case) resolves ON -- the opt-in escape
// hatch. An unset or unrecognized value, and any explicit off value ("0",
// "false", "off", "no"), resolves OFF.
func admissionCandidateEnvMode() uint32 {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GTS_ADMISSION_CANDIDATE"))) {
	case "1", "true", "on", "yes":
		return 2
	case "0", "false", "off", "no":
		return 1
	default:
		return 0
	}
}

func admissionCandidateEnvEnabled() bool { return admissionCandidateEnvMode() == 2 }

// admissionCandidateLanguageAllowlist names languages that use the compact
// route when the process-wide default is implicit OFF. It is empty by
// default: no language routes through the compact candidate on the strength
// of this list alone until a later change adds one, letting that change
// graduate a single language without flipping admissionCandidateRouteDefault
// (and therefore every other language) at once. Keys are Language.Name,
// lowercased. This list only ever ADDS eligibility on top of the other
// checks in admissionCandidateFullParseEligible; it never removes it, and a
// per-Parser override (SetAdmissionCandidateRoute) still wins over it in
// either direction.
var admissionCandidateLanguageAllowlist = map[string]bool{}

// admissionCandidateLanguageAllowlisted reports whether name is on the
// per-language allowlist that widens the implicit OFF default.
func admissionCandidateLanguageAllowlisted(name string) bool {
	if name == "" {
		return false
	}
	return admissionCandidateLanguageAllowlist[strings.ToLower(name)]
}

// SetAdmissionCandidateRouteDefault sets the process-wide default the Phase-3
// admission switch applies to Parsers with no explicit override. A per-Parser
// override still wins.
//
// Tranche B9 removed the source-length eligibility decline: every input,
// regardless of size, is eligible to attempt the candidate route. An explicit
// timeout, cancellation flag, or the scheduler's own memory-budget poll
// (tranche B8) is honored on the candidate route itself, with a compatible
// stop receipt, falling back to production when it trips; included ranges and
// observability hooks still keep a parse on production.
func SetAdmissionCandidateRouteDefault(enabled bool) {
	if enabled {
		admissionCandidateRouteDefault.Store(2)
	} else {
		admissionCandidateRouteDefault.Store(1)
	}
}

// AdmissionCandidateRouteDefault reports the current process-wide default.
func AdmissionCandidateRouteDefault() bool {
	return admissionCandidateRouteDefault.Load() == 2
}

// SetAdmissionCandidateRoute sets a per-Parser override that takes precedence
// over the process-wide default. enabled=true forces the candidate route on for
// eligible full parses; enabled=false forces the production route.
//
// enabled=true still respects every remaining eligibility decline: included
// ranges and observability hooks keep a parse on production. Source length no
// longer declines eligibility (tranche B9); a large input attempts the
// candidate route and either completes there or trips the scheduler's
// stop-control poll (tranche B8) and falls back to production with a
// compatible stop receipt honoring ParseStopMemoryBudget.
func (p *Parser) SetAdmissionCandidateRoute(enabled bool) {
	if p == nil {
		return
	}
	if enabled {
		p.admissionCandidateRoute = admissionRouteCandidateForced
	} else {
		p.admissionCandidateRoute = admissionRouteProductionForced
	}
}

// ClearAdmissionCandidateRoute drops the per-Parser override so the Parser
// follows the process-wide default again.
func (p *Parser) ClearAdmissionCandidateRoute() {
	if p != nil {
		p.admissionCandidateRoute = admissionRouteFollowDefault
	}
}

// AdmissionCandidateCounters returns the number of full parses the compact
// candidate route served, and the number of eligible full parses that fell back
// to production.
func AdmissionCandidateCounters() (routed, fallbacks uint64) {
	return admissionCandidateRouted.Load(), admissionCandidateFallback.Load()
}

// AdmissionCandidateLastFallbackReason returns the most recent candidate-route
// decline detail, or the empty string if none has been recorded.
func AdmissionCandidateLastFallbackReason() string {
	if v, ok := admissionCandidateLastFallbackReason.Load().(string); ok {
		return v
	}
	return ""
}

// ResetAdmissionCandidateCounters clears the process-global admission switch
// counters and the last fallback reason. It is a diagnostics helper: call it
// at the start of a test that asserts on AdmissionCandidateCounters or
// AdmissionCandidateLastFallbackReason, so an earlier test's fallback does
// not leak into the assertion.
func ResetAdmissionCandidateCounters() {
	admissionCandidateRouted.Store(0)
	admissionCandidateFallback.Store(0)
	admissionCandidateLastFallbackReason.Store("")
}

// resetAdmissionCandidateCounters clears the counters. Test-only.
func resetAdmissionCandidateCounters() {
	ResetAdmissionCandidateCounters()
}

// admissionCandidateRouteEnabled resolves the switch precedence for p.
func (p *Parser) admissionCandidateRouteEnabled() bool {
	if p == nil {
		return false
	}
	switch p.admissionCandidateRoute {
	case admissionRouteCandidateForced:
		return true
	case admissionRouteProductionForced:
		return false
	default:
		mode := admissionCandidateRouteDefault.Load()
		if mode != 0 {
			return mode == 2
		}
		return p.language != nil && admissionCandidateLanguageAllowlisted(p.language.Name)
	}
}

// admissionCandidateFullParseEligible reports whether a fresh full parse may be
// routed through the compact candidate. Policy comes first: a reparse
// (oldTree != nil), a suppressed nested parse, or a parser whose switch is off
// never starts compact. Checking the switch first keeps the default route
// cheap. The capability table in internal/sched decides the rest: a
// caller-supplied token source, included ranges, an observer, explicit work
// limits, or a forest-default language each keep the parse on the legacy
// route before compact starts. docs/v1-capability-flags.md lists why compact
// does not serve each of these modes. A timeout, a cancellation flag, the
// memory budget, and the source length do not keep a parse off compact: the
// compact scheduler polls all of them and falls back with a compatible stop
// reason (tranches B8 and B9; see attemptAdmissionCandidateFullParse).
func (p *Parser) admissionCandidateFullParseEligible(oldTree *Tree, usingProductionDFA bool) bool {
	if p == nil || oldTree != nil || p.admissionRouteSuppressed > 0 {
		return false
	}
	if !p.admissionCandidateRouteEnabled() {
		return false
	}
	var modes sched.Mode
	if !usingProductionDFA {
		modes = sched.TokenSource
	}
	return sched.Supports(sched.Request{Modes: modes | p.schedImpliedModes(modes, nil)})
}

// hasActiveParseObservability reports whether a consumer attached any parse-time
// observability hook that the compact route would not emit.
func (p *Parser) hasActiveParseObservability() bool {
	if p == nil {
		return false
	}
	if p.logger != nil || p.glrTrace || p.ambiguityProfile != nil {
		return true
	}
	return envKnobs().parseProgress
}

// suppressAdmissionCandidateRoute forces the production route for the returned
// function's lifetime. It is nestable: reuse-consuming calls and production-only
// correctness reparses raise it around any delegated fresh Parse so that a
// compact tree can never leak out of a path that must stay on production.
func (p *Parser) suppressAdmissionCandidateRoute() func() {
	if p == nil {
		return func() {}
	}
	p.admissionRouteSuppressed++
	return func() { p.admissionRouteSuppressed-- }
}

// pinToProductionRoute permanently forces an internally-created sub-parser onto
// the production route, independent of the process-wide default. Recovery,
// snippet, and injection sub-parsers parse fragments that feed recovery splicing
// or injection subtrees -- contexts the admission scorecard never validated --
// so they must never route a compact tree even if the global default flips on.
func (p *Parser) pinToProductionRoute() {
	if p == nil {
		return
	}
	p.admissionCandidateRoute = admissionRouteProductionForced
	p.admissionRouteSuppressed = 0
	p.admissionCandidateRunner = nil
}

// attemptAdmissionCandidateFullParse routes a fresh full parse through the
// compact candidate when the switch is on and the call is eligible. It returns
// (tree, true) when the candidate route produced a public tree, and (nil,
// false) otherwise. On an eligible-but-declined attempt it bumps the fallback
// counter (fail-closed, loud) so the caller re-runs the production route.
//
// Tranche B9 removed the source-length eligibility decline that used to keep
// every input at or above parseRuntimeMemoryMinSourceBytes on production. A
// large input now attempts the candidate route like any other: the scheduler's
// stop-control poll (tranche B8, pollStopControl in
// parsercore_phase0_driver.go) compares the compact core's own real retained
// footprint (Core.FootprintBytes(), a capacity-based gauge -- see its doc
// comment for the accounting it covers and the ephemeral-allocation gap it
// still has) against the same soft budget production's arena/scratch
// accounting honors, and the caller's own production/hard-ceiling backstop
// (parseMemoryHardCeilingBytes) stays armed even when the soft budget is
// disabled. On any decline -- a stop-control trip, an acceptance-gate
// decline, or a materialization decline -- the runner releases the compact
// core's retained capacity (Core.ResetReleasingRetention, not plain Reset:
// Reset alone truncates logical length but keeps backing-array capacity for
// reuse, which billed 157-193MB of retained memory to every later parse on
// the same cached runner before this gate) before returning control to this
// function, so a decline does not leave compact storage retained at an
// unbounded multiple of the configured budget while production's fallback
// parse runs. This bounds RETAINED footprint deterministically; it does not
// bound the compact scheduler's own CUMULATIVE ephemeral allocation during
// the declined attempt itself, which a GC-disabled measurement (unlike an
// ordinary, GC-enabled process) can still observe as a multiple of the
// configured budget above production's own contract -- see
// stopControlFootprintChurnRatio's doc comment for the measured range and
// why it is not fully closed. A pathological input that exceeds the budget
// or a hard node/stack cap declines here (an engine decline, counted below,
// which also bumps AdmissionCandidateCounters' fallback count -- an
// operator watching that counter sees every one of these) and production
// serves it, honoring ParseStopMemoryBudget exactly as it always has.
func (p *Parser) attemptAdmissionCandidateFullParse(source []byte, oldTree *Tree, usingProductionDFA bool) (*Tree, bool) {
	if !p.admissionCandidateFullParseEligible(oldTree, usingProductionDFA) {
		return nil, false
	}
	tree, ok, reason := p.tryCompactFullParseRoute(source)
	if ok && tree != nil {
		admissionCandidateRouted.Add(1)
		return tree, true
	}
	admissionCandidateFallback.Add(1)
	admissionCandidateLastFallbackReason.Store(reason)
	return nil, false
}

// schedCall returns the request for a public parse call whose method implies
// entry. oldTree is the edited tree of an incremental call, or nil. The
// request's Implied function adds the modes that the parser configuration and
// oldTree imply, only when a caller asks for them: computing them on every
// call would add a map lookup and an interface call to the no-edit reparse,
// which returns in a few nanoseconds.
func (p *Parser) schedCall(entry sched.Mode, oldTree *Tree) sched.Request {
	return sched.Request{Modes: entry, Implied: func() sched.Mode {
		return p.schedImpliedModes(entry, oldTree)
	}}
}

// schedImpliedModes returns the modes that the parser configuration and, for
// an incremental call, oldTree imply. A nil parser implies no modes, so each
// public method keeps its own nil-parser behavior.
func (p *Parser) schedImpliedModes(entry sched.Mode, oldTree *Tree) sched.Mode {
	if p == nil {
		return 0
	}
	modes := p.schedParserModes()
	if entry&sched.Incremental != 0 {
		modes |= p.schedOldTreeModes(oldTree)
	}
	return modes
}

// schedParserModes returns the modes that the parser configuration implies.
// admissionCandidateFullParseEligible reads its open modes through
// sched.Supports. TestAdmissionEligibilityMatchesPreTableChecks checks that
// the result equals the checks the eligibility function made before it read
// the table.
func (p *Parser) schedParserModes() sched.Mode {
	var modes sched.Mode
	if p.timeoutMicros != 0 || p.cancellationFlag != nil {
		modes |= sched.Deadline
	}
	if len(p.included) > 0 {
		modes |= sched.IncludedRanges
	}
	if p.hasActiveParseObservability() {
		modes |= sched.Observer
	}
	if p.parseWorkLimits.configured() {
		modes |= sched.WorkLimits
	}
	// A forced candidate route takes precedence over the forest default, as it
	// does in admissionCandidateFullParseEligible.
	if p.admissionCandidateRoute != admissionRouteCandidateForced && glrForestEnabled && parserWantsForest(p) {
		modes |= sched.ForestRoute
	}
	return modes
}

// schedOldTreeModes returns the modes that an incremental call implies.
// attemptCompactIncrementalParse reads them before the compact engine starts.
func (p *Parser) schedOldTreeModes(oldTree *Tree) sched.Mode {
	var modes sched.Mode
	if p.language != nil && compactReuseScannerUnsupported(p.language) {
		modes |= sched.ScannerStateReuse
	}
	if compactReuseOldTreeUnsupported(p.language, oldTree) {
		modes |= sched.OldTreeReuse
	}
	return modes
}

// compactReuseOldTreeUnsupported reports whether the compact engine cannot
// reuse oldTree for a parser of lang: compact did not build it, its root has
// an error, it lacks a replay proof, it has no recorded edit, or it belongs
// to another language.
func compactReuseOldTreeUnsupported(lang *Language, oldTree *Tree) bool {
	return oldTree == nil || oldTree.language != lang || !oldTree.compactMaterialized ||
		oldTree.incrementalReuseDisabled || len(oldTree.edits) == 0 ||
		oldTree.root == nil || oldTree.root.HasError()
}

// compactReuseScannerUnsupported reports whether the external scanner of lang
// keeps compact incremental reuse from starting. Compact reuse needs a
// stateless scanner. lang must not be nil.
func compactReuseScannerUnsupported(lang *Language) bool {
	if lang.ExternalScanner == nil {
		return false
	}
	stateless, ok := lang.ExternalScanner.(StatelessExternalScanner)
	return !ok || !stateless.ExternalScannerIsStateless()
}
