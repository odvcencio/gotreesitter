package gotreesitter

import (
	"crypto/sha256"
	"testing"
)

type recordingExternalScanner struct {
	seen [][]bool
}

func (s *recordingExternalScanner) Create() any { return nil }
func (s *recordingExternalScanner) Destroy(any) {}
func (s *recordingExternalScanner) Serialize(any, []byte) int {
	return 0
}
func (s *recordingExternalScanner) Deserialize(any, []byte) {}
func (s *recordingExternalScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	snapshot := append([]bool(nil), valid...)
	s.seen = append(s.seen, snapshot)
	for i, ok := range valid {
		if ok {
			switch i {
			case 0:
				lexer.SetResultSymbol(10)
			case 1:
				lexer.SetResultSymbol(20)
			}
			return true
		}
	}
	return false
}

type staleResultExternalScanner struct {
	result Symbol
}

type capabilityExternalScanner struct{ recordingExternalScanner }

func (*capabilityExternalScanner) SupportsIncrementalReuse() bool { return true }
func (*capabilityExternalScanner) SupportsIncrementalReuseFromErrorTree() bool {
	return false
}
func (*capabilityExternalScanner) UsesExternalScannerCheckpoints() bool {
	return true
}
func (*capabilityExternalScanner) CheckpointIdentity() (ExternalScannerCheckpointIdentity, bool) {
	return ExternalScannerCheckpointIdentity{
		Scanner: []byte("adapter-test-scanner-v1"),
		Grammar: []byte("adapter-test-grammar-v1"),
	}, true
}
func (*capabilityExternalScanner) RequiresIncrementalPrefixFrontierProof() bool {
	return true
}
func (*capabilityExternalScanner) AllowsIncrementalReuseWithoutCheckpoint() bool {
	return true
}
func (*capabilityExternalScanner) ExternalScannerIsStateless() bool { return true }
func (*capabilityExternalScanner) PreservesStateOnScanFailure() bool {
	return true
}

type retainingCapabilityExternalScanner struct{ recordingExternalScanner }

func (*retainingCapabilityExternalScanner) RetainsStateOnScanFailure() bool { return true }

func TestExternalScannerOrderAdapterPreservesOptionalCapabilities(t *testing.T) {
	scanner := &capabilityExternalScanner{}
	sourceLang := &Language{
		SymbolNames:     []string{"", "a"},
		ExternalSymbols: []Symbol{1},
		ExternalScanner: scanner,
	}
	targetLang := &Language{
		SymbolNames:     []string{"", "", "a"},
		ExternalSymbols: []Symbol{2},
	}
	targetGrammar := sha256.Sum256([]byte("adapter-target-grammar-v1"))
	targetLang.grammarBlobSHA256 = targetGrammar
	targetLang.grammarBlobSHA256Valid = true
	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	if reusable, ok := adapted.(IncrementalReuseExternalScanner); !ok || !reusable.SupportsIncrementalReuse() {
		t.Fatal("incremental-reuse capability was not preserved")
	}
	if errorTreeReusable, ok := adapted.(ErrorTreeIncrementalReuseExternalScanner); !ok ||
		errorTreeReusable.SupportsIncrementalReuseFromErrorTree() {
		t.Fatal("error-tree incremental reuse restriction was not preserved")
	}
	if checkpointed, ok := adapted.(CheckpointedExternalScanner); !ok || !checkpointed.UsesExternalScannerCheckpoints() {
		t.Fatal("checkpoint capability was not preserved")
	}
	if identityProvider, ok := adapted.(ExternalScannerCheckpointIdentityProvider); !ok {
		t.Fatal("checkpoint identity capability was not preserved")
	} else if got, valid := identityProvider.CheckpointIdentity(); !valid ||
		string(got.Scanner) != "adapter-test-scanner-v1" || string(got.Grammar) != string(targetGrammar[:]) {
		t.Fatalf("adapted checkpoint identity = %+v/%t, want the inner scanner and target grammar", got, valid)
	}
	if prefixSensitive, ok := adapted.(IncrementalPrefixFrontierExternalScanner); !ok || !prefixSensitive.RequiresIncrementalPrefixFrontierProof() {
		t.Fatal("incremental prefix-frontier capability was not preserved")
	}
	if checkpointless, ok := adapted.(CheckpointlessExternalScannerReuse); !ok || !checkpointless.AllowsIncrementalReuseWithoutCheckpoint() {
		t.Fatal("checkpointless capability was not preserved")
	}
	if stateless, ok := adapted.(StatelessExternalScanner); !ok || !stateless.ExternalScannerIsStateless() {
		t.Fatal("stateless capability was not preserved")
	}
	if preserving, ok := adapted.(FailurePreservingExternalScanner); !ok || !preserving.PreservesStateOnScanFailure() {
		t.Fatal("failure-preserving capability was not preserved")
	}
	if retaining, ok := adapted.(FailureStateRetainingExternalScanner); !ok || retaining.RetainsStateOnScanFailure() {
		t.Fatal("adapter reported failure retention for a preserving scanner")
	}
}

