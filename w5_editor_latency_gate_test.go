package gotreesitter_test

// Campaign O(edit) workstream W5 (spec.campaign.oedit): the editor-latency
// gate.
//
// This is the continuous CI instrument that locks in the W1 (fragility-gated
// top-level sibling block-splice), W1b (eager-default reduce settling), and
// W2 (memo-cache growth determinism) wins so a future change cannot silently
// reopen the O(file)-per-keystroke regression #380 found.
//
// Design principle: deterministic counters are the hard gate; wall time is
// informational only. CI runners are noisy shared hardware -- a wall-clock
// threshold gate would flake on unrelated host load. IncrementalParseProfile's
// counters (ReuseRejectRootNonLeafChanged, ReusedBytes, NewNodesAllocated,
// TokensConsumed, ...) are proven input-deterministic
// (TestW1BSettleIsDeterministicAcrossFreshParsers, w1b_reduce_settle_test.go,
// and TestW5DeterminismAcrossFreshParsers below), so they are what regresses
// the build. Wall time is still measured and logged for visibility.
//
// # What this gate actually measured (read before changing ceilings)
//
// Building the sweep below (six languages, three positions, three edit
// classes, two size tiers) surfaced two things a single near-top fixture
// cannot:
//
//  1. ReuseRejectRootNonLeafChanged used to be flat only "regardless of file
//     size AT A FIXED near-the-very-start position", NOT "regardless of
//     position in file". The original mechanism
//     (topLevelSiblingBlockSpliceEligible) fast-pathed only the TRAILING run
//     after the edit; content BEFORE the edit was walked one top-level item at
//     a time (~10-17 rejected candidates per leading item), so a "middle" or
//     "bottom" edit multiplied that per-item cost by the leading-item count,
//     which is proportional to file size.
//
//     Campaign post-admission-frontier T2a (the leading-run block-splice, this
//     change) closed that gap for the languages whose per-node fragility
//     marking is complete: go and the production (non-forest) css route now
//     splice the whole run of byte-identical leading top-level items as a block
//     from the initial parser state, the mirror of the trailing splice. Their
//     mid-file reject counter dropped to the near-top constant -- go 16450 ->
//     10 (middle) and 32854 -> 10 (bottom) at 137KB; css 15598 -> 3 and 31163
//     -> 3 -- and byte reuse rose to ~96% at every position. So go and css now
//     carry a per-position MaxRootNonLeafChanged bound (w5PositionCeiling),
//     not only the top-position one, and their middle/bottom byte-reuse floors
//     ratchet up accordingly.
//
//     JavaScript, TypeScript, and TSX are also flattened after #429 retired their
//     language holdback: direct fresh-oracle byte sweeps prove the generic
//     fragility, byte-identity, and scanner gates are sufficient for this
//     family. Their mid-file counters are now the same small constants at
//     20KB and 137KB. Python now reuses checkpoint-authenticated middle and
//     trailing siblings; a first-child length change still uses the
//     conservative frontier fallback.
//  2. ReusedBytes / len(editedSource), unlike the rejection counter, IS
//     input-deterministic AND size-independent when position is expressed
//     as a FRACTION of the file (not an absolute byte offset): a "middle"
//     (~50%-through) edit measures the same byte-reuse percentage at 20KB
//     and at 137KB for the same language, within noise. That makes it the
//     right metric for a ratchet that must hold "regardless of size" at
//     every position, not just at the top, and it is what this gate uses
//     for MinByteReusePercent below.
//
// Coverage: six languages (Go, JavaScript, TypeScript, TSX, Python, CSS) at two
// committed size tiers (~20KB, ~137KB) crossed with three edit classes (insert,
// delete, replace) at three file positions (top ~0%, middle ~50%, bottom
// ~99% through the file) -- 108 samples. A third size tier (~1MB) exists but
// is gated behind GTS_W5_SLOW_TIER=1 (see w5Tiers)
// because it pushes single-run wall time well past a fast local/CI budget;
// it still asserts the same oracle and counter ceilings, it is just not run
// on every `go test .`. JavaScript, TypeScript, and TSX additionally carry an
// explicit insert/delete/replace lane that creates an ERROR at all three
// positions and both committed sizes.
//
// Every sample enforces, unconditionally (the non-negotiable campaign
// invariant, not a soft check):
//
//  1. Oracle equality: the incremental tree is structurally identical to a
//     fresh parse of the same final text (w1bStructuralDiff, the same
//     recursive type/span/named/missing/childCount walk W1b's tests use --
//     equivalent to a byte-identical serialization diff, and strictly
//     stronger since it also catches mismatched node identity that a naive
//     byte compare of two different trees' printed forms could theoretically
//     miss).
//  2. Full-span coverage: the incremental root's end byte covers the whole
//     edited buffer.
//  3. Per-language, per-position counter ceilings (w5Ceilings), which
//     regress the build if a change reopens O(file) reparse cost.
//  4. Source-scaled node-allocation and arena+scratch memory ceilings for
//     both the incremental result and its independent fresh-parse oracle.
//  5. For JS/TS/TSX transient-error edits, explicit causal/work ceilings on
//     tokens, parser iterations, recovery checks, retries, and stack fanout.
//
// # Ceiling table and provenance
//
// Ceilings are the ratchet floor: today's achieved level (this box,
// linux/amd64, GOMAXPROCS default, measured via this harness) minus modest
// slack for cross-hardware noise, not an aspirational target.
//
// After campaign post-admission-frontier T2a (the leading-run block-splice):
//
//	language    | RootNonLeafChanged ceiling (top / mid / bot) | MinByteReusePercent (top / mid / bot)
//	go          | 16 / 16 / 16  (measured 9-10 flat)           | 90 / 90 / 90  (measured ~96 flat)
//	javascript  | 20 / 20 / 20  (measured 7-8 flat)            | 90 / 90 / 90  (measured ~97 flat)
//	typescript  | 24 / 24 / 24  (measured 15-16 flat)          | 90 / 90 / 90  (measured ~97 flat)
//	tsx         | 24 / 24 / 24  (measured 15-16 flat)          | 90 / 90 / 90  (measured ~97 flat)
//	css         | 12 / 12 / 12  (measured 3-7 flat)            | 90 / 90 / 90  (measured ~96 flat)
//	python      | 8  / 16 / 16  (first-child frontier fallback) | 0  / 90 / 90  (middle and trailing reuse)
//
// A "-" mid/bot ceiling means the cell's reject counter still scales with file
// size (that language is not flattened), so no fixed bound is asserted there.
//
//   - go: topRootNonLeafChanged=16. The campaign Status section
//     (hypha://m31labs/gotreesitter spec.campaign.oedit) records W1b's
//     achieved result as "rootNonLeafChanged constant 9 at any size" for
//     clean near-top Go edits; this harness measures 10-11 on its own
//     fixture (a different sibling count than w1b's own 300-sibling
//     fixture, hence the small difference) -- 16 keeps that as a small
//     constant with slack for box variance, satisfying the literal
//     campaign instruction "clean-Go top-level edits rootNonLeafChanged <=
//     16 regardless of size".
//   - css top MinByteReusePercent=90 for the SWEEP's synthetic fixture
//     (measured ~96 on this box). The campaign's literal instruction ("CSS
//     near-top reuse >= 95%, today 99.5%") is enforced precisely, not with
//     this generic sweep's slacker floor, but by a dedicated test using the
//     EXACT fixture the W1 PR #395 merge-gate review measured against
//     (testdata/incremental_gate/css_stylesheet.css):
//     TestW5CSSNearTopReuseAssertion below asserts ReusedBytes/len(edited)
//     >= 95 on that fixture (measured ~97 on this box via that formula; the
//     review's own "99.5%" figure used an unrecorded/different formula on
//     the same fixture -- both are comfortably >= 95). This is the tracked
//     "add a committed W1 measurement assertion" follow-up from that
//     review, now enforced in CI instead of only recorded in a comment.
//   - javascript, typescript, and python: measured on this box via this
//     harness; no campaign-recorded prior figure exists for these fixture
//     cells. JavaScript and TypeScript leading reuse is fresh-oracle certified
//     by #429. Python's complete scanner checkpoints certify unaffected middle
//     and trailing siblings. A first-child length change still lacks a generic
//     parser-frontier proof, so it takes a fresh parse.
//   - python: MinByteReusePercent=0 at the top position for insert/delete is
//     an intentional conservative floor, not a placeholder. The early
//     external_scanner_prefix_frontier_unproven fallback protects a changed
//     first child from stale reduction ownership. Middle and trailing cells
//     now use the 90% floor after checkpoint-authenticated reuse passed the
//     focused Python and locked-C parity proofs.
//   - Every language's REPLACE (same-length substitution) class measures
//     ReusedBytes at ~100% universally: a same-length edit that does not
//     change token boundaries or classification hits an unconditional
//     single-token in-place substitution fast path before the reuse cursor or
//     scanner-quiescence question is reached. See w5ReplaceMinByteReusePercent.
//
// # Follow-ups this gate closes
//
//   - The W1 PR #395 merge-gate review's tracked follow-up "add a committed
//     W1 measurement assertion" for the CSS splice profile:
//     TestW5CSSNearTopReuseAssertion below is exactly that assertion, now
//     enforced in CI rather than only recorded in a review comment.
//   - The W1b bound at w1b_reduce_settle_test.go:179
//     (TestW1BSettleUnlocksGoTopLevelSiblingSplice), which allows
//     ReuseRejectRootNonLeafChanged up to 64 ("a generous bound" per that
//     test's comment). This gate tightens the same property, at the "top"
//     position, to <=16 for go (w5Ceilings["go"].MaxRootNonLeafChangedAtTop).
//     That existing test is left as-is: it is a fixture-specific unit-level
//     proof (300 trailing siblings, single insert) already covered, at the
//     tighter bound, by this gate's cross-product sweep; loosening one to
//     match the other was unnecessary churn on a test that still passes and
//     still guards real regressions.
import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// w5SlowTier reports whether the ~1MB size tier should be included. Only
// the 1MB tier is opt-in, matching the campaign spec's "1MB env-gated as
// the slow tier" instruction. The 20KB and 137KB tiers are always committed.
func w5SlowTier() bool {
	return strings.TrimSpace(os.Getenv("GTS_W5_SLOW_TIER")) != ""
}

