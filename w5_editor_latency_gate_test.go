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
//   - go: topRootNonLeafChanged=16. An earlier
//     measurement records W1b's achieved result as "rootNonLeafChanged constant 9 at any size" for
//     clean near-top Go edits; this harness measures 10-11 on its own
//     fixture (a different sibling count than w1b's own 300-sibling
//     fixture, hence the small difference) -- 16 keeps that as a small
//     constant with slack for box variance, satisfying the literal
//     requirement "clean-Go top-level edits rootNonLeafChanged <=
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
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/incr"
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
	startPoint := pointForOffset(src, offset)
	switch class {
	case w5Insert:
		edited := make([]byte, 0, len(src)+1)
		edited = append(edited, src[:offset]...)
		edited = append(edited, src[offset])
		edited = append(edited, src[offset:]...)
		return edited, gts.InputEdit{
			StartByte: uint32(offset), OldEndByte: uint32(offset), NewEndByte: uint32(offset + 1),
			StartPoint: startPoint, OldEndPoint: startPoint, NewEndPoint: pointForOffset(edited, offset+1),
		}
	case w5Delete:
		edited := make([]byte, 0, len(src)-1)
		edited = append(edited, src[:offset]...)
		edited = append(edited, src[offset+1:]...)
		return edited, gts.InputEdit{
			StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset),
			StartPoint: startPoint, OldEndPoint: pointForOffset(src, offset+1), NewEndPoint: startPoint,
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
			StartPoint: startPoint, OldEndPoint: pointForOffset(src, offset+1), NewEndPoint: pointForOffset(edited, offset+1),
		}
	default:
		panic("unreachable w5EditClass")
	}
}