func TestExternalScannerOrderAdapterRequiresTargetGrammarCheckpointIdentity(t *testing.T) {
	sourceLang := &Language{
		SymbolNames:     []string{"", "a"},
		ExternalSymbols: []Symbol{1},
		ExternalScanner: &capabilityExternalScanner{},
	}
	targetLang := &Language{
		SymbolNames:     []string{"", "", "a"},
		ExternalSymbols: []Symbol{2},
	}
	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	provider, ok := adapted.(ExternalScannerCheckpointIdentityProvider)
	if !ok {
		t.Fatal("identity-bearing source did not produce an identity-bearing adapter")
	}
	if identity, valid := provider.CheckpointIdentity(); valid || identity.complete() {
		t.Fatalf("unauthenticated target identity = %+v/%t, want incomplete and invalid", identity, valid)
	}
	identity, required, valid := externalScannerCheckpointIdentityStatus(&Language{ExternalScanner: adapted})
	if !required || valid || identity.complete() {
		t.Fatalf("unauthenticated target status = %+v/%t/%t, want required and invalid", identity, required, valid)
	}
}

func TestExternalScannerOrderAdapterBindsAuthenticatedTargetAcrossNestedAdapters(t *testing.T) {
	sourceLang := &Language{
		SymbolNames:     []string{"", "a"},
		ExternalSymbols: []Symbol{1},
		ExternalScanner: &capabilityExternalScanner{},
	}
	intermediate := &Language{
		SymbolNames:     []string{"", "", "a"},
		ExternalSymbols: []Symbol{2},
	}
	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, intermediate)
	if !ok {
		t.Fatal("intermediate adapter not created")
	}
	intermediate.ExternalScanner = adapted

	targetGrammar := sha256.Sum256([]byte("nested-adapter-target-grammar-v1"))
	target := &Language{
		SymbolNames:            []string{"", "", "", "a"},
		ExternalSymbols:        []Symbol{3},
		grammarBlobSHA256:      targetGrammar,
		grammarBlobSHA256Valid: true,
	}
	adapted, ok = AdaptExternalScannerByExternalOrder(intermediate, target)
	if !ok {
		t.Fatal("target adapter not created")
	}
	identity, required, valid := externalScannerCheckpointIdentityStatus(&Language{ExternalScanner: adapted})
	if !required || !valid || string(identity.Scanner) != "adapter-test-scanner-v1" ||
		string(identity.Grammar) != string(targetGrammar[:]) {
		t.Fatalf("nested target status = %+v/%t/%t, want the source scanner and target grammar", identity, required, valid)
	}
}