// w5EditClass names the byte-level edit shape applied at a sample site.
type w5EditClass int

const (
	w5Insert w5EditClass = iota
	w5Delete
	w5Replace
)

func (c w5EditClass) String() string {
	switch c {
	case w5Insert:
		return "insert"
	case w5Delete:
		return "delete"
	case w5Replace:
		return "replace"
	default:
		return "unknown"
	}
}

// w5Position names which region of the file an edit site falls in,
// expressed as a fraction of total top-level items (see w5EditSiteIndices):
// this is what makes byte-reuse measurements comparable across size tiers.
type w5Position int

const (
	w5Top w5Position = iota
	w5Middle
	w5Bottom
)

func (p w5Position) String() string {
	switch p {
	case w5Top:
		return "start"
	case w5Middle:
		return "middle"
	case w5Bottom:
		return "end"
	default:
		return "unknown"
	}
}

// w5SizeTier names a committed corpus size tier.
type w5SizeTier struct {
	name       string
	targetSize int
}

var (
	w5Tier20KB  = w5SizeTier{name: "20KB", targetSize: 20 * 1024}
	w5Tier137KB = w5SizeTier{name: "137KB", targetSize: 137 * 1024}
	w5Tier1MB   = w5SizeTier{name: "1MB", targetSize: 1024 * 1024}
)

// w5LangSpec is one gate language: how to build a corpus of at least
// targetBytes (returning the source and the number of top-level items
// written) and how to find the edit site for a given item index.
//
// Every generator writes many small, syntactically unambiguous top-level
// items (functions or rule blocks), each carrying its index as a decimal
// literal. w5MinItems guarantees at least a few 2+-digit indices exist even
// for a tiny targetBytes, so "middle"/"bottom" (which select an index >= 10)
// are always safe to single-byte-delete without collapsing an index's last
// digit to nothing; "top" always selects index 0, which single-byte-delete
// reduces to a bare, still-valid identifier suffix in every gate language
// (verified empirically, not just asserted -- see the harness's own probe
// history).
type w5LangSpec struct {
	name        string
	lang        func() *gts.Language
	build       func(targetBytes int) (src []byte, itemCount int)
	marker      func(itemIndex int) string // substring ending right after the item's index digits
	errorMarker func(itemIndex int) string // optional substring whose last byte is required syntax
}

const w5MinItems = 40

func w5BuildGo(targetBytes int) ([]byte, int) {
	var b strings.Builder
	b.Grow(targetBytes + 256)
	b.WriteString("package p\n\n")
	n := 0
	for b.Len() < targetBytes || n < w5MinItems {
		fmt.Fprintf(&b, "func G%d(a int) int {\n\tx := a + %d\n\treturn x\n}\n\n", n, n)
		n++
	}
	return []byte(b.String()), n
}

func w5BuildTypeScript(targetBytes int) ([]byte, int) {
	var b strings.Builder
	b.Grow(targetBytes + 256)
	n := 0
	for b.Len() < targetBytes || n < w5MinItems {
		fmt.Fprintf(&b, "export function f%d(a: number): number {\n  const v%d = a + %d;\n  return v%d;\n}\n\n", n, n, n, n)
		n++
	}
	return []byte(b.String()), n
}

