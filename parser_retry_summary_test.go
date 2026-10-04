package gotreesitter

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
)

// This oracle retains the original stack-entry walk and does not call either
// rank cache, so a faster dense walk cannot hide an omitted descendant here.
func uncachedResultErrorRankOracle(entry stackEntry, arena *nodeArena) int {
	if !stackEntryMaterializesForResult(entry) {
		return 0
	}
	if stackEntryNodeSymbol(entry) == errorSymbol {
		return 2
	}
	rank := 0
	if stackEntryNodeHasError(entry) {
		rank = 1
	}
	for i := 0; i < stackEntryNodeChildCount(entry); i++ {
		child, ok := stackEntryAliasChild(entry, arena, i)
		if !ok {
			continue
		}
		rank = max(rank, uncachedResultErrorRankOracle(child, arena))
		if rank == 2 {
			break
		}
	}
	return rank
}

func TestBorrowedDenseErrorRankMatchesUncachedWalk(t *testing.T) {
	old := newNodeArena(arenaClassFull)
	defer old.Release()
	current := newNodeArena(arenaClassIncremental)
	defer current.Release()
	random := rand.New(rand.NewSource(61))
	for shape := 0; shape < 200; shape++ {
		var nodes []*Node
		for i := 0; i < 40; i++ {
			n := &Node{symbol: 1, ownerArena: old, errorRankCache: uint8(1 + random.Intn(3))}
			if shape%3 == 2 && random.Intn(12) == 0 {
				n.symbol = errorSymbol // Include deliberately under-flagged errors.
			}
			if shape%3 != 0 && random.Intn(8) == 0 {
				n.setHasError(true)
			}
			for child := 0; child < random.Intn(5); child++ {
				if len(nodes) != 0 {
					n.children = append(n.children, nodes[random.Intn(len(nodes))])
				}
			}
			nodes = append(nodes, n)
		}
		root := &Node{symbol: 1, ownerArena: old, children: nodes, errorRankCache: 3}
		entry := newStackEntryNode(0, root)
		want := uncachedResultErrorRankOracle(entry, current)
		if got := cachedStackEntryErrorRank(entry, current); got != want {
			t.Fatalf("shape %d: rank=%d, want %d", shape, got, want)
		}
		if root.errorRankCache != 3 {
			t.Fatal("borrowed rank scan wrote the foreign node cache")
		}
	}
}

func TestBorrowedDenseErrorRankKeepsLazyAndCurrentArenaPaths(t *testing.T) {
	old := newNodeArena(arenaClassFull)
	defer old.Release()
	current := newNodeArena(arenaClassIncremental)
	defer current.Release()
	leaf := newLeafNodeInArena(old, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	childRange, refs := old.allocPendingChildEntries(1)
	refs[0] = newPendingChildEntry(newStackEntryNode(0, leaf))
	lazy := newParentNodeInArenaWithFinalChildRefs(old, 1, true, childRange, 0, false)
	owned := newLeafNodeInArena(current, 1, true, 0, 1, Point{}, Point{Column: 1})
	owned.setHasError(true)
	root := &Node{symbol: 1, ownerArena: old, children: []*Node{nil, owned, lazy}}
	entry := newStackEntryNode(0, root)
	if want, got := uncachedResultErrorRankOracle(entry, current), cachedStackEntryErrorRank(entry, current); got != want || got != 2 {
		t.Fatalf("mixed rank=%d, oracle=%d, want 2", got, want)
	}
	if owned.errorRankCache != 2 {
		t.Fatalf("current arena cache=%d, want rank 1 encoded as 2", owned.errorRankCache)
	}
	if !nodeHasFinalChildRefs(lazy) || lazy.errorRankCache != 0 {
		t.Fatal("foreign lazy children were materialized or cached")
	}
}

func TestBorrowedDenseErrorRankConcurrentReaders(t *testing.T) {
	old := newNodeArena(arenaClassFull)
	defer old.Release()
	leaf := &Node{symbol: errorSymbol, ownerArena: old, errorRankCache: 1}
	root := &Node{symbol: 1, ownerArena: old, children: []*Node{nil, leaf}, errorRankCache: 1}
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			arena := newNodeArena(arenaClassIncremental)
			defer arena.Release()
			for j := 0; j < 100; j++ {
				if rank := cachedStackEntryErrorRank(newStackEntryNode(0, root), arena); rank != 2 {
					t.Errorf("rank=%d, want 2", rank)
				}
			}
		}()
	}
	readers.Wait()
	if root.errorRankCache != 1 || leaf.errorRankCache != 1 {
		t.Fatal("concurrent rank scans changed foreign caches")
	}
}