func TestExternalScannerOrderAdapterKeepsLegacyCheckpointIdentityOptional(t *testing.T) {
	sourceLang := &Language{
		ExternalSymbols: []Symbol{1},
		ExternalScanner: c26qLegacyCheckpointScanner{},
	}
	targetLang := &Language{ExternalSymbols: []Symbol{2}}
	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	identity, required, valid := externalScannerCheckpointIdentityStatus(&Language{ExternalScanner: adapted})
	if required || !valid || len(identity.Scanner) != 0 || len(identity.Grammar) != 0 {
		t.Fatalf("legacy adapted identity status = %+v/%t/%t, want optional and valid", identity, required, valid)
	}
}

func TestExternalScannerOrderAdapterPreservesFailureRetention(t *testing.T) {
	scanner := &retainingCapabilityExternalScanner{}
	sourceLang := &Language{
		SymbolNames:     []string{"", "a"},
		ExternalSymbols: []Symbol{1},
		ExternalScanner: scanner,
	}
	targetLang := &Language{
		SymbolNames:     []string{"", "", "a"},
		ExternalSymbols: []Symbol{2},
	}
	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	if retaining, ok := adapted.(FailureStateRetainingExternalScanner); !ok || !retaining.RetainsStateOnScanFailure() {
		t.Fatal("failure-state-retaining capability was not preserved")
	}
	if preserving, ok := adapted.(FailurePreservingExternalScanner); !ok || preserving.PreservesStateOnScanFailure() {
		t.Fatal("adapter reported failure preservation for a retaining scanner")
	}
}

func (s staleResultExternalScanner) Create() any { return nil }
func (s staleResultExternalScanner) Destroy(any) {}
func (s staleResultExternalScanner) Serialize(any, []byte) int {
	return 0
}
func (s staleResultExternalScanner) Deserialize(any, []byte) {}
func (s staleResultExternalScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	lexer.SetResultSymbol(s.result)
	return true
}

func TestExternalScannerOrderAdapterReusesAndClearsSourceValid(t *testing.T) {
	scanner := &recordingExternalScanner{}
	sourceLang := &Language{
		SymbolNames:     []string{"", "", "", "", "", "", "", "", "", "", "a", "", "", "", "", "", "", "", "", "", "b"},
		ExternalSymbols: []Symbol{10, 20},
		ExternalScanner: scanner,
	}
	targetNames := make([]string, 201)
	targetNames[100] = "a"
	targetNames[200] = "b"
	targetLang := &Language{
		SymbolNames:     targetNames,
		ExternalSymbols: []Symbol{100, 200},
	}
	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	payload := adapted.Create()
	defer adapted.Destroy(payload)

	lexer := &ExternalLexer{}
	if !adapted.Scan(payload, lexer, []bool{false, true}) {
		t.Fatal("first scan failed")
	}
	if lexer.resultSymbol != 200 {
		t.Fatalf("first result symbol = %d, want 200", lexer.resultSymbol)
	}
	lexer = &ExternalLexer{}
	if !adapted.Scan(payload, lexer, []bool{true, false}) {
		t.Fatal("second scan failed")
	}
	if lexer.resultSymbol != 100 {
		t.Fatalf("second result symbol = %d, want 100", lexer.resultSymbol)
	}
	if len(scanner.seen) != 2 {
		t.Fatalf("scanner saw %d calls, want 2", len(scanner.seen))
	}
	if got := scanner.seen[0]; len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("first source valid = %v, want [false true]", got)
	}
	if got := scanner.seen[1]; len(got) != 2 || !got[0] || got[1] {
		t.Fatalf("second source valid = %v, want [true false]", got)
	}
}

func TestExternalScannerOrderAdapterRejectsInvalidSourceResult(t *testing.T) {
	sourceLang := &Language{
		SymbolNames:     []string{"", "", "", "", "", "", "", "", "", "", "", "a", "", "", "", "", "", "", "", "", "", "", "b"},
		ExternalSymbols: []Symbol{11, 22},
		ExternalScanner: staleResultExternalScanner{result: 11},
	}
	targetNames := make([]string, 201)
	targetNames[100] = "a"
	targetNames[200] = "b"
	targetLang := &Language{
		SymbolNames:     targetNames,
		ExternalSymbols: []Symbol{100, 200},
	}
	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	payload := adapted.Create()
	defer adapted.Destroy(payload)

	lexer := &ExternalLexer{}
	if adapted.Scan(payload, lexer, []bool{false, true}) {
		t.Fatal("scan succeeded with invalid source result, want false")
	}
	if lexer.hasResult {
		t.Fatalf("lexer result was preserved after invalid source result: %d", lexer.resultSymbol)
	}
}