// w5Sample is one full run: base parse, apply edit, fresh parse of the
// result, profiled incremental parse of the result, plus the oracle diff
// and wall time.
type w5Sample struct {
	FixtureSHA256 string
	Lang          string
	Size          string
	Class         w5EditClass
	Position      w5Position
	Profile       gts.IncrementalParseProfile
	FreshRuntime  gts.ParseRuntime
	IncrRuntime   gts.ParseRuntime
	EditedLen     int
	OracleDiff    string
	FullSpanOK    bool
	FreshError    bool
	IncrError     bool
	IncrWallNs    int64
	FreshWallNs   int64
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
		FixtureSHA256: fmt.Sprintf("%x", sha256.Sum256(src)),
		Lang:          spec.name, Size: tier.name, Class: class, Position: pos,
		Profile: prof, FreshRuntime: freshRuntime, IncrRuntime: incrTree.ParseRuntime(), EditedLen: len(edited), OracleDiff: diff, FullSpanOK: fullSpan,
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
func w5CheckTransientErrorWork(t *testing.T, s w5Sample, exemptions ...*benchfixtures.W5WorkExemptions) {
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
		if check.name == "tokensConsumed" && len(exemptions) != 0 {
			cell := fmt.Sprintf("%s/%s/%s", s.Lang, s.Size, s.Position)
			class := s.Class.String() + "-transient-error"
			if err := exemptions[0].Check(s.FixtureSHA256, cell, class, check.got, check.max); err != nil {
				t.Fatal(err)
			}
			continue
		}
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

func TestW5CertifiedLargeEditWork(t *testing.T) {
	for _, tier := range []w5SizeTier{{name: "512KB", targetSize: 512 * 1024}, w5Tier1MB} {
		for _, class := range []w5EditClass{w5Insert, w5Delete, w5Replace} {
			t.Run(tier.name+"/"+class.String(), func(t *testing.T) {
				s := w5RunSample(t, w5Langs[0], tier, class, w5Middle)
				w5Check(t, s)
				if s.Profile.TokensConsumed != s.IncrRuntime.TokensConsumed {
					t.Fatalf("certified edit did extra parse work: total=%d selected=%d", s.Profile.TokensConsumed, s.IncrRuntime.TokensConsumed)
				}
				t.Logf("METRIC go/%s/%s reuse=%.1f%% total_tokens=%d total_nodes=%d", tier.name, class,
					100*float64(s.Profile.ReusedBytes)/float64(s.EditedLen), s.Profile.TokensConsumed, s.Profile.NewNodesAllocated)
			})
		}
	}
	for _, spec := range w5Langs {
		if spec.name != "javascript" {
			continue
		}
		t.Run("javascript/512KB/delete", func(t *testing.T) {
			tier := w5SizeTier{name: "512KB", targetSize: 512 * 1024}
			source, items := spec.build(tier.targetSize)
			at := w5LastByteOffset(t, source, spec.errorMarker(w5EditSiteIndices(items)[w5Middle]))
			s := w5RunSampleAtOffset(t, spec, tier, w5Delete, w5Middle, source, at)
			w5CheckCommon(t, s)
			w5CheckTransientErrorWork(t, s)
			if s.Profile.TokensConsumed != s.IncrRuntime.TokensConsumed {
				t.Fatalf("certified recovery did extra parse work: total=%d selected=%d", s.Profile.TokensConsumed, s.IncrRuntime.TokensConsumed)
			}
			t.Logf("METRIC javascript/512KB/transient-delete total_tokens=%d", s.Profile.TokensConsumed)
		})
	}
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
	entries, err := benchfixtures.W5TokenExemptions()
	if err != nil {
		t.Fatal(err)
	}
	exemptions, err := benchfixtures.NewW5WorkExemptions(entries, incr.PruningCertificateVersion)
	if err != nil {
		t.Fatal(err)
	}
	if w5SlowTier() {
		t.Cleanup(func() {
			if err := exemptions.Complete(); err != nil {
				t.Error(err)
			}
		})
	}
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
									w5CheckTransientErrorWork(t, s, exemptions)
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

// The W5 editor-latency gate edits one pinned Go file twice: byte 2959 changes
// from E to Z, then back. It fails when a reuse counter falls or a work counter
// rises against the base. Each step below carries the counters that the base
// (ba95e4ea1) reported. The file is the one the cliff report vendors, so this
// runs in the root-package race lane before the paired editor-latency job.
func TestGoOneByteUndoReuseDoesNotFall(t *testing.T) {
	compressed, err := os.ReadFile(filepath.Join("internal", "benchfixtures", "testdata", "cliffs", "go_elf.go.gz"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	source, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(source)
	if got := hex.EncodeToString(sum[:]); got != "ff2f811d29a56303bad96c3c7cff1a83814a974503a51cee918a314a96eb476e" || len(source) != 140026 {
		t.Fatalf("W5 Go fixture changed: %d bytes, sha256 %s", len(source), got)
	}
	const offset = 2959
	if source[offset] != 'E' {
		t.Fatalf("byte %d = %q, want 'E'", offset, source[offset])
	}
	entry := grammars.DetectLanguageByName("go")
	if entry == nil || entry.Language() == nil {
		t.Fatal("Go grammar unavailable")
	}
	lang := entry.Language()

	edited, forward := goUndoReuseEdit(source, offset, 'Z')
	restored, undo := goUndoReuseEdit(edited, offset, 'E')
	steps := []struct {
		name                string
		source              []byte
		edit                gts.InputEdit
		maxTokens, maxNodes uint64
		maxStacks           int
		minSubtrees         uint64
		minBytes            uint64
	}{
		// Re-based on the go blob built from upstream's grammar text. Tokens fall
		// (22,385 to 22,376 and 22,393 to 22,376), reused subtrees and bytes
		// rise, and allocated nodes rise by 5.6% (109,017 to 115,082 and 109,036
		// to 115,082). The cause of the node rise is not investigated.
		{"edit", edited, forward, 22376, 115082, 18, 44, 3166},
		{"undo", restored, undo, 22376, 115082, 18, 44, 3166},
	}

	parser := gts.NewParser(lang)
	old, err := parser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { old.Release() }()
	for _, step := range steps {
		old.Edit(step.edit)
		next, profile, err := parser.ParseIncrementalProfiled(step.source, old)
		if err != nil {
			t.Fatalf("%s step: %v", step.name, err)
		}
		if next != old {
			old.Release()
		}
		old = next
		fresh, err := gts.NewParser(lang).Parse(step.source)
		if err != nil {
			t.Fatalf("%s step fresh parse: %v", step.name, err)
		}
		incremental, err := benchfixtures.InspectGoTree(old.RootNode(), lang)
		if err != nil {
			t.Fatal(err)
		}
		want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
		if err != nil {
			t.Fatal(err)
		}
		fresh.Release()
		if incremental.SHA256 != want.SHA256 {
			t.Fatalf("%s step: incremental tree differs from a fresh parse", step.name)
		}
		t.Logf("%s: reused_subtrees=%d reused_bytes=%d tokens=%d nodes=%d max_stacks=%d", step.name, profile.ReusedSubtrees, profile.ReusedBytes, profile.TokensConsumed, profile.NewNodesAllocated, profile.MaxStacksSeen)
		if profile.ReusedSubtrees < step.minSubtrees {
			t.Errorf("%s step reused %d subtrees, the base reused %d", step.name, profile.ReusedSubtrees, step.minSubtrees)
		}
		if profile.ReusedBytes < step.minBytes {
			t.Errorf("%s step reused %d bytes, the base reused %d", step.name, profile.ReusedBytes, step.minBytes)
		}
		if profile.TokensConsumed > step.maxTokens || profile.NewNodesAllocated > step.maxNodes || profile.MaxStacksSeen > step.maxStacks {
			t.Errorf("%s step work rose: tokens=%d nodes=%d stacks=%d, the base used %d, %d and %d", step.name,
				profile.TokensConsumed, profile.NewNodesAllocated, profile.MaxStacksSeen, step.maxTokens, step.maxNodes, step.maxStacks)
		}
	}
}

func goUndoReuseEdit(source []byte, offset int, replacement byte) ([]byte, gts.InputEdit) {
	point := func(text []byte, end int) gts.Point {
		var p gts.Point
		for _, ch := range text[:end] {
			if ch == '\n' {
				p.Row++
				p.Column = 0
			} else {
				p.Column++
			}
		}
		return p
	}
	out := append([]byte(nil), source...)
	out[offset] = replacement
	return out, gts.InputEdit{
		StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1),
		StartPoint: point(source, offset), OldEndPoint: point(source, offset+1), NewEndPoint: point(out, offset+1),
	}
}

// TestGoCommentEditBoundaries compares incremental and fresh results around
// comment edits, including a change that extends the maximal-munch token.
func TestGoCommentEditBoundaries(t *testing.T) {
	cases := []struct {
		name, before, after   string
		start, oldEnd, newEnd uint32
	}{
		{"same-symbol-dirty", "package p\n// old\nvar x = 1\n// keep\nvar y = 2\n", "package p\n// new\nvar x = 1\n// keep\nvar y = 2\n", 13, 16, 16},
		{"extend-comment", "package p\n// old\nvar x = 1\n// keep\nvar y = 2\n", "package p\n// oldvar x = 1\n// keep\nvar y = 2\n", 16, 17, 16},
	}
	lang := grammars.DetectLanguageByName("go").Language()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			old, err := parser.Parse([]byte(tc.before))
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			point := func(s string, offset uint32) gts.Point {
				var p gts.Point
				for _, c := range []byte(s)[:offset] {
					if c == '\n' {
						p.Row++
						p.Column = 0
					} else {
						p.Column++
					}
				}
				return p
			}
			edit := gts.InputEdit{StartByte: tc.start, OldEndByte: tc.oldEnd, NewEndByte: tc.newEnd, StartPoint: point(tc.before, tc.start), OldEndPoint: point(tc.before, tc.oldEnd), NewEndPoint: point(tc.after, tc.newEnd)}
			findComment := func(root *gts.Node, text string, src []byte) *gts.Node {
				var visit func(*gts.Node) *gts.Node
				visit = func(n *gts.Node) *gts.Node {
					if n == nil {
						return nil
					}
					if n.IsExtra() && n.Text(src) == text {
						return n
					}
					for i := 0; i < n.ChildCount(); i++ {
						if found := visit(n.Child(i)); found != nil {
							return found
						}
					}
					return nil
				}
				return visit(root)
			}
			beforeKeep := findComment(old.RootNode(), "// keep", []byte(tc.before))
			if beforeKeep == nil {
				t.Fatal("missing original unchanged comment")
			}
			old.Edit(edit)
			next, profile, err := parser.ParseIncrementalProfiled([]byte(tc.after), old)
			if err != nil {
				t.Fatal(err)
			}
			if next != old {
				defer next.Release()
			}
			fresh, err := gts.NewParser(lang).Parse([]byte(tc.after))
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
			if err != nil {
				t.Fatal(err)
			}
			if got.SHA256 != want.SHA256 {
				t.Fatalf("incremental != fresh: %s != %s", got.SHA256, want.SHA256)
			}
			afterKeep := findComment(next.RootNode(), "// keep", []byte(tc.after))
			if afterKeep == nil {
				t.Fatal("unchanged comment missing after edit")
			}
			if tc.name == "same-symbol-dirty" && beforeKeep != afterKeep {
				t.Fatal("unchanged comment was not reused")
			}
			t.Logf("reused_subtrees=%d reused_bytes=%d same_comment_node=%t", profile.ReusedSubtrees, profile.ReusedBytes, beforeKeep == afterKeep)
		})
	}
}

// The editor loop of issue #454 on a certified grammar (CSS at 137 KB), in the
// two shapes an editor can use:
//
//   - inplace: Edit the highlighted tree, which the highlight query has read,
//     then reparse from it;
//   - copy: keep the highlighted tree untouched, Edit a Copy of it, and reparse
//     from the copy.
//
// Each keystroke is timed in its stages (copy, edit, reparse, highlight,
// release), and the Go node headers made in each stage are counted, with the
// route that served the reparse. The highlight is the language's own highlight
// query, either over the whole tree ("full") or over the top-level blocks the
// edit touches after a walk of the top-level children ("block"), which is the
// shape of the editor in the report.
//
//	go test -run '^$' -bench BenchmarkCcoreEditorLoop -benchtime 200x .

// ccoreEditorCSSSize is the size of the documents in the #454 report (137 KB).
const ccoreEditorCSSSize = 137 << 10

// ccoreEditorTestSize is the size the tests of the editor loop use: 137 KB, and
// 24 KB in a build with the race detector, where the root package's tests share
// a time limit and what these tests check does not depend on the size.
func ccoreEditorTestSize() int {
	if raceEnabled {
		return 24 << 10
	}
	return ccoreEditorCSSSize
}

// ccoreEditorCSS builds a stylesheet of at least n bytes from a few rule shapes:
// custom properties, compound and attribute selectors, pseudo-classes, media
// queries, comments, strings, functions and !important. letters are offsets
// inside a class name, where an inserted letter keeps the text valid; newlines
// are offsets just after a declaration's semicolon, where an inserted newline
// keeps it valid.
func ccoreEditorCSS(n int) (src []byte, letters, newlines []int) {
	var b strings.Builder
	b.Grow(n + 512)
	b.WriteString("/* Synthetic stylesheet for the editor loop benchmark. */\n")
	b.WriteString(":root {\n  --brand: #336699;\n  --gap: 8px;\n  --font: \"Inter\", sans-serif;\n}\n\n")
	for i := 0; b.Len() < n; i++ {
		switch i % 5 {
		case 0:
			letters = append(letters, b.Len()+2) // ".c|ard-"
			fmt.Fprintf(&b, ".card-%d > .title:hover {\n  color: var(--brand);", i)
			newlines = append(newlines, b.Len())
			fmt.Fprintf(&b, "\n  margin: calc(var(--gap) * %d) 0;\n  font-family: var(--font);\n}\n\n", i%7+1)
		case 1:
			letters = append(letters, b.Len()+6) // "nav .l|ink"
			fmt.Fprintf(&b, "nav .link-%d[data-state=\"open\"],\nnav .link-%d:focus-visible {\n  outline: 2px solid #%06x;", i, i, (uint32(i)*2654435761)&0xffffff)
			newlines = append(newlines, b.Len())
			b.WriteString("\n  padding: 4px 8px !important;\n}\n\n")
		case 2:
			fmt.Fprintf(&b, "@media (min-width: %dpx) {\n  ", 480+i%5*160)
			letters = append(letters, b.Len()+2) // ".g|rid-"
			fmt.Fprintf(&b, ".grid-%d {\n    display: grid;", i)
			newlines = append(newlines, b.Len())
			fmt.Fprintf(&b, "\n    grid-template-columns: repeat(%d, 1fr);\n  }\n}\n\n", i%6+2)
		case 3:
			fmt.Fprintf(&b, "/* section %d */\n", i)
			letters = append(letters, b.Len()+2) // ".b|tn-"
			fmt.Fprintf(&b, ".btn-%d::before {\n  content: \"\\2014  item %d\";", i, i)
			newlines = append(newlines, b.Len())
			fmt.Fprintf(&b, "\n  background: url(\"img/icon-%d.svg\") no-repeat;\n  transition: opacity 0.%ds ease-in-out;\n}\n\n", i, i%9+1)
		case 4:
			letters = append(letters, b.Len()+8) // "#main .p|anel-"
			fmt.Fprintf(&b, "#main .panel-%d:not(.hidden) {\n  border: 1px solid rgba(0, 0, 0, 0.%d);", i, i%9+1)
			newlines = append(newlines, b.Len())
			fmt.Fprintf(&b, "\n  transform: translate(%dpx, -%dpx) rotate(%ddeg);\n}\n\n", i%40, i%30, i%360)
		}
	}
	return []byte(b.String()), letters, newlines
}

// ccoreEditorKey is keystroke k of the loop over cur: an insert at an even k,
// the delete that undoes it at an odd k, so the document returns to the clean
// source every second keystroke. Every fourth insert is a newline; the others are
// a letter inside a class name. undo is the offset to delete at the next keystroke.
func ccoreEditorKey(cur []byte, k int, letters, newlines []int, undo int) (next []byte, edit gts.InputEdit, nextUndo int) {
	if k%2 == 1 {
		at := undo
		next = append(append([]byte(nil), cur[:at]...), cur[at+1:]...)
		p0 := ccorePoint(cur, at)
		return next, gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at),
			StartPoint: p0, OldEndPoint: ccorePoint(cur, at+1), NewEndPoint: p0}, -1
	}
	i := k / 2
	if i%4 == 3 {
		at := newlines[(i*7919)%len(newlines)]
		next, edit = ccoreInsert(cur, at, '\n')
		return next, edit, at
	}
	at := letters[(i*104729)%len(letters)]
	next, edit = ccoreInsert(cur, at, 'x')
	return next, edit, at
}