func w5BuildJavaScript(targetBytes int) ([]byte, int) {
	var b strings.Builder
	b.Grow(targetBytes + 256)
	n := 0
	for b.Len() < targetBytes || n < w5MinItems {
		fmt.Fprintf(&b, "export function j%d(a) {\n  const v%d = a + %d;\n  return v%d;\n}\n\n", n, n, n, n)
		n++
	}
	return []byte(b.String()), n
}

func w5BuildPython(targetBytes int) ([]byte, int) {
	var b strings.Builder
	b.Grow(targetBytes + 256)
	n := 0
	for b.Len() < targetBytes || n < w5MinItems {
		fmt.Fprintf(&b, "def f%d(a):\n    v = a + %d\n    return v\n\n", n, n)
		n++
	}
	return []byte(b.String()), n
}

func w5BuildCSS(targetBytes int) ([]byte, int) {
	var b strings.Builder
	b.Grow(targetBytes + 256)
	n := 0
	for b.Len() < targetBytes || n < w5MinItems {
		fmt.Fprintf(&b, ".item-%d {\n  color: #112233;\n  margin: %dpx;\n}\n\n", n, n%64)
		n++
	}
	return []byte(b.String()), n
}

var w5Langs = []w5LangSpec{
	{name: "go", lang: grammars.GoLanguage, build: w5BuildGo, marker: func(i int) string { return fmt.Sprintf("G%d(", i) }},
	{name: "javascript", lang: grammars.JavascriptLanguage, build: w5BuildJavaScript, marker: func(i int) string { return fmt.Sprintf("j%d(", i) }, errorMarker: func(i int) string { return fmt.Sprintf("const v%d =", i) }},
	{name: "typescript", lang: grammars.TypescriptLanguage, build: w5BuildTypeScript, marker: func(i int) string { return fmt.Sprintf("f%d(", i) }, errorMarker: func(i int) string { return fmt.Sprintf("const v%d =", i) }},
	{name: "tsx", lang: grammars.TsxLanguage, build: w5BuildTypeScript, marker: func(i int) string { return fmt.Sprintf("f%d(", i) }, errorMarker: func(i int) string { return fmt.Sprintf("const v%d =", i) }},
	{name: "python", lang: grammars.PythonLanguage, build: w5BuildPython, marker: func(i int) string { return fmt.Sprintf("f%d(", i) }},
	{name: "css", lang: grammars.CssLanguage, build: w5BuildCSS, marker: func(i int) string { return fmt.Sprintf("item-%d ", i) }},
}

// w5EditSiteIndices maps a w5Position to a concrete item index into a
// corpus of itemCount items: top is always the very first item (index 0,
// matching w1b_reduce_settle_test.go's own near-top methodology), middle is
// ~50% through, bottom is ~99% through (itemCount-4, not the literal last
// item, to avoid EOF-adjacent edge effects). itemCount >= w5MinItems (40)
// guarantees middle and bottom always select a 2+-digit index, which
// single-byte delete needs to leave a nonempty numeric literal behind.
func w5EditSiteIndices(itemCount int) map[w5Position]int {
	bottom := itemCount - 4
	if bottom < 12 {
		bottom = itemCount - 1
	}
	return map[w5Position]int{
		w5Top:    0,
		w5Middle: itemCount / 2,
		w5Bottom: bottom,
	}
}

// w5LastDigitOffset finds the byte offset of the last decimal digit of the
// item-index literal embedded in marker (marker's own last byte is always
// the fixed non-digit delimiter that follows the digits, e.g. "G12(" or
// "item-12 ", so the digit is one byte before the end of the match).
func w5LastDigitOffset(t *testing.T, src []byte, marker string) int {
	t.Helper()
	idx := bytes.Index(src, []byte(marker))
	if idx < 0 {
		t.Fatalf("w5 fixture invariant: marker %q not found in generated corpus", marker)
	}
	return idx + len(marker) - 2
}

// w5LastByteOffset finds the last byte in marker. The JavaScript-family
// transient-error lane edits a required assignment operator in a declaration,
// guaranteeing the edited fresh parse contains ERROR/MISSING without making
// recovery consume neighboring top-level declarations.
func w5LastByteOffset(t *testing.T, src []byte, marker string) int {
	t.Helper()
	idx := bytes.Index(src, []byte(marker))
	if idx < 0 {
		t.Fatalf("w5 fixture invariant: marker %q not found in generated corpus", marker)
	}
	return idx + len(marker) - 1
}

// w5ApplyEdit builds the edited buffer and the corresponding InputEdit for a
// single-byte insert/delete/replace at offset. The clean lane edits decimal
// digits; the transient lane edits required punctuation.
func w5ApplyEdit(src []byte, offset int, class w5EditClass) ([]byte, gts.InputEdit) {
	startPoint := w1bPointAt(src, offset)
	switch class {
	case w5Insert:
		edited := make([]byte, 0, len(src)+1)
		edited = append(edited, src[:offset]...)
		edited = append(edited, src[offset])
		edited = append(edited, src[offset:]...)
		return edited, gts.InputEdit{
			StartByte: uint32(offset), OldEndByte: uint32(offset), NewEndByte: uint32(offset + 1),
			StartPoint: startPoint, OldEndPoint: startPoint, NewEndPoint: w1bPointAt(edited, offset+1),
		}
	case w5Delete:
		edited := make([]byte, 0, len(src)-1)
		edited = append(edited, src[:offset]...)
		edited = append(edited, src[offset+1:]...)
		return edited, gts.InputEdit{
			StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset),
			StartPoint: startPoint, OldEndPoint: w1bPointAt(src, offset+1), NewEndPoint: startPoint,
		}
	case w5Replace:
		edited := append([]byte(nil), src...)
		if src[offset] >= '0' && src[offset] <= '9' {
			edited[offset] = (src[offset]-'0'+1)%10 + '0'
		} else if src[offset] == 'z' {
			edited[offset] = 'y'
		} else {
			edited[offset] = 'z'
		}
		return edited, gts.InputEdit{
			StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1),
			StartPoint: startPoint, OldEndPoint: w1bPointAt(src, offset+1), NewEndPoint: w1bPointAt(edited, offset+1),
		}
	default:
		panic("unreachable w5EditClass")
	}
}

// w5Sample is one full run: base parse, apply edit, fresh parse of the
// result, profiled incremental parse of the result, plus the oracle diff
// and wall time.
type w5Sample struct {
	Lang         string
	Size         string
	Class        w5EditClass
	Position     w5Position
	Profile      gts.IncrementalParseProfile
	FreshRuntime gts.ParseRuntime
	EditedLen    int
	OracleDiff   string
	FullSpanOK   bool
	FreshError   bool
	IncrError    bool
	IncrWallNs   int64
	FreshWallNs  int64
}

// w5ProductionParser keeps this incremental reuse gate on its certified route.
// W5 pins fresh and incremental parses to production; compact checkpoint
// admission has a separate first-child frontier gate.
func w5ProductionParser(lang *gts.Language) *gts.Parser {
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	return parser
}

