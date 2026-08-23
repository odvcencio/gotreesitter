//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	g18AlternativeSetTargetManifestSHA256 = "3a659138e3aac82563e5fdf46aeb3041a685d6311f32dfd6660c0337b02dbb0d"
	g18AlternativeSetFlagDenominator      = 15
	g18AlternativeSetGrammarDenominator   = 12
)

var g18AlternativeSetRouteGrammars = []string{
	"go", "erlang", "haskell", "javascript", "bash", "perl", "ada", "python",
}

type g18AlternativeSetTarget struct {
	grammar       string
	name          string
	source        []byte
	sourceSHA256  string
	load          func() *gotreesitter.Language
	wantReason    string
	wantCensus    gotreesitter.DiagnosticParserCoreThreeProofCensusTotals
	wantAdaImport bool
}

type g18CohortVerifierTelemetryProvider interface {
	DiagnosticDropCohortVerifierTelemetryForTest() []byte
}

// g18CohortVerifierReceiptProvider exposes one diagnostic receipt per election.
// The future parser must publish this data beside aggregate telemetry.
type g18CohortVerifierReceiptProvider interface {
	DiagnosticDropCohortVerifierReceiptsForTest() []byte
}

// g18DiagnosticDropCohortReceiptProvider is owned by the diagnostic runner.
// It must expose canonical bytes from that runner's own Core instance.
type g18DiagnosticProvider interface {
	DiagnosticDropCohortSessionForTest() (func() (uint64, uint64, error), func() (uint64, uint64, []byte, error), func(), error)
}

type g18DiagnosticProviderCompileContract struct{}

func (*g18DiagnosticProviderCompileContract) DiagnosticDropCohortSessionForTest() (func() (uint64, uint64, error), func() (uint64, uint64, []byte, error), func(), error) {
	return func() (uint64, uint64, error) { return 0, 0, nil }, func() (uint64, uint64, []byte, error) { return 1, 1, []byte("contract"), nil }, func() {}, nil
}

var _ g18DiagnosticProvider = (*g18DiagnosticProviderCompileContract)(nil)

// g18CertificateAdmissionActivator is private and test-only. Production
// profile flags never enable it. The returned restore function must run after
// one parser attempt and before the parser is reused.
type g18CertificateAdmissionActivator interface {
	DiagnosticEnableDropCohortCertificateAdmissionForTest() func()
}

// g18CertificateAdmissionSuppressor is a one-shot, test-only seam. Only the
// missing-certificate fallback test may call it.
type g18CertificateAdmissionSuppressor interface {
	DiagnosticSuppressDropCohortCertificateForTest()
}

type g18CertificateAdmissionNegativeProvider interface {
	DiagnosticInvalidateDropCohortCertificateForTest()
}

type g18CohortVerifierReceipt struct {
	ArenaOwner     uint64 `json:"arena_owner"`
	ArenaEpoch     uint64 `json:"arena_epoch"`
	CohortSequence uint64 `json:"cohort_sequence"`
	Verdict        string `json:"verdict"`
	Classification string `json:"classification"`
}

type g18CohortVerifierTelemetry struct {
	Schema                      string            `json:"schema"`
	ArenaOwner                  uint64            `json:"arena_owner"`
	ArenaEpoch                  uint64            `json:"arena_epoch"`
	VerifierElections           uint64            `json:"verifier_elections"`
	VerifierProofs              uint64            `json:"verifier_proofs"`
	VerifierDeclines            uint64            `json:"verifier_declines"`
	ProfileBypasses             uint64            `json:"profile_bypasses"`
	ActionIdentityDeclines      uint64            `json:"action_identity_declines"`
	DerivationIdentityDeclines  uint64            `json:"derivation_identity_declines"`
	DeclineReasons              map[string]uint64 `json:"decline_reasons"`
	AuthenticatedHistoryImports uint64            `json:"authenticated_history_imports"`
	UnprovedHistoryImports      uint64            `json:"unproved_history_imports"`
	ProducerWrites              map[string]uint64 `json:"producer_writes"`
}

// TestG18AlternativeSetCurrentFallbackCharacterization pins current main.
// Each shipped grant routes and matches locked C. A grant-free clone must use
// the exact fallback and census recorded before the future design changes it.
func TestG18AlternativeSetCurrentFallbackCharacterization(t *testing.T) {
	restoreCensus := gotreesitter.SetDiagnosticParserCoreShadowCensusEnabledForTest(true)
	defer restoreCensus()

	groups := make(map[string][]g18AlternativeSetTarget)
	for _, target := range g18AlternativeSetTargets(t) {
		groups[target.grammar] = append(groups[target.grammar], target)
	}
	for _, grammar := range g18AlternativeSetRouteGrammars {
		grammar := grammar
		t.Run(grammar, func(t *testing.T) {
			for _, target := range groups[grammar] {
				target := target
				t.Run(target.name, func(t *testing.T) {
					runG18AlternativeSetCurrentFallback(t, target)
				})
			}
		})
	}
}