// ccoreEditorStage is one keystroke of the loop.
type ccoreEditorStage struct {
	copy, edit, reparse, highlight, release, total time.Duration
	// Go node headers made in each stage (ccoreViewsMade).
	hCopy, hEdit, hReparse, hHighlight int64
	incr, fresh                        int64 // reparses the core served incrementally, fresh
	fallbacks                          map[string]int64
	conversions                        int64 // whole-tree conversions of a view tree during the edit
	ranges                             int   // highlight ranges or matches
}

type ccoreEditorLoop struct {
	tb       testing.TB
	lang     *gts.Language
	parser   *gts.Parser
	hl       *gts.Highlighter
	query    *gts.Query
	mode     string // "copy" or "inplace"
	hlMode   string // "full" or "block"
	src      []byte
	letters  []int
	newlines []int
	tree     *gts.Tree // the highlighted tree
	k, undo  int
}

func newCcoreEditorLoop(tb testing.TB, mode, hlMode string) *ccoreEditorLoop {
	tb.Helper()
	entry := grammars.DetectLanguageByName("css")
	if entry == nil || entry.Language() == nil {
		tb.Fatal("css unavailable")
	}
	lang := entry.Language()
	hl, err := entry.NewHighlighter()
	if err != nil {
		tb.Fatal(err)
	}
	q, err := gts.NewQuery(entry.HighlightQuery, lang)
	if err != nil {
		tb.Fatal(err)
	}
	size := ccoreEditorCSSSize
	if _, bench := tb.(*testing.B); !bench {
		size = ccoreEditorTestSize()
	}
	src, letters, newlines := ccoreEditorCSS(size)
	l := &ccoreEditorLoop{tb: tb, lang: lang, parser: gts.NewParser(lang), hl: hl, query: q, mode: mode, hlMode: hlMode,
		src: src, letters: letters, newlines: newlines, undo: -1}
	tree, err := l.parser.Parse(src)
	if err != nil || !gts.CcoreTestFromCore(tree) {
		tb.Fatalf("open: err %v, built by the core %v", err, gts.CcoreTestFromCore(tree))
	}
	gts.HighlighterTestTree(hl, tree) // the editor highlights the whole document when it opens it
	l.tree = tree
	return l
}

func ccoreViewsMadeNow() int64 { _, made, _ := gts.CcoreTestViewCounters(); return made }

func ccoreConversionsNow() int64 {
	_, _, conv := gts.CcoreTestViewCounters()
	// Only the reasons listed next to ccoreViewConversions in ccore_bind.go are
	// whole-tree conversions. Root-only edits, shortcut reparses, sparse edits,
	// refreshed headers, and sparse stops are other counters in the same map.
	var n int64
	for _, k := range []string{"tree_edit_with_views", "node_edit", "adopt", "offset_root", "test", "other"} {
		n += conv[k]
	}
	return n
}