func w5RunSample(t *testing.T, spec w5LangSpec, tier w5SizeTier, class w5EditClass, pos w5Position) w5Sample {
	t.Helper()
	src, itemCount := spec.build(tier.targetSize)
	sites := w5EditSiteIndices(itemCount)
	offset := w5LastDigitOffset(t, src, spec.marker(sites[pos]))
	return w5RunSampleAtOffset(t, spec, tier, class, pos, src, offset)
}

func w5RunSampleAtOffset(t *testing.T, spec w5LangSpec, tier w5SizeTier, class w5EditClass, pos w5Position, src []byte, offset int) w5Sample {
	t.Helper()
	lang := spec.lang()

	parser := w5ProductionParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("%s/%s base parse: %v", spec.name, tier.name, err)
	}
	if oldTree.RootNode().HasError() {
		t.Fatalf("%s/%s fixture invariant: base parse must be clean", spec.name, tier.name)
	}
	defer oldTree.Release()

	edited, edit := w5ApplyEdit(src, offset, class)

	freshParser := w5ProductionParser(lang)
	freshStart := time.Now()
	freshTree, err := freshParser.Parse(edited)
	freshWall := time.Since(freshStart)
	if err != nil {
		t.Fatalf("%s/%s fresh parse of edited: %v", spec.name, tier.name, err)
	}
	defer freshTree.Release()
	freshRuntime := freshTree.ParseRuntime()

	oldTree.Edit(edit)
	incrStart := time.Now()
	incrTree, prof, err := parser.ParseIncrementalProfiled(edited, oldTree)
	incrWall := time.Since(incrStart)
	if err != nil {
		t.Fatalf("%s/%s incremental parse: %v", spec.name, tier.name, err)
	}
	defer incrTree.Release()

	diff := w1bStructuralDiff(lang, freshTree.RootNode(), incrTree.RootNode(), "root")
	fullSpan := int(incrTree.RootNode().EndByte()) == len(edited)

	return w5Sample{
		Lang: spec.name, Size: tier.name, Class: class, Position: pos,
		Profile: prof, FreshRuntime: freshRuntime, EditedLen: len(edited), OracleDiff: diff, FullSpanOK: fullSpan,
		FreshError: freshTree.RootNode().HasError(), IncrError: incrTree.RootNode().HasError(),
		IncrWallNs: incrWall.Nanoseconds(), FreshWallNs: freshWall.Nanoseconds(),
	}
}

const (
	// Every W5 lane, including its independent fresh oracle, stays within a
	// deterministic linear allocation envelope. Runtime baselines exclude
	// capacity retained by earlier parses. The per-KiB term covers grammar and
	// tree density variation.
	//
	// Provenance (combined slow-tier run, all languages sequential): maximum
	// incremental-or-fresh arena+scratch was <42.0MB at 20KB, <49.5MB at
	// 137KB, and <162.5MB at 1MiB. The affine ceiling below is approximately
	// 53.8MB, 72.9MB, and 218.3MB at those generated-source sizes: enough
	// cross-platform/capacity-rounding slack while still bounding linear growth.
	w5MaxNodesPerKiB          = 1400
	w5MemoryFixedBytes  int64 = 48 << 20
	w5MemoryPerKiBBytes int64 = 160 << 10
)

func w5CheckMemoryAllocation(t *testing.T, s w5Sample) {
	t.Helper()
	kib := int64((s.EditedLen + 1023) / 1024)
	maxMemory := w5MemoryFixedBytes + kib*w5MemoryPerKiBBytes
	maxNodes := w5PerKiBCeiling(s.EditedLen, w5MaxNodesPerKiB)

	incrMemory := w5ProfileMemoryGrowth(s.Profile)
	if incrMemory > maxMemory || s.Profile.NewNodesAllocated > maxNodes {
		t.Fatalf("%s/%s/%s/%s: incremental allocation ceiling exceeded: memory=%d/%d newNodes=%d/%d",
			s.Lang, s.Size, s.Class, s.Position, incrMemory, maxMemory, s.Profile.NewNodesAllocated, maxNodes)
	}
	fresh := s.FreshRuntime
	if fresh.ArenaBytesAllocated < 0 || fresh.ScratchBytesAllocated < 0 ||
		fresh.EntryScratchBytesAllocated < 0 || fresh.GSSBytesAllocated < 0 || fresh.NodesAllocated < 0 {
		t.Fatalf("%s/%s/%s/%s: negative fresh-parse allocation counter: arena=%d scratch=%d entry=%d gss=%d nodes=%d",
			s.Lang, s.Size, s.Class, s.Position, fresh.ArenaBytesAllocated, fresh.ScratchBytesAllocated,
			fresh.EntryScratchBytesAllocated, fresh.GSSBytesAllocated, fresh.NodesAllocated)
	}
	freshMemory := w5RuntimeMemoryGrowth(fresh)
	if freshMemory > maxMemory || uint64(fresh.NodesAllocated) > maxNodes {
		t.Fatalf("%s/%s/%s/%s: fresh-oracle allocation ceiling exceeded: memory=%d/%d nodes=%d/%d",
			s.Lang, s.Size, s.Class, s.Position, freshMemory, maxMemory, fresh.NodesAllocated, maxNodes)
	}
}

func w5MemoryGrowth(arenaAllocated, arenaBaseline, scratchAllocated, scratchBaseline int64) int64 {
	arenaGrowth := arenaAllocated - arenaBaseline
	if arenaGrowth < 0 {
		arenaGrowth = 0
	}
	scratchGrowth := scratchAllocated - scratchBaseline
	if scratchGrowth < 0 {
		scratchGrowth = 0
	}
	return arenaGrowth + scratchGrowth
}

func w5ProfileMemoryGrowth(profile gts.IncrementalParseProfile) int64 {
	return w5MemoryGrowth(
		profile.ArenaBytesAllocated,
		profile.ArenaBaselineBytes,
		profile.ScratchBytesAllocated,
		profile.ScratchBaselineBytes,
	)
}

func w5RuntimeMemoryGrowth(runtime gts.ParseRuntime) int64 {
	return w5MemoryGrowth(
		runtime.ArenaBytesAllocated,
		runtime.ArenaBaselineBytes,
		runtime.ScratchBytesAllocated,
		runtime.ScratchBaselineBytes,
	)
}

func TestW5ParseMemoryGrowthExcludesRetainedPoolCapacity(t *testing.T) {
	runtime := gts.ParseRuntime{
		ArenaBytesAllocated:   15 << 20,
		ArenaBaselineBytes:    12 << 20,
		ScratchBytesAllocated: 23 << 20,
		ScratchBaselineBytes:  18 << 20,
	}
	if got, want := w5RuntimeMemoryGrowth(runtime), int64(8<<20); got != want {
		t.Fatalf("memory growth=%d, want %d", got, want)
	}
	runtime.ArenaBaselineBytes = runtime.ArenaBytesAllocated + 1
	runtime.ScratchBaselineBytes = runtime.ScratchBytesAllocated + 1
	if got := w5RuntimeMemoryGrowth(runtime); got != 0 {
		t.Fatalf("negative growth clamp=%d, want 0", got)
	}
}