func TestExternalScannerOrderAdapterLeavesNamedMismatchUnmapped(t *testing.T) {
	sourceLang := &Language{
		SymbolNames:     []string{"", "", "", "", "", "", "", "", "", "", "a", "source_only"},
		ExternalSymbols: []Symbol{10, 11},
		ExternalScanner: &recordingExternalScanner{},
	}
	targetNames := make([]string, 21)
	targetNames[20] = "target_only"
	targetNames[10] = "a"
	targetLang := &Language{
		SymbolNames:     targetNames,
		ExternalSymbols: []Symbol{20, 10},
	}
	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	orderAdapter, ok := adapted.(*externalScannerOrderAdapter)
	if !ok {
		t.Fatalf("adapter type = %T, want *externalScannerOrderAdapter", adapted)
	}
	if got, want := orderAdapter.targetToSource, []int{-1, 0}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("targetToSource = %v, want %v", got, want)
	}
}

func TestExternalScannerOrderAdapterUsesGeneratedOrderBeforeNameMatches(t *testing.T) {
	scanner := &recordingExternalScanner{}
	sourceNames := make([]string, 21)
	sourceNames[10] = "_source_external"
	sourceNames[20] = "shared_display_name"
	sourceLang := &Language{
		SymbolNames:     sourceNames,
		ExternalSymbols: []Symbol{10, 20},
		ExternalScanner: scanner,
	}
	targetNames := make([]string, 201)
	targetNames[100] = "shared_display_name"
	targetNames[200] = "target_alias"
	targetLang := &Language{
		GeneratedByGrammargen: true,
		SymbolNames:           targetNames,
		ExternalSymbols:       []Symbol{100, 200},
	}

	adapted, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	orderAdapter, ok := adapted.(*externalScannerOrderAdapter)
	if !ok {
		t.Fatalf("adapter type = %T, want *externalScannerOrderAdapter", adapted)
	}
	if got, want := orderAdapter.targetToSource, []int{0, 1}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("targetToSource = %v, want %v", got, want)
	}

	payload := adapted.Create()
	defer adapted.Destroy(payload)
	lexer := &ExternalLexer{}
	if !adapted.Scan(payload, lexer, []bool{false, true}) {
		t.Fatal("scan failed for alias-renamed external")
	}
	if got, want := lexer.resultSymbol, Symbol(200); got != want {
		t.Fatalf("result symbol = %d, want %d", got, want)
	}
	if len(scanner.seen) != 1 {
		t.Fatalf("scanner saw %d calls, want 1", len(scanner.seen))
	}
	if got := scanner.seen[0]; len(got) != 2 || got[0] || !got[1] {
		t.Fatalf("source valid = %v, want [false true]", got)
	}
}

