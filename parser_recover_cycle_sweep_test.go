package gotreesitter_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Acyclicity sweep + regression pins for the C-recovery cyclic-transient-tree
// defect: recovery constructions that wrapped transient reduce parents with
// the eager-link-wiring constructor corrupted the result-time materializer's
// parent-field sentinel, and the materializer then linked a recovery wrapper
// under itself (a back-edge). Every full-tree walk afterwards hung
// (wireParentLinksWithScratchUntil has no visited set) or overflowed the
// stack (the recursive walkResultTree normalizer family). See
// newRecoveryParentNodeInArena (parser_recover_c.go) for the mechanism fix
// and parser_recover_cycle_internal_test.go for the unit-level pins.
//
// Optionally run with GOT_DEBUG_RECOVERY_CYCLES=1 to also engage the
// construction-time detector inside the recovery engine itself.

const recoveryCycleCorporaRoot = "/home/draco/work/gotreesitter-corpora/corpus_sources"

// parseRecoveryWithGuard parses src and converts a hang regression into a
// deterministic failure.
func parseRecoveryWithGuard(t *testing.T, lang *gotreesitter.Language, src []byte, d time.Duration, label string) *gotreesitter.Tree {
	t.Helper()
	type res struct {
		tree *gotreesitter.Tree
		err  error
	}
	done := make(chan res, 1)
	go func() {
		tree, err := gotreesitter.NewParser(lang).Parse(src)
		done <- res{tree, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("%s: parse error: %v", label, r.err)
		}
		return r.tree
	case <-time.After(d):
		t.Fatalf("%s: parse did not complete within %v (cyclic-transient-tree hang regressed)", label, d)
	}
	return nil
}

// assertPublicTreeAcyclic walks the returned tree through the public child
// API with an on-path set and fails on any back-edge. Returns the node count.
func assertPublicTreeAcyclic(t *testing.T, lang *gotreesitter.Language, root *gotreesitter.Node, label string) int {
	t.Helper()
	if root == nil {
		t.Fatalf("%s: nil root", label)
	}
	type frame struct {
		n   *gotreesitter.Node
		idx int
	}
	onPath := map[*gotreesitter.Node]struct{}{root: {}}
	stack := []frame{{n: root}}
	count := 1
	for len(stack) > 0 {
		f := &stack[len(stack)-1]
		if f.idx >= f.n.ChildCount() {
			delete(onPath, f.n)
			stack = stack[:len(stack)-1]
			continue
		}
		c := f.n.Child(f.idx)
		f.idx++
		if c == nil {
			continue
		}
		if _, cyc := onPath[c]; cyc {
			t.Fatalf("%s: back-edge: %s [%d-%d] reachable from its own descendant",
				label, c.Type(lang), c.StartByte(), c.EndByte())
		}
		count++
		onPath[c] = struct{}{}
		stack = append(stack, frame{n: c})
	}
	return count
}

func truncateToLines(src []byte, lines int) []byte {
	idx := 0
	for i := 0; i < lines; i++ {
		next := bytes.IndexByte(src[idx:], '\n')
		if next < 0 {
			return src
		}
		idx += next + 1
	}
	return src[:idx]
}

// TestCRecoveryZerrorsTruncationAcyclic is the direct regression for the
// zerrors_windows.go defect: an 800-line truncation cuts the giant const
// block mid-declaration, engaging the full gated recovery (handle_error →
// strategy-1 recover_to_state → EOF accept). Pre-fix this hung forever in the
// first full-tree walk; the 600-line truncation stayed fast (the original
// cliff). Both must now terminate quickly with acyclic trees.
func TestCRecoveryZerrorsTruncationAcyclic(t *testing.T) {
	var src []byte
	var err error
	for _, p := range zerrorsCandidatePaths {
		if src, err = os.ReadFile(p); err == nil {
			break
		}
	}
	if src == nil {
		t.Skipf("zerrors_windows.go corpus not found: %v", err)
	}
	t.Setenv("GOT_C_RECOVERY", "all")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)
	lang := grammars.GoLanguage()

	for _, lines := range []int{600, 800} {
		trunc := truncateToLines(src, lines)
		label := fmt.Sprintf("zerrors@%dlines", lines)
		start := time.Now()
		tree := parseRecoveryWithGuard(t, lang, trunc, 120*time.Second, label)
		root := tree.RootNode()
		nodes := assertPublicTreeAcyclic(t, lang, root, label)
		if root.EndByte() != uint32(len(trunc)) {
			t.Fatalf("%s: root.EndByte=%d want %d", label, root.EndByte(), len(trunc))
		}
		t.Logf("%s: %v, %d nodes, hasError=%v", label, time.Since(start), nodes, root.HasError())
	}
}

