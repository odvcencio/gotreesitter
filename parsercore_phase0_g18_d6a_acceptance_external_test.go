//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

type g18D6aTarget struct {
	Grammar string
	Name    string
	Source  []byte
	Load    func() *gts.Language
}

type g18D6aSnapshot struct {
	Schema    string                 `json:"schema"`
	Frontiers []g18D6aFrontierRecord `json:"frontiers"`
}

type g18D6aObserverPayload struct {
	Frontier      g18D6aSnapshot         `json:"frontier"`
	HeaderHeads   []core.Head            `json:"header_heads"`
	HeaderRefs    [][]core.DropCohortRef `json:"header_refs"`
	DropIndices   []int                  `json:"drop_indices"`
	ElectionIndex int                    `json:"election_index"`
}

type g18D6aFrontierRecord struct {
	Handle               [3]uint64                    `json:"handle"`
	ElectionSequence     uint64                       `json:"election_sequence"`
	State                string                       `json:"state"`
	ExpectedParticipants uint16                       `json:"expected_participants"`
	WrittenParticipants  uint16                       `json:"written_participants"`
	WrittenMembers       uint32                       `json:"written_members"`
	Seal                 [32]byte                     `json:"seal"`
	Token                core.DropCohortFrontierToken `json:"token"`
	Participants         []g18D6aParticipant          `json:"participants"`
}

type g18D6aParticipant struct {
	Head           core.Head      `json:"head"`
	BranchOrder    uint64         `json:"branch_order"`
	ReferenceFlags uint8          `json:"reference_flags"`
	MemberCount    uint16         `json:"member_count"`
	Members        []g18D6aMember `json:"members"`
}

type g18D6aMember struct {
	Participant          uint16                          `json:"participant"`
	Ref                  core.DropCohortRef              `json:"ref"`
	ParticipantHead      core.Head                       `json:"participant_head"`
	SourceHead           core.Head                       `json:"source_head"`
	BranchOrder          uint64                          `json:"branch_order"`
	Action               core.DropCohortActionIdentity   `json:"action"`
	Derivation           core.DropCohortDerivationHandle `json:"derivation"`
	DerivationDigest     [32]byte                        `json:"derivation_digest"`
	DerivationLength     uint32                          `json:"derivation_length"`
	DerivationRootSymbol core.Symbol                     `json:"derivation_root_symbol"`
	DerivationStackDepth uint32                          `json:"derivation_stack_depth"`
	DerivationCheckpoint core.DropCohortSourceCheckpoint `json:"derivation_checkpoint"`
}

type g18D6aRouteResult struct {
	Routed, Fallback uint64
	FallbackReason   string
	RawDigest        string
	ResultDigest     string
	RuntimeDigest    string
	Runtime          gts.ParseRuntime
}

// g18D6aCandidateOutcome records the candidate's exact success or decline
// boundary. The producer test compares this value with recording disabled and
// enabled, so certificate recording cannot move or reword a candidate gate.
type g18D6aCandidateOutcome struct {
	Success   bool
	Boundary  string
	Detail    string
	ErrorType string
}

type g18D6bVerifierSnapshot struct {
	VerifierElections uint64 `json:"verifier_elections"`
	VerifierProofs    uint64 `json:"verifier_proofs"`
	VerifierDeclines  uint64 `json:"verifier_declines"`
}

func g18D6aCloneLanguage(language *gts.Language) *gts.Language {
	value := reflect.ValueOf(language).Elem()
	clone := reflect.New(value.Type()).Elem()
	for index := 0; index < value.NumField(); index++ {
		if value.Type().Field(index).IsExported() {
			clone.Field(index).Set(value.Field(index))
		}
	}
	return clone.Addr().Interface().(*gts.Language)
}

func g18D6aTargets(t *testing.T) []g18D6aTarget {
	t.Helper()
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 4 {
		t.Fatalf("Go fixtures=%d, want 4", len(fixtures))
	}
	targets := make([]g18D6aTarget, 0, 9)
	for _, fixture := range fixtures {
		targets = append(targets, g18D6aTarget{"go", fixture.Fixture.ID, fixture.Source, grammars.GoLanguage})
	}
	targets = append(targets,
		g18D6aTarget{"erlang", "macro_function_clauses", []byte("-module(m).\n-define(FN, foo(0) -> zero; foo(N) when N > 0 -> pos; foo(_) -> neg).\n?FN.\n"), grammars.ErlangLanguage},
		g18D6aTarget{"erlang", "macro_expanded_top_level_function", []byte("-module(m).\n-define(FN1, bar(1) -> one).\n?FN1.\n"), grammars.ErlangLanguage},
		g18D6aTarget{"haskell", "smoke", []byte(grammars.ParseSmokeSample("haskell")), grammars.HaskellLanguage},
	)
	for _, row := range []struct {
		grammar, name, file string
		load                func() *gts.Language
	}{
		{"javascript", "functions", "javascript.js", grammars.JavascriptLanguage},
		{"bash", "converged_split", "bash.sh", grammars.BashLanguage},
	} {
		source, err := os.ReadFile(filepath.Join("testdata", "compact_converged_split", row.file))
		if err != nil {
			t.Fatal(err)
		}
		targets = append(targets, g18D6aTarget{row.grammar, row.name, source, row.load})
	}
	return targets
}