// w5ByteReusePercent is this gate's "how much of the file was served from
// the old tree" metric: ReusedBytes over the edited buffer's total length.
// It is the metric this file's doc comment found to be size-independent by
// position FRACTION, and it is what the campaign's CSS PR review's "node
// reuse" figures are closest to when recomputed on that review's own
// fixture (see TestW5CSSNearTopReuseAssertion).
func w5ByteReusePercent(p gts.IncrementalParseProfile, editedLen int) float64 {
	if editedLen <= 0 {
		return 100
	}
	return float64(p.ReusedBytes) / float64(editedLen) * 100
}

// w5PositionCeiling is the byte-reuse floor for one (language, position)
// cell, applied to insert/delete classes.
type w5PositionCeiling struct {
	MinByteReusePercent float64
	// MaxRootNonLeafChanged bounds ReuseRejectRootNonLeafChanged at THIS
	// position. Zero means "not asserted here" (the pre-T2a default for every
	// non-top cell: a position whose reject counter still scales with file size
	// cannot carry a fixed, size-independent ceiling). It is set nonzero only
	// for a (language, position) the block-splice has FLATTENED -- made
	// size-independent -- so a fixed bound is meaningful and locks the win.
	// Campaign post-admission-frontier T2a (the leading-run splice) flattened
	// go and production-route css at middle and bottom, so those cells now carry
	// this bound; see w5Ceilings.
	MaxRootNonLeafChanged uint64
}

// w5Ceiling is the ratchet floor for one language. See the file doc comment
// for full provenance of every value.
type w5Ceiling struct {
	// MaxRootNonLeafChangedAtTop bounds ReuseRejectRootNonLeafChanged, but
	// ONLY at the "top" position -- see the file doc comment's "What this
	// gate actually measured" section for why middle/bottom positions do
	// not get this same flat ceiling today.
	MaxRootNonLeafChangedAtTop uint64
	Top, Middle, Bottom        w5PositionCeiling
}

// w5ReplaceMinByteReusePercent is the universal (language-independent)
// floor for the REPLACE edit class: a same-length substitution that does
// not change a token's boundaries or classification hits an unconditional
// single-token in-place fast path (measured ~100% on every gate language,
// while Python's top-position insert/delete lane uses the frontier fallback
// described in the file comment.
const w5ReplaceMinByteReusePercent = 95

// w5Ceilings is the committed ratchet: today's achieved level per
// (language, position), minus modest slack. See the file doc comment for
// full provenance of every value here.
var w5Ceilings = map[string]w5Ceiling{
	// go: campaign post-admission-frontier T2a (the leading-run block-splice)
	// flattened the mid-file reject counter and byte reuse to the near-top
	// constant. Measured on this box (137KB and 20KB agree, the counter is
	// size-independent): ReuseRejectRootNonLeafChanged 9-10 and byte reuse
	// ~95.8-96.1% at EVERY position (was middle 16450 / 70.7% and bottom 32854 /
	// 45.4% at 137KB before T2a). MaxRootNonLeafChanged=16 at every position
	// keeps that as a small constant with box slack; the byte-reuse floors ratchet
	// from the old 88/60/35 to 90/90/90 to lock the flattening.
	"go": {
		MaxRootNonLeafChangedAtTop: 16,
		Top:                        w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 16},
		Middle:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 16},
		Bottom:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 16},
	},
	// typescript: #429 admits the ambiguous-expression / ASI family to the
	// generic leading-run splice. The counter measures 15-16 at every position
	// and both committed sizes, with about 97% byte reuse.
	"typescript": {
		MaxRootNonLeafChangedAtTop: 24,
		Top:                        w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 24},
		Middle:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 24},
		Bottom:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 24},
	},
	// tsx shares TypeScript's generated corpus shape here and carries the same
	// independent language/parser oracle and counter ratchets.
	"tsx": {
		MaxRootNonLeafChangedAtTop: 24,
		Top:                        w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 24},
		Middle:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 24},
		Bottom:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 24},
	},
	// javascript shares TypeScript's ASI/ambiguous-expression constraints and
	// is likewise flattened: 7-8 rejects and about 97% byte reuse at every
	// position and size. The dedicated transient-error gate below carries
	// the stricter recovery-work contract for malformed edits.
	"javascript": {
		MaxRootNonLeafChangedAtTop: 20,
		Top:                        w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 20},
		Middle:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 20},
		Bottom:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 20},
	},
	// css: flattened by T2a on the production (non-forest) route, the same as go.
	// Measured ~95.8-95.9% byte reuse and ReuseRejectRootNonLeafChanged 3-7 at
	// every position (was middle 15598 / 85.8% and bottom 31163 / 75.7% at 137KB).
	// MaxRootNonLeafChanged=12 at every position; byte-reuse floors ratchet from
	// 88/75/65 to 90/90/90. (Forest-route css keeps its leading-prefix scanner
	// guard and is unaffected -- see forestFastPathDirtyPrefixScannerSensitive.)
	"css": {
		MaxRootNonLeafChangedAtTop: 12,
		Top:                        w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 12},
		Middle:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 12},
		Bottom:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 12},
	},
	// python: first-child length edits use the conservative frontier fallback;
	// middle and trailing edits reuse checkpoint-authenticated siblings.
	"python": {
		MaxRootNonLeafChangedAtTop: 8,
		Top:                        w5PositionCeiling{MinByteReusePercent: 0},
		Middle:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 16},
		Bottom:                     w5PositionCeiling{MinByteReusePercent: 90, MaxRootNonLeafChanged: 16},
	},
}

func (c w5Ceiling) forPosition(pos w5Position) w5PositionCeiling {
	switch pos {
	case w5Top:
		return c.Top
	case w5Middle:
		return c.Middle
	default:
		return c.Bottom
	}
}

