package gotreesitter_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func treeViewWitness(t testing.TB, name string) (*gts.Language, []byte, int) {
	t.Helper()
	size := 32 * 1024
	if value := os.Getenv("GTS_TREE_VIEW_BYTES"); value != "" {
		var err error
		size, err = strconv.Atoi(value)
		if err != nil || size < 1024 {
			t.Fatalf("invalid GTS_TREE_VIEW_BYTES=%q", value)
		}
	}
	source, _, err := benchfixtures.GeneratedSource(name, size)
	if err != nil {
		t.Fatal(err)
	}
	var lang *gts.Language
	switch name {
	case "go":
		lang = grammars.GoLanguage()
	case "c_sharp":
		lang = grammars.CSharpLanguage()
	default:
		t.Fatalf("unknown witness %q", name)
	}
	at := bytes.LastIndex(source, []byte("a + b")) + 2
	if at < 2 {
		t.Fatal("missing edit marker")
	}
	return lang, source, at
}

func assertTreeViewRelations(t *testing.T, root *gts.NodeView) int {
	t.Helper()
	if root == nil || root.Parent() != nil || root.NextSibling() != nil || root.PrevSibling() != nil {
		t.Fatal("invalid view root")
	}
	visited := 1
	var walk func(*gts.NodeView)
	walk = func(parent *gts.NodeView) {
		var previous, previousNamed *gts.NodeView
		namedIndex := 0
		for i := 0; i < parent.ChildCount(); i++ {
			child := parent.Child(i)
			if child == nil || child.Parent() != parent || child.Tree() != root.Tree() || child != parent.Child(i) {
				t.Fatalf("wrong view parent or identity: parent=%s child=%d", parent.Type(root.Tree().Language()), i)
			}
			if child.PrevSibling() != previous {
				t.Fatal("view disagrees with previous sibling")
			}
			if previous != nil && previous.NextSibling() != child {
				t.Fatal("wrong next sibling")
			}
			if child.IsNamed() {
				if parent.NamedChild(namedIndex) != child || child.PrevNamedSibling() != previousNamed {
					t.Fatal("wrong named child or previous named sibling")
				}
				if previousNamed != nil && previousNamed.NextNamedSibling() != child {
					t.Fatal("wrong next named sibling")
				}
				previousNamed = child
				namedIndex++
			}
			previous = child
			visited++
			walk(child)
		}
		if namedIndex != parent.NamedChildCount() || previous != nil && previous.NextSibling() != nil || previousNamed != nil && previousNamed.NextNamedSibling() != nil {
			t.Fatal("wrong child count or last sibling")
		}
	}
	walk(root)
	return visited
}

func walkTreeViews(root *gts.NodeView, visit func(*gts.NodeView)) {
	visit(root)
	for i := 0; i < root.ChildCount(); i++ {
		walkTreeViews(root.Child(i), visit)
	}
}

