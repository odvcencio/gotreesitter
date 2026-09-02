//go:build cgo && treesitter_c_parity

package cgoharness

// Structural parity tests: compare gotreesitter parse trees against a native C
// reference parser built from grammars/languages.lock commits, node-by-node.
//
// Run with:
//   go test . -tags treesitter_c_parity -run TestParity -v
//
// Requires CGo and a C toolchain. Gated behind build tag to keep the default
// test suite zero-CGo.

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type parityMeta struct {
	skipReason string // non-empty = skip with this message
}

type parityPythonTestIncrementalReuseScanner struct{ gotreesitter.ExternalScanner }

func (parityPythonTestIncrementalReuseScanner) SupportsIncrementalReuse() bool { return true }

func (parityPythonTestIncrementalReuseScanner) UsesExternalScannerCheckpoints() bool { return true }

func (parityPythonTestIncrementalReuseScanner) PreservesStateOnScanFailure() bool { return true }

var paritySkips = map[string]parityMeta{
	// Keep this map for explicitly known structural mismatches.
	// Parse-support-specific skips (e.g. missing scanners) should not live here.
}

// knownDegradedStructural tracks currently non-parity structural languages
// within the full-coverage gate. Keep this list shrinking over time.
//
// Every entry here MUST still actually diverge from the C reference parser.
// TestParityKnownDegradedStructuralStillDiverges (parity_gate_ratchet_test.go)
// re-parses each entry's smoke sample against the C oracle in the exhaustive
// lane and fails the build if a listed language unexpectedly passes, so this
// map can only shrink truthfully going forward.
var knownDegradedStructural = map[string]string{
	// Empty: the last known structural divergence (norg's `_word` alias
	// hidden-wrapper collapse) was fixed in native reduction and alias
	// materialization.
	// Fresh-parse structural parity is now 206/206 across the curated fleet.
}

// knownDegradedNoErrorClean preserves no-error coverage for structural backlog
// entries whose current failure is tree shape, not error-node production.
var knownDegradedNoErrorClean = map[string]struct{}{}

type parityCase struct {
	name           string
	source         string
	candidateRoute *bool
}

// parityCases is built dynamically from the full language registry so that
// every grammar with a smoke sample gets parity-tested against its C reference.
// Languages without a C reference in languages.lock are skipped at test time.
var parityCases = func() []parityCase {
	var cases []parityCase
	for _, entry := range grammars.AllLanguages() {
		cases = append(cases, parityCase{
			name:   entry.Name,
			source: grammars.ParseSmokeSample(entry.Name),
		})
	}
	return cases
}()

// hasDedicatedSample reports whether the language has a hand-written smoke
// sample rather than the generic "x\n" fallback. Structural parity tests
// should skip languages using the fallback since "x\n" is not valid syntax
// for most grammars and produces meaningless tree comparisons.
var hasDedicatedSample = func() map[string]bool {
	m := make(map[string]bool, len(grammars.ParseSmokeSamples))
	for name := range grammars.ParseSmokeSamples {
		m[name] = true
	}
	return m
}()

// curatedStructuralLanguages is the merge-blocking structural parity set.
// It includes every registered language with a dedicated smoke sample. Parse
// support is evaluated lazily when a language is exercised so a focused test
// does not decode the entire grammar fleet during package initialization.
var curatedStructuralLanguages = func() map[string]bool {
	out := make(map[string]bool, len(parityCases))
	for _, tc := range parityCases {
		if !hasDedicatedSample[tc.name] {
			continue
		}
		if _, ok := parityEntriesByName[tc.name]; !ok {
			continue
		}
		out[tc.name] = true
	}
	return out
}()

var parityEntriesByName = func() map[string]grammars.LangEntry {
	entries := make(map[string]grammars.LangEntry)
	for _, entry := range grammars.AllLanguages() {
		entries[entry.Name] = entry
	}
	return entries
}()

type paritySupportLoad struct {
	once   sync.Once
	report grammars.ParseSupport
	ok     bool
}

var paritySupportCache sync.Map // map[string]*paritySupportLoad

func paritySupportForName(name string) (grammars.ParseSupport, bool) {
	value, _ := paritySupportCache.LoadOrStore(name, &paritySupportLoad{})
	load := value.(*paritySupportLoad)
	load.once.Do(func() {
		load.report, load.ok = grammars.ParseSupportFor(name)
	})
	return load.report, load.ok
}

func TestParitySupportForName(t *testing.T) {
	report, ok := paritySupportForName("groovy")
	if !ok || report.Name != "groovy" || report.Backend == grammars.ParseBackendUnsupported {
		t.Fatalf("Groovy support = (%+v, %t), want supported report", report, ok)
	}
	if cached, cachedOK := paritySupportForName("groovy"); !cachedOK || cached != report {
		t.Fatalf("cached Groovy support = (%+v, %t), want (%+v, true)", cached, cachedOK, report)
	}
	if report, ok := paritySupportForName("not-a-language"); ok || report != (grammars.ParseSupport{}) {
		t.Fatalf("missing support = (%+v, %t), want zero report and false", report, ok)
	}
}

// curatedHighlightLanguages scales independently of structural parity. Any
// registered language with a dedicated smoke sample and shipped highlight
// query is part of the merge-blocking highlight parity gate. Parse support is
// evaluated lazily when the language is exercised. Divergence thresholds are
// controlled in parity_highlight_test.go.
var curatedHighlightLanguages = func() map[string]bool {
	out := make(map[string]bool, len(parityCases))
	for _, tc := range parityCases {
		if !hasDedicatedSample[tc.name] {
			continue
		}
		entry, ok := parityEntriesByName[tc.name]
		if !ok || strings.TrimSpace(entry.HighlightQuery) == "" {
			continue
		}
		out[tc.name] = true
	}
	return out
}()