func TestExternalScannerOrderAdapterMapsGeneratedExternalLexStatesByOrder(t *testing.T) {
	sourceLang := &Language{
		SymbolNames:     []string{"", "", "", "", "", "", "", "", "", "", "a", "", "", "", "", "", "", "", "", "", "b"},
		ExternalSymbols: []Symbol{10, 20},
		ExternalScanner: &recordingExternalScanner{},
		ExternalLexStates: [][]bool{
			{false, false},
			{true, false},
			{false, true},
			{true, true},
		},
	}
	targetNames := make([]string, 201)
	targetNames[100] = "b"
	targetNames[200] = "a"
	targetLang := &Language{
		GeneratedByGrammargen: true,
		SymbolNames:           targetNames,
		ExternalSymbols:       []Symbol{100, 200},
	}
	if _, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang); !ok {
		t.Fatal("adapter not created")
	}

	if got := len(targetLang.ExternalLexStates); got != len(sourceLang.ExternalLexStates) {
		t.Fatalf("ExternalLexStates rows = %d, want %d", got, len(sourceLang.ExternalLexStates))
	}
	want := [][]bool{
		{false, false},
		{true, false},
		{false, true},
		{true, true},
	}
	for i := range want {
		if len(targetLang.ExternalLexStates[i]) != len(want[i]) {
			t.Fatalf("row %d len = %d, want %d", i, len(targetLang.ExternalLexStates[i]), len(want[i]))
		}
		for j := range want[i] {
			if targetLang.ExternalLexStates[i][j] != want[i][j] {
				t.Fatalf("ExternalLexStates[%d][%d] = %v, want %v; rows=%v",
					i, j, targetLang.ExternalLexStates[i][j], want[i][j], targetLang.ExternalLexStates)
			}
		}
	}
}

func TestExternalScannerOrderAdapterPreservesGeneratedExternalLexStateRows(t *testing.T) {
	sourceLang := &Language{
		SymbolNames:     []string{"", "", "", "", "", "", "", "", "", "", "a", "", "", "", "", "", "", "", "", "", "b"},
		ExternalSymbols: []Symbol{10, 20},
		ExternalScanner: &recordingExternalScanner{},
		ExternalLexStates: [][]bool{
			{false, false},
			{true, false},
		},
	}
	targetNames := make([]string, 201)
	targetNames[100] = "b"
	targetNames[200] = "a"
	targetRows := [][]bool{
		{false, false},
		{true, true},
		{false, true},
	}
	targetLang := &Language{
		GeneratedByGrammargen:           true,
		CRecoveryCostCompetitionCapable: true,
		SymbolNames:                     targetNames,
		ExternalSymbols:                 []Symbol{100, 200},
		ExternalLexStates:               targetRows,
	}
	addCRecoveryRuntimeSurfaceForAdapterTest(targetLang, len(targetNames))
	scanner, ok := AdaptExternalScannerByExternalOrder(sourceLang, targetLang)
	if !ok {
		t.Fatal("adapter not created")
	}
	if len(targetLang.ExternalLexStates) != len(targetRows) {
		t.Fatalf("ExternalLexStates rows = %d, want %d", len(targetLang.ExternalLexStates), len(targetRows))
	}
	for i := range targetRows {
		for j := range targetRows[i] {
			if targetLang.ExternalLexStates[i][j] != targetRows[i][j] {
				t.Fatalf("ExternalLexStates[%d][%d] = %v, want preserved %v; rows=%v",
					i, j, targetLang.ExternalLexStates[i][j], targetRows[i][j], targetLang.ExternalLexStates)
			}
		}
	}
	if targetLang.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("generated capable language default-certified before scanner attachment")
	}
	targetLang.ExternalScanner = scanner
	CertifyCRecoveryCostCompetition(targetLang)
	if !targetLang.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("generated capable language was not default-certified after scanner attachment")
	}
}

func addCRecoveryRuntimeSurfaceForAdapterTest(lang *Language, symbolCount int) {
	lang.InitialState = 1
	lang.StateCount = 2
	lang.SymbolCount = uint32(symbolCount)
	lang.TokenCount = uint32(symbolCount)
	lang.SymbolMetadata = make([]SymbolMetadata, symbolCount)
	lang.ParseTable = [][]uint16{
		make([]uint16, symbolCount),
		make([]uint16, symbolCount),
	}
	lang.ParseActions = []ParseActionEntry{
		{},
		{Actions: []ParseAction{{Type: ParseActionRecover, State: 0}}},
	}
	lang.LexModes = []LexMode{{LexState: 0}, {LexState: 0}}
	lang.LexStates = []LexState{{Default: -1, EOF: -1}}
}