// w5CheckCommon applies the correctness and resource-observability contract
// shared by clean and transient-error lanes.
func w5CheckCommon(t *testing.T, s w5Sample) {
	t.Helper()
	if s.OracleDiff != "" {
		t.Fatalf("%s/%s/%s/%s: oracle divergence (incremental != fresh parse): %s",
			s.Lang, s.Size, s.Class, s.Position, s.OracleDiff)
	}
	if !s.FullSpanOK {
		t.Fatalf("%s/%s/%s/%s: incremental root does not cover the full edited buffer", s.Lang, s.Size, s.Class, s.Position)
	}
	if s.Profile.ArenaBytesAllocated < 0 || s.Profile.ScratchBytesAllocated < 0 ||
		s.Profile.EntryScratchBytesAllocated < 0 || s.Profile.GSSBytesAllocated < 0 {
		t.Fatalf("%s/%s/%s/%s: negative memory counter: arena=%d scratch=%d entry=%d gss=%d",
			s.Lang, s.Size, s.Class, s.Position, s.Profile.ArenaBytesAllocated,
			s.Profile.ScratchBytesAllocated, s.Profile.EntryScratchBytesAllocated, s.Profile.GSSBytesAllocated)
	}
	if s.Profile.SingleStackIterations < 0 || s.Profile.MultiStackIterations < 0 || s.Profile.MaxStacksSeen < 0 {
		t.Fatalf("%s/%s/%s/%s: negative causal-work counter: single=%d multi=%d maxStacks=%d",
			s.Lang, s.Size, s.Class, s.Position, s.Profile.SingleStackIterations,
			s.Profile.MultiStackIterations, s.Profile.MaxStacksSeen)
	}
	if (s.Profile.AcceptedErrorRetryAttempts == 0) != (s.Profile.AcceptedErrorRetryCause == gts.IncrementalRetryCauseNone) ||
		(s.Profile.AcceptedErrorRetryAdopted && s.Profile.AcceptedErrorRetryAttempts == 0) {
		t.Fatalf("%s/%s/%s/%s: inconsistent retry causality: attempts=%d adopted=%v cause=%v",
			s.Lang, s.Size, s.Class, s.Position, s.Profile.AcceptedErrorRetryAttempts,
			s.Profile.AcceptedErrorRetryAdopted, s.Profile.AcceptedErrorRetryCause)
	}
	if s.Profile.ExpectedEOFByte != uint32(s.EditedLen) {
		t.Fatalf("%s/%s/%s/%s: profile expectedEOF=%d, want editedLen=%d",
			s.Lang, s.Size, s.Class, s.Position, s.Profile.ExpectedEOFByte, s.EditedLen)
	}
	if s.FreshRuntime.ExpectedEOFByte != uint32(s.EditedLen) || s.FreshRuntime.RootEndByte != uint32(s.EditedLen) {
		t.Fatalf("%s/%s/%s/%s: fresh runtime span mismatch: expectedEOF=%d rootEnd=%d editedLen=%d",
			s.Lang, s.Size, s.Class, s.Position, s.FreshRuntime.ExpectedEOFByte,
			s.FreshRuntime.RootEndByte, s.EditedLen)
	}
	if s.Profile.ReuseRejectFragileNonLeaf > 0 {
		t.Logf("%s/%s/%s/%s: ReuseRejectFragileNonLeaf=%d (correctness rejection; observable, not a failure)",
			s.Lang, s.Size, s.Class, s.Position, s.Profile.ReuseRejectFragileNonLeaf)
	}
}

// w5Check applies the clean-edit reuse ratchets after the common contract.
func w5Check(t *testing.T, s w5Sample) {
	t.Helper()
	w5CheckCommon(t, s)
	w5CheckMemoryAllocation(t, s)

	ceiling, ok := w5Ceilings[s.Lang]
	if !ok {
		t.Fatalf("%s: no ceiling registered in w5Ceilings", s.Lang)
	}
	if s.Position == w5Top && s.Profile.ReuseRejectRootNonLeafChanged > ceiling.MaxRootNonLeafChangedAtTop {
		t.Fatalf("%s/%s/%s/%s: ReuseRejectRootNonLeafChanged=%d exceeds top-position ceiling %d -- O(edit) near-top reuse regressed toward O(file)",
			s.Lang, s.Size, s.Class, s.Position, s.Profile.ReuseRejectRootNonLeafChanged, ceiling.MaxRootNonLeafChangedAtTop)
	}
	// Per-position flat ceiling (campaign post-admission-frontier T2a): where the
	// leading-run splice flattened the reject counter (go, css at every
	// position), assert it stays a small size-independent constant so a future
	// change cannot silently reopen O(preceding-items) mid-file reparse cost. A
	// zero bound means the cell is not flattened (still scales with size) and is
	// left unasserted, exactly as before T2a.
	if posMax := ceiling.forPosition(s.Position).MaxRootNonLeafChanged; posMax > 0 && s.Profile.ReuseRejectRootNonLeafChanged > posMax {
		t.Fatalf("%s/%s/%s/%s: ReuseRejectRootNonLeafChanged=%d exceeds flattened position ceiling %d -- the T2a leading-run block-splice regressed toward O(file) mid-file",
			s.Lang, s.Size, s.Class, s.Position, s.Profile.ReuseRejectRootNonLeafChanged, posMax)
	}

	minReuse := w5ReplaceMinByteReusePercent
	if s.Class != w5Replace {
		minReuse = int(ceiling.forPosition(s.Position).MinByteReusePercent)
	}
	// Python 20KB/replace/start and 137KB/replace/start previously used the disabled shortcut.
	// Only Python start replacements use the existing start-position floor after this proof.
	if s.Lang == "python" && s.Position == w5Top && s.Class == w5Replace {
		p := s.Profile
		if !p.ReuseUnsupported || p.ReuseUnsupportedReason != "external_scanner_prefix_frontier_unproven" ||
			p.OldTreeReuseRoute || p.ReusedSubtrees != 0 || p.ReusedBytes != 0 ||
			p.ReparseNanos <= 0 || p.NewNodesAllocated == 0 || p.TokensConsumed == 0 ||
			p.StopReason != gts.ParseStopAccepted {
			t.Fatalf("%s/%s/%s/%s: expected the authenticated prefix-frontier fallback: %+v",
				s.Lang, s.Size, s.Class, s.Position, p)
		}
		minReuse = int(ceiling.forPosition(s.Position).MinByteReusePercent)
	}
	rate := w5ByteReusePercent(s.Profile, s.EditedLen)
	if rate < float64(minReuse) {
		t.Fatalf("%s/%s/%s/%s: byte reuse %.1f%% below floor %d%% (reusedBytes=%d editedLen=%d) -- O(edit) reuse regressed",
			s.Lang, s.Size, s.Class, s.Position, rate, minReuse, s.Profile.ReusedBytes, s.EditedLen)
	}

	t.Logf("%s/%s/%s/%s: rootNonLeafChanged=%d reusedBytes=%d byteReuse=%.1f%% newNodes=%d tokensConsumed=%d singleIters=%d multiIters=%d recoverChecks=%d retries=%d maxStacks=%d arenaBytes=%d scratchBytes=%d freshNodes=%d freshArenaBytes=%d freshScratchBytes=%d incrWall=%s freshWall=%s",
		s.Lang, s.Size, s.Class, s.Position,
		s.Profile.ReuseRejectRootNonLeafChanged, s.Profile.ReusedBytes, rate, s.Profile.NewNodesAllocated,
		s.Profile.TokensConsumed, s.Profile.SingleStackIterations, s.Profile.MultiStackIterations,
		s.Profile.RecoverStateChecks, s.Profile.AcceptedErrorRetryAttempts, s.Profile.MaxStacksSeen,
		s.Profile.ArenaBytesAllocated, s.Profile.ScratchBytesAllocated,
		s.FreshRuntime.NodesAllocated, s.FreshRuntime.ArenaBytesAllocated, s.FreshRuntime.ScratchBytesAllocated,
		time.Duration(s.IncrWallNs), time.Duration(s.FreshWallNs))
}

