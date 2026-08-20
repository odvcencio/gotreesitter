package grammars

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestSwiftCleanRecoveryProbeMatchesLegacyTree(t *testing.T) {
	lang := SwiftLanguage()
	cases := []struct {
		name      string
		source    string
		wantProbe bool
	}{
		{
			name: "for-range",
			source: "func f(n: Int) -> Int {\n" +
				"  var total = 0\n" +
				"  for i in 0..<n { total += i }\n" +
				"  return total\n" +
				"}\n",
			wantProbe: true,
		},
		{
			name: "for-closed-range",
			source: "func f(n: Int) -> Int {\n" +
				"  var total = 0\n" +
				"  for i in 0...n { total += i }\n" +
				"  return total\n" +
				"}\n",
			wantProbe: true,
		},
		{
			name:      "native-ternary",
			source:    "func f(x: Int) -> Int { return x > 0 ? 1 : 2 }\n",
			wantProbe: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			legacy, _ := parseSwiftRecoveryProbeRoute(t, lang, []byte(tc.source), "legacy")
			defer legacy.Release()
			initial, runtime := parseSwiftRecoveryProbeRoute(t, lang, []byte(tc.source), "")
			defer initial.Release()

			swiftRequireSameRecoveryProbeTree(t, lang, legacy.RootNode(), initial.RootNode(), "root")
			if root := initial.RootNode(); root == nil || root.HasError() {
				t.Fatalf("initial-only route returned an error tree: %v", root)
			}
			if got, want := initial.RootNode().EndByte(), uint32(len(tc.source)); got != want {
				t.Fatalf("initial-only root end = %d, want %d", got, want)
			}
			if tc.wantProbe {
				if got, want := runtime.RecoveryProbeInitialAttempts, uint64(1); got != want {
					t.Fatalf("initial probes = %d, want %d", got, want)
				}
				if got, want := runtime.RecoveryProbeInitialAccepted, uint64(1); got != want {
					t.Fatalf("accepted initial probes = %d, want %d", got, want)
				}
			}
			if got := runtime.RecoveryProbeLegacyFallbacks; got != 0 {
				t.Fatalf("legacy fallbacks = %d, want 0", got)
			}
			if got := runtime.RecoveryProbeInitialRetryPasses; got != 0 {
				t.Fatalf("initial retry passes = %d, want 0", got)
			}
			if got := runtime.RecoveryProbeLegacyRetryPasses; got != 0 {
				t.Fatalf("legacy retry passes = %d, want 0", got)
			}
		})
	}
}

