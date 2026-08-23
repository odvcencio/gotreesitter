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

func g18D6aRuntimeDiff(left, right gts.ParseRuntime) []string {
	var diffs []string
	var visit func(reflect.Value, reflect.Value, string)
	visit = func(a, b reflect.Value, path string) {
		if !a.IsValid() || !b.IsValid() {
			if a.IsValid() != b.IsValid() {
				diffs = append(diffs, fmt.Sprintf("%s: %v != %v", path, a.IsValid(), b.IsValid()))
			}
			return
		}
		if a.Type() != b.Type() {
			diffs = append(diffs, fmt.Sprintf("%s: type %s != %s", path, a.Type(), b.Type()))
			return
		}
		switch a.Kind() {
		case reflect.Struct:
			for index := 0; index < a.NumField(); index++ {
				field := a.Type().Field(index)
				visit(a.Field(index), b.Field(index), path+"."+field.Name)
			}
		case reflect.Array, reflect.Slice:
			if a.Len() != b.Len() {
				diffs = append(diffs, fmt.Sprintf("%s: len %d != %d", path, a.Len(), b.Len()))
				return
			}
			for index := 0; index < a.Len(); index++ {
				visit(a.Index(index), b.Index(index), fmt.Sprintf("%s[%d]", path, index))
			}
		default:
			if !reflect.DeepEqual(a.Interface(), b.Interface()) {
				diffs = append(diffs, fmt.Sprintf("%s: %v != %v", path, a.Interface(), b.Interface()))
			}
		}
	}
	visit(reflect.ValueOf(left), reflect.ValueOf(right), "ParseRuntime")
	return diffs
}

func TestG18D6aRouteEquivalentRuntimeDiagnostic(t *testing.T) {
	targets := g18D6aTargets(t)
	var target g18D6aTarget
	for _, candidate := range targets {
		if candidate.Grammar == "go" && candidate.Name == "query_compile" {
			target = candidate
			break
		}
	}
	if target.Source == nil {
		t.Fatal("Go query_compile target is unavailable")
	}
	run := func(record bool) (g18D6aRouteResult, int, error, error) {
		language := g18D6aCloneLanguage(target.Load())
		language.CompactConvergedReductionSplitDropsCertified = false
		parser := gts.NewParser(language)
		parser.SetAdmissionCandidateRoute(true)
		gts.ResetAdmissionCandidateCountersForTest()
		tree, published, candidateErr, err := gts.DiagnosticParseCandidateWithDropCohortFrontierModeForTest(parser, target.Source, record, nil)
		if tree == nil {
			return g18D6aRouteResult{}, published, candidateErr, err
		}
		result := g18D6aRouteResult{
			Routed: 0, Fallback: 0, FallbackReason: gts.AdmissionCandidateLastFallbackReason(),
			RawDigest: g18D6aDigest(target.Source), ResultDigest: g18D6aResultDigest(tree, language),
			RuntimeDigest: g18D6aRuntimeDigest(tree.ParseRuntime()), Runtime: tree.ParseRuntime(),
		}
		result.Routed, result.Fallback = gts.AdmissionCandidateCounters()
		tree.Release()
		return result, published, candidateErr, err
	}
	baseline, baselinePublished, baselineCandidateErr, baselineErr := run(false)
	observed, observedPublished, observedCandidateErr, observedErr := run(true)
	diffs := g18D6aRuntimeDiff(baseline.Runtime, observed.Runtime)
	t.Logf("candidate baseline err=%v published=%d route=%d/%d runtime=%s", baselineCandidateErr, baselinePublished, baseline.Routed, baseline.Fallback, baseline.RuntimeDigest)
	t.Logf("candidate observed err=%v published=%d route=%d/%d runtime=%s", observedCandidateErr, observedPublished, observed.Routed, observed.Fallback, observed.RuntimeDigest)
	t.Logf("candidate fallback errors: baseline=%v observed=%v; production errors: baseline=%v observed=%v", baselineCandidateErr, observedCandidateErr, baselineErr, observedErr)
	for _, diff := range diffs {
		t.Logf("runtime diff %s", diff)
	}
	if baselineErr != nil || observedErr != nil || baseline.ResultDigest != observed.ResultDigest || baseline.RawDigest != observed.RawDigest ||
		baseline.Routed != observed.Routed || baseline.Fallback != observed.Fallback || baseline.FallbackReason != observed.FallbackReason {
		t.Fatalf("route-equivalent result mismatch: baseline=%+v observed=%+v", baseline, observed)
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

func g18D6aSnapshotDecode(t *testing.T, bytes []byte) g18D6aSnapshot {
	t.Helper()
	var snapshot g18D6aSnapshot
	if err := json.Unmarshal(bytes, &snapshot); err != nil {
		t.Fatalf("decode frontier snapshot: %v; bytes=%s", err, bytes)
	}
	return snapshot
}

func g18D6aRequireCompleteFrontier(t *testing.T, snapshot g18D6aSnapshot) {
	t.Helper()
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

func TestG18D6aProducerTelemetry(t *testing.T) {
	for _, target := range g18D6aTargets(t) {
		target := target
		t.Run(target.Grammar+"/"+target.Name, func(t *testing.T) {
			baseline := g18D6aParseRoute(t, target)
			language := g18D6aCloneLanguage(target.Load())
			language.CompactConvergedReductionSplitDropsCertified = false
			parser := gts.NewParser(language)
			parser.SetAdmissionCandidateRoute(true)
			gts.ResetAdmissionCandidateCountersForTest()
			var snapshots []g18D6aSnapshot
			tree, published, err := gts.DiagnosticParseWithDropCohortFrontierObserverForTest(parser, target.Source, func(bytes []byte) {
				snapshots = append(snapshots, g18D6aSnapshotDecode(t, bytes))
			})
			if err != nil || tree == nil {
				t.Fatalf("producer parse err=%v tree_nil=%t published=%d snapshots=%d", err, tree == nil, published, len(snapshots))
			}
			if published == 0 || len(snapshots) == 0 {
				t.Fatalf("producer parse=%v published=%d snapshots=%d, want one complete frontier", err, published, len(snapshots))
			}
			for _, snapshot := range snapshots {
				g18D6aRequireCompleteFrontier(t, snapshot)
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
				if g18D6aDigest(target.Source) != baseline.RawDigest || g18D6aResultDigest(tree, language) != baseline.ResultDigest ||
					g18D6aRuntimeDigest(observedRuntime) != baseline.RuntimeDigest || !reflect.DeepEqual(observedRuntime, baseline.Runtime) {
					t.Fatalf("D6a producer changed D3 result: observed=%s/%s baseline=%s/%s", g18D6aResultDigest(tree, language), g18D6aRuntimeDigest(observedRuntime), baseline.ResultDigest, baseline.RuntimeDigest)
				}
			}
			t.Logf("parse_err=%v published=%d members=%d route=%d/%d reason=%q raw=%s result=%s runtime=%s", err, published,
				snapshots[len(snapshots)-1].Frontiers[0].WrittenMembers, baseline.Routed, baseline.Fallback,
				baseline.FallbackReason, baseline.RawDigest, baseline.ResultDigest, baseline.RuntimeDigest)
		})
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