const (
	// These are deterministic, source-size-scaled work ceilings, deliberately
	// not wall-clock thresholds. They are the measured 20KB/137KB maxima rounded
	// upward: about 1.1K nodes, 220 tokens, and 450 parser iterations per KiB.
	w5TransientMaxNewNodesPerKiB      = 1200
	w5TransientMaxTokensPerKiB        = 256
	w5TransientMaxIterationsPerKiB    = 512
	w5TransientMaxRecoverChecksPerKiB = 64
	w5TransientMaxRetryAttempts       = 1
	w5TransientMaxStacks              = 6
	// The transient lane remains tighter than the general envelope. Runtime
	// baselines exclude capacity retained by unrelated parses. The constants
	// yield approximately 37.0MB, 56.2MB, and 201.5MB for the three tiers.
	w5TransientMemoryFixedBytes  int64 = 32 << 20
	w5TransientMemoryPerKiBBytes int64 = 160 << 10
)

func w5PerKiBCeiling(editedLen int, perKiB uint64) uint64 {
	kib := uint64((editedLen + 1023) / 1024)
	return kib * perKiB
}

// w5CheckTransientErrorWork turns JavaScript-family error-edit work into a
// reproducible linear-work contract. Wall time remains evidence only.
func w5CheckTransientErrorWork(t *testing.T, s w5Sample) {
	t.Helper()
	p := s.Profile
	iterations := uint64(p.SingleStackIterations + p.MultiStackIterations)
	checks := []struct {
		name string
		got  uint64
		max  uint64
	}{
		{name: "newNodes", got: p.NewNodesAllocated, max: w5PerKiBCeiling(s.EditedLen, w5TransientMaxNewNodesPerKiB)},
		{name: "tokensConsumed", got: p.TokensConsumed, max: w5PerKiBCeiling(s.EditedLen, w5TransientMaxTokensPerKiB)},
		{name: "parserIterations", got: iterations, max: w5PerKiBCeiling(s.EditedLen, w5TransientMaxIterationsPerKiB)},
		{name: "recoverStateChecks", got: p.RecoverStateChecks, max: w5PerKiBCeiling(s.EditedLen, w5TransientMaxRecoverChecksPerKiB)},
	}
	for _, check := range checks {
		if check.got > check.max {
			t.Fatalf("%s/%s/%s-transient-error/%s: %s=%d exceeds deterministic linear-work ceiling %d",
				s.Lang, s.Size, s.Class, s.Position, check.name, check.got, check.max)
		}
	}
	if p.AcceptedErrorRetryAttempts > w5TransientMaxRetryAttempts || p.MaxStacksSeen > w5TransientMaxStacks {
		t.Fatalf("%s/%s/%s-transient-error/%s: causal bound exceeded: retries=%d/%d maxStacks=%d/%d",
			s.Lang, s.Size, s.Class, s.Position, p.AcceptedErrorRetryAttempts, w5TransientMaxRetryAttempts,
			p.MaxStacksSeen, w5TransientMaxStacks)
	}
	kib := int64((s.EditedLen + 1023) / 1024)
	memory := w5ProfileMemoryGrowth(p)
	maxMemory := w5TransientMemoryFixedBytes + kib*w5TransientMemoryPerKiBBytes
	if memory > maxMemory {
		t.Fatalf("%s/%s/%s-transient-error/%s: arena+scratch memory=%d exceeds affine ceiling %d",
			s.Lang, s.Size, s.Class, s.Position, memory, maxMemory)
	}
}

// w5Tiers returns the size tiers this invocation should sweep.
func w5Tiers() []w5SizeTier {
	tiers := []w5SizeTier{w5Tier20KB, w5Tier137KB}
	if w5SlowTier() {
		tiers = append(tiers, w5Tier1MB)
	}
	return tiers
}

// TestW5EditorLatencyGate is the gate. It sweeps every (language, size
// tier, edit class, position) combination for the tiers w5Tiers selects and
// enforces the oracle + counter ceilings on each one. See the file doc
// comment for design, coverage, and ceiling provenance.
//
// The default sweep commits both issue #380 reporter sizes: 6 languages x
// 2 sizes x 3 classes x 3 positions = 108 samples. The 1MB lane remains
// explicitly opt-in because it is diagnostic rather than presubmit-sized.
func TestW5EditorLatencyGate(t *testing.T) {
	classes := []w5EditClass{w5Insert, w5Delete, w5Replace}
	positions := []w5Position{w5Top, w5Middle, w5Bottom}

	for _, spec := range w5Langs {
		spec := spec
		t.Run(spec.name, func(t *testing.T) {
			for _, tier := range w5Tiers() {
				tier := tier
				t.Run(tier.name, func(t *testing.T) {
					for _, class := range classes {
						class := class
						t.Run(class.String(), func(t *testing.T) {
							for _, pos := range positions {
								pos := pos
								t.Run(pos.String(), func(t *testing.T) {
									s := w5RunSample(t, spec, tier, class, pos)
									w5Check(t, s)
								})
							}
						})
					}
				})
			}
		})
	}
}