func TestSwiftUnsafeWitnessKeepsCurrentGoTreeAcrossRecoveryProbe(t *testing.T) {
	if raceEnabled {
		t.Skip("this witness drives full error recovery twice with the dispatcher census enabled; its cost under the race detector exceeds the CI shard budget; the witness runs in every non-race lane")
	}
	lang := SwiftLanguage()
	source, err := os.ReadFile(filepath.Join("testdata", "swift_corpus", "stdlib_FloatingPointToString.swift"))
	if err != nil {
		t.Fatalf("read Swift witness: %v", err)
	}
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")

	legacy, _ := parseSwiftRecoveryProbeRoute(t, lang, source, "legacy")
	defer legacy.Release()
	initial, runtime := parseSwiftRecoveryProbeRoute(t, lang, source, "")
	defer initial.Release()

	legacyRoot := legacy.RootNode()
	initialRoot := initial.RootNode()
	swiftRequireSameRecoveryProbeTree(t, lang, legacyRoot, initialRoot, "root")
	if initialRoot == nil || !initialRoot.HasError() {
		t.Fatal("Swift unsafe witness no longer has its known error tree")
	}
	if got, want := initialRoot.EndByte(), uint32(len(source)); got != want {
		t.Fatalf("Swift unsafe witness root end = %d, want %d", got, want)
	}
	inspection, err := benchfixtures.InspectGoTree(initialRoot, lang)
	if err != nil {
		t.Fatalf("inspect Swift unsafe witness: %v", err)
	}
	const wantDigest = "ec51c633a3f99515cc0cd1c0cff435a44ddc7db8e83705977d28f78bdfb0fc0e"
	if inspection.SHA256 != wantDigest {
		t.Fatalf("Swift unsafe witness digest = %s, want %s", inspection.SHA256, wantDigest)
	}
	t.Logf("Swift unsafe witness gts-deep-tree-v1 digest: %s", inspection.SHA256)

	// This file carries too many independent parse errors for any whole-source
	// recovery reparse to reach a clean tree. The now-removed terminal-
	// diagnostic-count pre-gates used to skip the probe for a case like this
	// before it ever ran; without them the probe still runs (see
	// swiftRecoveryProbeMatchesLegacyRoute, parser_result_swift.go) and
	// correctly declines, and the legacy route it falls back to also declines,
	// so the tree this witness pins never changes. The exact attempt/retry
	// counts below are the legacy engine's own business, not this contract's,
	// so only the invariant and the "never accepted" guarantee are asserted.
	if got, want := runtime.RecoveryProbeInitialAttempts, runtime.RecoveryProbeInitialAccepted+runtime.RecoveryProbeLegacyFallbacks; got != want {
		t.Fatalf("initial probe attempts = %d, want accepted(%d)+legacy fallbacks(%d) = %d", got, runtime.RecoveryProbeInitialAccepted, runtime.RecoveryProbeLegacyFallbacks, want)
	}
	if got := runtime.RecoveryProbeInitialAttempts; got == 0 {
		t.Fatal("initial probe attempts = 0, want at least one whole-source recovery attempt on this witness")
	}
	if got := runtime.RecoveryProbeInitialAccepted; got != 0 {
		t.Fatalf("accepted initial probes = %d, want 0 (this witness cannot reach a clean tree)", got)
	}
	if got := runtime.RecoveryProbeInitialRetryPasses; got != 0 {
		t.Fatalf("initial retry passes = %d, want 0 (the initial-only probe never runs the retry ladder)", got)
	}
	t.Logf("legacy fallbacks=%d legacy retry passes=%d Swift legacy recovery subparses=%d Swift legacy recovery retry passes=%d",
		runtime.RecoveryProbeLegacyFallbacks, runtime.RecoveryProbeLegacyRetryPasses,
		runtime.SwiftLegacyRecoverySubparseAttempts, runtime.SwiftLegacyRecoveryRetryPasses)

	for _, name := range []string{
		"dispatch.swift.conditions",
		"dispatch.swift.top-level",
		"dispatch.swift.control",
	} {
		pass, ok := swiftRecoveryProbePass(runtime, name)
		if !ok {
			t.Fatalf("missing census pass %q", name)
		}
		if pass.Checked != 1 || pass.Run != 1 {
			t.Fatalf("census pass %q checked=%d run=%d, want 1/1", name, pass.Checked, pass.Run)
		}
		if pass.NodesVisited == 0 {
			t.Fatalf("census pass %q visited no nodes", name)
		}
		if pass.NodesRewritten != 0 {
			t.Fatalf("census pass %q rewrote %d nodes, want 0", name, pass.NodesRewritten)
		}
	}
	if _, ok := swiftRecoveryProbePass(runtime, "dispatch.swift.ternary"); ok {
		t.Fatal("the census reports the retired Swift ternary pass")
	}
}