// TestCRecoveryZerrorsFullFileAcyclic pins the two verification points on the
// full (syntactically valid) file: it parses without hang/overflow with the
// gate forced AND under the default env, and recovery never turns the clean
// parse erroneous.
func TestCRecoveryZerrorsFullFileAcyclic(t *testing.T) {
	var src []byte
	var err error
	for _, p := range zerrorsCandidatePaths {
		if src, err = os.ReadFile(p); err == nil {
			break
		}
	}
	if src == nil {
		t.Skipf("zerrors_windows.go corpus not found: %v", err)
	}
	lang := grammars.GoLanguage()
	for _, env := range []string{"", "all"} {
		t.Setenv("GOT_C_RECOVERY", env)
		gotreesitter.ResetParseEnvConfigCacheForTests()
		t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)
		label := fmt.Sprintf("zerrors-full env=%q", env)
		start := time.Now()
		tree := parseRecoveryWithGuard(t, lang, src, 180*time.Second, label)
		root := tree.RootNode()
		assertPublicTreeAcyclic(t, lang, root, label)
		if root.HasError() {
			t.Fatalf("%s: clean corpus parse reported an error", label)
		}
		t.Logf("%s: %v", label, time.Since(start))
	}
}

// recoveryCycleSweepCase names one elected language's worst corpus files
// (cgo_harness/perf_scan authoritative scoreboard order).
type recoveryCycleSweepCase struct {
	lang  string
	load  func() *gotreesitter.Language
	files []string
}

var recoveryCycleSweepCases = []recoveryCycleSweepCase{
	{"bash", grammars.BashLanguage, []string{"git-gui/git-gui.sh", "t/t1092-sparse-checkout-compatibility.sh"}},
	{"c_sharp", grammars.CSharpLanguage, []string{"src/Bicep.Core.IntegrationTests/ScenarioTests.cs", "src/Bicep.Core.UnitTests/Mock/FakeResourceTypes.cs"}},
	{"cmake", grammars.CmakeLanguage, []string{"Modules/ExternalProject.cmake", "Modules/FetchContent.cmake"}},
	{"cpp", grammars.CppLanguage, []string{"include/fmt/base.h", "include/fmt/chrono.h"}},
	{"crystal", grammars.CrystalLanguage, []string{"spec/compiler/parser/parser_spec.cr", "spec/std/float_printer/ryu_printf_test_cases.cr"}},
	{"go", grammars.GoLanguage, []string{"src/cmd/compile/internal/ssa/opGen.go", "src/cmd/compile/internal/ssa/rewriteAMD64.go"}},
	{"haskell", grammars.HaskellLanguage, []string{"Cabal-syntax/src/Distribution/SPDX/LicenseId.hs", "Cabal/src/Distribution/Simple/Configure.hs"}},
	{"java", grammars.JavaLanguage, []string{"spring-beans/src/main/java/org/springframework/beans/factory/support/DefaultListableBeanFactory.java", "spring-beans/src/test/java/org/springframework/beans/factory/DefaultListableBeanFactoryTests.java"}},
	{"javascript", grammars.JavascriptLanguage, []string{"deps/amaro/dist/index.js", "deps/undici/undici.js"}},
	{"json", grammars.JsonLanguage, []string{"src/api/json/catalog.json", "src/schemas/json/cloudify.json"}},
	{"kotlin", grammars.KotlinLanguage, []string{"kotlinx-coroutines-core/common/src/CoroutineScope.kt", "kotlinx-coroutines-core/common/src/Job.kt"}},
	{"lua", grammars.LuaLanguage, []string{"runtime/lua/vim/_meta/options.gen.lua", "runtime/lua/vim/_meta/vimfn.gen.lua"}},
	{"php", grammars.PhpLanguage, []string{"src/Symfony/Component/Emoji/Resources/data/emoji-as.php", "src/Symfony/Component/Emoji/Resources/data/emoji-bn.php"}},
	{"python", grammars.PythonLanguage, []string{"Lib/pydoc_data/topics.py", "Lib/test/_test_multiprocessing.py"}},
	{"ruby", grammars.RubyLanguage, []string{"actionpack/lib/action_dispatch/routing/mapper.rb", "actionpack/test/dispatch/routing_test.rb"}},
	{"rust", grammars.RustLanguage, []string{"library/stdarch/crates/core_arch/src/aarch64/neon/generated.rs", "library/stdarch/crates/core_arch/src/aarch64/sve/generated.rs"}},
	{"scala", grammars.ScalaLanguage, []string{"src/compiler/scala/tools/nsc/ast/parser/Parsers.scala", "src/compiler/scala/tools/nsc/typechecker/Implicits.scala"}},
	{"swift", grammars.SwiftLanguage, []string{"Sources/SwiftSyntax/generated/RenamedChildrenCompatibility.swift", "Sources/SwiftSyntax/generated/SyntaxRewriter.swift"}},
	{"tsx", grammars.TsxLanguage, []string{"cli/template/extras/src/app/page/with-better-auth-trpc-tw.tsx", "cli/template/extras/src/pages/index/with-better-auth-trpc-tw.tsx"}},
	{"typescript", grammars.TypescriptLanguage, []string{"src/compiler/checker.ts", "src/lib/dom.generated.d.ts"}},
}

