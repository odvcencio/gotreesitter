//go:build gts_parsercorephase0

package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

const (
	parserCoreWarmQueryCompileBytes  = 20168
	parserCoreWarmQueryCompileSHA256 = "b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894"
)

var (
	//go:embed internal/benchfixtures/testdata/query_compile.go.gz
	parserCoreWarmQueryCompileGzip []byte

	parserCoreWarmPrepareOnce sync.Once
	parserCoreWarmPreparedRun *parserCoreWarmPrepared
	parserCoreWarmPrepareErr  error
	parserCoreWarmGoScanner   ExternalScanner
)

// SetDiagnosticParserCoreWarmGoScannerForTest supplies the exact scanner type
// from the external tagged test package without creating a grammars -> root
// package import cycle in this same-package benchmark.
func SetDiagnosticParserCoreWarmGoScannerForTest(scanner ExternalScanner) {
	parserCoreWarmGoScanner = scanner
}

var parserCoreWarmLimits = core.Limits{
	MaxNodes: 65536, MaxLinks: 65536, MaxSubtrees: 65536,
	MaxChildren: 262144, MaxMetadata: 131072,
	MaxLinksPerBoundary: 8, MaxPopPaths: 4096, MaxDerivations: 4096,
}

var parserCoreWarmOptions = DiagnosticParserCorePrefixOptions{
	ReceiptMode: DiagnosticParserCoreReceiptSummary,
	MaxTokens:   25000, MaxDispatches: 50000,
	Limits: parserCoreWarmLimits,
}

type parserCoreWarmPrepared struct {
	*parserCoreFreshFullRunner
	source       []byte
	acceptedCore *core.Core
	acceptedHead core.Head
	baselineWork core.Work
}

func parserCoreWarmPrepare() (*parserCoreWarmPrepared, error) {
	parserCoreWarmPrepareOnce.Do(func() {
		reader, err := gzip.NewReader(bytes.NewReader(parserCoreWarmQueryCompileGzip))
		if err != nil {
			parserCoreWarmPrepareErr = fmt.Errorf("open query_compile fixture: %w", err)
			return
		}
		source, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			parserCoreWarmPrepareErr = fmt.Errorf("decompress query_compile fixture: %w", readErr)
			return
		}
		if closeErr != nil {
			parserCoreWarmPrepareErr = fmt.Errorf("close query_compile fixture: %w", closeErr)
			return
		}
		if len(source) != parserCoreWarmQueryCompileBytes || fmt.Sprintf("%x", sha256.Sum256(source)) != parserCoreWarmQueryCompileSHA256 {
			parserCoreWarmPrepareErr = fmt.Errorf("query_compile identity drifted: bytes=%d sha256=%x", len(source), sha256.Sum256(source))
			return
		}

		if parserCoreWarmGoScanner == nil {
			parserCoreWarmPrepareErr = errors.New("parser-core warm benchmark: authenticated Go scanner unavailable")
			return
		}
		runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreWarmOptions)
		if err != nil {
			parserCoreWarmPrepareErr = err
			return
		}
		acceptedCore, err := core.New(runner.tables, parserCoreWarmLimits)
		if err != nil {
			parserCoreWarmPrepareErr = err
			return
		}
		prepared := &parserCoreWarmPrepared{
			parserCoreFreshFullRunner: runner,
			source:                    source,
			acceptedCore:              acceptedCore,
		}
		scheduler, tokenSource, err := runner.executeSchedulerOpen(source, acceptedCore, false)
		if tokenSource != nil {
			tokenSource.Close()
		}
		if err != nil {
			parserCoreWarmPrepareErr = err
			return
		}
		prepared.baselineWork = acceptedCore.Work()
		if prepared.baselineWork.Overflow || prepared.baselineWork.Shifts == 0 || prepared.baselineWork.Reductions == 0 {
			parserCoreWarmPrepareErr = fmt.Errorf("parser-core warm benchmark: invalid baseline work: %+v", prepared.baselineWork)
			return
		}
		prepared.acceptedHead = scheduler.acceptedHead
		prepared.parser.SetAdmissionCandidateRoute(false)
		parserCoreWarmPreparedRun = prepared
	})
	return parserCoreWarmPreparedRun, parserCoreWarmPrepareErr
}