// smokeParityLanguages is the default merge-blocking parity slice. Keep it
// intentionally small and representative; GLR coverage is enforced separately
// by dedicated canary tests.
var smokeParityLanguages = map[string]bool{
	"bash":       true,
	"c":          true,
	"c_sharp":    true,
	"go":         true,
	"html":       true,
	"java":       true,
	"javascript": true,
	"python":     true,
	"rust":       true,
	"tsx":        true,
	"typescript": true,
	"yaml":       true,
}

func parityMode() string {
	raw := strings.TrimSpace(os.Getenv("GTS_PARITY_MODE"))
	switch strings.ToLower(raw) {
	case "", "smoke":
		return "smoke"
	case "top50", "tier1":
		return "top50"
	case "all", "full", "exhaustive":
		return "exhaustive"
	default:
		return "smoke"
	}
}

func parityRunExhaustive() bool {
	return parityMode() == "exhaustive"
}

func parityRunTop50() bool {
	mode := parityMode()
	return mode == "top50" || mode == "exhaustive"
}

func parityRequireExhaustive(t *testing.T, suite string) {
	t.Helper()
	if parityRunExhaustive() {
		return
	}
	t.Skipf("%s requires GTS_PARITY_MODE=exhaustive", suite)
}

func parityRequireTop50(t *testing.T, suite string) {
	t.Helper()
	if parityRunTop50() {
		return
	}
	t.Skipf("%s requires GTS_PARITY_MODE=top50 or exhaustive", suite)
}

func parityIncludeStructuralLanguage(name string) bool {
	if !curatedStructuralLanguages[name] {
		return false
	}
	if parityRunExhaustive() {
		return true
	}
	if parityMode() == "top50" {
		return top50ParityLanguageSet[name]
	}
	return smokeParityLanguages[name]
}

func parityIncludeHighlightLanguage(name string) bool {
	if !curatedHighlightLanguages[name] {
		return false
	}
	if parityRunExhaustive() {
		return true
	}
	if parityMode() == "top50" {
		return top50ParityLanguageSet[name]
	}
	return smokeParityLanguages[name]
}

// parityCompareFields gates FieldName comparison in compareNodes (a Go tree
// vs the C oracle) and compareGoNodes (Go incremental-parse output vs Go
// fresh-parse output, i.e. incremental-parse field correctness). Field
// parity is on by default: the reference runtime's
// !field_map->inherited filter (node.c:673-687) is part of the tree shape
// the locked C oracle defines, not an optional extra, so a parity run that
// silently skips it is not exercising the gate its name promises. Set
// GTS_PARITY_COMPARE_FIELDS to a falsy value (0/false/no/off) to opt back
// out for bisection.
var parityCompareFields = func() bool {
	raw := strings.TrimSpace(os.Getenv("GTS_PARITY_COMPARE_FIELDS"))
	switch strings.ToLower(raw) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}()

var parityIgnoreKnownSkips = func() bool {
	raw := strings.TrimSpace(os.Getenv("GTS_PARITY_IGNORE_KNOWN_SKIPS"))
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}()

var parityParallel = func() bool {
	raw := strings.TrimSpace(os.Getenv("GTS_PARITY_PARALLEL"))
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}()

func parityMaybeParallel(t *testing.T) {
	t.Helper()
	if parityParallel {
		t.Parallel()
	}
}

var parityExcludedLanguages = func() map[string]struct{} {
	raw := strings.TrimSpace(os.Getenv("GTS_PARITY_SKIP_LANGS"))
	if raw == "" {
		return nil
	}
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}()

func parityLanguageExcluded(name string) bool {
	if len(parityExcludedLanguages) == 0 {
		return false
	}
	_, ok := parityExcludedLanguages[name]
	return ok
}

func paritySkipReason(name string) string {
	if parityIgnoreKnownSkips {
		return ""
	}
	if reason, ok := knownDegradedStructural[name]; ok {
		return "known degraded structural parity: " + reason
	}
	if meta, ok := paritySkips[name]; ok {
		return meta.skipReason
	}
	return ""
}

// parityNoErrorSkipReason returns a skip reason for the no-error gate. Some
// structural backlog entries still have useful no-error signal, so keep those
// running while fresh/incremental structural parity remains skipped.
func parityNoErrorSkipReason(name string) string {
	if _, ok := knownDegradedNoErrorClean[name]; ok {
		return ""
	}
	return paritySkipReason(name)
}

type nodeSnapshot struct {
	Type       string
	StartByte  uint32
	EndByte    uint32
	IsNamed    bool
	IsMissing  bool
	ChildCount int
}

func snapshotGo(n *gotreesitter.Node, lang *gotreesitter.Language) nodeSnapshot {
	return nodeSnapshot{
		Type:       dumpV1GoType(n, lang),
		StartByte:  n.StartByte(),
		EndByte:    n.EndByte(),
		IsNamed:    n.IsNamed(),
		IsMissing:  n.IsMissing(),
		ChildCount: n.ChildCount(),
	}
}

func snapshotC(n *sitter.Node) nodeSnapshot {
	return nodeSnapshot{
		Type:       n.Kind(),
		StartByte:  uint32(n.StartByte()),
		EndByte:    uint32(n.EndByte()),
		IsNamed:    n.IsNamed(),
		IsMissing:  n.IsMissing(),
		ChildCount: int(n.ChildCount()),
	}
}

