package parse_test

// Declarations that the tests in this package share with test files that stay
// in the root package. Entries marked "// from <file>" are copies: the root
// package still uses the originals. Entries marked "// moved from <file>" left
// the root package with the tests that used them.

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// from benchmark_test.go
func makeGoBenchmarkSource(funcCount int) []byte {
	var sb strings.Builder
	sb.Grow(funcCount * 48)
	sb.WriteString("package main\n\n")
	for i := 0; i < funcCount; i++ {
		fmt.Fprintf(&sb, "func f%d() int { v := %d; return v }\n", i, i)
	}
	return []byte(sb.String())
}

// from benchmark_test.go
func pointAtOffset(src []byte, offset int) gotreesitter.Point {
	var row uint32
	var col uint32
	for i := 0; i < offset && i < len(src); {
		r, size := utf8.DecodeRune(src[i:])
		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
		i += size
	}
	return gotreesitter.Point{Row: row, Column: col}
}

// from benchmark_test.go
func mustGoTokenSource(tb testing.TB, src []byte, lang *gotreesitter.Language) *grammars.GoTokenSource {
	tb.Helper()
	ts, err := grammars.NewGoTokenSource(src, lang)
	if err != nil {
		tb.Fatalf("NewGoTokenSource failed: %v", err)
	}
	return ts
}

// from benchmark_test.go
func requireCompleteParse(tb testing.TB, tree *gotreesitter.Tree, src []byte, lang *gotreesitter.Language, phase string) *gotreesitter.Node {
	tb.Helper()
	if tree == nil {
		tb.Fatalf("%s parse returned nil tree", phase)
	}
	root := tree.RootNode()
	if root == nil {
		tb.Fatalf("%s parse returned nil root", phase)
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		rt := tree.ParseRuntime()
		tb.Fatalf("%s parse truncated: root.EndByte=%d want=%d type=%q hasError=%v %s",
			phase, got, want, root.Type(lang), root.HasError(), rt.Summary())
	}
	return root
}

// from glr_forest_dispatch_test.go
func explicitForestLanguage(t *testing.T, lang *gts.Language) *gts.Language {
	t.Helper()
	previous := lang.WantsForest
	lang.WantsForest = true
	t.Cleanup(func() { lang.WantsForest = previous })
	return lang
}

// from glr_forest_dispatch_test.go
func requireAcceptedForestRuntime(t *testing.T, label string, tree *gts.Tree) {
	t.Helper()
	runtime := tree.ParseRuntime()
	if runtime.StopReason != gts.ParseStopAccepted || !runtime.ForestFastPath ||
		!runtime.LastTokenWasEOF || runtime.TokensConsumed != 0 {
		t.Fatalf("%s did not use accepted forest runtime: %s", label, runtime.Summary())
	}
}

// from glr_forest_dispatch_test.go
func requireForestFallbackNodeIdentity(t *testing.T, path string, got, want *gts.Node, lang *gts.Language) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("%s presence got=%t want=%t", path, got != nil, want != nil)
		}
		return
	}
	if got.Type(lang) != want.Type(lang) || got.Symbol() != want.Symbol() {
		t.Fatalf("%s identity got=%q/%d want=%q/%d", path, got.Type(lang), got.Symbol(), want.Type(lang), want.Symbol())
	}
	if got.StartByte() != want.StartByte() || got.EndByte() != want.EndByte() ||
		got.StartPoint() != want.StartPoint() || got.EndPoint() != want.EndPoint() {
		t.Fatalf("%s range got=%d:%d %v:%v want=%d:%d %v:%v", path,
			got.StartByte(), got.EndByte(), got.StartPoint(), got.EndPoint(),
			want.StartByte(), want.EndByte(), want.StartPoint(), want.EndPoint())
	}
	if got.IsNamed() != want.IsNamed() || got.IsExtra() != want.IsExtra() ||
		got.IsMissing() != want.IsMissing() || got.HasError() != want.HasError() {
		t.Fatalf("%s flags differ", path)
	}
	if got.ChildCount() != want.ChildCount() {
		t.Fatalf("%s child count got=%d want=%d", path, got.ChildCount(), want.ChildCount())
	}
	for i := 0; i < got.ChildCount(); i++ {
		childPath := fmt.Sprintf("%s/%d", path, i)
		if got.FieldNameForChild(i, lang) != want.FieldNameForChild(i, lang) {
			t.Fatalf("%s field got=%q want=%q", childPath, got.FieldNameForChild(i, lang), want.FieldNameForChild(i, lang))
		}
		requireForestFallbackNodeIdentity(t, childPath, got.Child(i), want.Child(i), lang)
	}
}

// from glr_forest_dispatch_test.go
// pointForOffset returns the row and byte column of offset in src. An offset
// past the end of src clamps to the end.
func pointForOffset(src []byte, offset int) gts.Point {
	var pt gts.Point
	if offset > len(src) {
		offset = len(src)
	}
	for _, b := range src[:offset] {
		if b == '\n' {
			pt.Row++
			pt.Column = 0
		} else {
			pt.Column++
		}
	}
	return pt
}