func TestTreeViewNavigationWitness(t *testing.T) {
	for _, name := range []string{"go", "c_sharp"} {
		t.Run(name, func(t *testing.T) {
			lang, source, at := treeViewWitness(t, name)
			parser := gts.NewParser(lang)
			old, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			oldRoot := old.RootNodeView()
			if oldRoot.HasError() || oldRoot.EndByte() != uint32(len(source)) || old.ParseRuntime().StopReason != gts.ParseStopAccepted {
				t.Fatalf("incomplete clean witness: end=%d bytes=%d error=%t stop=%s", oldRoot.EndByte(), len(source), oldRoot.HasError(), old.ParseRuntime().StopReason)
			}
			oldChildren := oldRoot.Children()
			oldViews := make(map[*gts.Node]*gts.NodeView)
			walkTreeViews(oldRoot, func(view *gts.NodeView) { oldViews[view.Node()] = view })
			beforeRuntime := old.ParseRuntime()
			beforeNodes := assertTreeViewRelations(t, oldRoot)
			edited := applyIncrementalEditSteps(old, source, []incrementalEditStep{{uint32(at), uint32(at + 1), "-"}})
			next, profile, err := parser.ParseIncrementalProfiled(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			requireIncrementalMatchesFresh(t, lang, next, edited, "navigation witness")
			root := next.RootNodeView()
			if root.HasError() || root.EndByte() != uint32(len(edited)) || next.ParseRuntime().StopReason != gts.ParseStopAccepted {
				t.Fatalf("incomplete edited witness: end=%d bytes=%d error=%t stop=%s", root.EndByte(), len(edited), root.HasError(), next.ParseRuntime().StopReason)
			}
			wrongLegacyOld, wrongLegacyNew, reused := 0, 0, 0
			for i, previous := range oldChildren {
				child := root.Child(i)
				if child != nil && child.Node() == previous.Node() {
					reused++
					if child == previous || child.Parent() != root || previous.Parent() != oldRoot {
						t.Fatal("shared payload lost tree-local view identity")
					}
				}
			}
			shared := 0
			walkTreeViews(root, func(view *gts.NodeView) {
				if previous := oldViews[view.Node()]; previous != nil {
					shared++
					if previous == view || previous.Tree() != old || view.Tree() != next {
						t.Fatal("nested reuse lost tree-local identity")
					}
				}
				if parent := view.Parent(); parent != nil && view.Node().Parent() != parent.Node() {
					wrongLegacyNew++
				}
			})
			walkTreeViews(oldRoot, func(view *gts.NodeView) {
				if parent := view.Parent(); parent != nil && view.Node().Parent() != parent.Node() {
					wrongLegacyOld++
				}
			})
			if shared == 0 && !profile.ReuseUnsupported {
				t.Fatalf("witness did not share payloads: profile=%+v", profile)
			}
			afterNodes := assertTreeViewRelations(t, root)
			assertTreeViewRelations(t, oldRoot)
			if old.ParseRuntime().NodesAllocated != beforeRuntime.NodesAllocated {
				t.Fatal("view construction allocated parser payloads")
			}
			t.Logf("bytes=%d source_sha256=%x edited_sha256=%x visited_before=%d visited_after=%d top_level_reused=%d shared_payloads=%d legacy_wrong_parent_old=%d legacy_wrong_parent_new=%d view_wrong_parent=0 tokens=%d new_nodes=%d reused_subtrees=%d reused_bytes=%d reuse_unsupported=%t reuse_reason=%s", len(source), sha256.Sum256(source), sha256.Sum256(edited), beforeNodes, afterNodes, reused, shared, wrongLegacyOld, wrongLegacyNew, profile.TokensConsumed, profile.NewNodesAllocated, profile.ReusedSubtrees, profile.ReusedBytes, profile.ReuseUnsupported, profile.ReuseUnsupportedReason)
			// Release the older tree first. New views must remain navigable through
			// the borrowed arenas; old views must not expose recycled payloads.
			old.Release()
			if oldRoot.Node() != nil || oldRoot.Parent() != nil || oldRoot.Child(0) != nil || old.RootNodeView() != nil {
				t.Fatal("released tree still exposes view payloads")
			}
			assertTreeViewRelations(t, root)
		})
	}
}

func TestTreeViewEditSessions(t *testing.T) {
	for _, fixture := range []struct {
		name string
		lang *gts.Language
		src  string
	}{
		{"go", grammars.GoLanguage(), "package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n"},
		{"c_sharp", grammars.CSharpLanguage(), "class A { int F() { return 1; } }\nclass B { int F() { return 2; } }\n"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gts.NewParser(fixture.lang)
					parser.SetAdmissionCandidateRoute(compact)
					current := []byte(fixture.src)
					old, err := parser.Parse(current)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { old.Release() }()
					// Insert, undo, create a syntax error, and repair it. At each step
					// both live versions retain their own complete navigation graph.
					for _, change := range []struct{ from, to string }{{"1", "1+3"}, {"1+3", "1"}, {"1", "@"}, {"@", "1"}} {
						root := old.RootNodeView()
						assertTreeViewRelations(t, root)
						at := bytes.Index(current, []byte(change.from))
						if at < 0 {
							t.Fatal("missing session marker")
						}
						nextSource := applyIncrementalEditSteps(old, current, []incrementalEditStep{{uint32(at), uint32(at + len(change.from)), change.to}})
						next, err := parser.ParseIncremental(nextSource, old)
						if err != nil {
							t.Fatal(err)
						}
						requireIncrementalMatchesFresh(t, fixture.lang, next, nextSource, "view session")
						assertTreeViewRelations(t, root)
						assertTreeViewRelations(t, next.RootNodeView())
						old.Release()
						old, current = next, nextSource
					}
				})
			}
		})
	}
}