func g18D6aDigest(bytes []byte) string {
	digest := sha256.Sum256(bytes)
	return hex.EncodeToString(digest[:])
}

func g18D6aResultDigest(tree *gts.Tree, language *gts.Language) string {
	return g18D6aDigest([]byte(tree.RootNode().SExpr(language)))
}

func g18D6aRuntimeDigest(runtime gts.ParseRuntime) string {
	encoded, _ := json.Marshal(runtime)
	return g18D6aDigest(encoded)
}

func g18D6aRuntimeEqualExceptPoolBaselines(left, right gts.ParseRuntime) bool {
	left.ArenaBaselineBytes = 0
	left.ScratchBaselineBytes = 0
	left.GSSBaselineBytes = 0
	right.ArenaBaselineBytes = 0
	right.ScratchBaselineBytes = 0
	right.GSSBaselineBytes = 0
	return reflect.DeepEqual(left, right)
}

func g18D6aPoolBaselines(runtime gts.ParseRuntime) string {
	return fmt.Sprintf("ArenaBaselineBytes=%d ScratchBaselineBytes=%d GSSBaselineBytes=%d",
		runtime.ArenaBaselineBytes, runtime.ScratchBaselineBytes, runtime.GSSBaselineBytes)
}

func g18D6aCandidateOutcomeForError(err error) g18D6aCandidateOutcome {
	if err == nil {
		return g18D6aCandidateOutcome{Success: true}
	}
	const separator = ": "
	message := err.Error()
	boundary, detail := message, ""
	for index := 0; index+len(separator) <= len(message); index++ {
		if message[index:index+len(separator)] == separator {
			boundary, detail = message[:index], message[index+len(separator):]
			break
		}
	}
	return g18D6aCandidateOutcome{
		Boundary:  boundary,
		Detail:    detail,
		ErrorType: fmt.Sprintf("%T", err),
	}
}

func g18D6aParseRoute(t *testing.T, target g18D6aTarget) g18D6aRouteResult {
	t.Helper()
	language := g18D6aCloneLanguage(target.Load())
	language.CompactConvergedReductionSplitDropsCertified = false
	gts.ResetAdmissionCandidateCountersForTest()
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	restore := parser.DiagnosticEnableDropCohortCertificateAdmissionForTest()
	tree, err := parser.Parse(target.Source)
	restore()
	if err != nil {
		t.Fatalf("%s/%s parse: %v", target.Grammar, target.Name, err)
	}
	t.Cleanup(tree.Release)
	routed, fallback := gts.AdmissionCandidateCounters()
	runtime := tree.ParseRuntime()
	return g18D6aRouteResult{
		Routed: routed, Fallback: fallback, FallbackReason: gts.AdmissionCandidateLastFallbackReason(),
		RawDigest: g18D6aDigest(target.Source), ResultDigest: g18D6aResultDigest(tree, language),
		RuntimeDigest: g18D6aRuntimeDigest(runtime), Runtime: runtime,
	}
}

func g18D6aCandidateOutcomeForTarget(t *testing.T, target g18D6aTarget, record bool) g18D6aCandidateOutcome {
	t.Helper()
	language := g18D6aCloneLanguage(target.Load())
	language.CompactConvergedReductionSplitDropsCertified = false
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	tree, _, candidateErr, productionErr := gts.DiagnosticParseCandidateWithDropCohortFrontierModeForTest(parser, target.Source, record, nil)
	if tree != nil {
		tree.Release()
	}
	if productionErr != nil {
		t.Fatalf("record=%t production parse: %v", record, productionErr)
	}
	return g18D6aCandidateOutcomeForError(candidateErr)
}

func g18D6aSnapshotDecode(t *testing.T, bytes []byte) g18D6aObserverPayload {
	t.Helper()
	var snapshot g18D6aObserverPayload
	if err := json.Unmarshal(bytes, &snapshot); err != nil {
		t.Fatalf("decode frontier snapshot: %v; bytes=%s", err, bytes)
	}
	return snapshot
}

