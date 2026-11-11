package gotreesitter_test

// Campaign O(edit) workstream W4 (spec.campaign.oedit): the Go external-scanner
// quiescence proof. Go's automatic-semicolon scanner carries no cross-token
// state (grammars/go_scanner.go, external_scanner_quiescence.go), so every
// reuse boundary is quiescent. These real-parse tests enforce two things the
// W4 hard gate requires:
//
//   - Oracle equality on adversarial ASI edges. For every byte offset in each
//     fixture, for insert / delete / same-length replace, where the fresh
//     parse of the edited text is genuinely clean, ParseIncremental must be
//     byte-identical to Parse (a full-fidelity walk over type, span, named,
//     missing, error, and every child including anonymous ones). The fixtures
//     target the exact edges the proof reasons about: statements that end at a
//     newline versus a semicolon, raw strings that span a boundary, and
//     comments before a declaration.
//
//   - The scanner gate is never the binding constraint for Go. Across the whole
//     sweep, ReuseUnsupported stays false and ReuseRejectScannerUnquiescent
//     stays 0, while some site actually reuses subtrees. Go reuse is admitted
//     by the scanner gate; the constraint lives elsewhere (fragility and
//     root-non-leaf), which is the honest W4 finding.

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Compile-time proof that Go's scanner encodes the quiescence marker.
var _ gts.StatelessExternalScanner = grammars.GoExternalScanner{}

// w4GoAdversarialFixtures target the ASI edges the quiescence proof reasons
// about. Each has several top-level declarations so trailing-sibling reuse is
// available for the sweep to exercise.
func w4GoAdversarialFixtures() map[string][]byte {
	backtick := "`"
	return map[string][]byte{
		// Statements ending at a newline versus a semicolon (mixed ASI).
		"asi_newline_vs_semicolon": []byte(
			"package p\n\n" +
				"func a() int {\n\tx := 1\n\ty := 2; z := 3\n\treturn x + y + z\n}\n\n" +
				"func b() int {\n\treturn 7\n}\n\n" +
				"var c = 4\n"),
		// Raw string literals that span a boundary (newlines inside back quotes).
		"raw_string_spanning": []byte(
			"package p\n\n" +
				"var doc = " + backtick + "line one\nline two\nline three" + backtick + "\n\n" +
				"func d() string {\n\treturn " + backtick + "inner\nraw" + backtick + "\n}\n\n" +
				"var tail = 9\n"),
		// Comments (line and block) before a declaration.
		"comment_before_decl": []byte(
			"package p\n\n" +
				"// leading line comment\nfunc e() int {\n\treturn 1\n}\n\n" +
				"/* leading block comment */\nfunc f() int {\n\treturn 2\n}\n\n" +
				"var g = 3 // trailing\n"),
		// A dense top-level declaration list, the reuse workhorse. The bodies
		// are deliberately parameterless: a typed parameter list such as
		// (a, b int) degenerates into Go's untyped-parameter-list ambiguity
		// under a single-byte delete, which is a fragility-gate hazard, not a
		// scanner-quiescence one, and would confound this scanner-focused sweep.
		"many_top_level": func() []byte {
			var b bytes.Buffer
			b.WriteString("package p\n\n")
			for i := 0; i < 12; i++ {
				fmt.Fprintf(&b, "func h%d() int {\n\treturn %d\n}\n\n", i, i)
			}
			return b.Bytes()
		}(),
	}
}