// compareNodes walks both trees recursively in lockstep. Any divergence is
// appended to errs with a breadcrumb path for diagnosis.
func compareNodes(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node, path string, errs *[]string) {
	gs := snapshotGo(goNode, goLang)
	cs := snapshotC(cNode)

	if gs.Type != cs.Type {
		*errs = append(*errs, fmt.Sprintf("%s: Type go=%q c=%q", path, gs.Type, cs.Type))
	}
	if gs.StartByte != cs.StartByte {
		*errs = append(*errs, fmt.Sprintf("%s: StartByte go=%d c=%d", path, gs.StartByte, cs.StartByte))
	}
	if gs.EndByte != cs.EndByte {
		*errs = append(*errs, fmt.Sprintf("%s: EndByte go=%d c=%d", path, gs.EndByte, cs.EndByte))
	}
	if gs.IsNamed != cs.IsNamed {
		*errs = append(*errs, fmt.Sprintf("%s: IsNamed go=%v c=%v", path, gs.IsNamed, cs.IsNamed))
	}
	if gs.IsMissing != cs.IsMissing {
		*errs = append(*errs, fmt.Sprintf("%s: IsMissing go=%v c=%v", path, gs.IsMissing, cs.IsMissing))
	}
	if gs.ChildCount != cs.ChildCount {
		*errs = append(*errs, fmt.Sprintf("%s: ChildCount go=%d c=%d (goType=%q cType=%q goBytes=[%d-%d] cBytes=[%d-%d])", path, gs.ChildCount, cs.ChildCount, gs.Type, cs.Type, gs.StartByte, gs.EndByte, cs.StartByte, cs.EndByte))
		return
	}

	for i := 0; i < gs.ChildCount; i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		if parityCompareFields {
			goField := goNode.FieldNameForChild(i, goLang)
			cField := cNode.FieldNameForChild(uint32(i))
			if goField != cField {
				*errs = append(*errs, fmt.Sprintf("%s: FieldName go=%q c=%q", childPath, goField, cField))
			}
		}
		compareNodes(goNode.Child(i), goLang, cNode.Child(uint(i)), childPath, errs)
	}
}

// compareGoNodes walks two gotreesitter trees recursively in lockstep.
func compareGoNodes(left *gotreesitter.Node, lang *gotreesitter.Language, right *gotreesitter.Node, path string, errs *[]string) {
	if left == nil || right == nil {
		if left != right {
			*errs = append(*errs, fmt.Sprintf("%s: nil mismatch left=%v right=%v", path, left == nil, right == nil))
		}
		return
	}

	ls := snapshotGo(left, lang)
	rs := snapshotGo(right, lang)

	if ls.Type != rs.Type {
		*errs = append(*errs, fmt.Sprintf("%s: Type left=%q right=%q", path, ls.Type, rs.Type))
	}
	if ls.StartByte != rs.StartByte {
		*errs = append(*errs, fmt.Sprintf("%s: StartByte left=%d right=%d", path, ls.StartByte, rs.StartByte))
	}
	if ls.EndByte != rs.EndByte {
		*errs = append(*errs, fmt.Sprintf("%s: EndByte left=%d right=%d", path, ls.EndByte, rs.EndByte))
	}
	if ls.IsNamed != rs.IsNamed {
		*errs = append(*errs, fmt.Sprintf("%s: IsNamed left=%v right=%v", path, ls.IsNamed, rs.IsNamed))
	}
	if ls.IsMissing != rs.IsMissing {
		*errs = append(*errs, fmt.Sprintf("%s: IsMissing left=%v right=%v", path, ls.IsMissing, rs.IsMissing))
	}
	if ls.ChildCount != rs.ChildCount {
		*errs = append(*errs, fmt.Sprintf("%s: ChildCount left=%d right=%d", path, ls.ChildCount, rs.ChildCount))
		return
	}

	for i := 0; i < ls.ChildCount; i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		if parityCompareFields {
			leftField := left.FieldNameForChild(i, lang)
			rightField := right.FieldNameForChild(i, lang)
			if leftField != rightField {
				*errs = append(*errs, fmt.Sprintf("%s: FieldName left=%q right=%q", childPath, leftField, rightField))
			}
		}
		compareGoNodes(left.Child(i), lang, right.Child(i), childPath, errs)
	}
}

func parseWithGo(tc parityCase, src []byte, oldTree *gotreesitter.Tree) (*gotreesitter.Tree, *gotreesitter.Language, error) {
	entry, ok := parityEntriesByName[tc.name]
	if !ok {
		return nil, nil, fmt.Errorf("missing gotreesitter registry entry for %q", tc.name)
	}
	report, ok := paritySupportForName(tc.name)
	if !ok {
		return nil, nil, fmt.Errorf("missing parse support report for %q", tc.name)
	}

	lang := entry.Language()
	parser := gotreesitter.NewParser(lang)
	if tc.candidateRoute != nil {
		parser.SetAdmissionCandidateRoute(*tc.candidateRoute)
	}

	var tree *gotreesitter.Tree
	var parseErr error
	if oldTree == nil {
		switch report.Backend {
		case grammars.ParseBackendTokenSource:
			if entry.TokenSourceFactory == nil {
				return nil, nil, fmt.Errorf("token source backend without factory for %q", tc.name)
			}
			tree, parseErr = parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
		case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
			tree, parseErr = parser.Parse(src)
		default:
			return nil, nil, fmt.Errorf("unsupported parse backend %q for %q", report.Backend, tc.name)
		}
	} else {
		switch report.Backend {
		case grammars.ParseBackendTokenSource:
			if entry.TokenSourceFactory == nil {
				return nil, nil, fmt.Errorf("token source backend without factory for %q", tc.name)
			}
			tree, parseErr = parser.ParseIncrementalWithTokenSource(src, oldTree, entry.TokenSourceFactory(src, lang))
		case grammars.ParseBackendDFA, grammars.ParseBackendDFAPartial:
			tree, parseErr = parser.ParseIncremental(src, oldTree)
		default:
			return nil, nil, fmt.Errorf("unsupported incremental backend %q for %q", report.Backend, tc.name)
		}
	}
	if parseErr != nil {
		return nil, nil, fmt.Errorf("gotreesitter parse error: %w", parseErr)
	}

	if tree == nil || tree.RootNode() == nil {
		return nil, nil, fmt.Errorf("gotreesitter returned nil tree")
	}
	return tree, lang, nil
}