func TestSummarizeResultErrorsStopsOnActiveParseBudget(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	children := make([]*Node, 0, parseStopPollMask*2)
	for index := 0; index < parseStopPollMask*2; index++ {
		start := uint32(index)
		children = append(children, newLeafNodeInArena(arena, 1, false, start, start+1, Point{Column: start}, Point{Column: start + 1}))
	}
	root := newParentNodeInArena(arena, 2, true, children, nil, 0)
	checks := 0
	stopCheck := func() ParseStopReason {
		checks++
		if checks >= 2 {
			return ParseStopTimeout
		}
		return ParseStopNone
	}

	reason, summary := summarizeResultErrorsWithStop(root, stopCheck)

	if reason != ParseStopTimeout {
		t.Fatalf("stop reason = %q, want %q", reason, ParseStopTimeout)
	}
	if summary != resultErrorSummaryUnknown {
		t.Fatalf("error summary = %d, want unknown", summary)
	}
	if checks != 2 {
		t.Fatalf("stop checks = %d, want 2", checks)
	}
}

func TestSummarizeResultErrorsReturnsClean(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 1, false, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, 2, true, []*Node{leaf}, nil, 0)

	reason, summary := summarizeResultErrorsWithStop(root, nil)

	if reason != ParseStopNone {
		t.Fatalf("stop reason = %q, want none", reason)
	}
	if summary != resultErrorSummaryClean {
		t.Fatalf("error summary = %d, want clean", summary)
	}
}

func TestSummarizeResultErrorsFindsUnderFlaggedDescendant(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	errNode := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	errNode.setHasError(true)
	root := newParentNodeInArena(arena, 1, true, []*Node{errNode}, nil, 0)
	root.setHasError(false)

	reason, summary := summarizeResultErrorsWithStop(root, nil)

	if reason != ParseStopNone {
		t.Fatalf("stop reason = %q, want none", reason)
	}
	if root.HasError() {
		t.Fatal("test setup changed root HasError to true")
	}
	if summary != resultErrorSummaryPresent {
		t.Fatalf("error summary = %d, want present", summary)
	}
}

func TestSummarizeResultErrorsFindsUnderFlaggedErrorRoot(t *testing.T) {
	arena := newNodeArena(arenaClassFull)
	leaf := newLeafNodeInArena(arena, 1, false, 0, 1, Point{}, Point{Column: 1})
	root := newParentNodeInArena(arena, errorSymbol, true, []*Node{leaf}, nil, 0)
	root.setHasError(false)

	reason, summary := summarizeResultErrorsWithStop(root, nil)

	if reason != ParseStopNone {
		t.Fatalf("stop reason = %q, want none", reason)
	}
	if root.HasError() {
		t.Fatal("test setup changed root HasError to true")
	}
	if summary != resultErrorSummaryPresent {
		t.Fatalf("error summary = %d, want present", summary)
	}
}