func runG18AlternativeSetCurrentFallback(t *testing.T, target g18AlternativeSetTarget) {
	t.Helper()
	if got := fmt.Sprintf("%x", sha256.Sum256(target.source)); got != target.sourceSHA256 {
		t.Fatalf("source SHA-256 = %s, want %s", got, target.sourceSHA256)
	}

	cLanguage, err := COracleLanguage(target.grammar)
	if err != nil {
		t.Fatalf("load %s locked C language: %v", target.grammar, err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set %s locked C language: %v", target.grammar, err)
	}
	cTree := cParser.Parse(target.source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatalf("%s locked C parse returned no root", target.grammar)
	}
	t.Cleanup(cTree.Close)

	grantedLanguage := g18CloneLanguage(target.load())
	if !grantedLanguage.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("shipped profile does not carry the converged-split grant")
	}
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	grantedParser := gotreesitter.NewParser(grantedLanguage)
	grantedParser.SetAdmissionCandidateRoute(true)
	grantedTree, err := grantedParser.Parse(target.source)
	if err != nil {
		t.Fatalf("granted compact parse: %v", err)
	}
	t.Cleanup(grantedTree.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
		t.Fatalf("granted counters = %d/%d to %d/%d; want routed +1; reason=%q", routedBefore, fallbackBefore, routedAfter, fallbackAfter, gotreesitter.AdmissionCandidateLastFallbackReason())
	}
	assertG18LockedCExact(t, "granted compact", grantedTree, grantedLanguage, cTree)

	grantFreeLanguage := g18CloneLanguage(target.load())
	grantFreeLanguage.CompactConvergedReductionSplitDropsCertified = false
	productionParser := gotreesitter.NewParser(grantFreeLanguage)
	productionParser.SetAdmissionCandidateRoute(false)
	productionTree, err := productionParser.Parse(target.source)
	if err != nil {
		t.Fatalf("grant-free production parse: %v", err)
	}
	t.Cleanup(productionTree.Release)
	gotreesitter.DiagnosticParserCoreShadowCensusResetForTest()
	routedBefore, fallbackBefore = gotreesitter.AdmissionCandidateCounters()
	candidateParser := gotreesitter.NewParser(grantFreeLanguage)
	candidateParser.SetAdmissionCandidateRoute(true)
	defaultTelemetryProvider, ok := any(candidateParser).(g18CohortVerifierTelemetryProvider)
	if !ok {
		t.Log("RED boundary: candidate parser does not yet publish default-disabled verifier telemetry")
	}
	var defaultTelemetry []byte
	if ok {
		defaultTelemetry = append([]byte(nil), defaultTelemetryProvider.DiagnosticDropCohortVerifierTelemetryForTest()...)
	}
	candidateTree, err := candidateParser.Parse(target.source)
	if err != nil {
		t.Fatalf("grant-free candidate parse: %v", err)
	}
	t.Cleanup(candidateTree.Release)
	routedAfter, fallbackAfter = gotreesitter.AdmissionCandidateCounters()
	reason := gotreesitter.AdmissionCandidateLastFallbackReason()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Fatalf("grant-free counters = %d/%d to %d/%d; want fallback +1; reason=%q", routedBefore, fallbackBefore, routedAfter, fallbackAfter, reason)
	}
	if ok && string(defaultTelemetryProvider.DiagnosticDropCohortVerifierTelemetryForTest()) != string(defaultTelemetry) {
		t.Fatalf("default-disabled verifier telemetry changed: before=%s after=%s", defaultTelemetry, defaultTelemetryProvider.DiagnosticDropCohortVerifierTelemetryForTest())
	}
	if !strings.Contains(reason, target.wantReason) {
		t.Fatalf("grant-free reason = %q, want substring %q", reason, target.wantReason)
	}
	census := gotreesitter.DiagnosticParserCoreThreeProofCensusSnapshotForTest()
	if census != target.wantCensus {
		t.Fatalf("grant-free census = %+v, want %+v", census, target.wantCensus)
	}
	assertG18GoTreesExact(t, candidateTree, productionTree, grantFreeLanguage)
	assertG18LockedCExact(t, "grant-free fallback", candidateTree, grantFreeLanguage, cTree)
	t.Logf("grant-free route=0/1 reason=%q census=%+v", reason, census)
	if target.wantAdaImport && !strings.Contains(reason, "unproved historical boundary resurrection") {
		t.Fatalf("Ada fallback did not reach the F4 history veto: %q", reason)
	}
}

// TestG18AlternativeSetCertificateRED states the future behavior. Every
// grant-free source must route directly, match locked C, and publish the exact
// cohort-verifier telemetry. Current main fails this contract by design.
func TestG18AlternativeSetCertificateRED(t *testing.T) {
	groups := make(map[string][]g18AlternativeSetTarget)
	for _, target := range g18AlternativeSetTargets(t) {
		groups[target.grammar] = append(groups[target.grammar], target)
	}
	for _, grammar := range g18AlternativeSetRouteGrammars {
		grammar := grammar
		t.Run(grammar, func(t *testing.T) {
			for _, target := range groups[grammar] {
				target := target
				t.Run(target.name, func(t *testing.T) {
					runG18AlternativeSetFutureContractRED(t, target)
				})
			}
		})
	}
}