// step runs one keystroke and returns its stage times and counts.
func (l *ccoreEditorLoop) step() ccoreEditorStage {
	next, edit, undo := ccoreEditorKey(l.src, l.k, l.letters, l.newlines, l.undo)
	l.k++
	l.undo = undo
	var s ccoreEditorStage
	_, incr0 := gts.CcoreTestIncr()
	_, fresh0 := gts.CcoreTestCounters()
	fb0, _ := gts.CcoreTestIncr()
	conv0 := ccoreConversionsNow()

	t0 := time.Now()
	h0 := ccoreViewsMadeNow()
	old := l.tree
	working := old
	if l.mode == "copy" {
		working = old.Copy()
	}
	t1 := time.Now()
	h1 := ccoreViewsMadeNow()
	working.Edit(edit)
	t2 := time.Now()
	h2 := ccoreViewsMadeNow()
	s.conversions = ccoreConversionsNow() - conv0
	tree, err := l.parser.ParseIncremental(next, working)
	if err != nil {
		l.tb.Fatal(err)
	}
	t3 := time.Now()
	h3 := ccoreViewsMadeNow()
	s.ranges = l.highlight(tree, edit)
	t4 := time.Now()
	h4 := ccoreViewsMadeNow()
	if working != old {
		working.Release()
	}
	old.Release()
	t5 := time.Now()

	s.copy, s.edit, s.reparse, s.highlight, s.release, s.total = t1.Sub(t0), t2.Sub(t1), t3.Sub(t2), t4.Sub(t3), t5.Sub(t4), t5.Sub(t0)
	s.hCopy, s.hEdit, s.hReparse, s.hHighlight = h1-h0, h2-h1, h3-h2, h4-h3
	fb1, incr1 := gts.CcoreTestIncr()
	_, fresh1 := gts.CcoreTestCounters()
	s.incr, s.fresh = incr1-incr0, fresh1-fresh0
	for k, v := range fb1 {
		if d := v - fb0[k]; d != 0 {
			if s.fallbacks == nil {
				s.fallbacks = map[string]int64{}
			}
			s.fallbacks[k] = d
		}
	}
	l.tree, l.src = tree, next
	return s
}

// highlight is the editor's highlight of the new tree: the whole tree, or a walk
// of the top-level children (the editor matches each to its old colours by
// position, symbol, length and child count) and the query over the blocks the
// edit touches.
func (l *ccoreEditorLoop) highlight(tree *gts.Tree, edit gts.InputEdit) int {
	if l.hlMode == "full" {
		return len(gts.HighlighterTestTree(l.hl, tree))
	}
	root := tree.RootNode()
	n := 0
	for i := 0; i < root.ChildCount(); i++ {
		c := root.Child(i)
		_, _, _ = c.Symbol(), c.EndByte()-c.StartByte(), c.ChildCount()
		if c.EndByte() >= edit.StartByte && c.StartByte() <= edit.NewEndByte {
			n += len(l.query.ExecuteNode(c, l.lang, tree.Source()))
		}
	}
	return n
}

func (l *ccoreEditorLoop) close() {
	if l.tree != nil {
		l.tree.Release()
		l.tree = nil
	}
}

// TestCcoreEditorLoopRoutes checks what the benchmark measures, at 137 KB: the
// copy path makes one header for the copy's root and none in the edit, and the
// core serves its reparse incrementally; every incremental tree and its
// highlight equal a fresh parse's. It logs the in-place path's counts, the
// baseline that the sparse rebind of Tree.Edit is measured against.
func TestCcoreEditorLoopRoutes(t *testing.T) {
	if testing.Short() {
		t.Skip("137 KB editor loop skipped under -short")
	}
	for _, mode := range []string{"copy", "inplace"} {
		for _, hlMode := range []string{"full", "block"} {
			t.Run(mode+"/"+hlMode, func(t *testing.T) {
				l := newCcoreEditorLoop(t, mode, hlMode)
				defer l.close()
				fresh := gts.NewParser(l.lang)
				for k := 0; k < 8; k++ {
					s := l.step()
					if s.incr != 1 || s.fresh != 0 || len(s.fallbacks) != 0 {
						t.Fatalf("key %d: served incr %d fresh %d, fallbacks %v; want one incremental reparse", k, s.incr, s.fresh, s.fallbacks)
					}
					if mode == "copy" && (s.hCopy != 1 || s.hEdit != 0 || s.conversions != 0) {
						t.Fatalf("key %d: copy made %d headers, edit %d, conversions %d; want 1, 0, 0", k, s.hCopy, s.hEdit, s.conversions)
					}
					want, err := fresh.Parse(l.src)
					if err != nil {
						t.Fatal(err)
					}
					// The checks read a copy, so they make no view in the tree the next keystroke edits.
					chk := l.tree.Copy()
					if got, w := ccoreSExpr(chk, l.lang), ccoreSExpr(want, l.lang); got != w {
						t.Fatalf("key %d: incremental tree differs from a fresh parse", k)
					}
					if got, w := fmt.Sprint(gts.HighlighterTestTree(l.hl, chk)), fmt.Sprint(gts.HighlighterTestTree(l.hl, want)); got != w {
						t.Fatalf("key %d: highlight of the incremental tree differs from a fresh parse's", k)
					}
					chk.Release()
					want.Release()
					t.Logf("key %d: headers copy %d edit %d reparse %d highlight %d; conversions %d", k, s.hCopy, s.hEdit, s.hReparse, s.hHighlight, s.conversions)
				}
			})
		}
	}
}

// BenchmarkCcoreEditorLoop times one keystroke of the editor loop per op, by
// stage. The metrics are means over b.N keystrokes, GC included in the stage
// that ran when it struck.
func BenchmarkCcoreEditorLoop(b *testing.B) {
	for _, hlMode := range []string{"block", "full"} {
		for _, mode := range []string{"inplace", "copy"} {
			b.Run(hlMode+"/"+mode, func(b *testing.B) {
				l := newCcoreEditorLoop(b, mode, hlMode)
				defer l.close()
				for k := 0; k < 4; k++ { // the first keystrokes after the open
					l.step()
				}
				var sum ccoreEditorStage
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					s := l.step()
					sum.copy += s.copy
					sum.edit += s.edit
					sum.reparse += s.reparse
					sum.highlight += s.highlight
					sum.release += s.release
					sum.hCopy += s.hCopy
					sum.hEdit += s.hEdit
					sum.hReparse += s.hReparse
					sum.hHighlight += s.hHighlight
					sum.incr += s.incr
					sum.fresh += s.fresh
				}
				b.StopTimer()
				n := float64(b.N)
				ms := func(d time.Duration) float64 { return float64(d) / 1e6 / n }
				b.ReportMetric(ms(sum.copy), "copy-ms/op")
				b.ReportMetric(ms(sum.edit), "edit-ms/op")
				b.ReportMetric(ms(sum.reparse), "reparse-ms/op")
				b.ReportMetric(ms(sum.highlight), "highlight-ms/op")
				b.ReportMetric(ms(sum.release), "release-ms/op")
				b.ReportMetric(float64(sum.hEdit)/n, "edit-headers/op")
				b.ReportMetric(float64(sum.hCopy+sum.hEdit+sum.hReparse+sum.hHighlight)/n, "headers/op")
				b.ReportMetric(float64(sum.incr)/n, "incr/op")
				b.ReportMetric(float64(sum.fresh)/n, "fresh/op")
			})
		}
	}
}

// ---- The sparse rebind of Tree.Edit (issue #454) ----
//
// Tree.Edit on a view tree with views below the root refreshes the headers that
// exist from the edited core tree and makes none (ccoreEditSparse). The tests
// below hold it to three things: no new header in the edit, the held nodes equal
// to those of the path it replaces (the conversion, then the walk over Go
// nodes), and the tree after the edit equal to the eager conversion of the
// edited core tree. The reparse must be served incrementally and equal a fresh
// parse, with the same highlight.