// swiftForInRangeLoopCountSource builds a single Swift function containing
// count copies of the #123 for…in trailing-closure-ambiguity trigger
// (`for i in 0..<10 { }`). Each loop independently forces a recovery reparse
// of the whole function; a large count raises the number of live GLR stacks
// the raw (pre-recovery) parse explores, which is what the removed
// terminal-diagnostic-count pre-gates (mechanism 2) used to miscount as
// "too broken to recover" past a fixed threshold.
func swiftForInRangeLoopCountSource(count int) []byte {
	var b strings.Builder
	b.WriteString("func manyLoops() {\n")
	for i := 0; i < count; i++ {
		b.WriteString("    for i in 0..<10 { }\n")
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

// TestSwiftForInRangeLoopCountParityWitness is the committed regression test
// for the mechanism-2 parity cliff: swiftConditionRecoveryCanReachCleanTree
// (removed) declined the whole-source recovery reparse once a for…in-loop
// function's raw terminal-diagnostic count crossed
// swiftCleanRecoveryProbeMaxTerminalDiagnostics (8), even though the reparse
// would have reached a tree byte-identical to origin/main's. At 10 and 20
// loops the PR (before this fix) returned an ERROR tree where origin/main —
// and the locked C oracle — agree on a clean, full-span source_file; at 9
// loops the count stayed under the old gate's threshold and matched. This
// asserts hasError=false and pins the gts-deep-tree-v1 digest for 9, 10, and
// 20 loops so the cliff can never come back silently. The digests were
// computed on this fixed branch and independently verified to equal
// origin/main's digest for the same construction (byte-identical output).
func TestSwiftForInRangeLoopCountParityWitness(t *testing.T) {
	lang := SwiftLanguage()
	cases := []struct {
		count      int
		wantDigest string
	}{
		{count: 9, wantDigest: "f16a8f340ad64c0b3670a2b478cb0d38ffae1350fbaa6e46cc79b25aff5c4776"},
		{count: 10, wantDigest: "b0249428fe5a3f03ecac47bb836f7609d70fde35836bc1f01499c270051e13ae"},
		{count: 20, wantDigest: "6d379d19b0b253647b5e441d58278fb83eaa874bba874415143f436342205b49"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("loops=%d", tc.count), func(t *testing.T) {
			source := swiftForInRangeLoopCountSource(tc.count)

			legacy, _ := parseSwiftRecoveryProbeRoute(t, lang, source, "legacy")
			defer legacy.Release()
			initial, runtime := parseSwiftRecoveryProbeRoute(t, lang, source, "")
			defer initial.Release()

			legacyRoot := legacy.RootNode()
			initialRoot := initial.RootNode()
			swiftRequireSameRecoveryProbeTree(t, lang, legacyRoot, initialRoot, "root")

			if legacyRoot == nil || legacyRoot.HasError() {
				t.Fatalf("legacy route hasError=%v, want false (%d-loop witness must reach a clean tree)", legacyRoot == nil || legacyRoot.HasError(), tc.count)
			}
			if initialRoot == nil || initialRoot.HasError() {
				t.Fatalf("probe route hasError=%v, want false (%d-loop witness must reach a clean tree)", initialRoot == nil || initialRoot.HasError(), tc.count)
			}
			if got, want := initialRoot.EndByte(), uint32(len(source)); got != want {
				t.Fatalf("%d-loop witness root end = %d, want %d", tc.count, got, want)
			}

			inspection, err := benchfixtures.InspectGoTree(initialRoot, lang)
			if err != nil {
				t.Fatalf("inspect %d-loop witness: %v", tc.count, err)
			}
			if inspection.SHA256 != tc.wantDigest {
				t.Fatalf("%d-loop witness digest = %s, want %s (origin/main's digest for the same source)", tc.count, inspection.SHA256, tc.wantDigest)
			}
			t.Logf("%d-loop witness gts-deep-tree-v1 digest: %s", tc.count, inspection.SHA256)

			if got := runtime.RecoveryProbeLegacyFallbacks; got != 0 {
				t.Fatalf("%d-loop witness legacy fallbacks = %d, want 0 (the tightened accept gate must accept the probe)", tc.count, got)
			}
		})
	}
}

// TestSwiftCorpusProbeMatchesLegacy asserts that the initial-only recovery
// probe (mechanism 1) never diverges from the unchanged legacy recovery
// route for any file in the Swift corpus: for every *.swift file under
// testdata/swift_corpus, parsing through the probe route and through the
// forced-legacy route (GOT_SWIFT_CLEAN_RECOVERY_PROBE_MODE=legacy) must
// produce byte-identical trees. This is the corpus-wide counterpart to
// swiftRecoveryProbeMatchesLegacyRoute's per-accept-decision contract
// (parser_result_swift.go): the accept gate may only take the probe's answer
// when it is provably identical to what legacy would return, and this is the
// empirical check that the contract holds on real-world files, not just the
// hand-built unit cases above.
func TestSwiftCorpusProbeMatchesLegacy(t *testing.T) {
	dir := "testdata/swift_corpus"
	lang := SwiftLanguage()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".swift" {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			if exp, ok := swiftCorpusExpectations[name]; raceEnabled && ok && exp.status == swiftCorpusKnownFailing {
				t.Skip("known-failing file drives full error recovery twice here; its cost under the race detector exceeds the CI shard budget; equivalence still holds in non-race lanes")
			}
			src, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("reading corpus file: %v", err)
			}
			legacy, _ := parseSwiftRecoveryProbeRoute(t, lang, src, "legacy")
			defer legacy.Release()
			probe, _ := parseSwiftRecoveryProbeRoute(t, lang, src, "")
			defer probe.Release()
			swiftRequireSameRecoveryProbeTree(t, lang, legacy.RootNode(), probe.RootNode(), "root")
		})
	}
}