func runG18AlternativeSetFutureContractRED(t *testing.T, target g18AlternativeSetTarget) {
	t.Helper()
	if got := fmt.Sprintf("%x", sha256.Sum256(target.source)); got != target.sourceSHA256 {
		t.Fatalf("source SHA-256 = %s, want %s", got, target.sourceSHA256)
	}
	cLanguage, err := COracleLanguage(target.grammar)
	if err != nil {
		t.Fatalf("load %s locked C language: %v", target.grammar, err)
	}
	cParser := sitter.NewParser()
	t.Cleanup(cParser.Close)
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatalf("set %s locked C language: %v", target.grammar, err)
	}
	cTree := cParser.Parse(target.source, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatalf("%s locked C parse returned no root", target.grammar)
	}
	t.Cleanup(cTree.Close)

	language := g18CloneLanguage(target.load())
	language.CompactConvergedReductionSplitDropsCertified = false
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	activator, ok := any(parser).(g18CertificateAdmissionActivator)
	if !ok {
		t.Errorf("RED: parser does not expose the private certificate-admission activation seam")
		return
	}
	restoreAdmission := activator.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	var restoreOnce sync.Once
	restore := func() { restoreOnce.Do(restoreAdmission) }
	t.Cleanup(restore)
	tree, err := parser.Parse(target.source)
	if err != nil {
		t.Fatalf("future grant-free parse: %v", err)
	}
	t.Cleanup(tree.Release)
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
		t.Errorf(
			"RED: grant-free counters = %d/%d to %d/%d; want direct route +1 with no fallback; reason=%q",
			routedBefore,
			fallbackBefore,
			routedAfter,
			fallbackAfter,
			gotreesitter.AdmissionCandidateLastFallbackReason(),
		)
	}
	if err := g18LockedCExactError(tree, language, cTree); err != nil {
		t.Errorf("RED: grant-free direct tree differs from locked C: %v", err)
	}

	provider, ok := any(parser).(g18CohortVerifierTelemetryProvider)
	if !ok {
		t.Errorf("RED: parser does not publish drop-cohort verifier telemetry")
		return
	}
	var telemetry g18CohortVerifierTelemetry
	if err := json.Unmarshal(provider.DiagnosticDropCohortVerifierTelemetryForTest(), &telemetry); err != nil {
		t.Fatalf("decode drop-cohort verifier telemetry: %v", err)
	}
	g18RequireFutureVerifierTelemetry(t, target, telemetry)
	telemetryBeforeRestore := append([]byte(nil), provider.DiagnosticDropCohortVerifierTelemetryForTest()...)
	// Restore the private seam before the cached parser runs again. The next
	// candidate attempt must fall back and must not consume the certificate.
	restore()
	routedBefore, fallbackBefore = gotreesitter.AdmissionCandidateCounters()
	secondTree, secondErr := parser.Parse(target.source)
	if secondErr != nil {
		t.Fatalf("restored candidate parse: %v", secondErr)
	}
	secondTree.Release()
	routedAfter, fallbackAfter = gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Errorf("RED: restored activation leaked across cached parser run: routed=%d/%d fallback=%d/%d", routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}
	if got := string(provider.DiagnosticDropCohortVerifierTelemetryForTest()); got != string(telemetryBeforeRestore) {
		t.Fatalf("restored-disabled verifier telemetry changed: before=%s after=%s", telemetryBeforeRestore, got)
	}
}

func g18RequireFutureVerifierTelemetry(t *testing.T, target g18AlternativeSetTarget, telemetry g18CohortVerifierTelemetry) {
	t.Helper()
	if telemetry.Schema != "gts-drop-cohort-verifier/v1" || telemetry.ArenaOwner == 0 ||
		telemetry.ArenaEpoch == 0 || telemetry.VerifierElections == 0 ||
		telemetry.VerifierProofs != telemetry.VerifierElections ||
		telemetry.VerifierDeclines != 0 || telemetry.ProfileBypasses != 0 ||
		telemetry.ActionIdentityDeclines != 0 || telemetry.DerivationIdentityDeclines != 0 ||
		telemetry.UnprovedHistoryImports != 0 {
		t.Fatalf("future verifier telemetry = %+v, want one proof per election", telemetry)
	}
	for reason, count := range telemetry.DeclineReasons {
		if count != 0 {
			t.Fatalf("future positive decline reason %q=%d", reason, count)
		}
	}
	if len(telemetry.ProducerWrites) == 0 {
		t.Fatal("future verifier telemetry has no producer-path writes")
	}
	allowedProducers := map[string]bool{
		"reduction_establishment": true,
		"linear_canonicalizer":    true,
		"mapped_canonicalizer":    true,
		"sibling_adoption":        true,
		"conflict_reconciliation": true,
		"dead_history_import":     true,
	}
	for producer, count := range telemetry.ProducerWrites {
		if !allowedProducers[producer] {
			t.Fatalf("future verifier published unknown producer %q=%d", producer, count)
		}
		if count == 0 {
			t.Fatalf("future verifier producer write %q = %d", producer, count)
		}
	}
	if telemetry.ProducerWrites["reduction_establishment"] == 0 {
		t.Fatal("future verifier telemetry has no reduction establishment")
	}
	if target.wantAdaImport && telemetry.AuthenticatedHistoryImports == 0 {
		t.Fatal("Ada future verifier telemetry has no authenticated history import")
	}
	if target.wantAdaImport && telemetry.ProducerWrites["dead_history_import"] == 0 {
		t.Fatal("Ada future verifier telemetry has no dead history producer write")
	}
	if !target.wantAdaImport && telemetry.AuthenticatedHistoryImports != 0 {
		t.Fatalf("unexpected authenticated history imports = %d", telemetry.AuthenticatedHistoryImports)
	}
}

func TestG18CertificateAdmissionMissingFallsBackRED(t *testing.T) {
	runG18CertificateAdmissionDeclineRED(t, false, func(t *testing.T, parser *gotreesitter.Parser) {
		suppressor, ok := any(parser).(g18CertificateAdmissionSuppressor)
		if !ok {
			t.Fatal("RED: missing one-shot certificate suppression seam")
		}
		suppressor.DiagnosticSuppressDropCohortCertificateForTest()
	})
}

func TestG18CertificateAdmissionInvalidFallsBackRED(t *testing.T) {
	runG18CertificateAdmissionDeclineRED(t, true, nil)
}