func TestFinalizeResultRootSummarizesErrorsWithAndWithoutParentWiring(t *testing.T) {
	for _, wireParentLinks := range []bool{false, true} {
		t.Run(fmt.Sprintf("wire_parent_links_%t", wireParentLinks), func(t *testing.T) {
			language := buildArithmeticLanguage()
			parser := NewParser(language)
			arena := newNodeArena(arenaClassFull)
			errNode := newLeafNodeInArena(arena, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
			errNode.setHasError(true)
			root := newParentNodeInArena(arena, 1, true, []*Node{errNode}, nil, 0)
			root.setHasError(false)
			errNode.parent = nil
			errNode.childIndex = -1

			summary, compatibilityApplied := parser.finalizeResultRoot(
				root,
				[]byte("x"),
				nil,
				wireParentLinks,
				false,
				nil,
			)

			if summary != resultErrorSummaryPresent {
				t.Fatalf("error summary = %d, want present", summary)
			}
			if !compatibilityApplied {
				t.Fatal("result compatibility was not recorded as applied")
			}
			if root.HasError() {
				t.Fatal("test setup changed root HasError to true")
			}
			wantParent := (*Node)(nil)
			if wireParentLinks {
				wantParent = root
			}
			if errNode.Parent() != wantParent {
				t.Fatalf("parent = %p, want %p", errNode.Parent(), wantParent)
			}
		})
	}
}

func TestWireParentLinksSummaryPreservesKnownErrorOnStop(t *testing.T) {
	for _, test := range []struct {
		name         string
		rootHasError bool
		want         resultErrorSummary
	}{
		{name: "clean_is_unknown", want: resultErrorSummaryUnknown},
		{name: "known_error_stays_present", rootHasError: true, want: resultErrorSummaryPresent},
	} {
		t.Run(test.name, func(t *testing.T) {
			arena := newNodeArena(arenaClassFull)
			root := newLeafNodeInArena(arena, 1, true, 0, 1, Point{}, Point{Column: 1})
			root.setHasError(test.rootHasError)
			cancelled := uint32(1)
			parser := NewParser(buildArithmeticLanguage())
			parser.SetCancellationFlag(&cancelled)
			summary := resultErrorSummaryUnknown

			complete := wireParentLinksWithScratchUntil(root, nil, parser, &summary)

			if complete {
				t.Fatal("parent-link walk completed after cancellation")
			}
			if summary != test.want {
				t.Fatalf("error summary = %d, want %d", summary, test.want)
			}
		})
	}
}

func TestRetryTreeHasErrorUsesKnownSummaryAndUnknownFallback(t *testing.T) {
	root := newLeafNodeInArena(nil, 1, true, 0, 1, Point{}, Point{Column: 1})
	tree := NewTree(root, []byte("x"), nil)

	tree.resultErrorSummary = resultErrorSummaryClean
	if retryTreeHasError(tree) {
		t.Fatal("known-clean tree reported an error")
	}

	tree.resultErrorSummary = resultErrorSummaryPresent
	if !retryTreeHasError(tree) {
		t.Fatal("known-error tree reported clean")
	}

	errNode := newLeafNodeInArena(nil, errorSymbol, true, 0, 1, Point{}, Point{Column: 1})
	errNode.setHasError(true)
	root.children = []*Node{errNode}
	root.setHasError(false)
	tree.resultErrorSummary = resultErrorSummaryUnknown
	if !retryTreeHasError(tree) {
		t.Fatal("unknown tree did not find descendant error through fallback walk")
	}
}

func TestParseRecordsCleanRetryErrorSummary(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	tree, err := parser.Parse([]byte("1+2"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defer tree.Release()

	if tree.resultErrorSummary != resultErrorSummaryClean {
		t.Fatalf("result error summary = %d, want clean", tree.resultErrorSummary)
	}
	if !tree.resultCompatibilityApplied {
		t.Fatal("result compatibility was not recorded as applied")
	}
	if retryTreeHasError(tree) {
		t.Fatal("clean parsed tree reported an error")
	}
	if parser.goCompatFrames != nil {
		t.Fatal("parser retained active Go compatibility scratch after Parse")
	}
}

func TestVerifiedFreshIncrementalResultIsNotWidenedAgain(t *testing.T) {
	for _, stop := range []ParseStopReason{ParseStopAccepted, ParseStopNoStacksAlive, ParseStopNodeLimit} {
		t.Run(string(stop), func(t *testing.T) {
			tree := newTreeWithArenas(&Node{symbol: errorSymbol, endByte: 1}, []byte("x"), &Language{}, nil, nil)
			defer tree.Release()
			tree.setParseRuntime(ParseRuntime{StopReason: stop, IncrementalOldTreeReuseRoute: true})
			tree.setIncrementalFreshVerified(true)
			if shouldRetryIncrementalParseAsFull(tree, 100, 8) {
				t.Fatal("a complete fresh witness must not run the widening ladder again")
			}
			tree.Edit(InputEdit{StartByte: 0, OldEndByte: 1, NewEndByte: 1, OldEndPoint: Point{Column: 1}, NewEndPoint: Point{Column: 1}})
			if tree.incrementalFreshVerified() {
				t.Fatal("an edit retained verification for the preceding source")
			}
		})
	}
}