// from incremental_invariant_gate_test.go
// incrGateBuildEdit constructs the edited buffer and the corresponding
// InputEdit for one of the three edit classes swept by the gate:
//   - delete:  remove the byte at i
//   - insert:  duplicate the byte at i (a single-byte, non-noop insertion)
//   - replace: substitute the byte at i with a different fixed byte
//     (same-length edit)
func incrGateBuildEdit(src []byte, i int, class string) ([]byte, gts.InputEdit) {
	switch class {
	case "delete":
		edited := make([]byte, 0, len(src)-1)
		edited = append(edited, src[:i]...)
		edited = append(edited, src[i+1:]...)
		p := incrGatePointAt(src, i)
		return edited, gts.InputEdit{
			StartByte: uint32(i), OldEndByte: uint32(i + 1), NewEndByte: uint32(i),
			StartPoint: p, OldEndPoint: incrGatePointAt(src, i+1), NewEndPoint: p,
		}
	case "insert":
		edited := make([]byte, 0, len(src)+1)
		edited = append(edited, src[:i]...)
		edited = append(edited, src[i])
		edited = append(edited, src[i:]...)
		p := incrGatePointAt(src, i)
		return edited, gts.InputEdit{
			StartByte: uint32(i), OldEndByte: uint32(i), NewEndByte: uint32(i + 1),
			StartPoint: p, OldEndPoint: p, NewEndPoint: incrGatePointAt(edited, i+1),
		}
	case "replace":
		repByte := byte('z')
		if src[i] == 'z' {
			repByte = 'y'
		}
		edited := append([]byte{}, src...)
		edited[i] = repByte
		return edited, gts.InputEdit{
			StartByte: uint32(i), OldEndByte: uint32(i + 1), NewEndByte: uint32(i + 1),
			StartPoint: incrGatePointAt(src, i), OldEndPoint: incrGatePointAt(src, i+1), NewEndPoint: incrGatePointAt(edited, i+1),
		}
	default:
		panic("incrGateBuildEdit: unknown edit class " + class)
	}
}

// from incremental_invariant_gate_test.go
func incrGatePointAt(src []byte, off int) gts.Point {
	row, col := 0, 0
	for idx := 0; idx < off && idx < len(src); idx++ {
		if src[idx] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return gts.Point{Row: uint32(row), Column: uint32(col)}
}

// from incremental_invariant_gate_test.go
// incrGateTreeStats walks the tree counting IsError()/IsMissing() nodes.
// Deliberately does NOT use Node.HasError(): the root-cause investigation
// behind this gate found HasError() unreliable as a cleanliness signal.
func incrGateTreeStats(n *gts.Node) (errN, missN int) {
	if n == nil {
		return
	}
	if n.IsError() {
		errN++
	}
	if n.IsMissing() {
		missN++
	}
	for i := 0; i < n.ChildCount(); i++ {
		e, m := incrGateTreeStats(n.Child(i))
		errN += e
		missN += m
	}
	return
}

// moved from issue454_incremental_repro_test.go
// This scanner retains the ordinary reuse contract while withholding read
// certificates, so matching results must still include a fresh verifier.
type issue454UncertifiedReadScanner struct{ gts.ExternalScanner }

// moved from issue454_incremental_repro_test.go
func (issue454UncertifiedReadScanner) SupportsIncrementalReuse() bool { return true }

// moved from issue454_incremental_repro_test.go
func (s issue454UncertifiedReadScanner) ExternalScannerForLanguage(lang *gts.Language) gts.ExternalScanner {
	if provider, ok := s.ExternalScanner.(interface {
		ExternalScannerForLanguage(*gts.Language) gts.ExternalScanner
	}); ok {
		s.ExternalScanner = provider.ExternalScannerForLanguage(lang)
	}
	return s
}

// moved from issue454_incremental_repro_test.go
func issue454UncertifiedLanguage(original *gts.Language) *gts.Language {
	source := reflect.ValueOf(original).Elem()
	clone := reflect.New(source.Type()).Elem()
	for i := 0; i < source.NumField(); i++ {
		if source.Type().Field(i).IsExported() {
			clone.Field(i).Set(source.Field(i))
		}
	}
	language := clone.Addr().Interface().(*gts.Language)
	language.ExternalScanner = issue454UncertifiedReadScanner{original.ExternalScanner}
	return language
}

// from parser_external_scanner_incremental_test.go
// The release disables the unsafe whole-root shortcut, not ordinary reuse.
func requireReleaseSameWidthReparse(t *testing.T, profile gotreesitter.IncrementalParseProfile) {
	t.Helper()
	if profile.ReparseNanos <= 0 {
		t.Fatalf("real edit bypassed reparsing: %+v", profile)
	}
	if profile.ReuseUnsupported && (profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0) {
		t.Fatalf("unsupported reuse reported borrowed work: %+v", profile)
	}
}

// from parser_external_scanner_incremental_test.go
func requireIncrementalDeepTreeMatchesFresh(t *testing.T, incremental, fresh *gotreesitter.Tree, lang *gotreesitter.Language) {
	t.Helper()
	incrementalInspection, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	freshInspection, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	if incrementalInspection.SHA256 != freshInspection.SHA256 {
		t.Fatalf("incremental/fresh deep tree mismatch: incremental=%s fresh=%s", incrementalInspection.SHA256, freshInspection.SHA256)
	}
}

// from parser_go_test.go
// findNamedChild does a depth-first search of the subtree rooted at node,
// returning the first named descendant with the given type. It searches
// through both named and unnamed children recursively.
func findNamedChild(lang *gotreesitter.Language, node *gotreesitter.Node, typeName string) *gotreesitter.Node {
	if node == nil {
		return nil
	}
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child.IsNamed() && child.Type(lang) == typeName {
			return child
		}
		if found := findNamedChild(lang, child, typeName); found != nil {
			return found
		}
	}
	return nil
}