func runG18CertificateAdmissionDeclineRED(t *testing.T, invalid bool, prepare func(*testing.T, *gotreesitter.Parser)) {
	targets := g18AlternativeSetTargets(t)
	if len(targets) == 0 {
		t.Fatal("missing G18 target")
	}
	target := targets[0]
	language := g18CloneLanguage(target.load())
	language.CompactConvergedReductionSplitDropsCertified = false
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	activator, ok := any(parser).(g18CertificateAdmissionActivator)
	if !ok {
		t.Fatal("RED: missing private certificate activation seam")
	}
	restore := activator.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	var restoreAdmissionOnce sync.Once
	restoreAdmission := func() { restoreAdmissionOnce.Do(restore) }
	t.Cleanup(restoreAdmission)
	if prepare != nil {
		prepare(t, parser)
	}
	if invalid {
		invalidator, ok := any(parser).(g18CertificateAdmissionNegativeProvider)
		if !ok {
			t.Fatal("RED: missing invalid-certificate test hook")
		}
		invalidator.DiagnosticInvalidateDropCohortCertificateForTest()
	}
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	tree, err := parser.Parse(target.source)
	if err != nil {
		t.Fatal(err)
	}
	tree.Release()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
		t.Fatalf("certificate decline route=%d/%d fallback=%d/%d, want unchanged route and fallback +1", routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}
}