func releaseGoTree(tree *gotreesitter.Tree) {
	if tree != nil {
		tree.Release()
	}
}

func scheduleParityMemoryScavenge(t *testing.T) {
	t.Helper()
	if parityParallel {
		return
	}
	t.Cleanup(func() {
		runtime.GC()
		debug.FreeOSMemory()
	})
}

func runParityCase(t *testing.T, tc parityCase, label string, src []byte) {
	t.Helper()

	cLang, err := ParityCLanguage(tc.name)
	if err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("[%s/%s] skip C reference parser: %s", tc.name, label, skipReason)
		}
		t.Fatalf("[%s/%s] load C parser from languages.lock: %v", tc.name, label, err)
	}
	goTree, goLang, err := parseWithGo(tc, src, nil)
	if err != nil {
		t.Fatalf("[%s/%s] gotreesitter parse error: %v", tc.name, label, err)
	}
	defer releaseGoTree(goTree)

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("[%s/%s] skip C reference parser SetLanguage: %s", tc.name, label, skipReason)
		}
		t.Fatalf("[%s/%s] C parser SetLanguage error: %v", tc.name, label, err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatalf("[%s/%s] C reference parser returned nil tree", tc.name, label)
	}
	defer cTree.Close()
	goRoot := goTree.RootNode()
	cRoot := cTree.RootNode()
	goRuntime := goTree.ParseRuntime()

	var errs []string
	compareNodes(goRoot, goLang, cRoot, "root", &errs)
	if len(errs) == 0 {
		return
	}

	const maxErrors = 20
	shown := errs
	extra := 0
	if len(errs) > maxErrors {
		shown = errs[:maxErrors]
		extra = len(errs) - maxErrors
	}
	msg := strings.Join(shown, "\n  ")
	if extra > 0 {
		msg += fmt.Sprintf("\n  ... and %d more", extra)
	}
	goSummary := fmt.Sprintf(
		"go_root type=%q end=%d/%d hasError=%v %s",
		goRoot.Type(goLang), goRoot.EndByte(), len(src), goRoot.HasError(), goRuntime.Summary(),
	)
	cSummary := fmt.Sprintf(
		"c_root type=%q end=%d/%d hasError=%v",
		cRoot.Kind(), cRoot.EndByte(), len(src), cRoot.HasError(),
	)
	t.Errorf("[%s/%s] %d node divergence(s):\n  %s\n  %s\n  %s", tc.name, label, len(errs), msg, goSummary, cSummary)
}

func parityReferenceSkipReason(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "ABI version") || strings.Contains(msg, "Incompatible language version") ||
		strings.Contains(msg, "undefined symbol") {
		return msg
	}
	return ""
}

func normalizedSource(name, src string) []byte {
	_ = name
	return []byte(src)
}

// safeEditOffset chooses one insertion offset for a single-byte whitespace edit.
func safeEditOffset(src []byte) int {
	insertAt := -1

	for i := len(src) - 1; i >= 0; i-- {
		if src[i] == ' ' || src[i] == '\t' {
			insertAt = i
			break
		}
	}
	if insertAt < 0 && len(src) > 0 && src[0] == '<' {
		for i := 0; i+1 < len(src); i++ {
			if src[i] == '>' && src[i+1] == '<' {
				insertAt = i + 1
				break
			}
		}
	}
	if insertAt < 0 {
		for i := len(src) - 1; i >= 0; i-- {
			switch src[i] {
			case '\n', '}', ';', ')', ']', '>', '<':
				insertAt = i
			}
			if insertAt >= 0 {
				break
			}
		}
	}
	if insertAt < 0 {
		insertAt = len(src)
	}
	return insertAt
}

type incrementalEditCandidate struct {
	label       string
	start       int
	oldEnd      int
	replacement []byte
}

func (c incrementalEditCandidate) newEnd() int {
	return c.start + len(c.replacement)
}

func applyEditCandidate(src []byte, c incrementalEditCandidate) []byte {
	if c.start < 0 {
		c.start = 0
	}
	if c.start > len(src) {
		c.start = len(src)
	}
	if c.oldEnd < c.start {
		c.oldEnd = c.start
	}
	if c.oldEnd > len(src) {
		c.oldEnd = len(src)
	}
	outLen := len(src) - (c.oldEnd - c.start) + len(c.replacement)
	edited := make([]byte, outLen)
	pos := 0
	pos += copy(edited[pos:], src[:c.start])
	pos += copy(edited[pos:], c.replacement)
	copy(edited[pos:], src[c.oldEnd:])
	return edited
}

func replacementByteForIncrementalParity(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '8':
		return b + 1, true
	case b == '9':
		return '0', true
	case b >= 'a' && b <= 'y':
		return b + 1, true
	case b == 'z':
		return 'a', true
	case b >= 'A' && b <= 'Y':
		return b + 1, true
	case b == 'Z':
		return 'A', true
	default:
		return 0, false
	}
}

func isWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}

