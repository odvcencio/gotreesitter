package scannercert

import (
	"bytes"
	"encoding/json"
)

// Schema names the report layout. Change it with any field change.
const Schema = "scannercert-report-v2"

// Verdicts. No findings means no check failed on the recorded calls; it is
// evidence, not proof, and it grants nothing. Not transparent means the parse
// with the recorder differed from the parse without it, so the recorded calls
// are not the production parser's and the report says nothing about the
// scanner. See the package comment.
const (
	VerdictNoFindings     = "no-findings"
	VerdictFindings       = "findings"
	VerdictNoEvidence     = "no-evidence"
	VerdictNotTransparent = "not-transparent"
)

// Witness kinds, one per check that can fail.
const (
	KindResult            = "result"
	KindSymbol            = "symbol"
	KindSpan              = "span"
	KindState             = "state"
	KindRoundTrip         = "roundtrip"
	KindFailedScanState   = "failed-scan-state"
	KindSerializeUnstable = "serialize-unstable"
	KindSerializeOverflow = "serialize-overflow"
	KindReplayPanic       = "replay-panic"
)

// MaxWitnessesPerKind bounds the witnesses a report keeps for each kind. The
// counts in Checks still include every mismatch.
const MaxWitnessesPerKind = 3

// Report is the certification result for one grammar's external scanner.
type Report struct {
	Schema   string   `json:"schema"`
	Language string   `json:"language"`
	Scanner  string   `json:"scanner"`
	Declared Declared `json:"declared"`
	// Environment names the GOT_ and GTS_ variables set during the run. They
	// change parser behavior, so they change which calls the parser makes.
	Environment []string   `json:"environment"`
	Corpus      CorpusInfo `json:"corpus"`
	Parses      ParseInfo  `json:"parses"`
	Calls       CallInfo   `json:"calls"`
	Checks      Checks     `json:"checks"`
	Verdict     string     `json:"verdict"`
	Witnesses   []Witness  `json:"witnesses"`
}

// Declared records the scanner's own capability claims: "true", "false", or
// "absent" when the scanner does not implement the interface.
type Declared struct {
	IncrementalReuse        string `json:"incremental_reuse"`
	Checkpointed            string `json:"checkpointed"`
	Stateless               string `json:"stateless"`
	PreservesStateOnFailure string `json:"preserves_state_on_failure"`
	RetainsStateOnFailure   string `json:"retains_state_on_failure"`
}

// CorpusInfo identifies the pinned inputs. Sources counts the sample and the
// sources of its editing session.
type CorpusInfo struct {
	Path           string `json:"path"`
	SampleSHA256   string `json:"sample_sha256"`
	SessionSHA256  string `json:"session_sha256"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Sources        int    `json:"sources"`
}

// ParseInfo summarizes the parses that produced the recorded calls.
type ParseInfo struct {
	Sources int `json:"sources"`
	// SampleHasError is true when the committed sample itself parsed with an
	// error. ErrorTrees counts every source that did, and the editing session
	// breaks most of them.
	SampleHasError bool `json:"sample_has_error"`
	ErrorTrees     int  `json:"error_trees"`
	// Transparent is true when every source parsed to the same tree with the
	// recorder installed as without it.
	Transparent bool `json:"transparent"`
	Divergent   int  `json:"divergent"`
}

// CallInfo counts the scanner calls. Distinct calls differ in source, origin,
// valid-symbol mask, serialized state or outcome; only they are replayed, and
// every count below except Recorded is over them.
//
// States and Masks count the different serialized states and valid-symbol
// masks the calls began with, so they show how much of the scanner's state
// space the corpus reached. EmptyStates counts the calls that began with a
// zero-byte state. MaxStateBytes is the longest state any call began with.
type CallInfo struct {
	Recorded      int `json:"recorded"`
	Distinct      int `json:"distinct"`
	Accepted      int `json:"accepted"`
	Rejected      int `json:"rejected"`
	States        int `json:"states"`
	Masks         int `json:"masks"`
	EmptyStates   int `json:"empty_states"`
	MaxStateBytes int `json:"max_state_bytes"`
}

// Checks counts how many distinct calls failed each check. One call can fail
// several checks.
type Checks struct {
	Replayed               int `json:"replayed"`
	ResultMismatches       int `json:"result_mismatches"`
	SymbolMismatches       int `json:"symbol_mismatches"`
	SpanMismatches         int `json:"span_mismatches"`
	StateMismatches        int `json:"state_mismatches"`
	RoundTripMismatches    int `json:"roundtrip_mismatches"`
	FailedScanStateChanges int `json:"failed_scan_state_changes"`
	SerializeUnstable      int `json:"serialize_unstable"`
	SerializeOverflow      int `json:"serialize_overflow"`
	ReplayPanics           int `json:"replay_panics"`
}

// add counts one failed check of the given witness kind.
func (c *Checks) add(kind string) {
	switch kind {
	case KindResult:
		c.ResultMismatches++
	case KindSymbol:
		c.SymbolMismatches++
	case KindSpan:
		c.SpanMismatches++
	case KindState:
		c.StateMismatches++
	case KindRoundTrip:
		c.RoundTripMismatches++
	case KindFailedScanState:
		c.FailedScanStateChanges++
	case KindSerializeUnstable:
		c.SerializeUnstable++
	case KindSerializeOverflow:
		c.SerializeOverflow++
	case KindReplayPanic:
		c.ReplayPanics++
	default:
		panic("scannercert: unknown witness kind " + kind)
	}
}

func (c Checks) failures() int {
	return c.ResultMismatches + c.SymbolMismatches + c.SpanMismatches + c.StateMismatches +
		c.RoundTripMismatches + c.FailedScanStateChanges + c.SerializeUnstable +
		c.SerializeOverflow + c.ReplayPanics
}

// Origin is where a scan began.
type Origin struct {
	Byte   uint32 `json:"byte"`
	Row    uint32 `json:"row"`
	Column uint32 `json:"column"`
}

// Outcome is what one run of Scan did. State is the hex of the serialized
// scanner state after the scan.
type Outcome struct {
	Accepted   bool   `json:"accepted"`
	HasSymbol  bool   `json:"has_symbol"`
	Symbol     int    `json:"symbol"`
	SymbolName string `json:"symbol_name,omitempty"`
	StartByte  uint32 `json:"start_byte"`
	EndByte    uint32 `json:"end_byte"`
	EndRow     uint32 `json:"end_row"`
	EndColumn  uint32 `json:"end_column"`
	State      string `json:"state"`
}

// Witness is one failing call, with enough detail to replay it by hand.
// Source 0 is the pinned sample; source k is step k of its editing session.
// State is the hex of the serialized scanner state before the scan, and Valid
// lists the external tokens the parser allowed as "index:name".
type Witness struct {
	Kind    string   `json:"kind"`
	Source  int      `json:"source"`
	Origin  Origin   `json:"origin"`
	Context string   `json:"context"`
	Valid   []string `json:"valid"`
	State   string   `json:"state"`
	Live    *Outcome `json:"live,omitempty"`
	Replay  *Outcome `json:"replay,omitempty"`
	Detail  string   `json:"detail,omitempty"`
}

// JSON encodes the report deterministically: fixed field order, two-space
// indentation, no HTML escaping, one trailing newline.
func (r *Report) JSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(r); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