func TestG18CertificateAdmissionNonCandidateAndDiagnosticIgnoreRED(t *testing.T) {
	targets := g18AlternativeSetTargets(t)
	if len(targets) == 0 {
		t.Fatal("missing G18 target")
	}
	language := g18CloneLanguage(targets[0].load())
	parser := gotreesitter.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	activator, ok := any(parser).(g18CertificateAdmissionActivator)
	if !ok {
		t.Fatal("RED: missing private certificate activation seam")
	}
	restore := activator.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	t.Cleanup(restore)
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	tree, err := parser.Parse(targets[0].source)
	if err != nil {
		t.Fatal(err)
	}
	tree.Release()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore {
		t.Fatalf("non-candidate parser consumed certificate: routed=%d/%d fallback=%d/%d", routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}
}

func TestG18CertificateAdmissionDiagnosticRunnerIgnoresRED(t *testing.T) {
	routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
	// The diagnostic runner owns this snapshot. The dedicated API must expose
	// canonical bytes from the same runner Core before bounded execution.
	parser := gotreesitter.NewParser(grammars.GoLanguage())
	provider, ok := any(parser).(g18DiagnosticProvider)
	if !ok {
		t.Errorf("RED: production parser lacks primitive diagnostic drop-cohort session provider")
		return
	}
	run, snapshot, closeSession, err := provider.DiagnosticDropCohortSessionForTest()
	if err != nil {
		t.Fatalf("diagnostic session: %v", err)
	}
	if run == nil || snapshot == nil || closeSession == nil {
		t.Fatalf("diagnostic provider returned a nil session closure")
	}
	var closeOnce sync.Once
	guardedClose := func() { closeOnce.Do(closeSession) }
	defer guardedClose()
	ownerBefore, generationBefore, telemetryBefore, err := snapshot()
	if err != nil {
		t.Fatalf("diagnostic snapshot before: %v", err)
	}
	routeDelta, fallbackDelta, err := run()
	if err != nil {
		t.Fatalf("diagnostic runner: %v", err)
	}
	ownerAfter, generationAfter, telemetryAfter, err := snapshot()
	if err != nil {
		t.Fatalf("diagnostic snapshot after: %v", err)
	}
	if ownerAfter != ownerBefore || generationAfter != generationBefore {
		t.Fatalf("diagnostic session swapped Core identity")
	}
	if routeDelta != 0 || fallbackDelta != 0 {
		t.Fatalf("diagnostic session changed route/fallback: %d/%d", routeDelta, fallbackDelta)
	}
	if string(telemetryAfter) != string(telemetryBefore) {
		t.Fatalf("diagnostic runner verifier telemetry changed: before=%s after=%s", telemetryBefore, telemetryAfter)
	}
	guardedClose()
	guardedClose()
	routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
	if routedAfter != routedBefore || fallbackAfter != fallbackBefore {
		t.Fatalf("diagnostic runner consumed certificate state: route=%d/%d fallback=%d/%d", routedBefore, routedAfter, fallbackBefore, fallbackAfter)
	}
}

// TestG18AlternativeSetKotlinFutureDeclineTelemetryRED binds both opposite
// controls to the future real verifier. Current main does not publish this
// telemetry. The future verifier must record both identity decline classes.
func TestG18AlternativeSetKotlinFutureDeclineTelemetryRED(t *testing.T) {
	tests := []struct {
		name         string
		source       []byte
		sourceSHA256 string
	}{
		{
			name:         "annotated_getter_line",
			source:       []byte("@Deprecated(\"old\")\nval Int.double: Int\n    get() = this * 2\n// trailing\n"),
			sourceSHA256: "7aad51909730adbec84060f5445384ff82c9517e3603fb0da86c9ac2397548b7",
		},
		{
			name:         "annotated_getter_block",
			source:       []byte("@Deprecated(\"old\")\nval Int.double: Int\n    get() = this * 2\n/* trailing */\n"),
			sourceSHA256: "3c745d430894c3c739f217b807f747a4bd2f521173b441e625e4ca109b773ec2",
		},
	}
	cLanguage, err := COracleLanguage("kotlin")
	if err != nil {
		t.Fatalf("load Kotlin locked C language: %v", err)
	}
	var actionDeclines uint64
	var derivationDeclines uint64
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(test.source)); got != test.sourceSHA256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, test.sourceSHA256)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatalf("set Kotlin locked C language: %v", err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("Kotlin locked C parse returned no root")
			}
			t.Cleanup(cTree.Close)

			language := g18CloneLanguage(grammars.KotlinLanguage())
			language.CompactConvergedReductionSplitDropsCertified = false
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			parser := gotreesitter.NewParser(language)
			parser.SetAdmissionCandidateRoute(true)
			activator, ok := any(parser).(g18CertificateAdmissionActivator)
			if !ok {
				t.Fatal("RED: missing Kotlin certificate-admission activation seam")
			}
			restoreAdmission := activator.DiagnosticEnableDropCohortCertificateAdmissionForTest()
			var restoreOnce sync.Once
			restore := func() { restoreOnce.Do(restoreAdmission) }
			t.Cleanup(restore)
			tree, err := parser.Parse(test.source)
			if err != nil {
				t.Fatalf("Kotlin future decline parse: %v", err)
			}
			t.Cleanup(tree.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
				t.Fatalf("Kotlin future decline counters = %d/%d to %d/%d; want fallback +1", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			}
			assertG18LockedCExact(t, "Kotlin future decline", tree, language, cTree)

			provider, ok := any(parser).(g18CohortVerifierTelemetryProvider)
			if !ok {
				t.Errorf("RED: parser does not publish Kotlin verifier decline telemetry")
				return
			}
			var telemetry g18CohortVerifierTelemetry
			if err := json.Unmarshal(provider.DiagnosticDropCohortVerifierTelemetryForTest(), &telemetry); err != nil {
				t.Fatalf("decode Kotlin verifier telemetry: %v", err)
			}
			if telemetry.Schema != "gts-drop-cohort-verifier/v1" || telemetry.ArenaOwner == 0 ||
				telemetry.ArenaEpoch == 0 || telemetry.VerifierElections == 0 ||
				telemetry.VerifierProofs+telemetry.VerifierDeclines != telemetry.VerifierElections ||
				telemetry.VerifierProofs != 0 || telemetry.VerifierDeclines == 0 ||
				telemetry.ProfileBypasses != 0 {
				t.Fatalf("Kotlin future decline telemetry = %+v", telemetry)
			}
			classified := telemetry.ActionIdentityDeclines + telemetry.DerivationIdentityDeclines
			if classified != telemetry.VerifierDeclines ||
				telemetry.DeclineReasons["action_identity_mismatch"] != telemetry.ActionIdentityDeclines ||
				telemetry.DeclineReasons["derivation_identity_mismatch"] != telemetry.DerivationIdentityDeclines {
				t.Fatalf("Kotlin decline classification = %+v", telemetry)
			}
			for reason, count := range telemetry.DeclineReasons {
				if reason != "action_identity_mismatch" && reason != "derivation_identity_mismatch" && count != 0 {
					t.Fatalf("Kotlin unclassified decline reason %q=%d", reason, count)
				}
			}
			var classifiedReasons uint64
			for _, reason := range []string{"action_identity_mismatch", "derivation_identity_mismatch"} {
				classifiedReasons += telemetry.DeclineReasons[reason]
			}
			if classifiedReasons != telemetry.VerifierDeclines {
				t.Fatalf("Kotlin decline reason total=%d, declines=%d", classifiedReasons, telemetry.VerifierDeclines)
			}
			if telemetry.VerifierElections != 1 {
				t.Fatalf("Kotlin control elections=%d, want exactly one", telemetry.VerifierElections)
			}
			receiptProvider, ok := any(parser).(g18CohortVerifierReceiptProvider)
			if !ok {
				t.Errorf("RED: parser does not publish per-election Kotlin verifier receipts")
				return
			}
			var receipts []g18CohortVerifierReceipt
			if err := json.Unmarshal(receiptProvider.DiagnosticDropCohortVerifierReceiptsForTest(), &receipts); err != nil {
				t.Fatalf("decode Kotlin verifier receipts: %v", err)
			}
			if len(receipts) != int(telemetry.VerifierElections) {
				t.Fatalf("Kotlin verifier receipts=%d, elections=%d", len(receipts), telemetry.VerifierElections)
			}
			var receiptDeclines uint64
			var receiptActionDeclines uint64
			var receiptDerivationDeclines uint64
			for index, receipt := range receipts {
				if receipt.ArenaOwner != telemetry.ArenaOwner || receipt.ArenaEpoch != telemetry.ArenaEpoch ||
					receipt.CohortSequence == 0 {
					t.Fatalf("Kotlin receipt %d has invalid cohort identity: %+v", index, receipt)
				}
				switch receipt.Verdict {
				case "proved":
					if receipt.Classification != "" {
						t.Fatalf("Kotlin proof receipt %d has a decline classification: %+v", index, receipt)
					}
				case "declined":
					if receipt.Classification != "action_identity_mismatch" && receipt.Classification != "derivation_identity_mismatch" {
						t.Fatalf("Kotlin decline receipt %d has one unknown or missing classification: %+v", index, receipt)
					}
					receiptDeclines++
					if receipt.Classification == "action_identity_mismatch" {
						receiptActionDeclines++
					} else {
						receiptDerivationDeclines++
					}
				default:
					t.Fatalf("Kotlin receipt %d has verdict %q, want proved or declined", index, receipt.Verdict)
				}
			}
			if receiptDeclines != telemetry.VerifierDeclines ||
				receiptActionDeclines != telemetry.ActionIdentityDeclines ||
				receiptDerivationDeclines != telemetry.DerivationIdentityDeclines {
				t.Fatalf("Kotlin receipt census=%d/%d/%d, telemetry=%d/%d/%d", receiptDeclines, receiptActionDeclines, receiptDerivationDeclines, telemetry.VerifierDeclines, telemetry.ActionIdentityDeclines, telemetry.DerivationIdentityDeclines)
			}
			actionDeclines += telemetry.ActionIdentityDeclines
			derivationDeclines += telemetry.DerivationIdentityDeclines
		})
	}
	if actionDeclines == 0 || derivationDeclines == 0 {
		t.Fatalf(
			"RED: Kotlin decline telemetry action=%d derivation=%d, want both classes",
			actionDeclines,
			derivationDeclines,
		)
	}
}

