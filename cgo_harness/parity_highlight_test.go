//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// highlightCapture is a normalized capture for comparison between Go and C.
type highlightCapture struct {
	Name      string
	StartByte uint32
	EndByte   uint32
}

func (h highlightCapture) String() string {
	return fmt.Sprintf("@%s [%d-%d]", h.Name, h.StartByte, h.EndByte)
}

func includeHighlightCaptureName(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	return !strings.HasPrefix(name, "_")
}

// --- Three-tier highlight parity tracking ---
//
// Tier 1: curatedHighlightLanguages (in parity_cgo_test.go) — merge-blocking. These
//         use knownHighlightDivergence / knownHighlightGoOnly as strict
//         thresholds. Any regression fails the test.
//
// Tier 2: knownDegradedHighlight — "no new degradations" list. Non-curated
//         languages with known parity gaps. This list can shrink but not grow.
//         New languages appearing here means a regression.
//
// Tier 3: All other languages — diagnostic only (logged, never blocking).

// knownHighlightDivergence tracks C-only capture counts for curated languages
// with strict custom thresholds.
var knownHighlightDivergence = map[string]int{}

// knownHighlightGoOnly tracks Go-only capture counts for curated languages with
// strict custom thresholds.
var knownHighlightGoOnly = map[string]int{
	// All curated languages currently have 0 Go-only captures.
}

// highlightTolerance records maximum tolerated divergence counts.
type highlightTolerance struct {
	cMissing int
	goOnly   int
}

// knownDegradedHighlight is the "no new degradations" list for non-curated
// languages. Each entry records the maximum tolerated (cMissing, goOnly).
// This list can shrink (as fixes land) but must not grow (regressions block).
var knownDegradedHighlight = map[string]highlightTolerance{}

// collectGoHighlightCaptures runs a highlight query against a Go parse tree
// and returns sorted, deduplicated captures.
func collectGoHighlightCaptures(t *testing.T, lang *gotreesitter.Language, tree *gotreesitter.Tree, queryStr string, source []byte) []highlightCapture {
	t.Helper()

	q, err := gotreesitter.NewQuery(queryStr, lang)
	if err != nil {
		t.Fatalf("Go NewQuery error: %v", err)
	}

	matches := q.Execute(tree)
	var caps []highlightCapture
	for _, m := range matches {
		for _, c := range m.Captures {
			if !includeHighlightCaptureName(c.Name) {
				continue
			}
			caps = append(caps, highlightCapture{
				Name:      c.Name,
				StartByte: c.Node.StartByte(),
				EndByte:   c.Node.EndByte(),
			})
		}
	}
	return deduplicateCaptures(caps)
}

// collectCHighlightCaptures runs the same highlight query against a C parse tree
// and returns sorted, deduplicated captures.
func collectCHighlightCaptures(t *testing.T, cLang *sitter.Language, cTree *sitter.Tree, queryStr string, source []byte) []highlightCapture {
	t.Helper()

	cQuery, qErr := sitter.NewQuery(cLang, queryStr)
	if qErr != nil {
		t.Fatalf("C NewQuery error: %v", qErr)
	}
	defer cQuery.Close()

	cursor := sitter.NewQueryCursor()
	defer cursor.Close()

	cRoot := cTree.RootNode()
	matches := cursor.Matches(cQuery, cRoot, source)

	captureNames := cQuery.CaptureNames()

	var caps []highlightCapture
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		if !cQueryMatchSatisfiesGeneralPredicates(m, cQuery, source) {
			continue
		}
		for _, c := range m.Captures {
			name := ""
			if int(c.Index) < len(captureNames) {
				name = captureNames[c.Index]
			}
			if !includeHighlightCaptureName(name) {
				continue
			}
			caps = append(caps, highlightCapture{
				Name:      name,
				StartByte: uint32(c.Node.StartByte()),
				EndByte:   uint32(c.Node.EndByte()),
			})
		}
	}
	return deduplicateCaptures(caps)
}

func cQueryMatchSatisfiesGeneralPredicates(m *sitter.QueryMatch, query *sitter.Query, source []byte) bool {
	if m == nil || query == nil {
		return true
	}
	for _, pred := range query.GeneralPredicates(m.PatternIndex) {
		switch pred.Operator {
		case "lua-match?":
			if len(pred.Args) != 2 || pred.Args[0].CaptureId == nil || pred.Args[1].String == nil {
				return false
			}
			text, ok := cFirstCaptureTextForID(m, *pred.Args[0].CaptureId, source)
			if !ok {
				return false
			}
			rx, err := compileHighlightLuaPattern(*pred.Args[1].String)
			if err != nil || !rx.MatchString(text) {
				return false
			}
		}
	}
	return true
}

