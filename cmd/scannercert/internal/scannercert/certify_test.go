package scannercert

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// hashScanner turns "#" into the external PLUS token, but only for its first
// two uses: its one byte of state counts them. With omitState set, Serialize
// forgets the count, which makes the scanner unsafe to resume from a snapshot.
type hashScanner struct{ omitState bool }

type hashState struct{ seen uint8 }

func (hashScanner) Create() any { return &hashState{} }
func (hashScanner) Destroy(any) {}
func (s hashScanner) Serialize(payload any, buf []byte) int {
	if s.omitState {
		return 0
	}
	buf[0] = payload.(*hashState).seen
	return 1
}

func (hashScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*hashState)
	st.seen = 0
	if len(buf) > 0 {
		st.seen = buf[0]
	}
}

func (hashScanner) Scan(payload any, lexer *gts.ExternalLexer, valid []bool) bool {
	st := payload.(*hashState)
	if len(valid) == 0 || !valid[0] || lexer.Lookahead() != '#' || st.seen >= 2 {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(2)
	st.seen++
	return true
}

type panickingScanner struct{ hashScanner }

func (panickingScanner) Scan(any, *gts.ExternalLexer, []bool) bool { panic("scanner exploded") }

// miniLanguage is the smallest grammar the parser can run an external scanner
// in: expression -> NUMBER | expression PLUS NUMBER, with PLUS external.
func miniLanguage(scanner gts.ExternalScanner) *gts.Language {
	return &gts.Language{
		Name:              "mini",
		SymbolCount:       4,
		TokenCount:        3,
		StateCount:        5,
		ProductionIDCount: 2,
		SymbolNames:       []string{"EOF", "NUMBER", "+", "expression"},
		SymbolMetadata: []gts.SymbolMetadata{
			{Name: "EOF"},
			{Name: "NUMBER", Visible: true, Named: true},
			{Name: "+", Visible: true},
			{Name: "expression", Visible: true, Named: true},
		},
		FieldNames: []string{""},
		ParseActions: []gts.ParseActionEntry{
			{},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionShift, State: 1}}},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionReduce, Symbol: 3, ChildCount: 1, ProductionID: 0}}},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionShift, State: 2}}},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionShift, State: 3}}},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionAccept}}},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionShift, State: 4}}},
			{Actions: []gts.ParseAction{{Type: gts.ParseActionReduce, Symbol: 3, ChildCount: 3, ProductionID: 1}}},
		},
		ParseTable: [][]uint16{
			{0, 1, 0, 3},
			{2, 2, 2, 0},
			{5, 0, 4, 0},
			{0, 6, 0, 0},
			{7, 7, 7, 0},
		},
		LexModes: make([]gts.LexMode, 5),
		LexStates: []gts.LexState{
			{Default: -1, EOF: -1, Transitions: []gts.LexTransition{
				{Lo: '0', Hi: '9', NextState: 1},
				{Lo: ' ', Hi: ' ', NextState: 2},
			}},
			{AcceptToken: 1, Default: -1, EOF: -1, Transitions: []gts.LexTransition{{Lo: '0', Hi: '9', NextState: 1}}},
			{Skip: true, Default: -1, EOF: -1, Transitions: []gts.LexTransition{{Lo: ' ', Hi: ' ', NextState: 2}}},
		},
		ExternalScanner: scanner,
		ExternalSymbols: []gts.Symbol{2},
	}
}

func miniCorpus() *Corpus {
	return &Corpus{
		Language: "mini",
		Sources:  [][]byte{[]byte("1#2"), []byte("1#2#3#4"), []byte("1 # 2 # 3")},
	}
}