// TestW5JavaScriptFamilyTransientErrorGate covers malformed edits directly.
// Editing required syntax while typing creates a temporary ERROR tree. This is
// separate from the valid cells above so the normal reuse floors remain
// meaningful.
func TestW5JavaScriptFamilyTransientErrorGate(t *testing.T) {
	classes := []w5EditClass{w5Insert, w5Delete, w5Replace}
	positions := []w5Position{w5Top, w5Middle, w5Bottom}
	for _, spec := range w5Langs {
		spec := spec
		if spec.errorMarker == nil {
			continue
		}
		t.Run(spec.name, func(t *testing.T) {
			for _, tier := range w5Tiers() {
				tier := tier
				t.Run(tier.name, func(t *testing.T) {
					src, itemCount := spec.build(tier.targetSize)
					sites := w5EditSiteIndices(itemCount)
					for _, class := range classes {
						class := class
						t.Run(class.String(), func(t *testing.T) {
							for _, pos := range positions {
								pos := pos
								t.Run(pos.String(), func(t *testing.T) {
									offset := w5LastByteOffset(t, src, spec.errorMarker(sites[pos]))
									s := w5RunSampleAtOffset(t, spec, tier, class, pos, src, offset)
									w5CheckCommon(t, s)
									w5CheckMemoryAllocation(t, s)
									if !s.FreshError || !s.IncrError {
										t.Fatalf("%s/%s/%s-transient-error/%s: fixture did not produce matching ERROR trees (fresh=%v incremental=%v)",
											s.Lang, s.Size, s.Class, s.Position, s.FreshError, s.IncrError)
									}
									w5CheckTransientErrorWork(t, s)
									t.Logf("%s/%s/%s-transient-error/%s: newNodes=%d tokensConsumed=%d singleIters=%d multiIters=%d recoverChecks=%d retries=%d maxStacks=%d arenaBytes=%d scratchBytes=%d freshNodes=%d freshArenaBytes=%d freshScratchBytes=%d incrWall=%s freshWall=%s",
										s.Lang, s.Size, s.Class, s.Position, s.Profile.NewNodesAllocated, s.Profile.TokensConsumed,
										s.Profile.SingleStackIterations, s.Profile.MultiStackIterations, s.Profile.RecoverStateChecks,
										s.Profile.AcceptedErrorRetryAttempts, s.Profile.MaxStacksSeen,
										s.Profile.ArenaBytesAllocated, s.Profile.ScratchBytesAllocated,
										s.FreshRuntime.NodesAllocated, s.FreshRuntime.ArenaBytesAllocated, s.FreshRuntime.ScratchBytesAllocated,
										time.Duration(s.IncrWallNs), time.Duration(s.FreshWallNs))
								})
							}
						})
					}
				})
			}
		})
	}
}

// TestW5CSSNearTopReuseAssertion is the committed W1 measurement assertion
// the W1 PR #395 merge-gate review tracked as a follow-up ("add a committed
// W1 measurement assertion" for the CSS splice profile). It runs the exact
// methodology and the exact fixture
// (testdata/incremental_gate/css_stylesheet.css, the same file
// TestIncrementalInvariantGate sweeps) that review recorded "node reuse
// 63.8% -> 99.5%" for on a near-top single-char insert, and asserts the
// literal campaign floor: byte reuse >= 95%.
func TestW5CSSNearTopReuseAssertion(t *testing.T) {
	src, err := os.ReadFile("testdata/incremental_gate/css_stylesheet.css")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	lang := grammars.CssLanguage()

	// Insert immediately before the first rule's opening brace, mirroring
	// the PR #395 review's "near-top insert" methodology.
	braceIdx := bytes.IndexByte(src, '{')
	if braceIdx <= 0 {
		t.Fatal("fixture invariant: no CSS rule found near the top of the fixture")
	}
	offset := braceIdx - 1

	parser := w5ProductionParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("base parse: %v", err)
	}
	if oldTree.RootNode().HasError() {
		t.Fatal("fixture invariant: base parse must be clean")
	}
	defer oldTree.Release()

	edited, edit := w5ApplyEdit(src, offset, w5Insert)

	freshParser := w5ProductionParser(lang)
	freshTree, err := freshParser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse of edited: %v", err)
	}
	defer freshTree.Release()

	oldTree.Edit(edit)
	incrTree, prof, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer incrTree.Release()

	if d := w1bStructuralDiff(lang, freshTree.RootNode(), incrTree.RootNode(), "root"); d != "" {
		t.Fatalf("oracle divergence (incremental != fresh parse): %s", d)
	}
	if int(incrTree.RootNode().EndByte()) != len(edited) {
		t.Fatalf("incremental root does not cover the full edited buffer")
	}

	rate := w5ByteReusePercent(prof, len(edited))
	t.Logf("css_stylesheet.css near-top insert: rootNonLeafChanged=%d reusedBytes=%d byteReuse=%.1f%% tokensConsumed=%d",
		prof.ReuseRejectRootNonLeafChanged, prof.ReusedBytes, rate, prof.TokensConsumed)
	if rate < 95 {
		t.Fatalf("CSS near-top byte reuse %.1f%% below the campaign floor of 95%% (spec.campaign.oedit W5 point 2; PR #395 review recorded 99.5%% via a different formula on this same fixture)", rate)
	}
}

// TestW5DeterminismAcrossFreshParsers is the gate's self-check: the same
// (source, edit, language) run on two independent, freshly constructed Parser
// instances must produce identical profile counters. The JavaScript family is
// checked at every enabled tier; the remaining gate languages retain a 20KB
// witness. A future parser-history dependency (the class of bug #380's
// cNodeMemoCache warm/cold lottery was) therefore fails here instead of only
// appearing as gate flakiness.
func TestW5DeterminismAcrossFreshParsers(t *testing.T) {
	for _, spec := range w5Langs {
		spec := spec
		tiers := []w5SizeTier{w5Tier20KB}
		if spec.errorMarker != nil {
			tiers = w5Tiers()
		}
		t.Run(spec.name, func(t *testing.T) {
			for _, tier := range tiers {
				tier := tier
				t.Run(tier.name, func(t *testing.T) {
					src, itemCount := spec.build(tier.targetSize)
					sites := w5EditSiteIndices(itemCount)
					offset := w5LastDigitOffset(t, src, spec.marker(sites[w5Middle]))

					run := func() gts.IncrementalParseProfile {
						parser := w5ProductionParser(spec.lang())
						oldTree, err := parser.Parse(src)
						if err != nil {
							t.Fatalf("base parse: %v", err)
						}
						defer oldTree.Release()
						edited, edit := w5ApplyEdit(src, offset, w5Insert)
						oldTree.Edit(edit)
						incrTree, prof, err := parser.ParseIncrementalProfiled(edited, oldTree)
						if err != nil {
							t.Fatalf("incremental parse: %v", err)
						}
						defer incrTree.Release()
						return prof
					}

					a := run()
					b := run()
					// Allocation bytes have independent affine ceilings above, but
					// are intentionally not exact-equal: shared pool capacity can
					// differ between otherwise equivalent fresh parser runs.
					if a.ReuseRejectRootNonLeafChanged != b.ReuseRejectRootNonLeafChanged ||
						a.ReusedBytes != b.ReusedBytes ||
						a.NewNodesAllocated != b.NewNodesAllocated ||
						a.TokensConsumed != b.TokensConsumed ||
						a.ReuseRejectFragileNonLeaf != b.ReuseRejectFragileNonLeaf ||
						a.BlockSpliceSteps != b.BlockSpliceSteps ||
						a.SingleStackIterations != b.SingleStackIterations ||
						a.MultiStackIterations != b.MultiStackIterations ||
						a.RecoverStateChecks != b.RecoverStateChecks ||
						a.AcceptedErrorRetryAttempts != b.AcceptedErrorRetryAttempts ||
						a.MaxStacksSeen != b.MaxStacksSeen {
						t.Fatalf("W5 non-determinism: two fresh Parsers produced different gate counters for the same (source, edit, language):\n a=%+v\n b=%+v", a, b)
					}
				})
			}
		})
	}
}