func cFirstCaptureTextForID(m *sitter.QueryMatch, captureID uint, source []byte) (string, bool) {
	for _, c := range m.Captures {
		if uint(c.Index) != captureID {
			continue
		}
		start := uint32(c.Node.StartByte())
		end := uint32(c.Node.EndByte())
		if start > end || end > uint32(len(source)) {
			return "", false
		}
		return string(source[start:end]), true
	}
	return "", false
}

func compileHighlightLuaPattern(pattern string) (*regexp.Regexp, error) {
	var out strings.Builder
	inClass := false
	classContentStart := false

	writeLuaClass := func(ch byte, inClass bool, classContentStart bool) bool {
		inClassText := ""
		outsideText := ""
		if inClass {
			switch ch {
			case 'a':
				inClassText = "A-Za-z"
			case 'A':
				inClassText = "^A-Za-z"
			case 'c':
				inClassText = "[:cntrl:]"
			case 'C':
				inClassText = "^[:cntrl:]"
			case 'd':
				inClassText = "0-9"
			case 'D':
				inClassText = "^0-9"
			case 'l':
				inClassText = "a-z"
			case 'L':
				inClassText = "^a-z"
			case 'p':
				inClassText = "[:punct:]"
			case 'P':
				inClassText = "^[:punct:]"
			case 's':
				inClassText = "\\s"
			case 'S':
				inClassText = "^\\s"
			case 'u':
				inClassText = "A-Z"
			case 'U':
				inClassText = "^A-Z"
			case 'w':
				inClassText = "A-Za-z0-9"
			case 'W':
				inClassText = "^A-Za-z0-9"
			case 'x':
				inClassText = "A-Fa-f0-9"
			case 'X':
				inClassText = "^A-Fa-f0-9"
			case 'z':
				inClassText = "\\x00"
			case 'Z':
				inClassText = "^\\x00"
			default:
				return false
			}
			if strings.HasPrefix(inClassText, "^") && !classContentStart {
				return false
			}
			out.WriteString(inClassText)
			return true
		}

		switch ch {
		case 'a':
			outsideText = "[A-Za-z]"
		case 'A':
			outsideText = "[^A-Za-z]"
		case 'c':
			outsideText = "[[:cntrl:]]"
		case 'C':
			outsideText = "[^[:cntrl:]]"
		case 'd':
			outsideText = "[0-9]"
		case 'D':
			outsideText = "[^0-9]"
		case 'l':
			outsideText = "[a-z]"
		case 'L':
			outsideText = "[^a-z]"
		case 'p':
			outsideText = "[[:punct:]]"
		case 'P':
			outsideText = "[^[:punct:]]"
		case 's':
			outsideText = "\\s"
		case 'S':
			outsideText = "\\S"
		case 'u':
			outsideText = "[A-Z]"
		case 'U':
			outsideText = "[^A-Z]"
		case 'w':
			outsideText = "[A-Za-z0-9]"
		case 'W':
			outsideText = "[^A-Za-z0-9]"
		case 'x':
			outsideText = "[A-Fa-f0-9]"
		case 'X':
			outsideText = "[^A-Fa-f0-9]"
		case 'z':
			outsideText = "\\x00"
		case 'Z':
			outsideText = "[^\\x00]"
		default:
			return false
		}
		out.WriteString(outsideText)
		return true
	}

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '[':
			inClass = true
			classContentStart = true
			out.WriteByte(ch)
		case ']':
			inClass = false
			classContentStart = false
			out.WriteByte(ch)
		case '%':
			if i+1 >= len(pattern) {
				out.WriteString("%")
				continue
			}
			i++
			next := pattern[i]
			if writeLuaClass(next, inClass, classContentStart) {
				if inClass {
					classContentStart = false
				}
				continue
			}
			out.WriteString(regexp.QuoteMeta(string(next)))
			if inClass {
				classContentStart = false
			}
		case '-':
			if inClass {
				out.WriteByte(ch)
				classContentStart = false
				continue
			}
			out.WriteString("*?")
		default:
			out.WriteByte(ch)
			if inClass {
				classContentStart = false
			}
		}
	}

	return regexp.Compile(out.String())
}