func BenchmarkSwiftCleanFullSourceRecoveryProbe(b *testing.B) {
	lang := SwiftLanguage()
	source := []byte("func f(n: Int) -> Int {\n" +
		"  var total = 0\n" +
		"  for i in 0..<n { total += i }\n" +
		"  return total\n" +
		"}\n")

	for _, route := range []struct {
		name string
		mode string
	}{
		{name: "legacy-retry", mode: "legacy"},
		{name: "initial-only", mode: ""},
	} {
		b.Run(route.name, func(b *testing.B) {
			b.Setenv("GOT_SWIFT_CLEAN_RECOVERY_PROBE_MODE", route.mode)
			parser := gotreesitter.NewParser(lang)
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree, err := parser.Parse(source)
				if err != nil {
					b.Fatalf("parse: %v", err)
				}
				if root := tree.RootNode(); root == nil || root.HasError() {
					tree.Release()
					b.Fatal("recovery probe benchmark returned an error tree")
				}
				tree.Release()
			}
		})
	}
}

func parseSwiftRecoveryProbeRoute(t *testing.T, lang *gotreesitter.Language, source []byte, mode string) (*gotreesitter.Tree, gotreesitter.ParseRuntime) {
	t.Helper()
	t.Setenv("GOT_SWIFT_CLEAN_RECOVERY_PROBE_MODE", mode)
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse Swift recovery probe route %q: %v", mode, err)
	}
	if tree == nil || tree.RootNode() == nil {
		if tree != nil {
			tree.Release()
		}
		t.Fatalf("Swift recovery probe route %q returned no tree/root", mode)
	}
	return tree, tree.ParseRuntime()
}

func swiftRecoveryProbePass(runtime gotreesitter.ParseRuntime, name string) (gotreesitter.NormalizationPassRuntime, bool) {
	if runtime.NormalizationPasses == nil {
		return gotreesitter.NormalizationPassRuntime{}, false
	}
	for _, pass := range *runtime.NormalizationPasses {
		if pass.Name == name {
			return pass, true
		}
	}
	return gotreesitter.NormalizationPassRuntime{}, false
}

func swiftRequireSameRecoveryProbeTree(t *testing.T, lang *gotreesitter.Language, want, got *gotreesitter.Node, path string) {
	t.Helper()
	if want == nil || got == nil {
		if want != got {
			t.Fatalf("%s node presence differs", path)
		}
		return
	}
	if want.Type(lang) != got.Type(lang) ||
		want.StartByte() != got.StartByte() ||
		want.EndByte() != got.EndByte() ||
		want.StartPoint() != got.StartPoint() ||
		want.EndPoint() != got.EndPoint() ||
		want.IsNamed() != got.IsNamed() ||
		want.IsExtra() != got.IsExtra() ||
		want.IsMissing() != got.IsMissing() ||
		want.IsError() != got.IsError() ||
		want.HasError() != got.HasError() ||
		want.ChildCount() != got.ChildCount() {
		t.Fatalf("%s node differs: legacy=%s[%d:%d] initial=%s[%d:%d]", path, want.Type(lang), want.StartByte(), want.EndByte(), got.Type(lang), got.StartByte(), got.EndByte())
	}
	for i := 0; i < want.ChildCount(); i++ {
		if want.FieldNameForChild(i, lang) != got.FieldNameForChild(i, lang) {
			t.Fatalf("%s child %d field differs: legacy=%q initial=%q", path, i, want.FieldNameForChild(i, lang), got.FieldNameForChild(i, lang))
		}
		swiftRequireSameRecoveryProbeTree(t, lang, want.Child(i), got.Child(i), path+"/child")
	}
}