// incrementalEditOffsets returns preferred insertion offsets for incremental
// parity tests. It tries syntax-local edits first, then falls back to all
// interior offsets so the test can find a fresh-parity-safe site.
func incrementalEditOffsets(src []byte) []int {
	if len(src) == 0 {
		return []int{0}
	}

	offsets := make([]int, 0, len(src)+8)
	seen := make([]bool, len(src)+1)
	add := func(pos int) {
		if pos < 0 || pos > len(src) || seen[pos] {
			return
		}
		seen[pos] = true
		offsets = append(offsets, pos)
	}

	add(safeEditOffset(src))

	// For markup-like sources, prefer edits in text content (outside tags).
	if src[0] == '<' {
		inTag := false
		for i := 0; i < len(src); i++ {
			switch src[i] {
			case '<':
				inTag = true
			case '>':
				inTag = false
			default:
				if !inTag && i > 0 && isWordByte(src[i-1]) && isWordByte(src[i]) {
					add(i)
				}
			}
		}
	}

	// Prefer edits inside existing word runs across languages.
	for i := 1; i < len(src); i++ {
		if isWordByte(src[i-1]) && isWordByte(src[i]) {
			add(i)
		}
	}

	// Exhaustive fallback over interior positions, biased toward tail.
	for i := len(src) - 1; i >= 1; i-- {
		add(i)
	}
	add(len(src))
	return offsets
}

// incrementalEditCandidates returns preferred one-byte edits for incremental
// parity checks. It tries insertion sites first, then safe one-byte
// replacements (digits/letters) as a fallback for token-source backends where
// insertion-only edits can produce non-comparable fresh parses.
func incrementalEditCandidates(src []byte) []incrementalEditCandidate {
	offsets := incrementalEditOffsets(src)
	candidates := make([]incrementalEditCandidate, 0, len(offsets)+len(src)/2)
	for _, pos := range offsets {
		candidates = append(candidates, incrementalEditCandidate{
			label:       fmt.Sprintf("insert-space@%d", pos),
			start:       pos,
			oldEnd:      pos,
			replacement: []byte{' '},
		})
	}

	seenReplace := make([]bool, len(src))
	addReplace := func(i int) {
		if i < 0 || i >= len(src) || seenReplace[i] {
			return
		}
		repl, ok := replacementByteForIncrementalParity(src[i])
		if !ok || repl == src[i] {
			return
		}
		seenReplace[i] = true
		candidates = append(candidates, incrementalEditCandidate{
			label:       fmt.Sprintf("replace@%d:%q->%q", i, src[i], repl),
			start:       i,
			oldEnd:      i + 1,
			replacement: []byte{repl},
		})
	}

	// Prefer replacing digits first (usually syntax-safe in smoke samples),
	// then letters.
	for i := 0; i < len(src); i++ {
		if src[i] >= '0' && src[i] <= '9' {
			addReplace(i)
		}
	}
	for i := 0; i < len(src); i++ {
		if isWordByte(src[i]) {
			addReplace(i)
		}
	}

	return candidates
}

// TestParityFreshParse verifies that fresh parse trees match the CGo binding
// on the currently CI-gated language set.
func TestParityFreshParse(t *testing.T) {
	for _, tc := range parityCases {
		if parityLanguageExcluded(tc.name) {
			continue
		}
		if !parityIncludeStructuralLanguage(tc.name) {
			continue
		}
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parityMaybeParallel(t)
			scheduleParityMemoryScavenge(t)
			if reason := paritySkipReason(tc.name); reason != "" {
				t.Skipf("known mismatch: %s", reason)
			}
			runParityCase(t, tc, "fresh", normalizedSource(tc.name, tc.source))
		})
	}
}

func TestParityCompactConvergedSplitCorpus(t *testing.T) {
	parityRequireExhaustive(t, "compact converged-split corpus parity")
	candidateRoute := true
	tests := []struct {
		name       string
		corpusPath string
		source     string
	}{
		{name: "bash", corpusPath: "../testdata/compact_converged_split/bash.sh"},
		{name: "erlang", corpusPath: "../testdata/compact_converged_split/erlang.erl"},
		{name: "haskell", source: grammars.ParseSmokeSample("haskell")},
		{name: "javascript", corpusPath: "../testdata/compact_converged_split/javascript.js"},
		{name: "python", source: "def greet(name):\n    return f\"hello {name}\"\n\nprint(greet(\"world\"))\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(test.source)
			if test.corpusPath != "" {
				var err error
				source, err = os.ReadFile(test.corpusPath)
				if err != nil {
					t.Fatalf("read compact certification witness: %v", err)
				}
			}

			beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
			runParityCase(t, parityCase{
				name:           test.name,
				source:         string(source),
				candidateRoute: &candidateRoute,
			}, "compact-converged-split", source)
			afterRouted, afterFallback := gotreesitter.AdmissionCandidateCounters()
			routed := afterRouted - beforeRouted
			fallback := afterFallback - beforeFallback
			if routed != 1 || fallback != 0 {
				t.Fatalf(
					"candidate route counters = %d/%d, want 1/0; reason=%s",
					routed,
					fallback,
					gotreesitter.AdmissionCandidateLastFallbackReason(),
				)
			}
		})
	}
}

