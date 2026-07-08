package gotreesitter

import (
	"strings"
	"testing"
)

type cRecoveryGateScanner struct{}

func (cRecoveryGateScanner) Create() any                           { return nil }
func (cRecoveryGateScanner) Destroy(any)                           {}
func (cRecoveryGateScanner) Serialize(any, []byte) int             { return 0 }
func (cRecoveryGateScanner) Deserialize(any, []byte)               {}
func (cRecoveryGateScanner) Scan(any, *ExternalLexer, []bool) bool { return false }

func cRecoveryGateLanguage() *Language {
	return &Language{
		Name:         "gate_test",
		InitialState: 1,
		StateCount:   2,
		SymbolCount:  3,
		TokenCount:   2,
		SymbolNames:  []string{"end", "tok", "root"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end", Named: true},
			{Name: "tok", Visible: true, Named: true},
			{Name: "root", Visible: true, Named: true},
		},
		ParseTable: [][]uint16{
			{0, 1, 0},
			{0, 1, 0},
		},
		ParseActions: []ParseActionEntry{
			{},
			{Actions: []ParseAction{{Type: ParseActionRecover, State: 0}}},
		},
		LexModes:                                 []LexMode{{LexState: 0}, {LexState: 0}},
		LexStates:                                []LexState{{Default: -1, EOF: -1}},
		CRecoveryCostCompetitionCapable:          true,
		CRecoveryCostCompetitionEnabledByDefault: true,
	}
}

func TestCRecoveryGateExplicitDefaultAndCapabilityEnableDefault(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	if !errorCostCompetitionLanguage(cRecoveryGateLanguage()) {
		t.Fatal("certified capable language did not enable C recovery by default")
	}
}

func TestCRecoveryGateCapabilityDoesNotEnableDefault(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	lang := cRecoveryGateLanguage()
	lang.CRecoveryCostCompetitionEnabledByDefault = false
	if errorCostCompetitionLanguage(lang) {
		t.Fatal("capable language default-enabled without explicit certification")
	}
}

func TestCRecoveryGateEnvOverrides(t *testing.T) {
	lang := cRecoveryGateLanguage()

	t.Setenv("GOT_C_RECOVERY", "0")
	if errorCostCompetitionLanguage(lang) {
		t.Fatal("GOT_C_RECOVERY=0 did not disable C recovery")
	}

	lang.CRecoveryCostCompetitionEnabledByDefault = false
	t.Setenv("GOT_C_RECOVERY", "all")
	if !errorCostCompetitionLanguage(lang) {
		t.Fatal("GOT_C_RECOVERY=all did not force-enable C recovery")
	}

	t.Setenv("GOT_C_RECOVERY", "other, gate_test ")
	if !errorCostCompetitionLanguage(lang) {
		t.Fatal("GOT_C_RECOVERY comma-list did not force-enable named language")
	}

	lang.CRecoveryCostCompetitionCapable = false
	t.Setenv("GOT_C_RECOVERY", "all")
	if !errorCostCompetitionLanguage(lang) {
		t.Fatal("GOT_C_RECOVERY=all should rely on runtime validation, not capability metadata")
	}
}

