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

var parserCoreWarmQueryCompileWork = core.Work{
	Shifts: 6685, Reductions: 7440, ReductionPopRequests: 7440,
	EmittedPopPaths: 8103, EmittedPopPayloads: 14703,
	GraphLinkAdditionsProxy: 14749, LeafConstructionsProxy: 5546,
	ParentConstructionsProxy: 7537,
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
	source         []byte
	lang           *Language
	parser         *Parser
	compact        *core.Core
	scannerScratch []byte
	acceptedCore   *core.Core
	acceptedHead   core.Head
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
		lang, err := authenticatedParserCoreGoLanguage(parserCoreWarmGoScanner)
		if err != nil {
			parserCoreWarmPrepareErr = err
			return
		}
		parser := NewParser(lang)
		tables, err := newParserCoreRootTables(parser)
		if err != nil {
			parserCoreWarmPrepareErr = err
			return
		}
		compact, err := core.New(tables, parserCoreWarmLimits)
		if err != nil {
			parserCoreWarmPrepareErr = err
			return
		}
		acceptedCore, err := core.New(tables, parserCoreWarmLimits)
		if err != nil {
			parserCoreWarmPrepareErr = err
			return
		}
		prepared := &parserCoreWarmPrepared{
			source: source, lang: lang, parser: parser,
			compact: compact, acceptedCore: acceptedCore,
		}
		scheduler, err := prepared.executeScheduler(acceptedCore, false)
		if err != nil {
			parserCoreWarmPrepareErr = err
			return
		}
		prepared.acceptedHead = scheduler.acceptedHead
		parserCoreWarmPreparedRun = prepared
	})
	return parserCoreWarmPreparedRun, parserCoreWarmPrepareErr
}

func (p *parserCoreWarmPrepared) executeSchedulerOpen(compact *core.Core, reset bool) (*diagnosticParserCoreGenericScheduler, *dfaTokenSource, error) {
	if p == nil || compact == nil || p.parser == nil || p.lang == nil {
		return nil, nil, errors.New("parser-core warm benchmark: incomplete prepared scheduler")
	}
	if reset {
		if err := compact.Reset(); err != nil {
			return nil, nil, err
		}
	}
	tokenSource := p.parser.acquireParserDFATokenSource(p.source)
	if tokenSource == nil {
		return nil, nil, errors.New("parser-core warm benchmark: production DFA unavailable")
	}
	scheduler, err := executeDiagnosticParserCoreGenericSchedulerFromSeed(
		compact, tokenSource, &p.scannerScratch, p.lang.InitialState,
		parserCoreWarmOptions, diagnosticParserCoreSeedObserver{},
	)
	if err != nil {
		tokenSource.Close()
		return scheduler, nil, err
	}
	if scheduler == nil || scheduler.receipt == nil || scheduler.receipt.Acceptance == nil || scheduler.acceptedHead.Node == 0 {
		tokenSource.Close()
		return scheduler, nil, errors.New("parser-core warm benchmark: scheduler did not accept EOF")
	}
	if work := compact.Work(); work != parserCoreWarmQueryCompileWork || work.Overflow {
		tokenSource.Close()
		return scheduler, nil, fmt.Errorf("parser-core warm benchmark: work drifted: got=%+v want=%+v", work, parserCoreWarmQueryCompileWork)
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
	return materializeDiagnosticParserCoreAcceptedTree(compact, head, p.parser, p.source)
}

func (p *parserCoreWarmPrepared) executeTotal() (*Tree, error) {
	scheduler, tokenSource, err := p.executeSchedulerOpen(p.compact, true)
	if err != nil {
		return nil, err
	}
	tree, materializeErr := p.materialize(p.compact, scheduler.acceptedHead)
	tokenSource.Close()
	return tree, materializeErr
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
	second.Release()

	production, err := prepared.parser.Parse(prepared.source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(production.Release)
	parserCoreWarmRequireExactTree(t, production, len(prepared.source))
	parserCoreWarmRequireDeepEqual(t, tree, production, prepared.lang)
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