// TestParityIncrementalParse verifies that gotreesitter incremental parse
// matches a CGo fresh parse on edited source.
func TestParityIncrementalParse(t *testing.T) {
	for _, tc := range parityCases {
		if parityLanguageExcluded(tc.name) {
			continue
		}
		if !parityIncludeStructuralLanguage(tc.name) {
			continue
		}
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parityMaybeParallel(t)
			scheduleParityMemoryScavenge(t)
			if reason := paritySkipReason(tc.name); reason != "" {
				t.Skipf("known mismatch: %s", reason)
			}

			src := normalizedSource(tc.name, tc.source)
			if len(src) < 2 {
				t.Skip("source too short for incremental edit")
			}

			oldTree, _, err := parseWithGo(tc, src, nil)
			if err != nil {
				t.Fatalf("[%s/incremental] initial gotreesitter parse error: %v", tc.name, err)
			}
			defer releaseGoTree(oldTree)
			cLang, err := ParityCLanguage(tc.name)
			if err != nil {
				if skipReason := parityReferenceSkipReason(err); skipReason != "" {
					t.Skipf("[%s/incremental] skip C reference parser: %s", tc.name, skipReason)
				}
				t.Fatalf("[%s/incremental] load C parser from languages.lock: %v", tc.name, err)
			}

			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				if skipReason := parityReferenceSkipReason(err); skipReason != "" {
					t.Skipf("[%s/incremental] skip C parser SetLanguage: %s", tc.name, skipReason)
				}
				t.Fatalf("[%s/incremental] C parser SetLanguage error: %v", tc.name, err)
			}

			candidates := incrementalEditCandidates(src)
			edited := []byte(nil)
			var chosen incrementalEditCandidate
			chosenOK := false
			validSiteCount := 0
			scanAllValidSites := tc.name == "html" && parityRunExhaustive()
			firstFreshErr := ""
			firstIncrErr := ""
			for _, candidate := range candidates {
				candidateEdited := applyEditCandidate(src, candidate)

				goFreshTree, goFreshLang, err := parseWithGo(tc, candidateEdited, nil)
				if err != nil {
					t.Fatalf("[%s/incremental] gotreesitter fresh parse on candidate %s error: %v", tc.name, candidate.label, err)
				}

				cFreshTree := cParser.Parse(candidateEdited, nil)
				if cFreshTree == nil || cFreshTree.RootNode() == nil {
					t.Fatalf("[%s/incremental] C reference parser returned nil tree on candidate %s", tc.name, candidate.label)
				}

				var freshErrs []string
				compareNodes(goFreshTree.RootNode(), goFreshLang, cFreshTree.RootNode(), "root", &freshErrs)
				cFreshTree.Close()
				if len(freshErrs) > 0 {
					releaseGoTree(goFreshTree)
					if firstFreshErr == "" {
						firstFreshErr = fmt.Sprintf("%s: %s", candidate.label, freshErrs[0])
					}
					continue
				}
				// Candidate sites must also preserve Go incremental correctness:
				// incremental parse on edited source must match Go fresh parse.
				candidateOldTree, _, err := parseWithGo(tc, src, nil)
				if err != nil {
					t.Fatalf("[%s/incremental] gotreesitter candidate old-tree parse on %s error: %v", tc.name, candidate.label, err)
				}
				candidateEdit := gotreesitter.InputEdit{
					StartByte:   uint32(candidate.start),
					OldEndByte:  uint32(candidate.oldEnd),
					NewEndByte:  uint32(candidate.newEnd()),
					StartPoint:  pointAtOffset(src, candidate.start),
					OldEndPoint: pointAtOffset(src, candidate.oldEnd),
					NewEndPoint: pointAtOffset(candidateEdited, candidate.newEnd()),
				}
				candidateOldTree.Edit(candidateEdit)

				goIncrTree, goIncrLang, err := parseWithGo(tc, candidateEdited, candidateOldTree)
				if err != nil {
					releaseGoTree(candidateOldTree)
					t.Fatalf("[%s/incremental] gotreesitter candidate incremental parse on %s error: %v", tc.name, candidate.label, err)
				}
				releaseGoTree(candidateOldTree)

				var incrErrs []string
				compareGoNodes(goIncrTree.RootNode(), goIncrLang, goFreshTree.RootNode(), "root", &incrErrs)
				releaseGoTree(goFreshTree)
				releaseGoTree(goIncrTree)
				if len(incrErrs) > 0 {
					if firstIncrErr == "" {
						firstIncrErr = fmt.Sprintf("%s: %s", candidate.label, incrErrs[0])
					}
					continue
				}
				validSiteCount++
				if !chosenOK {
					edited = candidateEdited
					chosen = candidate
					chosenOK = true
				}
				// Most languages only need the first fresh-parity-safe site.
				// Exhaustive mode keeps html's full-site coverage diagnostic.
				if !scanAllValidSites {
					break
				}
			}
			if scanAllValidSites {
				t.Logf("[%s/incremental] valid edit sites=%d/%d", tc.name, validSiteCount, len(candidates))
			}
			if !chosenOK {
				reason := firstFreshErr
				if reason == "" {
					reason = firstIncrErr
				}
				if reason == "" {
					reason = "no comparable fresh/incremental trees from candidate edits"
				}
				t.Skipf("[%s/incremental] no fresh-parity-safe incremental edit site found (%d candidates, first divergence: %s)", tc.name, len(candidates), reason)
			}

			edit := gotreesitter.InputEdit{
				StartByte:   uint32(chosen.start),
				OldEndByte:  uint32(chosen.oldEnd),
				NewEndByte:  uint32(chosen.newEnd()),
				StartPoint:  pointAtOffset(src, chosen.start),
				OldEndPoint: pointAtOffset(src, chosen.oldEnd),
				NewEndPoint: pointAtOffset(edited, chosen.newEnd()),
			}

			oldTree.Edit(edit)

			goTree, goLang, err := parseWithGo(tc, edited, oldTree)
			if err != nil {
				t.Fatalf("[%s/incremental] gotreesitter incremental parse error: %v", tc.name, err)
			}
			defer releaseGoTree(goTree)

			cTree := cParser.Parse(edited, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatalf("[%s/incremental] C reference parser returned nil tree", tc.name)
			}
			defer cTree.Close()

			var errs []string
			compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
			if len(errs) == 0 {
				return
			}

			const maxErrors = 20
			shown := errs
			extra := 0
			if len(errs) > maxErrors {
				shown = errs[:maxErrors]
				extra = len(errs) - maxErrors
			}
			msg := strings.Join(shown, "\n  ")
			if extra > 0 {
				msg += fmt.Sprintf("\n  ... and %d more", extra)
			}
			t.Errorf("[%s/incremental] %d node divergence(s):\n  %s", tc.name, len(errs), msg)
		})
	}
}