func TestCompileHighlightLuaPatternUppercaseClassesAndNonGreedy(t *testing.T) {
	tests := []struct {
		name      string
		pattern   string
		matches   []string
		nonMatch  []string
		findInput string
		findWant  string
	}{
		{
			name:     "non-digit",
			pattern:  `^%D+$`,
			matches:  []string{"abc", "_-"},
			nonMatch: []string{"123", "abc1"},
		},
		{
			name:     "non-space",
			pattern:  `^%S+$`,
			matches:  []string{"abc"},
			nonMatch: []string{"a b", "\t"},
		},
		{
			name:     "non-word",
			pattern:  `^%W+$`,
			matches:  []string{"-!", "_"},
			nonMatch: []string{"abc", "A1"},
		},
		{
			name:      "non-greedy",
			pattern:   `a.-b`,
			findInput: "a123b456b",
			findWant:  "a123b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rx, err := compileHighlightLuaPattern(tt.pattern)
			if err != nil {
				t.Fatalf("compileHighlightLuaPattern(%q): %v", tt.pattern, err)
			}
			for _, input := range tt.matches {
				if !rx.MatchString(input) {
					t.Fatalf("compileHighlightLuaPattern(%q).MatchString(%q) = false, want true", tt.pattern, input)
				}
			}
			for _, input := range tt.nonMatch {
				if rx.MatchString(input) {
					t.Fatalf("compileHighlightLuaPattern(%q).MatchString(%q) = true, want false", tt.pattern, input)
				}
			}
			if tt.findInput != "" {
				if got := rx.FindString(tt.findInput); got != tt.findWant {
					t.Fatalf("compileHighlightLuaPattern(%q).FindString(%q) = %q, want %q", tt.pattern, tt.findInput, got, tt.findWant)
				}
			}
		})
	}
}

// deduplicateCaptures sorts captures by (start, end, name) and removes exact duplicates.
func deduplicateCaptures(caps []highlightCapture) []highlightCapture {
	sort.Slice(caps, func(i, j int) bool {
		if caps[i].StartByte != caps[j].StartByte {
			return caps[i].StartByte < caps[j].StartByte
		}
		if caps[i].EndByte != caps[j].EndByte {
			return caps[i].EndByte < caps[j].EndByte
		}
		return caps[i].Name < caps[j].Name
	})
	out := caps[:0]
	for i, c := range caps {
		if i > 0 && c == caps[i-1] {
			continue
		}
		out = append(out, c)
	}
	return out
}

// runHighlightParity runs highlight capture comparison for a single language.
// Returns (goOnlyCount, cMissingCount).
func runHighlightParityForSource(t *testing.T, tc parityCase, src []byte) (goOnlyCount, cMissingCount int) {
	t.Helper()

	entry, ok := parityEntriesByName[tc.name]
	if !ok {
		t.Skipf("no registry entry for %q", tc.name)
	}
	queryStr := entry.HighlightQuery
	if queryStr == "" {
		t.Skipf("no highlight query for %q", tc.name)
	}

	// --- Go side ---
	goTree, goLang, err := parseWithGo(tc, src, nil)
	if err != nil {
		t.Fatalf("Go parse error: %v", err)
	}
	defer releaseGoTree(goTree)

	goCaps := collectGoHighlightCaptures(t, goLang, goTree, queryStr, src)

	// --- C side ---
	cLang, err := ParityCLanguage(tc.name)
	if err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("skip C reference: %s", skipReason)
		}
		t.Fatalf("load C parser: %v", err)
	}

	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		if skipReason := parityReferenceSkipReason(err); skipReason != "" {
			t.Skipf("skip C SetLanguage: %s", skipReason)
		}
		t.Fatalf("C SetLanguage: %v", err)
	}
	cTree := cParser.Parse(src, nil)
	if cTree == nil {
		t.Fatal("C parser returned nil tree")
	}
	defer cTree.Close()

	_, cQueryErr := sitter.NewQuery(cLang, queryStr)
	if cQueryErr != nil {
		t.Skipf("C query compilation error (ABI mismatch): %v", cQueryErr)
	}

	cCaps := collectCHighlightCaptures(t, cLang, cTree, queryStr, src)

	// --- Compare ---
	onlyGo, onlyC := diffCaptures(goCaps, cCaps)

	for _, c := range onlyGo {
		t.Logf("  Go-only: %s %q", c, textSlice(src, c))
	}
	for _, c := range onlyC {
		t.Logf("  C-only: %s %q", c, textSlice(src, c))
	}

	if len(onlyGo) == 0 && len(onlyC) == 0 {
		t.Logf("HIGHLIGHT PARITY OK: %d captures match", len(goCaps))
	} else {
		t.Logf("highlight parity: %d match, %d Go-only, %d C-only",
			len(goCaps)-len(onlyGo), len(onlyGo), len(onlyC))
	}

	return len(onlyGo), len(onlyC)
}

// runHighlightParity runs highlight capture comparison for a single language's
// canonical smoke sample source.
func runHighlightParity(t *testing.T, tc parityCase) (goOnlyCount, cMissingCount int) {
	t.Helper()
	return runHighlightParityForSource(t, tc, normalizedSource(tc.name, tc.source))
}