func g18D6aRequireCompleteFrontier(t *testing.T, payload g18D6aObserverPayload) {
	t.Helper()
	snapshot := payload.Frontier
	if snapshot.Schema != "gts-drop-cohort-frontier/v1" || len(snapshot.Frontiers) != 1 {
		t.Fatalf("snapshot schema/frontier count=%q/%d", snapshot.Schema, len(snapshot.Frontiers))
	}
	record := snapshot.Frontiers[0]
	if record.State != "complete" || record.Handle[0] == 0 || record.Handle[1] == 0 || record.Handle[2] == 0 ||
		record.ElectionSequence == 0 || record.ExpectedParticipants == 0 ||
		record.ExpectedParticipants != record.WrittenParticipants || record.WrittenMembers == 0 ||
		len(record.Participants) != int(record.WrittenParticipants) || record.Seal == [32]byte{} {
		t.Fatalf("frontier is incomplete: %+v", record)
	}
	if record.Token.ScannerBeforeDigest == [32]byte{} || record.Token.ScannerAfterDigest == [32]byte{} {
		t.Fatalf("frontier checkpoint identity is incomplete: %+v", record.Token)
	}
	for index, participant := range record.Participants {
		if participant.Head.Node == 0 || participant.MemberCount == 0 || len(participant.Members) != int(participant.MemberCount) {
			t.Fatalf("participant %d is incomplete: %+v", index, participant)
		}
		for memberIndex, member := range participant.Members {
			if member.Participant != uint16(index) || member.Ref.Owner != record.Handle[0] || member.Ref.Epoch != record.Handle[1] ||
				member.Ref.Sequence == 0 || member.ParticipantHead != participant.Head || member.SourceHead.Node == 0 ||
				member.Derivation.Owner != record.Handle[0] || member.Derivation.Epoch != record.Handle[1] ||
				member.DerivationDigest == [32]byte{} || member.DerivationLength == 0 {
				t.Fatalf("participant %d member %d is not authenticated: %+v", index, memberIndex, member)
			}
		}
	}
}

func g18D6aIsTargetDropFrontier(payload g18D6aObserverPayload) bool {
	if len(payload.DropIndices) == 0 || len(payload.Frontier.Frontiers) != 1 || len(payload.Frontier.Frontiers[0].Participants) < 2 ||
		len(payload.HeaderHeads) != len(payload.HeaderRefs) || len(payload.HeaderHeads) != len(payload.Frontier.Frontiers[0].Participants) {
		return false
	}
	frontier := payload.Frontier.Frontiers[0]
	for index, participant := range frontier.Participants {
		if participant.Head != payload.HeaderHeads[index] || len(participant.Members) != len(payload.HeaderRefs[index]) {
			return false
		}
		for memberIndex, member := range participant.Members {
			if member.Ref != payload.HeaderRefs[index][memberIndex] || member.ParticipantHead != payload.HeaderHeads[index] {
				return false
			}
		}
	}
	for _, dropIndex := range payload.DropIndices {
		if dropIndex < 0 || dropIndex >= len(payload.HeaderHeads) {
			return false
		}
	}
	return true
}