// TestCRecoveryCorpusTruncationSweepAcyclic truncates the elected languages'
// worst corpus files mid-construct (forcing error recovery on real-world
// structure), parses with the recovery gate forced, and asserts every
// resulting tree is acyclic. -short keeps a fast trio; the full sweep runs the
// whole elected set.
func TestCRecoveryCorpusTruncationSweepAcyclic(t *testing.T) {
	if os.Getenv("GOT_RECOVERY_SWEEP") != "1" {
		t.Skip("set GOT_RECOVERY_SWEEP=1 to run the corpus truncation acyclicity sweep (slow; multi-language corpora)")
	}
	if _, err := os.Stat(recoveryCycleCorporaRoot); err != nil {
		t.Skipf("corpora not found: %v", err)
	}
	os.Setenv("GOT_C_RECOVERY", "all")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	defer gotreesitter.ResetParseEnvConfigCacheForTests()
	defer os.Unsetenv("GOT_C_RECOVERY")

	const truncateBytes = 48 * 1024
	shortSet := map[string]bool{"go": true, "bash": true, "java": true}
	for _, tc := range recoveryCycleSweepCases {
		if testing.Short() && !shortSet[tc.lang] {
			continue
		}
		tc := tc
		t.Run(tc.lang, func(t *testing.T) {
			lang := tc.load()
			if lang == nil {
				t.Skipf("grammar %s unavailable", tc.lang)
			}
			for _, rel := range tc.files {
				path := filepath.Join(recoveryCycleCorporaRoot, tc.lang, rel)
				src, err := os.ReadFile(path)
				if err != nil {
					t.Logf("skip %s: %v", rel, err)
					continue
				}
				if len(src) > truncateBytes {
					src = src[:truncateBytes]
				}
				label := tc.lang + "/" + filepath.Base(rel)
				start := time.Now()
				tree := parseRecoveryWithGuard(t, lang, src, 120*time.Second, label)
				root := tree.RootNode()
				nodes := assertPublicTreeAcyclic(t, lang, root, label)
				t.Logf("%s: %d bytes, %v, %d nodes, hasError=%v",
					label, len(src), time.Since(start), nodes, root.HasError())
			}
		})
	}
}