func (p *parserCoreWarmPrepared) executeSchedulerOpen(compact *core.Core, reset bool) (*diagnosticParserCoreGenericScheduler, *dfaTokenSource, error) {
	if p == nil || p.parserCoreFreshFullRunner == nil {
		return nil, nil, errors.New("parser-core warm benchmark: incomplete prepared scheduler")
	}
	scheduler, tokenSource, err := p.parserCoreFreshFullRunner.executeSchedulerOpen(p.source, compact, reset)
	if err != nil {
		return scheduler, tokenSource, err
	}
	if work := compact.Work(); work != p.baselineWork || work.Overflow {
		tokenSource.Close()
		return scheduler, nil, fmt.Errorf("parser-core warm benchmark: work drifted: got=%+v want=%+v", work, p.baselineWork)
	}
	return scheduler, tokenSource, nil
}

func (p *parserCoreWarmPrepared) executeScheduler(compact *core.Core, reset bool) (*diagnosticParserCoreGenericScheduler, error) {
	scheduler, tokenSource, err := p.executeSchedulerOpen(compact, reset)
	if tokenSource != nil {
		tokenSource.Close()
	}
	return scheduler, err
}

func (p *parserCoreWarmPrepared) materialize(compact *core.Core, head core.Head) (*Tree, error) {
	return p.parserCoreFreshFullRunner.materialize(p.source, compact, head)
}

func (p *parserCoreWarmPrepared) executeTotal() (*Tree, error) {
	return p.parserCoreFreshFullRunner.parse(p.source)
}

func parserCoreWarmRequireExactTree(t testing.TB, tree *Tree, sourceLen int) {
	t.Helper()
	if tree == nil || tree.root == nil {
		t.Fatal("parser-core warm benchmark returned no tree")
	}
	root := tree.root
	runtime := tree.ParseRuntime()
	if root.startByte != 0 || root.endByte != uint32(sourceLen) || root.IsError() || root.HasError() || runtime.StopReason != ParseStopAccepted || runtime.Truncated || runtime.TokenSourceEOFEarly || runtime.SourceLen != uint32(sourceLen) || runtime.ExpectedEOFByte != uint32(sourceLen) || runtime.RootEndByte != uint32(sourceLen) || !runtime.LastTokenWasEOF {
		t.Fatalf("parser-core warm benchmark exactness drifted: root=%d..%d error=%v runtime=%s", root.startByte, root.endByte, root.HasError(), runtime.Summary())
	}
	census := diagnosticParserCoreSelectedNodeCensus(root)
	if census.total != 7524 || census.parents != 2853 || census.leaves != 4671 {
		t.Fatalf("parser-core warm benchmark selected census drifted: total=%d parents=%d leaves=%d", census.total, census.parents, census.leaves)
	}
}

func parserCoreWarmRequireDeepEqual(t testing.TB, left, right *Tree, lang *Language) {
	t.Helper()
	if left == nil || right == nil || left.root == nil || right.root == nil || lang == nil {
		t.Fatal("parser-core warm benchmark cannot compare incomplete trees")
	}
	type pair struct {
		left, right *Node
	}
	stack := []pair{{left: left.root, right: right.root}}
	for visited := 0; len(stack) != 0; visited++ {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		a, b := current.left, current.right
		if a == nil || b == nil {
			if a != b {
				t.Fatalf("parser-core warm benchmark deep tree nil mismatch at node %d", visited)
			}
			continue
		}
		if a.Type(lang) != b.Type(lang) || a.StartByte() != b.StartByte() || a.EndByte() != b.EndByte() ||
			a.StartPoint() != b.StartPoint() || a.EndPoint() != b.EndPoint() || a.IsNamed() != b.IsNamed() ||
			a.IsExtra() != b.IsExtra() || a.IsMissing() != b.IsMissing() || a.IsError() != b.IsError() ||
			a.HasError() != b.HasError() || a.ChildCount() != b.ChildCount() {
			t.Fatalf("parser-core warm benchmark deep tree mismatch at node %d: left=%s %d..%d children=%d right=%s %d..%d children=%d", visited, a.Type(lang), a.StartByte(), a.EndByte(), a.ChildCount(), b.Type(lang), b.StartByte(), b.EndByte(), b.ChildCount())
		}
		for child := a.ChildCount() - 1; child >= 0; child-- {
			if a.FieldNameForChild(child, lang) != b.FieldNameForChild(child, lang) {
				t.Fatalf("parser-core warm benchmark field mismatch at node %d child %d: left=%q right=%q", visited, child, a.FieldNameForChild(child, lang), b.FieldNameForChild(child, lang))
			}
			stack = append(stack, pair{left: a.Child(child), right: b.Child(child)})
		}
	}
}