func TestCertifyFindsNothingInAScannerThatSerializesItsWholeState(t *testing.T) {
	scanner := hashScanner{}
	lang := miniLanguage(scanner)
	report, err := Certify(lang, miniCorpus())
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictNoFindings {
		t.Fatalf("verdict = %q, want no findings; checks %+v witnesses %+v", report.Verdict, report.Checks, report.Witnesses)
	}
	if report.Calls.Accepted == 0 || report.Calls.Rejected == 0 {
		t.Fatalf("calls = %+v, want accepted and rejected scans (the third # is refused)", report.Calls)
	}
	if !report.Parses.Transparent || report.Parses.Sources != 3 {
		t.Fatalf("parses = %+v, want 3 sources parsed identically with and without the recorder", report.Parses)
	}
	if report.Parses.SampleHasError || report.Parses.ErrorTrees != 1 {
		t.Fatalf("parses = %+v, want a clean sample and one error tree (the refused third '#')", report.Parses)
	}
	if report.Language != "mini" || report.Scanner != "scannercert.hashScanner" || report.Schema != Schema {
		t.Fatalf("identity = %q %q %q", report.Schema, report.Language, report.Scanner)
	}
	if lang.ExternalScanner != gts.ExternalScanner(scanner) {
		t.Fatal("Certify did not restore the language's scanner")
	}
}

// The scanner refuses the third "#" because it counted two. A replay from the
// serialized state starts the count at zero and accepts it.
func TestCertifyReportsAScannerThatForgetsStateWhenSerializing(t *testing.T) {
	lang := miniLanguage(hashScanner{omitState: true})
	report, err := Certify(lang, miniCorpus())
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdict != VerdictFindings || report.Checks.ResultMismatches != 1 || report.Checks.failures() != 1 {
		t.Fatalf("verdict %q checks %+v, want one result mismatch and nothing derived from it", report.Verdict, report.Checks)
	}
	var found bool
	for _, w := range report.Witnesses {
		if w.Kind == KindResult && w.Live != nil && !w.Live.Accepted && w.Replay.Accepted {
			found = true
			if !strings.HasPrefix(w.Context, "#") || len(w.Valid) != 1 || w.Valid[0] != "0:+" {
				t.Fatalf("witness = %+v, want a scan at '#' with valid symbols [0:+]", w)
			}
		}
	}
	if !found {
		t.Fatalf("no witness shows the live scan refusing what the replay accepted: %+v", report.Witnesses)
	}
}

// savesScanner counts its Serialize calls in its state and refuses "#" once
// the count passes two. The parser serializes once before each scan. The
// recorder serializes around every scan as well, so the recorded parse reaches
// the limit sooner and refuses a "#" that the production parse accepts.
type savesScanner struct{}

type savesState struct{ saves uint8 }

func (savesScanner) Create() any { return &savesState{} }
func (savesScanner) Destroy(any) {}
func (savesScanner) Serialize(payload any, buf []byte) int {
	st := payload.(*savesState)
	st.saves++
	buf[0] = st.saves
	return 1
}

func (savesScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*savesState)
	st.saves = 0
	if len(buf) > 0 {
		st.saves = buf[0]
	}
}