func TestCRecoveryGateRequiresExternalLexStatesForExternalScanners(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	lang := cRecoveryGateLanguage()
	lang.ExternalScanner = cRecoveryGateScanner{}
	lang.ExternalSymbols = []Symbol{1}
	lang.ExternalTokenCount = 1

	if errorCostCompetitionLanguage(lang) {
		t.Fatal("external scanner without ExternalLexStates default-enabled C recovery")
	}
	diag := DiagnoseCRecoveryGate(lang)
	if diag.Supported {
		t.Fatal("diagnostic supported external scanner without ExternalLexStates")
	}
	if !strings.Contains(diag.Reason, "precise ExternalLexStates") {
		t.Fatalf("diagnostic reason = %q, want precise ExternalLexStates", diag.Reason)
	}
	if !diag.HasExternalScanner || diag.ExternalSymbolCount != 1 || diag.ExternalLexStateRows != 0 {
		t.Fatalf("diagnostic external counts = scanner:%v symbols:%d rows:%d",
			diag.HasExternalScanner, diag.ExternalSymbolCount, diag.ExternalLexStateRows)
	}

	lang.ExternalLexStates = [][]bool{{false}, {true}}
	if !errorCostCompetitionLanguage(lang) {
		t.Fatal("external scanner with precise ExternalLexStates did not default-enable C recovery")
	}
	diag = DiagnoseCRecoveryGate(lang)
	if !diag.Supported || diag.Reason != "" {
		t.Fatalf("diagnostic rejected valid ExternalLexStates: supported=%v reason=%q", diag.Supported, diag.Reason)
	}
	if diag.ExternalLexStateRows != 2 || diag.ExternalLexStateMinLen != 1 {
		t.Fatalf("diagnostic external row counts = rows:%d min:%d, want rows:2 min:1",
			diag.ExternalLexStateRows, diag.ExternalLexStateMinLen)
	}
}

func TestCRecoveryGateDefaultOptOutKeepsCapability(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	lang := cRecoveryGateLanguage()
	lang.Name = "cpp"
	lang.ExternalScanner = cRecoveryGateScanner{}
	lang.ExternalSymbols = []Symbol{1}
	lang.ExternalTokenCount = 1
	lang.ExternalLexStates = [][]bool{{false}, {true}}

	diag := CertifyCRecoveryCostCompetition(lang)
	if !diag.Supported || diag.Reason != "" {
		t.Fatalf("diagnostic rejected valid opt-out language: supported=%v reason=%q", diag.Supported, diag.Reason)
	}
	if !lang.CRecoveryCostCompetitionCapable {
		t.Fatal("opt-out language did not keep C recovery capability")
	}
	if lang.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("opt-out language default-enabled C recovery")
	}
	if errorCostCompetitionLanguage(lang) {
		t.Fatal("opt-out language enabled C recovery without env override")
	}

	t.Setenv("GOT_C_RECOVERY", "cpp")
	if !errorCostCompetitionLanguage(lang) {
		t.Fatal("opt-out language did not force-enable via GOT_C_RECOVERY")
	}
}

func TestCRecoveryGateDiagnosticsExternalLexStateFailures(t *testing.T) {
	lang := cRecoveryGateLanguage()
	lang.ExternalScanner = cRecoveryGateScanner{}
	lang.ExternalSymbols = []Symbol{1, 2}
	lang.ExternalTokenCount = 2
	lang.ExternalLexStates = [][]bool{{true}}
	diag := DiagnoseCRecoveryGate(lang)
	if diag.Supported {
		t.Fatal("diagnostic supported short ExternalLexStates row")
	}
	if !strings.Contains(diag.Reason, "row is shorter") {
		t.Fatalf("diagnostic reason = %q, want short row reason", diag.Reason)
	}
	if diag.ExternalLexStateRows != 1 || diag.ExternalLexStateMinLen != 1 {
		t.Fatalf("diagnostic counts = rows:%d min:%d, want rows:1 min:1",
			diag.ExternalLexStateRows, diag.ExternalLexStateMinLen)
	}

	lang.ExternalSymbols = []Symbol{1}
	lang.ExternalTokenCount = 1
	lang.ExternalLexStates = [][]bool{{true}}
	lang.LexModes[1].ExternalLexState = 1
	diag = DiagnoseCRecoveryGate(lang)
	if diag.Supported {
		t.Fatal("diagnostic supported missing ExternalLexStates row reference")
	}
	if !strings.Contains(diag.Reason, "references missing ExternalLexStates row") {
		t.Fatalf("diagnostic reason = %q, want missing row reference reason", diag.Reason)
	}
}