// ccoreSparseSig is what an edit must keep right about a node.
type ccoreSparseSig struct {
	ok    bool
	sym   gts.Symbol
	r     gts.Range
	named bool
	kids  int
}

func ccoreSparseSigOf(n *gts.Node) ccoreSparseSig {
	if n == nil {
		return ccoreSparseSig{}
	}
	return ccoreSparseSig{true, n.Symbol(), n.Range(), n.IsNamed(), n.ChildCount()}
}

// ccoreSparseHeldDiffs compares each held node of the edited view tree with the
// same node of the twin, which took the conversion: the node, and what
// navigation from it reaches (parent, siblings, first and last child; for more
// than 4,000 held nodes the navigation of every node of a stride, because a
// sibling of a view costs the width of its parent). It returns the number of
// nodes that differ and describes the first.
//
// boundary counts the differences of one known kind, which are not in diffs. The
// walk over Go nodes that follows the conversion is not C's edit: when an edit
// replaces text that starts exactly where a node ends, it moves that node's end
// to the end of the new text, and C leaves the node as it is (the replaced text
// is padding of the next node). The sparse rebind reads the core, so it gives
// C's span.
func ccoreSparseHeldDiffs(held, twin []*gts.Node, edit gts.InputEdit) (diffs, boundary int, first string) {
	stride := 1
	if len(held) > 4000 {
		stride = len(held) / 2000
	}
	for i := range held {
		a, b := held[i], twin[i]
		bad, known := "", false
		check := func(what string, x, y *gts.Node) bool {
			sx, sy := ccoreSparseSigOf(x), ccoreSparseSigOf(y)
			if sx == sy {
				return true
			}
			if sx.ok && sy.ok && sx.sym == sy.sym && sx.named == sy.named && sx.kids == sy.kids &&
				sx.r.StartByte == sy.r.StartByte && sx.r.StartPoint == sy.r.StartPoint &&
				edit.OldEndByte > edit.StartByte && edit.NewEndByte > edit.StartByte &&
				sx.r.EndByte == edit.StartByte && sy.r.EndByte == edit.NewEndByte {
				known = true
				return false
			}
			bad = fmt.Sprintf("%s: sparse %+v, conversion %+v", what, sx, sy)
			return false
		}
		ok := check("node", a, b) && check("parent", a.Parent(), b.Parent())
		if ok && i%stride == 0 {
			ok = check("next sibling", a.NextSibling(), b.NextSibling()) && check("previous sibling", a.PrevSibling(), b.PrevSibling())
			if ok && a.ChildCount() > 0 {
				_ = check("first child", a.Child(0), b.Child(0)) && check("last child", a.Child(a.ChildCount()-1), b.Child(b.ChildCount()-1))
			}
		}
		switch {
		case bad != "":
			if diffs == 0 {
				first = fmt.Sprintf("held node %d: %s", i, bad)
			}
			diffs++
		case known:
			boundary++
		}
	}
	return diffs, boundary, first
}

// ccoreSparseAll returns every node of the tree in one fixed order. It makes a
// view of each.
func ccoreSparseAll(tree *gts.Tree) []*gts.Node {
	out := []*gts.Node{tree.RootNode()}
	for i := 0; i < len(out); i++ {
		out = append(out, out[i].Children()...)
	}
	return out
}