func TestTreeViewAdaptersAndNoEdit(t *testing.T) {
	lang := grammars.GoLanguage()
	source := []byte("package p\nfunc a(x int) { _ = x }\nfunc b() { _ = 2 }\n")
	parser := gts.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	root := tree.RootNodeView()
	cursor := gts.NewTreeCursorFromTree(tree)
	if cursor.CurrentNodeView() != root || !cursor.GotoLastChild() {
		t.Fatal("cursor root adapter failed")
	}
	view := cursor.CurrentNodeView()
	if view.Parent() != root || tree.NodeViewForNode(cursor.CurrentNode()) != view || tree.NodeViewForNode(&gts.Node{}) != nil {
		t.Fatal("cursor or legacy payload adapter lost tree context")
	}
	query, err := gts.NewQuery("(function_declaration name: (identifier) @name)", lang)
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range query.Execute(tree) {
		for _, capture := range match.Captures {
			adapted := tree.NodeViewForNode(capture.Node)
			if adapted == nil || adapted.Parent().Type(lang) != "function_declaration" {
				t.Fatal("query capture adapter lost its parent")
			}
		}
	}
	allocs := testing.AllocsPerRun(100, func() {
		next, err := parser.ParseIncremental(source, tree)
		if err != nil || next.RootNodeView() != root {
			panic("no-edit parse changed view identity")
		}
		next.Release()
	})
	if allocs != 0 {
		t.Fatalf("no-edit parse with views allocated %.2f times", allocs)
	}
	copy := tree.Copy()
	defer copy.Release()
	if copy.RootNodeView() == root || copy.RootNodeView().Child(0) == root.Child(0) {
		t.Fatal("copied tree inherited view identity")
	}
}

func TestTreeViewConcurrentNavigation(t *testing.T) {
	parser := gts.NewParser(grammars.GoLanguage())
	parser.SetAdmissionCandidateRoute(true)
	source := []byte("package p\nfunc a() { _ = 1 }\nfunc b() { _ = 2 }\n")
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	at := bytes.IndexByte(source, '1')
	edited := applyIncrementalEditSteps(old, source, []incrementalEditStep{{uint32(at), uint32(at + 1), "3"}})
	next, err := parser.ParseIncremental(edited, old)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	var group sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			for _, tree := range []*gts.Tree{old, next} {
				root := tree.RootNodeView()
				assertTreeViewRelations(t, root)
				if tree.RootNodeView() != root {
					t.Error("concurrent root creation lost identity")
				}
			}
		}()
	}
	close(start)
	group.Wait()
}

// TestTreeViewLanguageSmoke runs one grammar per invocation when configured,
// so broad API coverage can stay inside isolated per-grammar Docker processes.
func TestTreeViewLanguageSmoke(t *testing.T) {
	names := []string{"go", "c_sharp"}
	if name := os.Getenv("GTS_TREE_VIEW_LANGUAGE"); name != "" {
		names = []string{name}
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(name)
			if entry == nil || entry.Language() == nil {
				t.Fatalf("missing grammar %s", name)
			}
			source := []byte(grammars.ParseSmokeSample(name))
			parser := gts.NewParser(entry.Language())
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNodeView()
			assertV1InvariantTree(t, "view smoke", tree, entry.Language(), source)
			visited := assertTreeViewRelations(t, root)
			allocs := testing.AllocsPerRun(5, func() {
				next, err := parser.ParseIncremental(source, tree)
				if err != nil || next.RootNodeView() != root {
					panic("smoke no-edit reparse changed tree view")
				}
				next.Release()
			})
			if allocs != 0 {
				t.Fatalf("view smoke no-edit allocations=%g, want 0", allocs)
			}
			t.Logf("language=%s bytes=%d root_end=%d has_error=%t stop=%s views=%d wrong_parent=0 no_edit_allocs=%g", name, len(source), root.EndByte(), root.HasError(), tree.ParseRuntime().StopReason, visited, allocs)
		})
	}
}