func TestG18D6aProducerTelemetry(t *testing.T) {
	for _, target := range g18D6aTargets(t) {
		target := target
		t.Run(target.Grammar+"/"+target.Name, func(t *testing.T) {
			baseline := g18D6aParseRoute(t, target)
			offCandidate := g18D6aCandidateOutcomeForTarget(t, target, false)
			onCandidate := g18D6aCandidateOutcomeForTarget(t, target, true)
			if offCandidate != onCandidate {
				t.Fatalf("recording changed candidate outcome: off=%+v on=%+v", offCandidate, onCandidate)
			}
			language := g18D6aCloneLanguage(target.Load())
			language.CompactConvergedReductionSplitDropsCertified = false
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(true)
			gts.ResetAdmissionCandidateCountersForTest()
			var snapshots []g18D6aObserverPayload
			tree, published, observerCandidateErr, err := gts.DiagnosticParseWithDropCohortFrontierObserverForTest(parser, target.Source, func(bytes []byte) {
				snapshots = append(snapshots, g18D6aSnapshotDecode(t, bytes))
			})
			if err != nil || tree == nil {
				t.Fatalf("producer parse err=%v tree_nil=%t published=%d snapshots=%d", err, tree == nil, published, len(snapshots))
			}
			if published == 0 || len(snapshots) == 0 {
				t.Fatalf("producer parse=%v published=%d snapshots=%d, want one complete frontier", err, published, len(snapshots))
			}
			if observerCandidate := g18D6aCandidateOutcomeForError(observerCandidateErr); observerCandidate != onCandidate {
				t.Fatalf("observer probe changed candidate outcome: observer=%+v direct=%+v", observerCandidate, onCandidate)
			}
			targetDrop := false
			for _, snapshot := range snapshots {
				g18D6aRequireCompleteFrontier(t, snapshot)
				targetDrop = targetDrop || g18D6aIsTargetDropFrontier(snapshot)
			}
			if !targetDrop {
				t.Fatalf("producer telemetry did not capture an authenticated multi-participant frontier immediately before a target drop")
			}
			if tree != nil {
				t.Cleanup(tree.Release)
			}
			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != baseline.Routed || fallback != baseline.Fallback || gts.AdmissionCandidateLastFallbackReason() != baseline.FallbackReason {
				t.Fatalf("producer observer changed admission decision: observed=%d/%d/%q baseline=%d/%d/%q", routed, fallback, gts.AdmissionCandidateLastFallbackReason(), baseline.Routed, baseline.Fallback, baseline.FallbackReason)
			}
			if tree != nil {
				observedRuntime := tree.ParseRuntime()
				if g18D6aDigest(target.Source) != baseline.RawDigest || g18D6aResultDigest(tree, language) != baseline.ResultDigest || !g18D6aRuntimeEqualExceptPoolBaselines(observedRuntime, baseline.Runtime) {
					t.Fatalf("D6a producer changed D3 runtime outside pool baselines: observed=(%s) baseline=(%s)", g18D6aPoolBaselines(observedRuntime), g18D6aPoolBaselines(baseline.Runtime))
				}
			}
			t.Logf("parse_err=%v published=%d members=%d route=%d/%d reason=%q raw=%s result=%s runtime=%s", err, published,
				snapshots[len(snapshots)-1].Frontier.Frontiers[0].WrittenMembers, baseline.Routed, baseline.Fallback,
				baseline.FallbackReason, baseline.RawDigest, baseline.ResultDigest, baseline.RuntimeDigest)
		})
	}
}

func TestG18D6aIncompletePublicationPreservesNaturalNoActionDecline(t *testing.T) {
	var target g18D6aTarget
	for _, candidate := range g18D6aTargets(t) {
		if candidate.Grammar == "go" && candidate.Name == "rewrite" {
			target = candidate
			break
		}
	}
	if target.Source == nil {
		t.Fatal("Go rewrite target is unavailable")
	}
	off := g18D6aCandidateOutcomeForTarget(t, target, false)
	on := g18D6aCandidateOutcomeForTarget(t, target, true)
	if off.Success || on.Success || off != on {
		t.Fatalf("incomplete frontier publication changed natural no-action decline: off=%+v on=%+v", off, on)
	}
	if off.Boundary != "no_action" || off.Detail != "converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor" {
		t.Fatalf("rewrite natural decline=%+v", off)
	}
}

func TestG18D6aKotlinControlsRemainNonCandidate(t *testing.T) {
	for _, row := range []struct {
		name, source string
	}{
		{"line", "@Deprecated(\"old\")\nval Int.double: Int\n    get() = this * 2\n// trailing\n"},
		{"block", "@Deprecated(\"old\")\nval Int.double: Int\n    get() = this * 2\n/* trailing */\n"},
	} {
		row := row
		t.Run(row.name, func(t *testing.T) {
			language := g18D6aCloneLanguage(grammars.KotlinLanguage())
			language.CompactConvergedReductionSplitDropsCertified = false
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(false)
			gts.ResetAdmissionCandidateCountersForTest()
			restore := parser.DiagnosticEnableDropCohortCertificateAdmissionForTest()
			tree, err := parser.Parse([]byte(row.source))
			restore()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tree.Release)
			routed, fallback := gts.AdmissionCandidateCounters()
			if routed != 0 || fallback != 0 {
				t.Fatalf("Kotlin non-candidate parser consumed admission: %d/%d", routed, fallback)
			}
		})
	}
}