func (savesScanner) Scan(payload any, lexer *gts.ExternalLexer, valid []bool) bool {
	st := payload.(*savesState)
	if len(valid) == 0 || !valid[0] || lexer.Lookahead() != '#' || st.saves > 2 {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(2)
	return true
}

// A report is evidence about the production parser only if the parse with the
// recorder equals the parse without it. When it does not, the verdict must say
// so rather than present the recorded calls as findings or as a clean result.
func TestCertifyReportsAParseThatTheRecorderChanged(t *testing.T) {
	report, err := Certify(miniLanguage(savesScanner{}), miniCorpus())
	if err != nil {
		t.Fatal(err)
	}
	if report.Parses.Transparent || report.Parses.Divergent == 0 {
		t.Fatalf("parses = %+v, want a recorded parse that differs from the production parse", report.Parses)
	}
	if report.Verdict != VerdictNotTransparent {
		t.Fatalf("verdict = %q with %d of %d sources changed, want %q", report.Verdict, report.Parses.Divergent, report.Parses.Sources, VerdictNotTransparent)
	}
}

func TestCertifyIsDeterministic(t *testing.T) {
	var out [2][]byte
	for i := range out {
		report, err := Certify(miniLanguage(hashScanner{omitState: true}), miniCorpus())
		if err != nil {
			t.Fatal(err)
		}
		out[i], err = report.JSON()
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(out[0], out[1]) {
		t.Fatalf("two runs differ:\n%s\n%s", out[0], out[1])
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(out[0], &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema", "language", "scanner", "declared", "environment", "corpus", "parses", "calls", "checks", "informational", "verdict", "witnesses"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("report has no %q field: %s", key, out[0])
		}
	}
	if out[0][len(out[0])-1] != '\n' {
		t.Fatal("report does not end with a newline")
	}
}

func TestCertifyReportsAnEmptyWitnessListAsAList(t *testing.T) {
	report, err := Certify(miniLanguage(hashScanner{}), miniCorpus())
	if err != nil {
		t.Fatal(err)
	}
	data, err := report.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"witnesses": []`)) || !bytes.Contains(data, []byte(`"environment": [`)) {
		t.Fatalf("empty lists must encode as [], not null: %s", data)
	}
}

func TestCertifyDescribesTheDeclaredCapabilities(t *testing.T) {
	report, err := Certify(miniLanguage(hashScanner{}), miniCorpus())
	if err != nil {
		t.Fatal(err)
	}
	want := Declared{IncrementalReuse: "absent", Checkpointed: "absent", Stateless: "absent", PreservesStateOnFailure: "absent", RetainsStateOnFailure: "absent", FailureContract: ContractRestored}
	if report.Declared != want {
		t.Fatalf("declared = %+v, want %+v", report.Declared, want)
	}
	stateless, err := Certify(miniLanguage(statelessHash{}), miniCorpus())
	if err != nil {
		t.Fatal(err)
	}
	if stateless.Declared.Stateless != "true" || stateless.Declared.PreservesStateOnFailure != "true" || stateless.Declared.IncrementalReuse != "false" ||
		stateless.Declared.FailureContract != ContractPreserved {
		t.Fatalf("declared = %+v, want stateless, failure-preserving and reuse opt-out", stateless.Declared)
	}
}

// statelessHash is hashScanner without the counter; it declares its
// capabilities so the report can echo them.
type statelessHash struct{}

func (statelessHash) Create() any                       { return nil }
func (statelessHash) Destroy(any)                       {}
func (statelessHash) Serialize(any, []byte) int         { return 0 }
func (statelessHash) Deserialize(any, []byte)           {}
func (statelessHash) ExternalScannerIsStateless() bool  { return true }
func (statelessHash) PreservesStateOnScanFailure() bool { return true }
func (statelessHash) SupportsIncrementalReuse() bool    { return false }
func (statelessHash) Scan(_ any, lexer *gts.ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] || lexer.Lookahead() != '#' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(2)
	return true
}

func TestCertifyRestoresTheScannerWhenAScanPanics(t *testing.T) {
	scanner := panickingScanner{}
	lang := miniLanguage(scanner)
	_, err := Certify(lang, miniCorpus())
	if err == nil || !strings.Contains(err.Error(), "scanner exploded") {
		t.Fatalf("err = %v, want the scanner's panic", err)
	}
	if lang.ExternalScanner != gts.ExternalScanner(scanner) {
		t.Fatal("Certify left the recorder installed after a panic")
	}
}

func TestCertifyNeedsAScanner(t *testing.T) {
	lang := miniLanguage(nil)
	lang.ExternalScanner = nil
	if _, err := Certify(lang, miniCorpus()); !errors.Is(err, ErrNoScanner) {
		t.Fatalf("err = %v, want ErrNoScanner", err)
	}
	if _, err := Certify(nil, miniCorpus()); !errors.Is(err, ErrNoScanner) {
		t.Fatalf("err = %v, want ErrNoScanner for a nil language", err)
	}
}