func parserCoreWarmAdmitAgainstProduction(t testing.TB, prepared *parserCoreWarmPrepared, candidate *Tree) {
	t.Helper()
	parserCoreWarmRequireExactTree(t, candidate, len(prepared.source))
	production, err := prepared.parser.Parse(prepared.source)
	if err != nil {
		t.Fatal(err)
	}
	defer production.Release()
	parserCoreWarmRequireExactTree(t, production, len(prepared.source))
	parserCoreWarmRequireDeepEqual(t, candidate, production, prepared.lang)
}

func parserCoreWarmRequireParentLinks(t testing.TB, tree *Tree) {
	t.Helper()
	if tree == nil || tree.root == nil {
		t.Fatal("parser-core materialization produced no root")
	}
	if tree.root.Parent() != nil {
		t.Fatal("parser-core materialization root has a parent")
	}
	stack := []*Node{tree.root}
	for len(stack) != 0 {
		last := len(stack) - 1
		parent := stack[last]
		stack = stack[:last]
		for index := 0; index < parent.ChildCount(); index++ {
			child := parent.Child(index)
			if child == nil || child.Parent() != parent {
				t.Fatalf("parser-core materialization child %d/%d has incorrect parent", index, parent.ChildCount())
			}
			if index+1 < parent.ChildCount() && child.NextSibling() != parent.Child(index+1) {
				t.Fatalf("parser-core materialization child %d/%d has incorrect sibling index", index, parent.ChildCount())
			}
			if index+1 == parent.ChildCount() && child.NextSibling() != nil {
				t.Fatalf("parser-core materialization final child %d/%d has a next sibling", index, parent.ChildCount())
			}
			stack = append(stack, child)
		}
	}
}

func TestDiagnosticParserCoreWarmBenchmarkPreflight(t *testing.T) {
	prepared, err := parserCoreWarmPrepare()
	if err != nil {
		t.Fatal(err)
	}
	scheduler, err := prepared.executeScheduler(prepared.compact, true)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := prepared.materialize(prepared.compact, scheduler.acceptedHead)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tree.Release)
	parserCoreWarmRequireExactTree(t, tree, len(prepared.source))
	parserCoreWarmRequireParentLinks(t, tree)
	fixture := loadDiagnosticParserCoreCanonicalFixture(t, "query_compile")
	if digest := requireDiagnosticParserCoreCanonicalTreeDigest(t, tree, prepared.lang); digest != fixture.DeepTreeSHA256 {
		t.Fatalf("warm tree digest=%s want=%s", digest, fixture.DeepTreeSHA256)
	}

	first, err := prepared.materialize(prepared.acceptedCore, prepared.acceptedHead)
	if err != nil {
		t.Fatal(err)
	}
	parserCoreWarmRequireExactTree(t, first, len(prepared.source))
	first.Release()
	second, err := prepared.materialize(prepared.acceptedCore, prepared.acceptedHead)
	if err != nil {
		t.Fatalf("repeated immutable materialization changed behavior: %v", err)
	}
	parserCoreWarmRequireExactTree(t, second, len(prepared.source))
	parserCoreWarmRequireParentLinks(t, second)
	second.Release()

	production, err := prepared.parser.Parse(prepared.source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	parserCoreWarmRequireExactTree(t, production, len(prepared.source))
	parserCoreWarmRequireDeepEqual(t, tree, production, prepared.lang)
}