// from parser_sql_incremental_certification_test.go
func requireSQLAcceptedEOF(t *testing.T, tree *gts.Tree, source []byte, phase string) {
	t.Helper()
	if tree == nil {
		t.Fatalf("%s: nil tree", phase)
	}
	rt := tree.ParseRuntime()
	if tree.ParseStoppedEarly() || rt.StopReason != gts.ParseStopAccepted || rt.Truncated ||
		!rt.LastTokenWasEOF || rt.LastTokenEndByte != uint32(len(source)) || rt.ExpectedEOFByte != uint32(len(source)) {
		t.Fatalf("%s: parse was not accepted, non-truncated, and EOF-complete: %s", phase, rt.Summary())
	}
}

// from w1b_reduce_settle_test.go
// w1bStructuralDiff walks fresh and incr in lockstep and returns the first
// structural difference (type, span, named, missing, childCount), or "" when
// the two subtrees are identical. This is the same oracle contract the
// incremental invariant gate enforces, scoped to one edit site.
func w1bStructuralDiff(lang *gts.Language, fresh, incr *gts.Node, path string) string {
	if fresh == nil || incr == nil {
		if fresh == incr {
			return ""
		}
		return fmt.Sprintf("%s: nil mismatch fresh=%v incr=%v", path, fresh != nil, incr != nil)
	}
	ft, it := fresh.Type(lang), incr.Type(lang)
	if ft != it {
		return fmt.Sprintf("%s: type %s != %s", path, ft, it)
	}
	cur := path + ">" + ft
	if fresh.StartByte() != incr.StartByte() || fresh.EndByte() != incr.EndByte() {
		return fmt.Sprintf("%s: span [%d,%d) != [%d,%d)", cur, fresh.StartByte(), fresh.EndByte(), incr.StartByte(), incr.EndByte())
	}
	if fresh.IsNamed() != incr.IsNamed() {
		return fmt.Sprintf("%s: named %v != %v", cur, fresh.IsNamed(), incr.IsNamed())
	}
	if fresh.IsMissing() != incr.IsMissing() {
		return fmt.Sprintf("%s: missing %v != %v", cur, fresh.IsMissing(), incr.IsMissing())
	}
	if fresh.ChildCount() != incr.ChildCount() {
		return fmt.Sprintf("%s: childCount %d != %d", cur, fresh.ChildCount(), incr.ChildCount())
	}
	for i := 0; i < fresh.ChildCount(); i++ {
		if d := w1bStructuralDiff(lang, fresh.Child(i), incr.Child(i), cur); d != "" {
			return d
		}
	}
	return ""
}

// from w1b_reduce_settle_test.go
// w1bInsertByte builds the edited buffer and InputEdit for a single-byte insert
// (duplicating the byte at off), the same near-top keystroke the #380 repro
// uses.
func w1bInsertByte(src []byte, off int) ([]byte, gts.InputEdit) {
	edited := make([]byte, 0, len(src)+1)
	edited = append(edited, src[:off]...)
	edited = append(edited, src[off])
	edited = append(edited, src[off:]...)
	p := pointForOffset(src, off)
	return edited, gts.InputEdit{
		StartByte: uint32(off), OldEndByte: uint32(off), NewEndByte: uint32(off + 1),
		StartPoint: p, OldEndPoint: p, NewEndPoint: pointForOffset(edited, off+1),
	}
}

// from w1b_reduce_settle_test.go
func w1bParseProfiledWithParser(t *testing.T, lang *gts.Language, src []byte, off int, newParser func(*gts.Language) *gts.Parser) (fresh, incr *gts.Node, profile gts.IncrementalParseProfile, edited []byte) {
	t.Helper()
	parser := newParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("base parse: %v", err)
	}
	if oldTree.RootNode().HasError() {
		t.Fatal("fixture invariant: base parse must be clean")
	}

	edited, edit := w1bInsertByte(src, off)

	freshParser := newParser(lang)
	freshTree, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse of edited: %v", err)
	}

	oldTree.Edit(edit)
	incrTree, prof, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	t.Cleanup(func() {
		freshTree.Release()
		incrTree.Release()
		oldTree.Release()
	})
	return freshTree.RootNode(), incrTree.RootNode(), prof, edited
}