func TestG18D6bGrammargenLRNoCommonDerivationFallsBack(t *testing.T) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var fixture benchfixtures.LoadedFixture
	for _, candidate := range fixtures {
		if candidate.Fixture.ID == "grammargen_lr" {
			fixture = candidate
			break
		}
	}
	if fixture.Fixture.ID != "grammargen_lr" {
		t.Fatal("grammargen_lr fixture is unavailable")
	}

	language := g18D6aCloneLanguage(grammars.GoLanguage())
	language.CompactConvergedReductionSplitDropsCertified = false
	// Inspect one candidate frontier and fallback, without recovery retries.
	language.CompactOwnedEOFRecoveryCertified = false
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(true)
	gts.ResetAdmissionCandidateCountersForTest()
	var snapshots []g18D6aObserverPayload
	restore := parser.DiagnosticEnableDropCohortFrontierVerificationForTest(func(bytes []byte) {
		snapshots = append(snapshots, g18D6aSnapshotDecode(t, bytes))
	})
	candidate, err := parser.Parse(fixture.Source)
	restore()
	if err != nil {
		t.Fatalf("candidate parse: %v", err)
	}
	if candidate == nil {
		t.Fatal("candidate parse returned nil tree")
	}
	t.Cleanup(candidate.Release)

	routed, fallback := gts.AdmissionCandidateCounters()
	wantFallbackReason := "compact route declined at no_action: converged-path reduction split no-action drop lacks alternative-set coverage by one non-blended survivor"
	if routed != 0 || fallback != 1 || gts.AdmissionCandidateLastFallbackReason() != wantFallbackReason {
		t.Fatalf("candidate route counters=%d/%d reason=%q, want 0/1 and %q", routed, fallback, gts.AdmissionCandidateLastFallbackReason(), wantFallbackReason)
	}
	targetSnapshot := -1
	for index, snapshot := range snapshots {
		g18D6aRequireCompleteFrontier(t, snapshot)
		if g18D6aIsTargetDropFrontier(snapshot) {
			if targetSnapshot >= 0 {
				t.Fatalf("target frontier publications=%d, want 1", targetSnapshot+1)
			}
			targetSnapshot = index
		}
	}
	if targetSnapshot < 0 {
		t.Fatalf("frontier publications=%d, none bind the target drop", len(snapshots))
	}

	candidateInspection, err := benchfixtures.InspectGoTree(candidate.RootNode(), language)
	if err != nil {
		t.Fatalf("inspect candidate tree: %v", err)
	}
	if err := fixture.Fixture.VerifyDeepTreeDigest(candidateInspection.SHA256); err != nil {
		t.Fatalf("candidate tree is not exact locked-C parity: %v", err)
	}

	productionLanguage := g18D6aCloneLanguage(grammars.GoLanguage())
	productionParser := gts.NewParser(productionLanguage)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(fixture.Source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	if production == nil {
		t.Fatal("production parse returned nil tree")
	}
	t.Cleanup(production.Release)
	productionInspection, err := benchfixtures.InspectGoTree(production.RootNode(), productionLanguage)
	if err != nil {
		t.Fatalf("inspect production tree: %v", err)
	}
	if err := fixture.Fixture.VerifyDeepTreeDigest(productionInspection.SHA256); err != nil {
		t.Fatalf("production tree is not exact locked-C parity: %v", err)
	}
	if candidateInspection.SHA256 != productionInspection.SHA256 {
		t.Fatalf("candidate and production digests differ: candidate=%s production=%s", candidateInspection.SHA256, productionInspection.SHA256)
	}

	var telemetry g18D6bVerifierSnapshot
	if err := json.Unmarshal(parser.DiagnosticDropCohortSnapshotForTest(), &telemetry); err != nil {
		t.Fatalf("decode verifier telemetry: %v", err)
	}
	if telemetry.VerifierElections != 0 || telemetry.VerifierProofs != 0 || telemetry.VerifierDeclines != 0 {
		t.Fatalf("verifier telemetry=%+v, want no D6b proof after the typed decline", telemetry)
	}

	publicationCount := len(snapshots)
	gts.ResetAdmissionCandidateCountersForTest()
	repeated, err := parser.Parse(fixture.Source)
	if err != nil {
		t.Fatalf("restored candidate parse: %v", err)
	}
	if repeated == nil {
		t.Fatal("restored candidate parse returned nil tree")
	}
	t.Cleanup(repeated.Release)
	routed, fallback = gts.AdmissionCandidateCounters()
	if routed != 0 || fallback != 1 || gts.AdmissionCandidateLastFallbackReason() != wantFallbackReason {
		t.Fatalf("restored candidate route counters=%d/%d reason=%q, want 0/1 and %q", routed, fallback, gts.AdmissionCandidateLastFallbackReason(), wantFallbackReason)
	}
	if len(snapshots) != publicationCount {
		t.Fatalf("restored parse published new frontier telemetry: before=%d after=%d", publicationCount, len(snapshots))
	}
}
