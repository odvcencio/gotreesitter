// Package sched is the engine seam (design item E-A0).
//
// Every public parse method builds one Request and calls Parse. A Request
// names the modes that the call needs. The capability table records whether
// the compact engine serves each mode. A mode that compact does not serve is
// an open flag: only the legacy engine serves a request that needs it.
//
// Capability and policy are separate. The capability table says whether the
// compact engine can serve a request. Policy says whether the caller wants
// compact at all: the admission switch, the per-language allowlist, the
// per-parser override, the pins on internal sub-parsers, and the suppression
// of nested legacy parses. Policy stays in the root package.
//
// This package does not import the root package, because the root package
// imports it (design decision D9). The engines live in the root package, so
// Parse takes one function value that runs the request. That function value
// is the whole interface between the seam and the engines. Parse only calls
// it, so the function value does not escape and the seam adds no allocation.
package sched

// Mode is a set of request modes. Each bit is one row of the capability
// table.
type Mode uint32

// Request modes that the compact engine serves today.
const (
	// Incremental marks a reparse that receives an edited old tree.
	Incremental Mode = 1 << iota
	// UTF16 marks a call that parses UTF-16 input through its UTF-8 view.
	UTF16
	// Profiling marks a call that returns incremental attribution.
	Profiling
	// Strict marks a call that reports a partial tree as an error.
	Strict
	// Deadline marks a parser with a timeout or a cancellation flag.
	Deadline

	// Request modes that the compact engine cannot serve today. Each one is
	// an open flag.

	// TokenSource marks a caller-supplied TokenSource or TokenSourceFactory.
	TokenSource
	// IncludedRanges marks a parser with included ranges.
	IncludedRanges
	// Observer marks a parser with a logger, a GLR trace, an ambiguity
	// profile, or parse-progress events.
	Observer
	// WorkLimits marks a parser with explicit ParseWorkLimits.
	WorkLimits
	// ForestRoute marks a language that defaults to the GSS-forest route,
	// unless its parser forces the compact route, and the explicit forest
	// entry point.
	ForestRoute
	// Measurement marks the benchmark-only parse modes that measure legacy
	// internals.
	Measurement
	// ScannerStateReuse marks incremental reuse with an external scanner
	// that is not stateless.
	ScannerStateReuse
	// OldTreeReuse marks incremental reuse of an old tree that the compact
	// engine cannot reuse.
	OldTreeReuse
)

// openModes is the set of open flags. TestCapabilityTableMatchesModes keeps
// it equal to the open rows of the capability table.
const openModes = TokenSource | IncludedRanges | Observer | WorkLimits |
	ForestRoute | Measurement | ScannerStateReuse | OldTreeReuse

// Capability is one row of the capability table.
type Capability struct {
	// Mode is the request mode that this row describes.
	Mode Mode
	// Name is a stable identifier for reports.
	Name string
	// Compact reports whether the compact engine serves the mode today. A
	// row with Compact == false is an open flag.
	Compact bool
	// Closes names the design item that closes an open flag. For a served
	// mode it names the mechanism that serves the mode.
	Closes string
	// Detail says what the mode is and, for an open flag, why the compact
	// engine does not serve it today.
	Detail string
}

// capabilities is the capability table, in Mode bit order.
var capabilities = [...]Capability{
	{Incremental, "incremental", true, "compact incremental reuse",
		"A reparse with an edited old tree. Compact reuses subtrees when the old tree and the scanner allow it; see scanner_state_reuse and old_tree_reuse."},
	{UTF16, "utf16", true, "UTF-8 view",
		"UTF-16 input. Every engine parses the UTF-8 view, and the tree maps ranges back to UTF-16."},
	{Profiling, "profiling", true, "compact incremental timing",
		"Incremental attribution in IncrementalParseProfile. The compact incremental route records the same timing fields."},
	{Strict, "strict", true, "result check",
		"A partial tree becomes ErrParseStoppedEarly. The check reads the returned tree, so it does not depend on the engine."},
	{Deadline, "deadline", true, "tranche B8",
		"A timeout or a cancellation flag. The compact scheduler polls both and falls back with a compatible stop reason."},
	{TokenSource, "token_source", false, "E-D1",
		"A caller-supplied TokenSource or TokenSourceFactory. Compact reproduces only the built-in DFA token stream."},
	{IncludedRanges, "included_ranges", false, "E-D2",
		"Included ranges. Compact lexes the whole source and does not apply ranges."},
	{Observer, "observer", false, "E-D3",
		"A logger, a GLR trace, an ambiguity profile, or GOT_PARSE_PROGRESS events. Compact emits none of these events."},
	{WorkLimits, "work_limits", false, "E-D4",
		"Explicit ParseWorkLimits. Compact keeps separate work counters and must not spend work before a legacy limit trips."},
	{ForestRoute, "forest_route", false, "E-D6",
		"A language that defaults to the GSS-forest route, unless its parser forces the compact route, and ParseForestExperimental. The forest engine serves these requests."},
	{Measurement, "measurement", false, "D-A3",
		"The benchmark-only modes: no tree, no tree with external checkpoints, and no result compatibility. They measure legacy internals."},
	{ScannerStateReuse, "scanner_state_reuse", false, "E-E1",
		"Incremental reuse with an external scanner that is not stateless. Compact reuse needs a stateless scanner."},
	{OldTreeReuse, "old_tree_reuse", false, "E-E7",
		"Incremental reuse of an old tree that compact did not build, whose root has an error, that lacks a replay proof, that has no recorded edit, or that belongs to another language."},
}

// Capabilities returns a copy of the capability table in Mode bit order.
func Capabilities() []Capability {
	out := make([]Capability, len(capabilities))
	copy(out, capabilities[:])
	return out
}

// Open returns the set of open flags.
func Open() Mode { return openModes }

// OpenCount returns the number of open flags. It is the scoreboard metric
// "capability flags open".
func OpenCount() int {
	count := 0
	for _, row := range capabilities {
		if !row.Compact {
			count++
		}
	}
	return count
}

// Request describes one public parse call.
type Request struct {
	// Modes holds the modes that the public method itself implies.
	Modes Mode
	// Implied returns the modes that the parser configuration and, for a
	// reparse, the old tree imply. It may be nil. Request methods call it only
	// when they need the full mode set, because computing it on every call
	// would add a map lookup and an interface call to a reparse without edits.
	Implied func() Mode
}

// All returns every mode the request needs.
func (r Request) All() Mode {
	if r.Implied == nil {
		return r.Modes
	}
	return r.Modes | r.Implied()
}

// Has reports whether the request needs every mode in m.
func (r Request) Has(m Mode) bool { return r.All()&m == m }

// OpenModes returns the open flags that the request needs.
func (r Request) OpenModes() Mode { return r.All() & openModes }

// Supports reports whether the compact engine can serve every mode that req
// needs. It does not decide policy; see the package comment. It calls
// req.Implied only when the method modes are all served.
func Supports(req Request) bool {
	if req.Modes&openModes != 0 {
		return false
	}
	return req.All()&openModes == 0
}

// Parse is the single entry point for every public parse method. It runs req
// through run, which executes the request on the engines in the root package.
// Parse does not choose an engine: the root package keeps its route ladder,
// and the compact engine applies its own eligibility checks.
//
// Parse and run inline into the public method. The public methods themselves
// are marked //go:noinline, so their callers do not change.
func Parse[T any](req Request, run func(Request) (T, error)) (T, error) {
	return run(req)
}