// TestG18AlternativeSetKotlinOppositeControls keeps the known unsafe side of
// the class-3 boundary. The current proof must decline. A forced profile grant
// must route and differ from locked C.
func TestG18AlternativeSetKotlinOppositeControls(t *testing.T) {
	restoreCensus := gotreesitter.SetDiagnosticParserCoreShadowCensusEnabledForTest(true)
	defer restoreCensus()

	tests := []struct {
		name         string
		source       []byte
		sourceSHA256 string
	}{
		{
			name:         "annotated_getter_line",
			source:       []byte("@Deprecated(\"old\")\nval Int.double: Int\n    get() = this * 2\n// trailing\n"),
			sourceSHA256: "7aad51909730adbec84060f5445384ff82c9517e3603fb0da86c9ac2397548b7",
		},
		{
			name:         "annotated_getter_block",
			source:       []byte("@Deprecated(\"old\")\nval Int.double: Int\n    get() = this * 2\n/* trailing */\n"),
			sourceSHA256: "3c745d430894c3c739f217b807f747a4bd2f521173b441e625e4ca109b773ec2",
		},
	}
	cLanguage, err := COracleLanguage("kotlin")
	if err != nil {
		t.Fatalf("load Kotlin locked C language: %v", err)
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if got := fmt.Sprintf("%x", sha256.Sum256(test.source)); got != test.sourceSHA256 {
				t.Fatalf("source SHA-256 = %s, want %s", got, test.sourceSHA256)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatalf("set Kotlin locked C language: %v", err)
			}
			cTree := cParser.Parse(test.source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("Kotlin locked C parse returned no root")
			}
			t.Cleanup(cTree.Close)

			declinedLanguage := g18CloneLanguage(grammars.KotlinLanguage())
			if declinedLanguage.CompactConvergedReductionSplitDropsCertified {
				t.Fatal("Kotlin unexpectedly carries the split grant")
			}
			gotreesitter.DiagnosticParserCoreShadowCensusResetForTest()
			routedBefore, fallbackBefore := gotreesitter.AdmissionCandidateCounters()
			declinedParser := gotreesitter.NewParser(declinedLanguage)
			declinedParser.SetAdmissionCandidateRoute(true)
			declinedTree, err := declinedParser.Parse(test.source)
			if err != nil {
				t.Fatalf("declined parse: %v", err)
			}
			t.Cleanup(declinedTree.Release)
			routedAfter, fallbackAfter := gotreesitter.AdmissionCandidateCounters()
			if routedAfter != routedBefore || fallbackAfter != fallbackBefore+1 {
				t.Fatalf("Kotlin no-grant counters = %d/%d to %d/%d; want fallback +1", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			}
			if reason := gotreesitter.AdmissionCandidateLastFallbackReason(); !strings.Contains(reason, "lacks alternative-set coverage") {
				t.Fatalf("Kotlin no-grant reason = %q", reason)
			}
			wantCensus := gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
				Elections: 1, V1Proved: 1, Class3V1ProvesV2Declines: 1,
				SpillElections: 1, SpillObserved: 1, MaxBranchOrdinalObserved: 1,
			}
			census := gotreesitter.DiagnosticParserCoreThreeProofCensusSnapshotForTest()
			if census != wantCensus {
				t.Fatalf("Kotlin no-grant census = %+v, want %+v", census, wantCensus)
			}
			assertG18LockedCExact(t, "Kotlin declined fallback", declinedTree, declinedLanguage, cTree)
			t.Logf("Kotlin no-grant route=0/1 census=%+v", census)

			forcedLanguage := g18CloneLanguage(grammars.KotlinLanguage())
			forcedLanguage.CompactConvergedReductionSplitDropsCertified = true
			routedBefore, fallbackBefore = gotreesitter.AdmissionCandidateCounters()
			forcedParser := gotreesitter.NewParser(forcedLanguage)
			forcedParser.SetAdmissionCandidateRoute(true)
			forcedTree, err := forcedParser.Parse(test.source)
			if err != nil {
				t.Fatalf("forced compact parse: %v", err)
			}
			t.Cleanup(forcedTree.Release)
			routedAfter, fallbackAfter = gotreesitter.AdmissionCandidateCounters()
			if routedAfter != routedBefore+1 || fallbackAfter != fallbackBefore {
				t.Fatalf("Kotlin forced counters = %d/%d to %d/%d; want routed +1", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
			}
			if diff := FirstDivergenceDumpV1(forcedTree.RootNode(), forcedLanguage, cTree.RootNode()); diff == nil {
				t.Fatal("forced Kotlin split grant unexpectedly matches locked C")
			} else {
				t.Logf("forced Kotlin split grant remains unsafe: %+v", diff)
			}
		})
	}
}