// TestParityHighlight is the merge-blocking highlight parity test for
// curated languages. Failures here block CI.
func TestParityHighlight(t *testing.T) {
	for _, tc := range parityCases {
		if parityLanguageExcluded(tc.name) {
			continue
		}
		if !parityIncludeHighlightLanguage(tc.name) {
			continue
		}
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			goOnlyCount, cMissingCount := runHighlightParity(t, tc)
			goThresh, cThresh := highlightThresholdsForLanguage(tc.name)

			// Check Go-only (false positives) against curated threshold.
			if goOnlyCount > goThresh {
				t.Errorf("Go-only captures: %d (threshold %d, %d new)",
					goOnlyCount, goThresh, goOnlyCount-goThresh)
			} else if goOnlyCount == 0 && goThresh > 0 {
				t.Logf("IMPROVED: Go-only was %d, now 0 — lower threshold for %q", goThresh, tc.name)
			}

			// Check C-missing against curated threshold.
			if cMissingCount > cThresh {
				t.Errorf("C-missing captures: %d (threshold %d, %d new)",
					cMissingCount, cThresh, cMissingCount-cThresh)
			} else if cMissingCount == 0 && cThresh > 0 {
				t.Logf("IMPROVED: C-missing was %d, now 0 — lower threshold for %q", cThresh, tc.name)
			}
		})
	}
}

func highlightThresholdsForLanguage(name string) (goOnly, cMissing int) {
	if v, ok := knownHighlightGoOnly[name]; ok {
		goOnly = v
	}
	if v, ok := knownHighlightDivergence[name]; ok {
		cMissing = v
	}
	if tol, ok := knownDegradedHighlight[name]; ok {
		if tol.goOnly > goOnly {
			goOnly = tol.goOnly
		}
		if tol.cMissing > cMissing {
			cMissing = tol.cMissing
		}
	}
	return goOnly, cMissing
}

// TestParityHighlightAllGrammars runs highlight parity across all 206 languages
// as a diagnostic. Non-curated languages use the knownDegradedHighlight list:
// regressions (worse than recorded) fail; improvements are logged.
func TestParityHighlightAllGrammars(t *testing.T) {
	parityRequireExhaustive(t, "TestParityHighlightAllGrammars")
	for _, tc := range parityCases {
		if parityLanguageExcluded(tc.name) {
			continue
		}
		if curatedHighlightLanguages[tc.name] {
			continue // tested by TestParityHighlight
		}
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			goOnlyCount, cMissingCount := runHighlightParity(t, tc)

			tol, isDegraded := knownDegradedHighlight[tc.name]

			if goOnlyCount == 0 && cMissingCount == 0 {
				if isDegraded {
					t.Logf("IMPROVED to full parity — remove %q from knownDegradedHighlight", tc.name)
				}
				return
			}

			if !isDegraded {
				// New degradation: this language was clean but now has issues.
				t.Errorf("NEW DEGRADATION: Go-only=%d C-missing=%d — add %q to knownDegradedHighlight",
					goOnlyCount, cMissingCount, tc.name)
				return
			}

			// Check for regressions against known tolerance.
			if goOnlyCount > tol.goOnly {
				t.Errorf("Go-only REGRESSION: %d (tolerance %d)", goOnlyCount, tol.goOnly)
			}
			if cMissingCount > tol.cMissing {
				t.Errorf("C-missing REGRESSION: %d (tolerance %d)", cMissingCount, tol.cMissing)
			}

			// Log improvements.
			if goOnlyCount < tol.goOnly {
				t.Logf("Go-only improved: %d (was %d)", goOnlyCount, tol.goOnly)
			}
			if cMissingCount < tol.cMissing {
				t.Logf("C-missing improved: %d (was %d)", cMissingCount, tol.cMissing)
			}
		})
	}
}

func textSlice(src []byte, c highlightCapture) string {
	if c.StartByte < uint32(len(src)) && c.EndByte <= uint32(len(src)) {
		s := string(src[c.StartByte:c.EndByte])
		if len(s) > 40 {
			return s[:40] + "..."
		}
		return s
	}
	return ""
}

// diffCaptures returns captures only in Go and only in C.
func diffCaptures(goCaps, cCaps []highlightCapture) (onlyGo, onlyC []highlightCapture) {
	type captureKey struct {
		name      string
		startByte uint32
		endByte   uint32
	}
	key := func(c highlightCapture) captureKey {
		return captureKey{
			name:      c.Name,
			startByte: c.StartByte,
			endByte:   c.EndByte,
		}
	}

	goSet := make(map[captureKey]struct{}, len(goCaps))
	for _, c := range goCaps {
		goSet[key(c)] = struct{}{}
	}
	cSet := make(map[captureKey]struct{}, len(cCaps))
	for _, c := range cCaps {
		cSet[key(c)] = struct{}{}
	}

	for _, c := range goCaps {
		if _, ok := cSet[key(c)]; !ok {
			onlyGo = append(onlyGo, c)
		}
	}
	for _, c := range cCaps {
		if _, ok := goSet[key(c)]; !ok {
			onlyC = append(onlyC, c)
		}
	}
	return
}