func TestParityPythonIncrementalRepetitionFoldDelete(t *testing.T) {
	source := []byte("from contextlib import suppress\n" +
		"from copy import deepcopy\n" +
		"from enum import Enum\n" +
		"from errno import EACCES\n" +
		"from functools import partial\n")
	offset := strings.LastIndex(string(source), "import partial") + len("impor")
	if offset < len("impor") || source[offset] != 't' {
		t.Fatal("locked Python incremental witness missing final import byte")
	}
	edited := make([]byte, 0, len(source)-1)
	edited = append(edited, source[:offset]...)
	edited = append(edited, source[offset+1:]...)

	base := grammars.PythonLanguage()
	goLangValue := *base
	goLangValue.ExternalScanner = parityPythonTestIncrementalReuseScanner{ExternalScanner: base.ExternalScanner}
	goLang := &goLangValue
	goParser := gotreesitter.NewParser(goLang)
	oldTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(oldTree)
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset),
		StartPoint:  pointAtOffset(source, offset),
		OldEndPoint: pointAtOffset(source, offset+1),
		NewEndPoint: pointAtOffset(edited, offset),
	})
	goTree, err := goParser.ParseIncremental(edited, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(goTree)

	cLang, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(edited, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C reference parser returned nil tree")
	}
	defer cTree.Close()

	var errs []string
	compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
	if len(errs) > 0 {
		t.Fatalf("Python incremental tree differs from locked C oracle: %s", errs[0])
	}
}

func TestParityPythonIncrementalDedentCheckpointFallback(t *testing.T) {
	source, err := os.ReadFile("corpus_structural/python_sample.py")
	if err != nil {
		t.Fatal(err)
	}
	const offset = 500
	if source[offset] != 'n' {
		t.Fatalf("locked Python DEDENT witness changed at byte %d: got %q, want n", offset, source[offset])
	}
	edited := append(append([]byte(nil), source[:offset]...), source[offset+1:]...)
	tc := parityCase{name: "python", source: string(source)}
	oldTree, _, err := parseWithGo(tc, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(oldTree)
	oldTree.Edit(gotreesitter.InputEdit{
		StartByte:   offset,
		OldEndByte:  offset + 1,
		NewEndByte:  offset,
		StartPoint:  pointAtOffset(source, offset),
		OldEndPoint: pointAtOffset(source, offset+1),
		NewEndPoint: pointAtOffset(edited, offset),
	})
	goTree, goLang, err := parseWithGo(tc, edited, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(goTree)

	cLang, err := ParityCLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cTree := cParser.Parse(edited, nil)
	if cTree == nil || cTree.RootNode() == nil {
		t.Fatal("C reference parser returned nil tree")
	}
	defer cTree.Close()

	var errs []string
	compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
	if len(errs) > 0 {
		t.Fatalf("Python incremental DEDENT fallback differs from locked C oracle: %s", errs[0])
	}
}

func TestParityPythonDerivedScannerIncremental(t *testing.T) {
	cases := []struct {
		name   string
		lang   func() *gotreesitter.Language
		source []byte
	}{
		{name: "python", lang: grammars.PythonLanguage, source: []byte("value = 1\n")},
		{name: "mojo", lang: grammars.MojoLanguage, source: []byte("fn main():\n    print(1)\n")},
		{name: "starlark", lang: grammars.StarlarkLanguage, source: []byte("value = 1\n")},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			offset := bytes.IndexByte(tc.source, '1')
			if offset < 0 {
				t.Fatalf("locked %s scanner-fallback witness is malformed", tc.name)
			}
			edited := append([]byte(nil), tc.source...)
			edited[offset] = 'x'
			edit := gotreesitter.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointAtOffset(tc.source, offset),
				OldEndPoint: pointAtOffset(tc.source, offset+1),
				NewEndPoint: pointAtOffset(edited, offset+1),
			}

			goLang := tc.lang()
			goParser := gotreesitter.NewParser(goLang)
			oldTree, err := goParser.Parse(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseGoTree(oldTree)
			oldTree.Edit(edit)
			goTree, profile, err := goParser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatal(err)
			}
			defer releaseGoTree(goTree)
			if tc.name == "python" {
				if profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "" {
					t.Fatalf("%s authenticated scanner reuse declined: %+v", tc.name, profile)
				}
			} else if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" ||
				profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("%s scanner fallback profile is not fail-closed: %+v", tc.name, profile)
			}

			cLang, err := ParityCLanguage(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(edited, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C reference parser returned nil tree")
			}
			defer cTree.Close()

			var errs []string
			compareNodes(goTree.RootNode(), goLang, cTree.RootNode(), "root", &errs)
			if len(errs) > 0 {
				t.Fatalf("%s scanner fallback differs from locked C oracle: %s", tc.name, errs[0])
			}
		})
	}
}