func TestDiagnosticParserCoreWarmBenchmarkRejectsWorkDrift(t *testing.T) {
	prepared, err := parserCoreWarmPrepare()
	if err != nil {
		t.Fatal(err)
	}
	wrong := *prepared
	wrong.baselineWork.Shifts++
	_, tokenSource, err := wrong.executeSchedulerOpen(prepared.compact, true)
	if tokenSource != nil {
		tokenSource.Close()
		t.Fatal("rejected warm run retained its token source")
	}
	if err == nil || !strings.Contains(err.Error(), "work drifted") {
		t.Fatalf("altered baseline did not reject work drift: %v", err)
	}
	if _, err := prepared.executeScheduler(prepared.compact, true); err != nil {
		t.Fatalf("baseline rejection poisoned the next run: %v", err)
	}
}

// These diagnostic lanes compare lifecycle-matched Go paths. They are not a
// static-C publication ratio and must not be presented as one.
func BenchmarkDiagnosticParserCoreWarmSchedulerOnlyQueryCompile(b *testing.B) {
	prepared, err := parserCoreWarmPrepare()
	if err != nil {
		b.Fatal(err)
	}
	scheduler, err := prepared.executeScheduler(prepared.compact, true)
	if err != nil {
		b.Fatal(err)
	}
	tree, err := prepared.materialize(prepared.compact, scheduler.acceptedHead)
	if err != nil {
		b.Fatal(err)
	}
	parserCoreWarmAdmitAgainstProduction(b, prepared, tree)
	tree.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(prepared.source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := prepared.executeScheduler(prepared.compact, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDiagnosticParserCoreWarmMaterializationOnlyQueryCompile(b *testing.B) {
	prepared, err := parserCoreWarmPrepare()
	if err != nil {
		b.Fatal(err)
	}
	tree, err := prepared.materialize(prepared.acceptedCore, prepared.acceptedHead)
	if err != nil {
		b.Fatal(err)
	}
	parserCoreWarmAdmitAgainstProduction(b, prepared, tree)
	tree.Release()
	if perfCountersEnabled {
		ResetPerfCounters()
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(prepared.source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := prepared.materialize(prepared.acceptedCore, prepared.acceptedHead)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
	if perfCountersEnabled {
		counters := PerfCountersSnapshot()
		iterations := float64(b.N)
		b.ReportMetric(float64(counters.ReduceChildrenFastGSS)/iterations, "fast_gss_children/tree")
		b.ReportMetric(float64(counters.ReduceChildrenAllVis)/iterations, "all_visible_children/tree")
		b.ReportMetric(float64(counters.ReduceChildrenScratch)/iterations, "scratch_children/tree")
		b.ReportMetric(float64(counters.ReduceScratchNoAlias)/iterations, "scratch_no_alias_children/tree")
		b.ReportMetric(float64(counters.ReduceScratchGeneral)/iterations, "scratch_general_children/tree")
		b.ReportMetric(float64(counters.ReduceChildEmptyBuilds)/iterations, "empty_builds/tree")
		b.ReportMetric(float64(counters.ReduceChildAllVisibleBuilds)/iterations, "all_visible_builds/tree")
		b.ReportMetric(float64(counters.ReduceChildScratchNoAliasBuilds)/iterations, "scratch_no_alias_builds/tree")
		b.ReportMetric(float64(counters.ReduceChildScratchGeneralBuilds)/iterations, "scratch_general_builds/tree")
	}
}

func BenchmarkDiagnosticParserCoreWarmTotalQueryCompile(b *testing.B) {
	prepared, err := parserCoreWarmPrepare()
	if err != nil {
		b.Fatal(err)
	}
	tree, err := prepared.executeTotal()
	if err != nil {
		b.Fatal(err)
	}
	parserCoreWarmAdmitAgainstProduction(b, prepared, tree)
	tree.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(prepared.source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := prepared.executeTotal()
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}

func BenchmarkDiagnosticParserCoreWarmProductionParseQueryCompile(b *testing.B) {
	prepared, err := parserCoreWarmPrepare()
	if err != nil {
		b.Fatal(err)
	}
	compactTree, err := prepared.executeTotal()
	if err != nil {
		b.Fatal(err)
	}
	parserCoreWarmAdmitAgainstProduction(b, prepared, compactTree)
	compactTree.Release()
	b.ReportAllocs()
	b.SetBytes(int64(len(prepared.source)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := prepared.parser.Parse(prepared.source)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}