func g18AlternativeSetTargets(t *testing.T) []g18AlternativeSetTarget {
	t.Helper()
	goFixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatalf("load Go fixtures: %v", err)
	}
	if len(goFixtures) != 4 {
		t.Fatalf("Go fixture count = %d, want 4", len(goFixtures))
	}
	coverageReason := "lacks alternative-set coverage by one non-blended survivor"
	historyReason := "descends from an unproved historical boundary resurrection"
	targets := []g18AlternativeSetTarget{
		{
			grammar: "go", name: goFixtures[0].Fixture.ID, source: goFixtures[0].Source,
			sourceSHA256: goFixtures[0].Fixture.SHA256, load: grammars.GoLanguage, wantReason: coverageReason,
			wantCensus: gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
				Elections: 3, ScalarProved: 2, V1Proved: 2, V2Proved: 2,
				SpillElections: 3, SpillObserved: 1, MaxBranchOrdinalObserved: 1,
			},
		},
		{
			grammar: "go", name: goFixtures[1].Fixture.ID, source: goFixtures[1].Source,
			sourceSHA256: goFixtures[1].Fixture.SHA256, load: grammars.GoLanguage, wantReason: coverageReason,
			wantCensus: gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
				Elections: 3, ScalarProved: 2, V1Proved: 2, V2Proved: 2,
				SpillElections: 3, SpillObserved: 1, MaxBranchOrdinalObserved: 1,
			},
		},
		{
			grammar: "go", name: goFixtures[2].Fixture.ID, source: goFixtures[2].Source,
			sourceSHA256: goFixtures[2].Fixture.SHA256, load: grammars.GoLanguage, wantReason: coverageReason,
			wantCensus: gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
				Elections: 4, ScalarProved: 3, V1Proved: 4, V2Proved: 3,
				Class3V1ProvesV2Declines: 1, SpillElections: 4, MaxBranchOrdinalObserved: 1,
			},
		},
		{
			grammar: "go", name: goFixtures[3].Fixture.ID, source: goFixtures[3].Source,
			sourceSHA256: goFixtures[3].Fixture.SHA256, load: grammars.GoLanguage, wantReason: coverageReason,
			wantCensus: gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
				Elections: 1, V1Proved: 1, Class3V1ProvesV2Declines: 1,
				SpillElections: 1, MaxBranchOrdinalObserved: 1,
			},
		},
		g18SimpleAlternativeSetTarget(
			"erlang", "macro_function_clauses",
			"-module(m).\n-define(FN, foo(0) -> zero; foo(N) when N > 0 -> pos; foo(_) -> neg).\n?FN.\n",
			"796647c2730c3f3d9d88abb589d606df700c2806396a8b79769fe89f65a5bea0", grammars.ErlangLanguage, true,
		),
		g18SimpleAlternativeSetTarget(
			"erlang", "macro_expanded_top_level_function",
			"-module(m).\n-define(FN1, bar(1) -> one).\n?FN1.\n",
			"36f66cda874299c4f9de7aa86bd5653c6d18cfd218d68d249c806acb39eb046e", grammars.ErlangLanguage, true,
		),
		g18SimpleAlternativeSetTarget(
			"haskell", "smoke", grammars.ParseSmokeSample("haskell"),
			"8e46b697006a890d6629efa91e5be5ba778ecf5c1315b3dd3b2265f2549bc854", grammars.HaskellLanguage, false,
		),
		{
			grammar: "javascript", name: "functions",
			source:       g18ReadSource(t, filepath.Join("..", "testdata", "compact_converged_split", "javascript.js")),
			sourceSHA256: "0bbd2cdb0a0492055e442c44b533797386ec9c8aeb7ce8a4d0f5f5a4681e3b90",
			load:         grammars.JavascriptLanguage, wantReason: coverageReason,
			wantCensus: gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
				Elections: 1, V1Proved: 1, Class3V1ProvesV2Declines: 1,
				SpillElections: 1, SpillObserved: 1, MaxBranchOrdinalObserved: 1,
			},
		},
		{
			grammar: "bash", name: "converged_split",
			source:       g18ReadSource(t, filepath.Join("..", "testdata", "compact_converged_split", "bash.sh")),
			sourceSHA256: "cccefdfff900acc9873a16805c244725b08d0bf85e15c7dacc3886ba8d5c7b4c",
			load:         grammars.BashLanguage, wantReason: coverageReason,
			wantCensus: gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
				Elections: 3, ScalarProved: 2, V1Proved: 3, V2Proved: 2,
				Class3V1ProvesV2Declines: 1, SpillElections: 3, SpillObserved: 1, MaxBranchOrdinalObserved: 1,
			},
		},
		g18SimpleAlternativeSetTarget(
			"perl", "push_two", "push @found, $_;\n",
			"08ac06c62278aa8bb26361629ac930bbbfbe5031da04a54ee3aeec4875ce0b3b", grammars.PerlLanguage, true,
		),
		g18SimpleAlternativeSetTarget(
			"perl", "push_three", "push @found, $a, $b;\n",
			"7be8389f1e6981c2e1e6324357df96ffb34063e9ea6811b8c332143e76015cd1", grammars.PerlLanguage, true,
		),
		{
			grammar: "ada", name: "positional_array",
			source:       []byte("package P is\n   type A is array (1 .. 3) of Boolean;\n   V : constant A := (1, 2, 3);\nend;\n"),
			sourceSHA256: "cce847719840bac903a5e52bd6d7b31d9f67a28353706ae3fc82ac07d0511e9b",
			load:         grammars.AdaLanguage, wantReason: historyReason, wantAdaImport: true,
			wantCensus: gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
				Elections: 1, ScalarProved: 1, V1Proved: 1, V2Proved: 1, SpillElections: 1,
			},
		},
		{
			grammar: "python", name: "greet",
			source:       []byte("def greet(name):\n    return f\"hello {name}\"\n\nprint(greet(\"world\"))\n"),
			sourceSHA256: "d29a356c8115cbf7b87f6644c6ff6f8b1fa530f7dcbd0fbc200f2ac9400827dd",
			load:         grammars.PythonLanguage, wantReason: coverageReason,
			wantCensus: gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
				Elections: 2, ScalarProved: 1, V1Proved: 2, V2Proved: 1,
				Class3V1ProvesV2Declines: 1, SpillElections: 2, SpillObserved: 1, MaxBranchOrdinalObserved: 1,
			},
		},
	}
	g18AssertTargetManifest(t, targets)
	return targets
}

func g18SimpleAlternativeSetTarget(
	grammar, name, source, sourceSHA256 string,
	load func() *gotreesitter.Language,
	spilled bool,
) g18AlternativeSetTarget {
	spill := uint64(0)
	if spilled {
		spill = 1
	}
	return g18AlternativeSetTarget{
		grammar: grammar, name: name, source: []byte(source), sourceSHA256: sourceSHA256, load: load,
		wantReason: "lacks alternative-set coverage by one non-blended survivor",
		wantCensus: gotreesitter.DiagnosticParserCoreThreeProofCensusTotals{
			Elections: 1, V1Proved: 1, Class3V1ProvesV2Declines: 1,
			SpillElections: 1, SpillObserved: spill, MaxBranchOrdinalObserved: 1,
		},
	}
}