// ccoreSparseTopChild is the index of the top-level child that holds byte at,
// found on a copy so that the tree itself gets no view.
func ccoreSparseTopChild(tree *gts.Tree, at uint32) int {
	cp := tree.Copy()
	defer cp.Release()
	root := cp.RootNode()
	lo, hi := 0, root.ChildCount()-1
	for lo < hi {
		mid := (lo + hi) / 2
		if root.Child(mid).EndByte() <= at {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

type ccoreSparseCounts struct {
	made    int64
	convert int64 // whole-tree conversions
	sparse  int64
	rootly  int64
	stops   int64
}

func ccoreSparseCountsNow() ccoreSparseCounts {
	_, made, m := gts.CcoreTestViewCounters()
	c := ccoreSparseCounts{made: made, sparse: m["sparse_edits"], rootly: m["root_only_edits"]}
	for k, v := range m {
		switch {
		case strings.HasPrefix(k, "sparse_stop_"):
			c.stops += v
		case k == "sparse_edits" || k == "sparse_refreshed" || k == "root_only_edits" || k == "shortcut_reparses":
		default:
			c.convert += v
		}
	}
	return c
}

// ccoreSparseEdit applies edit to tree and checks the counters of the edit: no
// header made, no conversion, and one sparse edit (one root-only edit when the
// tree has no view below its root).
func ccoreSparseEdit(t *testing.T, what string, tree *gts.Tree, edit gts.InputEdit) {
	t.Helper()
	madeBefore, isView := gts.CcoreTestViewHeaders(tree)
	if !isView {
		t.Fatalf("%s: the tree is not a view tree before the edit", what)
	}
	c0 := ccoreSparseCountsNow()
	tree.Edit(edit)
	c1 := ccoreSparseCountsNow()
	madeAfter, isView := gts.CcoreTestViewHeaders(tree)
	if !isView {
		t.Fatalf("%s: the edit converted the tree", what)
	}
	if madeAfter != madeBefore || c1.made != c0.made {
		t.Fatalf("%s: the edit made %d headers in the tree (%d in the process), want 0", what, madeAfter-madeBefore, c1.made-c0.made)
	}
	wantSparse, wantRoot := int64(1), int64(0)
	if madeBefore <= 1 {
		wantSparse, wantRoot = 0, 1
	}
	if c1.convert != c0.convert || c1.stops != c0.stops || c1.sparse-c0.sparse != wantSparse || c1.rootly-c0.rootly != wantRoot {
		t.Fatalf("%s: conversions %d, sparse stops %d, sparse edits %d, root-only edits %d; want 0, 0, %d, %d",
			what, c1.convert-c0.convert, c1.stops-c0.stops, c1.sparse-c0.sparse, c1.rootly-c0.rootly, wantSparse, wantRoot)
	}
	small, large, repeated, _ := gts.CcoreTestViewTable(tree)
	if int64(small+large+repeated) != madeAfter-1 {
		t.Fatalf("%s: the view table holds %d+%d+%d views for %d headers below the root", what, small, large, repeated, madeAfter-1)
	}
}

// ccoreSparseTwinEdit applies edit to the twin with the sparse rebind off, so
// the twin takes the conversion and the walk over Go nodes.
func ccoreSparseTwinEdit(t *testing.T, what string, twin *gts.Tree, edit gts.InputEdit) {
	t.Helper()
	madeBefore, _ := gts.CcoreTestViewHeaders(twin)
	restore := gts.CcoreTestSparseOff()
	twin.Edit(edit)
	restore()
	if _, isView := gts.CcoreTestViewHeaders(twin); isView && madeBefore > 1 {
		t.Fatalf("%s: the twin with views below its root was not converted", what)
	}
}

// ccoreSparsePos is an inserted byte and the offset it went to in the clean source.
type ccoreSparsePos struct {
	at int
	b  byte
}

type ccoreSparseKey struct {
	next []byte
	edit gts.InputEdit
}

// ccoreSparseKeys returns the edits of one step on cur, each with the source
// after it. Steps 0 and 2 insert a letter and a newline; step 4 makes three
// inserts (two letters and a newline) at three places; each odd step deletes
// what the step before inserted, so the source is clean again after it. The
// edits of one step never touch text an earlier edit of the step wrote, which
// the route requires to carry them.
func ccoreSparseKeys(step int, cur []byte, letters, newlines []int, pending *[]ccoreSparsePos) []ccoreSparseKey {
	var keys []ccoreSparseKey
	ins := func(p ccoreSparsePos) {
		next, edit := ccoreInsert(cur, p.at, p.b)
		keys = append(keys, ccoreSparseKey{next, edit})
		cur = next
	}
	letter := func(i int) ccoreSparsePos { return ccoreSparsePos{letters[(i*104729)%len(letters)], 'x'} }
	newline := func(i int) ccoreSparsePos { return ccoreSparsePos{newlines[(i*7919)%len(newlines)], '\n'} }
	switch step % 6 {
	case 0:
		*pending = []ccoreSparsePos{letter(step)}
		ins((*pending)[0])
	case 2:
		*pending = []ccoreSparsePos{newline(step)}
		ins((*pending)[0])
	case 4:
		at := []ccoreSparsePos{letter(step), newline(step + 1), letter(step + 2)}
		for i := 1; i < len(at); i++ { // in the order of the offsets
			for j := i; j > 0 && at[j].at < at[j-1].at; j-- {
				at[j], at[j-1] = at[j-1], at[j]
			}
		}
		for i := len(at) - 1; i >= 0; i-- { // the last first, so the earlier offsets still hold
			ins(at[i])
		}
		*pending = at
	default:
		for _, p := range *pending { // the first first: each delete moves the later ones back by one
			next := append(append([]byte(nil), cur[:p.at]...), cur[p.at+1:]...)
			p0 := ccorePoint(cur, p.at)
			keys = append(keys, ccoreSparseKey{next, gts.InputEdit{StartByte: uint32(p.at), OldEndByte: uint32(p.at + 1), NewEndByte: uint32(p.at),
				StartPoint: p0, OldEndPoint: ccorePoint(cur, p.at+1), NewEndPoint: p0}})
			cur = next
		}
		*pending = nil
	}
	return keys
}

// TestCcoreSparseRebindGate is the gate of the sparse rebind on the 137 KB CSS
// document of issue #454, for a tree with no view below the root, with one held
// child, with the editor's block walk, after the language's highlight query, and
// with a view of every node. The keystrokes are a letter, a newline and the
// deletes that undo them, then three inserts before one reparse and the three
// deletes that undo those before the next.
func TestCcoreSparseRebindGate(t *testing.T) {
	if testing.Short() {
		t.Skip("137 KB sparse rebind gate skipped under -short")
	}
	entry := grammars.DetectLanguageByName("css")
	if entry == nil || entry.Language() == nil {
		t.Fatal("css unavailable")
	}
	lang := entry.Language()
	hl, err := entry.NewHighlighter()
	if err != nil {
		t.Fatal(err)
	}
	query, err := gts.NewQuery(entry.HighlightQuery, lang)
	if err != nil {
		t.Fatal(err)
	}
	type exposure struct {
		name string
		hold func(tree *gts.Tree, at uint32) []*gts.Node
	}
	exposures := []exposure{
		{"root", func(tree *gts.Tree, _ uint32) []*gts.Node { return []*gts.Node{tree.RootNode()} }},
		{"child", func(tree *gts.Tree, at uint32) []*gts.Node {
			root := tree.RootNode()
			return []*gts.Node{root, root.Child(ccoreSparseTopChild(tree, at))}
		}},
		{"block", func(tree *gts.Tree, at uint32) []*gts.Node {
			root := tree.RootNode()
			held := []*gts.Node{root}
			for i := 0; i < root.ChildCount(); i++ {
				c := root.Child(i)
				held = append(held, c)
				if c.EndByte() >= at && c.StartByte() <= at {
					query.ExecuteNode(c, lang, tree.Source())
				}
			}
			return held
		}},
		{"highlight", func(tree *gts.Tree, _ uint32) []*gts.Node {
			gts.HighlighterTestTree(hl, tree)
			return []*gts.Node{tree.RootNode()}
		}},
		{"all", func(tree *gts.Tree, _ uint32) []*gts.Node {
			gts.HighlighterTestTree(hl, tree)
			return ccoreSparseAll(tree)
		}},
	}
	const steps = 6
	for _, ex := range exposures {
		t.Run(ex.name, func(t *testing.T) {
			src, letters, newlines := ccoreEditorCSS(ccoreEditorTestSize())
			parser := gts.NewParser(lang)
			fresh := gts.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil || !gts.CcoreTestFromCore(tree) {
				t.Fatalf("open: err %v, built by the core %v", err, gts.CcoreTestFromCore(tree))
			}
			defer func() { tree.Release() }()
			keys := 0
			var pending []ccoreSparsePos
			var heldNodes, refreshed int64
			for step := 0; step < steps; step++ {
				cur := src
				var held, twinHeld []*gts.Node
				var twin *gts.Tree
				_, incr0 := gts.CcoreTestIncr()
				fb0, _ := gts.CcoreTestIncr()
				for e, key := range ccoreSparseKeys(step, src, letters, newlines, &pending) {
					next, edit := key.next, key.edit
					what := fmt.Sprintf("step %d edit %d (at byte %d)", step, e, edit.StartByte)
					keys++
					if e == 0 {
						twin = tree.Copy()
						held, twinHeld = ex.hold(tree, edit.StartByte), ex.hold(twin, edit.StartByte)
						if len(held) != len(twinHeld) {
							t.Fatalf("%s: %d held nodes, %d in the twin", what, len(held), len(twinHeld))
						}
						heldNodes += int64(len(held))
					}
					_, _, m0 := gts.CcoreTestViewCounters()
					ccoreSparseEdit(t, what, tree, edit)
					_, _, m1 := gts.CcoreTestViewCounters()
					refreshed += m1["sparse_refreshed"] - m0["sparse_refreshed"]
					ccoreSparseTwinEdit(t, what, twin, edit)
					if held[0] != tree.RootNode() {
						t.Fatalf("%s: the root is another pointer after the edit", what)
					}
					if ex.name == "child" {
						made, _ := gts.CcoreTestViewHeaders(tree)
						// The index of the held child can change (a comment's newline is a
						// child of its own), so find it again by where it starts now.
						if again := tree.RootNode().Child(ccoreSparseTopChild(tree, held[1].StartByte())); again != held[1] {
							t.Fatalf("%s: the held child is another pointer after the edit", what)
						}
						if after, _ := gts.CcoreTestViewHeaders(tree); after != made {
							t.Fatalf("%s: reading the held child again made %d headers", what, after-made)
						}
					}
					if diffs, boundary, first := ccoreSparseHeldDiffs(held, twinHeld, edit); diffs != 0 || boundary != 0 {
						t.Fatalf("%s: %d of %d held nodes differ from the conversion path (and %d at an edit boundary); %s", what, diffs, len(held), boundary, first)
					}
					cur = next
				}
				twin.Release()
				nextTree, err := parser.ParseIncremental(cur, tree)
				if err != nil {
					t.Fatal(err)
				}
				fb1, incr1 := gts.CcoreTestIncr()
				if incr1-incr0 != 1 || fmt.Sprint(fb1) != fmt.Sprint(fb0) {
					t.Fatalf("step %d: %d incremental reparses, fallbacks %v then %v; want one incremental reparse", step, incr1-incr0, fb0, fb1)
				}
				want, err := fresh.Parse(cur)
				if err != nil {
					t.Fatal(err)
				}
				chk := nextTree.Copy() // the checks read a copy, so the next step's tree has the views its exposure makes
				if ccoreSExpr(chk, lang) != ccoreSExpr(want, lang) {
					t.Fatalf("step %d: the incremental tree differs from a fresh parse", step)
				}
				if fmt.Sprint(gts.HighlighterTestTree(hl, chk)) != fmt.Sprint(gts.HighlighterTestTree(hl, want)) {
					t.Fatalf("step %d: the highlight of the incremental tree differs from a fresh parse's", step)
				}
				chk.Release()
				want.Release()
				if step == 0 || step == steps-1 {
					// The edited tree, read through the headers it kept and the ones the walk
					// now makes, against the eager conversion of the edited core tree.
					eager := gts.CcoreTestEager(tree)
					if eager == nil {
						t.Fatalf("step %d: no eager conversion of the edited tree", step)
					}
					o := &ccoreOracle{t: t, lang: lang, name: "css/sparse/" + ex.name, src: cur}
					o.compare(tree.RootNode(), eager.RootNode())
					eager.Release()
					if o.diffs != 0 || o.checks == 0 {
						t.Fatalf("step %d: %d differences from the eager conversion of the edited tree in %d checks", step, o.diffs, o.checks)
					}
				}
				tree.Release()
				tree, src = nextTree, cur
			}
			t.Logf("%s: %d bytes, %d steps, %d edits, %d held nodes compared with the conversion path, %d headers refreshed, 0 made", ex.name, len(src), steps, keys, heldNodes, refreshed)
		})
	}
}

// ccoreSparseExpose makes views in tree in one of four ways and returns the
// nodes it read, in an order that is the same for a copy of the tree: every
// node; the path from the root to the smallest node at byte at; the children of
// the root; the first-child and last-child chains with the named children of
// each node on them.
func ccoreSparseExpose(way int, tree *gts.Tree, at uint32) []*gts.Node {
	root := tree.RootNode()
	switch way % 4 {
	case 0:
		return ccoreSparseAll(tree)
	case 1:
		held := []*gts.Node{root}
		for n := root.DescendantForByteRange(at, at); n != nil && n != root; n = n.Parent() {
			held = append(held, n)
		}
		return held
	case 2:
		return append([]*gts.Node{root}, root.Children()...)
	default:
		held := []*gts.Node{root}
		for _, last := range []bool{false, true} {
			for n := root; n != nil && n.ChildCount() > 0; {
				for j := 0; j < n.NamedChildCount() && j < 8; j++ {
					held = append(held, n.NamedChild(j))
				}
				if last {
					n = n.Child(n.ChildCount() - 1)
				} else {
					n = n.Child(0)
				}
				held = append(held, n)
			}
		}
		return held
	}
}

// TestCcoreSparseRebindSessions runs the 72-edit session (inserts, deletes and
// replacements at seeded offsets, most of which leave a syntax error) on the
// sample of several languages. Before each edit it makes views in one of four
// ways. Every edit with a view below the root must be a sparse edit that makes
// no header; the held nodes must equal those of the conversion path, but for
// the spans the walk over Go nodes gives at an edit boundary (counted; see
// ccoreSparseHeldDiffs); the whole edited tree must equal the eager conversion
// of the edited core tree; every reparse must equal a fresh parse.
func TestCcoreSparseRebindSessions(t *testing.T) {
	defer ccoreCertifyAll()()
	defer gts.CcoreTestViewsOn(true)()
	samples := ccoreOracleSamples(t)
	names := []string{"json", "css", "typescript", "tsx", "python", "c", "rust", "html", "ruby", "bash", "javascript", "markdown", "toml", "yaml", "java", "lua"}
	if raceEnabled { // a third of the languages, and the whole-tree comparison at every eighth step
		names = []string{"json", "css", "typescript", "rust", "markdown"}
	}
	if only := os.Getenv("CCORE_VIEWS_LANGS"); only != "" {
		names = strings.Split(only, ",")
	}
	served := 0
	for _, name := range names {
		src := samples[name]
		entry := grammars.DetectLanguageByName(name)
		if src == nil || entry == nil || entry.Language() == nil {
			if os.Getenv("CCORE_VIEWS_LANGS") != "" {
				t.Fatalf("%s unavailable", name)
			}
			t.Logf("%s unavailable", name)
			continue
		}
		lang := entry.Language()
		p := ccoreRouteParser(lang)
		tree, err := p.Parse(src)
		if err != nil || tree == nil {
			t.Fatalf("%s: parse: %v", name, err)
		}
		if _, ok := gts.CcoreTestViewHeaders(tree); !ok {
			tree.Release()
			if os.Getenv("CCORE_VIEWS_LANGS") != "" {
				t.Fatalf("%s: the route declined it", name)
			}
			t.Logf("%s: the route declined it", name)
			continue
		}
		served++
		o := &ccoreOracle{t: t, lang: lang, name: name + "/sparse-session"}
		var held, refreshed int64
		steps, repeated, differs, boundary := 0, 0, 0, 0
		_, _, m0 := gts.CcoreTestViewCounters()
		for i, step := range benchfixtures.EditingSession(src) {
			steps++
			what := fmt.Sprintf("%s step %d (way %d, edit at byte %d)", name, i+1, i%4, step.Edit.StartByte)
			twin := tree.Copy()
			a, b := ccoreSparseExpose(i, tree, step.Edit.StartByte), ccoreSparseExpose(i, twin, step.Edit.StartByte)
			if len(a) != len(b) {
				t.Fatalf("%s: %d held nodes, %d in the twin", what, len(a), len(b))
			}
			held += int64(len(a))
			if _, _, r, _ := gts.CcoreTestViewTable(tree); r != 0 {
				repeated++
			}
			ccoreSparseEdit(t, what, tree, step.Edit)
			ccoreSparseTwinEdit(t, what, twin, step.Edit)
			diffs, known, first := ccoreSparseHeldDiffs(a, b, step.Edit)
			if diffs != 0 {
				t.Fatalf("%s: %d of %d held nodes differ from the conversion path; %s", what, diffs, len(a), first)
			}
			boundary += known
			twin.Release()
			if !raceEnabled || i%8 == 0 {
				eager := gts.CcoreTestEager(tree)
				if eager == nil {
					t.Fatalf("%s: no eager conversion of the edited tree", what)
				}
				o.src = step.Source
				o.compare(tree.RootNode(), eager.RootNode())
				eager.Release()
			}
			next, err := p.ParseIncremental(step.Source, tree)
			if err != nil || next == nil {
				t.Fatalf("%s: reparse: %v", what, err)
			}
			tree.Release()
			tree = next
			freshTree, err := ccoreRouteParser(lang).Parse(step.Source)
			if err != nil || freshTree == nil {
				t.Fatalf("%s: fresh parse: %v", what, err)
			}
			cp := tree.Copy() // read the whole tree through a copy: the next step chooses the views of `tree`
			if cp.RootNode().SExpr(lang) != freshTree.RootNode().SExpr(lang) || tree.RootNode().Range() != freshTree.RootNode().Range() {
				differs++
				t.Errorf("%s: the reparse differs from a fresh parse", what)
			}
			cp.Release()
			freshTree.Release()
		}
		tree.Release()
		_, _, m1 := gts.CcoreTestViewCounters()
		refreshed = m1["sparse_refreshed"] - m0["sparse_refreshed"]
		fmt.Printf("SPARSESESSION lang=%s steps=%d sparse_edits=%d held_nodes=%d refreshed=%d trees_with_repeated_slots=%d differ_from_conversion_path=0 go_walk_boundary_spans=%d oracle_checks=%d differences=%d reparse_differs=%d\n",
			name, steps, m1["sparse_edits"]-m0["sparse_edits"], held, refreshed, repeated, boundary, o.checks, o.diffs, differs)
		if o.diffs != 0 || o.checks == 0 {
			t.Errorf("%s: %d differences from the eager conversion in %d checks", name, o.diffs, o.checks)
		}
	}
	if served == 0 {
		t.Fatal("no edit sessions were compared")
	}
}

// TestCcoreSparseRebindLargeList: a record with more children than a child page
// holds (4,096 handles) has a list of its own, and the views of its children are
// in the map of large lists. Recovery makes such a record from thousands of
// unclosed brackets. The edit copies the record with a new list; every view
// must move to it. The test parses once: a parse of this input takes seconds.
func TestCcoreSparseRebindLargeList(t *testing.T) {
	defer ccoreCertifyAll()()
	defer gts.CcoreTestViewsOn(true)()
	lang := ccoreRouteLang(t, "json")
	const unit = "[ "
	src := []byte(strings.Repeat(unit, 4100))
	tree, err := ccoreRouteParser(lang).Parse(src)
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if _, ok := gts.CcoreTestViewHeaders(tree); !ok {
		t.Fatal("the route declined json")
	}
	twin := tree.Copy()
	defer twin.Release()
	held, twinHeld := ccoreSparseAll(tree), ccoreSparseAll(twin)
	_, large, _, _ := gts.CcoreTestViewTable(tree)
	if large <= 4096 {
		t.Fatalf("%d views in large lists: the input no longer gives a large list", large)
	}
	// The last offset first, so the earlier offsets still hold and no edit
	// touches text another wrote.
	for step, at := range []int{len(src) - 1, len(unit) * 2050, 1} {
		b := byte(' ')
		if step == 1 {
			b = '\n'
		}
		what := fmt.Sprintf("edit %d at byte %d", step, at)
		next, edit := ccoreInsert(src, at, b)
		ccoreSparseEdit(t, what, tree, edit)
		ccoreSparseTwinEdit(t, what, twin, edit)
		if _, after, _, _ := gts.CcoreTestViewTable(tree); after != large {
			t.Fatalf("%s: %d views in large lists after the edit, %d before", what, after, large)
		}
		if diffs, _, first := ccoreSparseHeldDiffs(held, twinHeld, edit); diffs != 0 {
			t.Fatalf("%s: %d of %d held nodes differ from the conversion path; %s", what, diffs, len(held), first)
		}
		src = next
	}
	eager := gts.CcoreTestEager(tree)
	if eager == nil {
		t.Fatal("no eager conversion of the edited tree")
	}
	defer eager.Release()
	o := &ccoreOracle{t: t, lang: lang, name: "json/sparse-large", src: src}
	o.compare(tree.RootNode(), eager.RootNode())
	if o.diffs != 0 || o.checks == 0 {
		t.Fatalf("%d differences from the eager conversion in %d checks", o.diffs, o.checks)
	}
}

// TestCcoreSparseRebindRefusedEdit: when the budget of the tree refuses the
// staged edit, Tree.Edit converts the tree as before the sparse rebind: the
// held nodes move with the edit, the link is broken, and the reparse is a fresh
// parse by the core.
func TestCcoreSparseRebindRefusedEdit(t *testing.T) {
	defer ccoreCertifyAll()()
	defer gts.CcoreTestViewsOn(true)()
	lang := ccoreRouteLang(t, "json")
	src := ccoreJSONSource(60)
	p := ccoreRouteParser(lang)
	tree, err := p.Parse(src)
	if err != nil || tree == nil {
		t.Fatal(err)
	}
	defer tree.Release()
	held := tree.RootNode().Child(0)
	last := held.Child(held.ChildCount() - 1)
	startBefore, endBefore := last.StartByte(), last.EndByte()
	gts.CcoreTestEditBudget(tree, 1) // any claim of the edit is over the budget
	_, _, before := gts.CcoreTestViewCounters()
	out, edit := ccoreInsert(src, 3, ' ')
	tree.Edit(edit)
	_, _, after := gts.CcoreTestViewCounters()
	if after["sparse_stop_refused"] != before["sparse_stop_refused"]+1 || after["tree_edit_with_views"] != before["tree_edit_with_views"]+1 ||
		after["sparse_edits"] != before["sparse_edits"] {
		t.Fatalf("counters before %v, after %v; want one refused sparse edit and one conversion", before, after)
	}
	if has, _, broken := gts.CcoreTestLink(tree); !has || !broken {
		t.Fatalf("link: has %v, broken %v; want a broken link", has, broken)
	}
	if held != tree.RootNode().Child(0) || last.StartByte() != startBefore+1 || last.EndByte() != endBefore+1 {
		t.Fatalf("a held node did not move with the edit: %d-%d, was %d-%d", last.StartByte(), last.EndByte(), startBefore, endBefore)
	}
	fb0, incr0 := gts.CcoreTestIncr()
	next, err := p.ParseIncremental(out, tree)
	if err != nil || next == nil {
		t.Fatal(err)
	}
	defer next.Release()
	fb1, incr1 := gts.CcoreTestIncr()
	if incr1 != incr0 || fb1["edits_not_carried"] != fb0["edits_not_carried"]+1 {
		t.Fatalf("reparse: incremental %d, fallbacks %v then %v; want a fresh parse for edits_not_carried", incr1-incr0, fb0, fb1)
	}
	fresh, _ := ccoreRouteParser(lang).Parse(out)
	defer fresh.Release()
	if next.RootNode().SExpr(lang) != fresh.RootNode().SExpr(lang) {
		t.Error("the reparse differs from a fresh parse")
	}
}

// A tree built by the certified core, given to a parser pinned to the legacy
// route, is reparsed fresh. The reported reason names the certified core, not
// the GSS forest fast path.
func TestLegacyReparseOfCertifiedCoreTreeReportsCoreReason(t *testing.T) {
	lang := grammars.JsonLanguage()
	oldTree, err := gts.NewParser(lang).Parse([]byte(`{"a": 1}`))
	if err != nil {
		t.Fatal(err)
	}
	defer oldTree.Release()
	if !gts.CcoreTestFromCore(oldTree) {
		t.Skip("the default route did not build this tree with the certified core")
	}
	oldTree.Edit(gts.InputEdit{StartByte: 6, OldEndByte: 7, NewEndByte: 8,
		StartPoint: gts.Point{Row: 0, Column: 6}, OldEndPoint: gts.Point{Row: 0, Column: 7}, NewEndPoint: gts.Point{Row: 0, Column: 8}})

	legacy := gts.NewParser(lang)
	legacy.SetAdmissionCandidateRoute(false)
	newTree, prof, err := legacy.ParseIncrementalProfiled([]byte(`{"a": 12}`), oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer newTree.Release()
	const want = "old tree was built by the certified core and is reparsed fresh by the legacy engine"
	if !prof.ReuseUnsupported || prof.ReuseUnsupportedReason != want {
		t.Fatalf("reuse unsupported = %t, reason = %q; want %q", prof.ReuseUnsupported, prof.ReuseUnsupportedReason, want)
	}
}