func TestCRecoveryGateValidatesParseTableActionAndGotoBounds(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	lang := cRecoveryGateLanguage()
	lang.StateCount = 3
	lang.LexModes = []LexMode{{LexState: 0}, {LexState: 0}, {LexState: 0}}
	lang.ParseTable[1][2] = 2 // nonterminal goto state, not a parse-action index.
	if !errorCostCompetitionLanguage(lang) {
		t.Fatal("valid nonterminal goto equal to len(ParseActions) was rejected as an action index")
	}

	lang.SmallParseTableMap = []uint32{0}
	lang.SmallParseTable = []uint16{1, uint16(len(lang.ParseActions)), 1, 1}
	if errorCostCompetitionLanguage(lang) {
		t.Fatal("invalid small-table terminal action index did not disable C recovery")
	}
	diag := DiagnoseCRecoveryGate(lang)
	if diag.Supported {
		t.Fatal("diagnostic supported invalid small-table terminal action index")
	}
	if !strings.Contains(diag.Reason, "terminal action index") {
		t.Fatalf("diagnostic reason = %q, want terminal action index reason", diag.Reason)
	}
}

func TestCRecoveryGateAcceptsLargeStateGotos(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	lang := cRecoveryGateLanguage()
	lang.StateCount = 70002
	lang.LexModes = make([]LexMode, lang.StateCount)
	for i := range lang.LexModes {
		lang.LexModes[i].LexState = 0
	}
	lang.LargeStateGotos = map[uint64]StateID{
		largeStateGotoKey(1, 2): 70001,
	}
	validHash := largeStateGotosFingerprint(lang.LargeStateGotos)
	if !errorCostCompetitionLanguage(lang) {
		t.Fatalf("large-state nonterminal goto rejected: %+v", DiagnoseCRecoveryGate(lang))
	}

	lang.LargeStateGotos = map[uint64]StateID{
		largeStateGotoKey(1, 1): 70001,
	}
	if invalidHash := largeStateGotosFingerprint(lang.LargeStateGotos); invalidHash == validHash {
		t.Fatal("large-state goto fingerprint did not change after map contents changed")
	}
	diag := DiagnoseCRecoveryGate(lang)
	if errorCostCompetitionLanguage(lang) {
		t.Fatalf("terminal symbol accepted in large-state goto table: %+v", diag)
	}
	if !strings.Contains(diag.Reason, "large-state goto symbol is terminal") {
		t.Fatalf("diagnostic reason = %q, want terminal large-state goto reason", diag.Reason)
	}
}

func TestCRecoveryGateGrammargenRequiresExplicitCertification(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	lang := cRecoveryGateLanguage()
	lang.GeneratedByGrammargen = true
	lang.CRecoveryCostCompetitionCapable = true
	lang.CRecoveryCostCompetitionEnabledByDefault = false

	if errorCostCompetitionLanguage(lang) {
		t.Fatal("grammargen language default-enabled without explicit certification")
	}

	lang.CRecoveryCostCompetitionEnabledByDefault = true
	if !errorCostCompetitionLanguage(lang) {
		t.Fatal("explicitly certified grammargen language did not enable")
	}
}