func g18AssertTargetManifest(t *testing.T, targets []g18AlternativeSetTarget) {
	t.Helper()
	if len(g18AlternativeSetRouteGrammars) != 8 {
		t.Fatalf("future route grammar groups=%d, want 8", len(g18AlternativeSetRouteGrammars))
	}
	if g18AlternativeSetFlagDenominator != 15 || g18AlternativeSetGrammarDenominator != 12 {
		t.Fatalf(
			"locked denominator=%d/%d, want 15/12",
			g18AlternativeSetFlagDenominator,
			g18AlternativeSetGrammarDenominator,
		)
	}
	if len(targets) != 13 {
		t.Fatalf("target manifest has %d cases, want 13", len(targets))
	}
	keys := make(map[string]bool, len(targets))
	hashes := make(map[string]bool, len(targets))
	manifestHash := sha256.New()
	for _, target := range targets {
		key := target.grammar + "/" + target.name
		if keys[key] {
			t.Fatalf("duplicate target key %q", key)
		}
		keys[key] = true
		if hashes[target.sourceSHA256] {
			t.Fatalf("duplicate target source SHA-256 %q", target.sourceSHA256)
		}
		hashes[target.sourceSHA256] = true
		for _, value := range []string{target.grammar, target.name, target.sourceSHA256} {
			_, _ = manifestHash.Write([]byte(value))
			_, _ = manifestHash.Write([]byte{0})
		}
	}
	if got := fmt.Sprintf("%x", manifestHash.Sum(nil)); got != g18AlternativeSetTargetManifestSHA256 {
		t.Fatalf("target manifest SHA-256 = %s, want %s", got, g18AlternativeSetTargetManifestSHA256)
	}
}

func g18ReadSource(t *testing.T, path string) []byte {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return source
}

// g18CloneLanguage copies exported configuration only. It does not copy the
// private synchronization and atomic state in Language.
func g18CloneLanguage(language *gotreesitter.Language) *gotreesitter.Language {
	source := reflect.ValueOf(language).Elem()
	clone := reflect.New(source.Type()).Elem()
	for index := 0; index < source.NumField(); index++ {
		if source.Type().Field(index).IsExported() {
			clone.Field(index).Set(source.Field(index))
		}
	}
	return clone.Addr().Interface().(*gotreesitter.Language)
}

func assertG18GoTreesExact(t *testing.T, left, right *gotreesitter.Tree, language *gotreesitter.Language) {
	t.Helper()
	leftInspection, err := benchfixtures.InspectGoTree(left.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect left Go tree: %v", err)
	}
	rightInspection, err := benchfixtures.InspectGoTree(right.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect right Go tree: %v", err)
	}
	if leftInspection.SHA256 != rightInspection.SHA256 {
		t.Fatalf("Go tree inspections differ: left=%+v right=%+v", leftInspection, rightInspection)
	}
}

func assertG18LockedCExact(
	t *testing.T,
	label string,
	tree *gotreesitter.Tree,
	language *gotreesitter.Language,
	cTree *sitter.Tree,
) {
	t.Helper()
	if err := g18LockedCExactError(tree, language, cTree); err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	goInspection, err := benchfixtures.InspectGoTree(tree.RootNode(), language)
	if err != nil {
		t.Fatalf("%s inspect Go tree: %v", label, err)
	}
	t.Logf("%s locked-C deep digest=%s", label, goInspection.SHA256)
}

func g18LockedCExactError(tree *gotreesitter.Tree, language *gotreesitter.Language, cTree *sitter.Tree) error {
	goRoot := tree.RootNode()
	cRoot := cTree.RootNode()
	if diff := FirstDivergenceDumpV1(goRoot, language, cRoot); diff != nil {
		return fmt.Errorf("node or field divergence: %+v", diff)
	}
	if err := g18FirstLockedCFlagDifference(goRoot, language, cRoot, "/"+goRoot.Type(language)); err != nil {
		return fmt.Errorf("flag divergence: %w", err)
	}
	goInspection, err := benchfixtures.InspectGoTree(goRoot, language)
	if err != nil {
		return fmt.Errorf("inspect Go tree: %w", err)
	}
	cDigest, err := COracleDeepDigest(cTree)
	if err != nil {
		return fmt.Errorf("inspect C tree: %w", err)
	}
	if goInspection.SHA256 != cDigest {
		return fmt.Errorf("deep digest Go=%s C=%s", goInspection.SHA256, cDigest)
	}
	return nil
}

func g18FirstLockedCFlagDifference(
	goNode *gotreesitter.Node,
	language *gotreesitter.Language,
	cNode *sitter.Node,
	path string,
) error {
	if goNode == nil || cNode == nil {
		return fmt.Errorf("%s: nil mismatch Go=%v C=%v", path, goNode == nil, cNode == nil)
	}
	if goNode.IsMissing() != cNode.IsMissing() {
		return fmt.Errorf("%s: missing Go=%v C=%v", path, goNode.IsMissing(), cNode.IsMissing())
	}
	if goNode.IsError() != cNode.IsError() {
		return fmt.Errorf("%s: error Go=%v C=%v", path, goNode.IsError(), cNode.IsError())
	}
	for index := 0; index < goNode.ChildCount(); index++ {
		goChild := goNode.Child(index)
		cChild := cNode.Child(uint(index))
		childPath := fmt.Sprintf("%s/%s[%d]", path, goChild.Type(language), index)
		if err := g18FirstLockedCFlagDifference(goChild, language, cChild, childPath); err != nil {
			return err
		}
	}
	return nil
}