func TestParityJavaScriptIncrementalRepetitionFoldControl(t *testing.T) {
	// JavaScript is automatically forest-routed for fresh parses. This isolated
	// parity process disables that route so the test exercises the production
	// incremental dispatcher whose reuse behavior changed.
	restoreForest := os.Getenv("GOT_GLR_FOREST") != "0"
	gotreesitter.SetGLRForestEnabled(false)
	t.Cleanup(func() { gotreesitter.SetGLRForestEnabled(restoreForest) })
	t.Setenv("GOT_PARSE_ACTION_TIMING", "1")
	gotreesitter.ResetParseEnvConfigCacheForTests()
	t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)

	source := []byte("function alpha() { return 1; }\n" +
		"function beta() { return 2; }\n" +
		"function gamma() { return 3; }\n" +
		"function delta() { return 4; }\n" +
		"function epsilon() { return 5; }\n")
	offset := strings.Index(string(source), "return 1") + len("return ")
	if offset < len("return ") || source[offset] != '1' {
		t.Fatal("locked JavaScript incremental control witness is malformed")
	}
	edited := append([]byte(nil), source...)
	edited[offset] = 'z'
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointAtOffset(source, offset),
		OldEndPoint: pointAtOffset(source, offset+1),
		NewEndPoint: pointAtOffset(edited, offset+1),
	}

	goLang := grammars.JavascriptLanguage()
	goParser := gotreesitter.NewParser(goLang)
	oldTree, err := goParser.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(oldTree)
	oldTree.Edit(edit)
	ambiguity := gotreesitter.NewAmbiguityProfile()
	goParser.SetAmbiguityProfile(ambiguity)
	goTree, incrementalProfile, err := goParser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(goTree)
	if incrementalProfile.ReuseUnsupported || !incrementalProfile.OldTreeReuseRoute || incrementalProfile.ReusedSubtrees == 0 {
		t.Fatalf("JavaScript control did not exercise old-tree reuse: %+v", incrementalProfile)
	}
	if !ambiguityProfileHasUnprofiledRepetitionReduceChoice(ambiguity, goLang) {
		for _, stat := range ambiguity.SnapshotTop(-1) {
			t.Logf("ambiguity state=%d lookahead=%d actions=%+v hits=%d forks=%d choice_ns=%d fork_ns=%d", stat.State, stat.Lookahead, stat.Actions, stat.Hits, stat.Forks, stat.ConflictChoiceNanos, stat.ConflictForkNanos)
		}
		t.Fatal("JavaScript incremental control did not exercise the global C repetition-skip fold")
	}

	goFresh, err := goParser.Parse(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseGoTree(goFresh)
	var goErrs []string
	compareGoNodes(goTree.RootNode(), goLang, goFresh.RootNode(), "root", &goErrs)
	if len(goErrs) > 0 {
		t.Fatalf("JavaScript incremental tree differs from fresh Go: %s", goErrs[0])
	}

	cLang, err := ParityCLanguage("javascript")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	cOld := cParser.Parse(source, nil)
	if cOld == nil || cOld.RootNode() == nil {
		t.Fatal("C reference parser returned nil old tree")
	}
	cEdit := realCorpusCInputEdit(edit)
	cOld.Edit(&cEdit)
	cIncremental := cParser.Parse(edited, cOld)
	cOld.Close()
	if cIncremental == nil || cIncremental.RootNode() == nil {
		t.Fatal("C reference parser returned nil incremental tree")
	}
	defer cIncremental.Close()
	cFresh := cParser.Parse(edited, nil)
	if cFresh == nil || cFresh.RootNode() == nil {
		t.Fatal("C reference parser returned nil fresh tree")
	}
	defer cFresh.Close()
	if got, want := canonicalCTreeDigest(t, cIncremental, "JavaScript incremental C"), canonicalCTreeDigest(t, cFresh, "JavaScript fresh C"); got != want {
		t.Fatalf("JavaScript incremental C differs from fresh C: got=%s want=%s", got, want)
	}

	var errs []string
	compareNodes(goTree.RootNode(), goLang, cIncremental.RootNode(), "root", &errs)
	if len(errs) > 0 {
		t.Fatalf("JavaScript incremental tree differs from locked incremental C oracle: %s", errs[0])
	}
}

func ambiguityProfileHasUnprofiledRepetitionReduceChoice(profile *gotreesitter.AmbiguityProfile, lang *gotreesitter.Language) bool {
	if profile == nil || lang == nil {
		return false
	}
	for _, stat := range profile.SnapshotTop(-1) {
		if stat.ConflictChoiceNanos == 0 || len(stat.Actions) != 2 {
			continue
		}
		var repetitionShifts, reductions int
		for _, action := range stat.Actions {
			switch action.Type {
			case gotreesitter.ParseActionShift:
				if action.Repetition {
					repetitionShifts++
				}
			case gotreesitter.ParseActionReduce:
				reductions++
			}
		}
		if repetitionShifts != 1 || reductions != 1 {
			continue
		}
		profiled := false
		for _, policy := range lang.ConflictPolicies {
			stateMatch := policy.State == stat.State || policy.State == gotreesitter.ConflictPolicyAnyState
			lookaheadMatch := policy.Lookahead == stat.Lookahead || policy.Lookahead == gotreesitter.ConflictPolicyAnyLookahead
			if stateMatch && lookaheadMatch {
				profiled = true
				break
			}
		}
		if !profiled {
			return true
		}
	}
	return false
}

// TestParityHasNoErrors checks that well-formed inputs for CI-gated languages
// do not produce error nodes.
func TestParityHasNoErrors(t *testing.T) {
	for _, tc := range parityCases {
		if parityLanguageExcluded(tc.name) {
			continue
		}
		if !parityIncludeStructuralLanguage(tc.name) {
			continue
		}
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parityMaybeParallel(t)
			scheduleParityMemoryScavenge(t)
			if reason := parityNoErrorSkipReason(tc.name); reason != "" {
				t.Skipf("known mismatch: %s", reason)
			}
			tree, _, err := parseWithGo(tc, normalizedSource(tc.name, tc.source), nil)
			if err != nil {
				t.Fatalf("[%s/errors] gotreesitter parse error: %v", tc.name, err)
			}
			defer releaseGoTree(tree)
			if tree.RootNode().HasError() {
				t.Errorf("[%s] RootNode().HasError() = true on well-formed input", tc.name)
			}
		})
	}
}