func TestCRecoveryCertificationDefaultRequiresAttachedExternalScanner(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")

	noExternal := cRecoveryGateLanguage()
	noExternal.CRecoveryCostCompetitionCapable = false
	noExternal.CRecoveryCostCompetitionEnabledByDefault = false
	diag := CertifyCRecoveryCostCompetition(noExternal)
	if !diag.Supported {
		t.Fatalf("no-external diagnostic unsupported: %q", diag.Reason)
	}
	if !noExternal.CRecoveryCostCompetitionCapable {
		t.Fatal("no-external non-generated language was not marked capable")
	}
	if !noExternal.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("no-external non-generated language was not default-certified")
	}
	if !errorCostCompetitionLanguage(noExternal) {
		t.Fatal("no-external non-generated language did not enable parser gate")
	}

	externalWithoutScanner := cRecoveryGateLanguage()
	externalWithoutScanner.CRecoveryCostCompetitionCapable = false
	externalWithoutScanner.CRecoveryCostCompetitionEnabledByDefault = true
	externalWithoutScanner.ExternalSymbols = []Symbol{1}
	externalWithoutScanner.ExternalTokenCount = 1
	externalWithoutScanner.ExternalLexStates = [][]bool{{false}, {true}}
	diag = CertifyCRecoveryCostCompetition(externalWithoutScanner)
	if !diag.Supported {
		t.Fatalf("external diagnostic unsupported: %q", diag.Reason)
	}
	if !externalWithoutScanner.CRecoveryCostCompetitionCapable {
		t.Fatal("external non-generated language was not marked capable")
	}
	if externalWithoutScanner.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("external non-generated language default-certified without scanner attachment")
	}
	if errorCostCompetitionLanguage(externalWithoutScanner) {
		t.Fatal("external non-generated language enabled parser gate without scanner attachment")
	}

	externalWithoutScanner.ExternalScanner = cRecoveryGateScanner{}
	diag = CertifyCRecoveryCostCompetition(externalWithoutScanner)
	if !diag.Supported {
		t.Fatalf("external scanner diagnostic unsupported: %q", diag.Reason)
	}
	if !externalWithoutScanner.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("external non-generated language was not default-certified after scanner attachment")
	}
	if !errorCostCompetitionLanguage(externalWithoutScanner) {
		t.Fatal("external non-generated language did not enable parser gate after scanner attachment")
	}
}

func TestCRecoveryCertificationUsesDiagnoseGate(t *testing.T) {
	t.Setenv("GOT_C_RECOVERY", "")
	lang := cRecoveryGateLanguage()
	lang.CRecoveryCostCompetitionCapable = false
	lang.CRecoveryCostCompetitionEnabledByDefault = true
	lang.ParseActions = nil

	diag := CertifyCRecoveryCostCompetition(lang)
	if diag.Supported {
		t.Fatal("certification supported language rejected by DiagnoseCRecoveryGate")
	}
	if !strings.Contains(diag.Reason, "parse actions are empty") {
		t.Fatalf("diagnostic reason = %q, want parse actions failure", diag.Reason)
	}
	if lang.CRecoveryCostCompetitionCapable {
		t.Fatal("certification marked unsupported language capable")
	}
	if lang.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("certification left unsupported language default-enabled")
	}

	t.Setenv("GOT_C_RECOVERY", "all")
	if errorCostCompetitionLanguage(lang) {
		t.Fatal("GOT_C_RECOVERY=all bypassed DiagnoseCRecoveryGate")
	}
}

func TestSetLexerErrorRunLexStateUsesCRecoveryGate(t *testing.T) {
	lang := cRecoveryGateLanguage()
	lang.LexModes[0].SetLexStateIndex(7)
	lang.LexStates = make([]LexState, 8)

	t.Setenv("GOT_C_RECOVERY", "")
	lexer := &Lexer{}
	setLexerErrorRunLexState(lexer, lang)
	if !lexer.hasErrorRunLexState || lexer.errorRunLexState != 7 || !lexer.errorModeRetry {
		t.Fatalf("lexer gate not enabled: has=%v state=%d retry=%v", lexer.hasErrorRunLexState, lexer.errorRunLexState, lexer.errorModeRetry)
	}

	t.Setenv("GOT_C_RECOVERY", "0")
	lexer = &Lexer{}
	setLexerErrorRunLexState(lexer, lang)
	if lexer.hasErrorRunLexState || lexer.errorModeRetry {
		t.Fatalf("lexer gate enabled despite env disable: has=%v retry=%v", lexer.hasErrorRunLexState, lexer.errorModeRetry)
	}
}