// w4FullFidelityDiff returns the first structural difference between fresh and
// incr, or "" when they are byte-identical trees. It walks every child,
// including anonymous ones, and every attribute the oracle contract covers.
func w4FullFidelityDiff(lang *gts.Language, fresh, incr *gts.Node, path string) string {
	if fresh == nil || incr == nil {
		if fresh == incr {
			return ""
		}
		return fmt.Sprintf("%s: nil mismatch fresh=%v incr=%v", path, fresh != nil, incr != nil)
	}
	ft, it := fresh.Type(lang), incr.Type(lang)
	if ft != it {
		return fmt.Sprintf("%s: type %q != %q", path, ft, it)
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
	if fresh.IsError() != incr.IsError() {
		return fmt.Sprintf("%s: error %v != %v", cur, fresh.IsError(), incr.IsError())
	}
	if fresh.ChildCount() != incr.ChildCount() {
		return fmt.Sprintf("%s: childCount %d != %d", cur, fresh.ChildCount(), incr.ChildCount())
	}
	for i := 0; i < fresh.ChildCount(); i++ {
		if d := w4FullFidelityDiff(lang, fresh.Child(i), incr.Child(i), cur); d != "" {
			return d
		}
	}
	return ""
}

func w4TreeIsGenuinelyClean(root *gts.Node, editedLen int) bool {
	if root == nil {
		return false
	}
	if root.StartByte() != 0 || int(root.EndByte()) != editedLen {
		return false
	}
	errN, missN := incrGateTreeStats(root)
	return errN == 0 && missN == 0
}

// TestGoScannerQuiescenceOracleSweep is the adversarial sweep. It proves
// incremental == fresh byte-for-byte on every freshly-clean ASI-edge site and
// that the scanner gate never blocks or rejects Go reuse.
func TestGoScannerQuiescenceOracleSweep(t *testing.T) {
	lang := grammars.GoLanguage()
	editClasses := []string{"insert", "delete", "replace"}

	var totalSites, cleanSites, reusedSites int
	var freshFallbackSites []string
	var maxReused uint64

	for name, src := range w4GoAdversarialFixtures() {
		// A clean base parse is a fixture invariant.
		// Pin to production: this sweep asserts incremental Go reuse across edits;
		// a compact-materialized base tree is hard-barred from reuse (decision
		// 0008), so the base parse must stay on the production engine.
		baseParser := gts.NewParser(lang)
		baseParser.SetAdmissionCandidateRoute(false)
		baseTree, err := baseParser.Parse(src)
		if err != nil {
			t.Fatalf("%s: base parse: %v", name, err)
		}
		if errN, missN := incrGateTreeStats(baseTree.RootNode()); errN != 0 || missN != 0 {
			t.Fatalf("%s: fixture base parse not clean (err=%d miss=%d)", name, errN, missN)
		}
		baseTree.Release()

		for _, class := range editClasses {
			for off := 0; off < len(src); off++ {
				if class != "insert" && off >= len(src) {
					continue
				}
				edited, edit := incrGateBuildEdit(src, off, class)

				// Fresh parse of the edited text.
				// Pin to production: this sweep compares production incremental
				// reuse against a production fresh parse; keep both on production.
				freshParser := gts.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(false)
				freshTree, err := freshParser.Parse(edited)
				if err != nil {
					t.Fatalf("%s/%s@%d: fresh parse: %v", name, class, off, err)
				}
				freshClean := w4TreeIsGenuinelyClean(freshTree.RootNode(), len(edited))

				// Incremental parse from the edited old tree.
				// Pin to production: a compact-materialized base tree is hard-barred
				// from reuse (decision 0008), so the base parse must stay on production.
				incrParser := gts.NewParser(lang)
				incrParser.SetAdmissionCandidateRoute(false)
				oldTree, err := incrParser.Parse(src)
				if err != nil {
					t.Fatalf("%s/%s@%d: old parse: %v", name, class, off, err)
				}
				oldTree.Edit(edit)
				incrTree, prof, err := incrParser.ParseIncrementalProfiled(edited, oldTree)
				if err != nil {
					t.Fatalf("%s/%s@%d: incremental parse: %v", name, class, off, err)
				}

				totalSites++
				if prof.ReusedSubtrees > 0 {
					reusedSites++
					if prof.ReusedSubtrees > maxReused {
						maxReused = prof.ReusedSubtrees
					}
				}

				// The scanner gate must never block or reject Go reuse, at any
				// site, clean or not.
				// The one allowed exception is the verified fresh fallback
				// "recovery_frontier_unproven"; the sites that take it are
				// recorded and checked against knownFreshFallbackSites below.
				if prof.ReuseUnsupported {
					if prof.ReuseUnsupportedReason != "recovery_frontier_unproven" {
						t.Fatalf("%s/%s@%d: Go reuse unexpectedly unsupported: %q",
							name, class, off, prof.ReuseUnsupportedReason)
					}
					freshFallbackSites = append(freshFallbackSites, fmt.Sprintf("%s/%s@%d", name, class, off))
				}
				if prof.ReuseRejectScannerUnquiescent != 0 {
					t.Fatalf("%s/%s@%d: ReuseRejectScannerUnquiescent=%d, want 0 (Go is quiescent everywhere)",
						name, class, off, prof.ReuseRejectScannerUnquiescent)
				}

				// Oracle equality on freshly-clean sites.
				if freshClean {
					cleanSites++
					if d := w4FullFidelityDiff(lang, freshTree.RootNode(), incrTree.RootNode(), name+"/"+class); d != "" {
						t.Fatalf("%s/%s@%d: incremental != fresh: %s\n--- edited ---\n%s",
							name, class, off, d, string(edited))
					}
				}

				incrTree.Release()
				oldTree.Release()
				freshTree.Release()
			}
		}
	}

	if reusedSites == 0 || maxReused == 0 {
		t.Fatalf("sweep reused no subtrees (reusedSites=%d maxReused=%d): the quiescence assertion is vacuous",
			reusedSites, maxReused)
	}
	// One known site takes the verified fresh fallback. The edit deletes one
	// "/" of the trailing "// trailing" comment in comment_before_decl, so the
	// comment becomes a division ("var g = 3 / trailing"). The leading-run
	// block-splice rule checks only the byte at the end of the "var g = 3"
	// item, not the token after it, so the incremental parse splices the item
	// and gets an error tree; the retry repeats the splice, because the first
	// attempt no longer moves the parent links of the old tree's top-level
	// items. The parse then publishes the verified fresh tree, so the result
	// equals the fresh parse (the oracle comparison above ran for this site).
	// Before the parent-link fix the retry avoided the splice only because
	// those stale links made it reject every top-level item.
	sort.Strings(freshFallbackSites)
	knownFreshFallbackSites := []string{"comment_before_decl/delete@130"}
	if strings.Join(freshFallbackSites, ",") != strings.Join(knownFreshFallbackSites, ",") {
		t.Fatalf("sites that take the verified fresh fallback = %v, want exactly %v", freshFallbackSites, knownFreshFallbackSites)
	}
	t.Logf("sweep: sites=%d freshClean=%d reusedSites=%d maxReusedSubtrees=%d", totalSites, cleanSites, reusedSites, maxReused)
}

// TestGoNearTopInsertScannerGateNotBinding proves, on a representative
// multi-declaration Go file, that a near-top single-char insert reuses
// subtrees with the scanner gate fully open: ReuseUnsupported is false and
// ReuseRejectScannerUnquiescent is 0. This is the measured basis for the W4
// binding-constraint verdict.
func TestGoNearTopInsertScannerGateNotBinding(t *testing.T) {
	lang := grammars.GoLanguage()
	var b bytes.Buffer
	b.WriteString("package p\n\n")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "func f%d(a, b int) int {\n\treturn a + b\n}\n\n", i)
	}
	src := b.Bytes()

	parser := gts.NewParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("base parse: %v", err)
	}
	if oldTree.RootNode().HasError() {
		t.Fatal("fixture base parse not clean")
	}

	const off = 40 // inside the first func's identifier, well past the package clause
	edited := make([]byte, 0, len(src)+1)
	edited = append(edited, src[:off]...)
	edited = append(edited, 'q')
	edited = append(edited, src[off:]...)
	p := incrGatePointAt(src, off)
	oldTree.Edit(gts.InputEdit{
		StartByte: off, OldEndByte: off, NewEndByte: off + 1,
		StartPoint: p, OldEndPoint: p, NewEndPoint: incrGatePointAt(edited, off+1),
	})

	incrTree, prof, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer incrTree.Release()
	defer oldTree.Release()

	if prof.ReuseUnsupported {
		t.Fatalf("Go reuse unsupported: %q", prof.ReuseUnsupportedReason)
	}
	if prof.ReuseRejectScannerUnquiescent != 0 {
		t.Fatalf("ReuseRejectScannerUnquiescent=%d, want 0", prof.ReuseRejectScannerUnquiescent)
	}
	if prof.ReusedSubtrees == 0 {
		t.Fatal("expected Go reuse to take some subtrees with the scanner gate open")
	}

	// Oracle equality with a fresh parse of the edited text.
	freshParser := gts.NewParser(lang)
	freshTree, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	if d := w4FullFidelityDiff(lang, freshTree.RootNode(), incrTree.RootNode(), "nearTop"); d != "" {
		t.Fatalf("incremental != fresh: %s", d)
	}
}
