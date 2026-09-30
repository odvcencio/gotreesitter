package gotreesitter_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/scannercert"
)

// TestStatelessScannerIncrementalCertification certifies a capability class,
// not a name-based runtime allowlist. Each scanner has a nil payload, empty
// serialization, and a Scan decision derived only from lookahead plus the
// parser-provided valid-symbol set. The fresh oracle still crosses every
// grammar independently so parser ownership remains part of admission.
func TestStatelessScannerIncrementalCertification(t *testing.T) {
	cases := []struct {
		name                 string
		lang                 func() *gts.Language
		prefix               string
		suffix               string
		line                 func(int) string
		minMacroReusePercent uint64
		minMacroReuseBytes   uint64
	}{
		{name: "cue", lang: grammars.CueLanguage, line: func(i int) string { return fmt.Sprintf("field_%06d: %d\n", i, i) }, minMacroReusePercent: 15},
		{name: "d", lang: grammars.DLanguage, line: func(i int) string { return fmt.Sprintf("int value_%06d = %d;\n", i, i) }, minMacroReusePercent: 15},
		{name: "elixir", lang: grammars.ElixirLanguage, line: func(i int) string { return fmt.Sprintf("value_%06d = %d\n", i, i) }, minMacroReuseBytes: 16},
		{name: "erlang", lang: grammars.ErlangLanguage, line: func(i int) string { return fmt.Sprintf("value_%06d() -> %d.\n", i, i) }, minMacroReusePercent: 25},
		{name: "gleam", lang: grammars.GleamLanguage, line: func(i int) string { return fmt.Sprintf("const value_%06d = \"%d\"\n", i, i) }, minMacroReusePercent: 3},
		{name: "move", lang: grammars.MoveLanguage, line: func(i int) string { return fmt.Sprintf("module 0x1::value_%06d { const VALUE: u64 = %d; }\n", i, i) }, minMacroReusePercent: 5},
		{name: "tcl", lang: grammars.TclLanguage, line: func(i int) string { return fmt.Sprintf("set value_%06d %d\n", i, i) }, minMacroReuseBytes: 32},
		{name: "wgsl", lang: grammars.WgslLanguage, line: func(i int) string { return fmt.Sprintf("/* value */ fn value_%06d() { let x: i32 = %d; }\n", i, i) }, minMacroReusePercent: 45},
		{name: "editorconfig", lang: grammars.EditorconfigLanguage, line: func(i int) string { return fmt.Sprintf("[value_%06d]\nkey = %d\n", i, i) }, minMacroReusePercent: 30},
		{name: "fennel", lang: grammars.FennelLanguage, line: func(i int) string { return fmt.Sprintf("(local value_%06d %d)\n", i, i) }, minMacroReusePercent: 85},
		{name: "fish", lang: grammars.FishLanguage, line: func(i int) string { return fmt.Sprintf("set value_%06d %d\n", i, i) }, minMacroReuseBytes: 32},
		{name: "gn", lang: grammars.GnLanguage, line: func(i int) string { return fmt.Sprintf("value_%06d = %d\n", i, i) }, minMacroReusePercent: 20},
		{name: "janet", lang: grammars.JanetLanguage, line: func(i int) string { return fmt.Sprintf("(def value_%06d %d)\n", i, i) }, minMacroReusePercent: 60},
		{name: "julia", lang: grammars.JuliaLanguage, line: func(i int) string { return fmt.Sprintf("value_%06d = %d\n", i, i) }, minMacroReusePercent: 90},
		{name: "less", lang: grammars.LessLanguage, line: func(i int) string { return fmt.Sprintf(".value_%06d { width: %dpx; }\n", i, i) }, minMacroReusePercent: 40},
		{name: "liquid", lang: grammars.LiquidLanguage, line: func(i int) string { return fmt.Sprintf("{%% assign value_%06d = %d %%}\n", i, i) }, minMacroReusePercent: 90},
		{name: "pkl", lang: grammars.PklLanguage, line: func(i int) string { return fmt.Sprintf("value_%06d = %d\n", i, i) }, minMacroReusePercent: 20},
		{name: "racket", lang: grammars.RacketLanguage, line: func(i int) string {
			prefix := ""
			if i == 0 {
				prefix = "#lang racket\n"
			}
			return fmt.Sprintf("%s(define value_%06d %d)\n", prefix, i, i)
		}, minMacroReusePercent: 90},
		{name: "tablegen", lang: grammars.TablegenLanguage, line: func(i int) string { return fmt.Sprintf("class Value_%06d;\n", i) }, minMacroReusePercent: 60},
		{name: "yuck", lang: grammars.YuckLanguage, line: func(i int) string { return fmt.Sprintf("(defvar value_%06d %d)\n", i, i) }, minMacroReusePercent: 35},
		{name: "comment", lang: grammars.CommentLanguage, line: func(i int) string { return fmt.Sprintf("TODO_%06d: value %d\n", i, i) }, minMacroReusePercent: 3},
		{name: "dhall", lang: grammars.DhallLanguage, prefix: "{ ", suffix: "final = 0 }\n", line: func(i int) string { return fmt.Sprintf("value_%06d = %d,\n", i, i) }, minMacroReusePercent: 60},
		{name: "dtd", lang: grammars.DtdLanguage, line: func(i int) string { return fmt.Sprintf("<!ELEMENT value_%06d (#PCDATA)>\n", i) }, minMacroReusePercent: 90},
		{name: "foam", lang: grammars.FoamLanguage, line: func(i int) string { return fmt.Sprintf("value_%06d %d;\n", i, i) }, minMacroReusePercent: 10},
		{name: "godot_resource", lang: grammars.GodotResourceLanguage, line: func(i int) string { return fmt.Sprintf("value_%06d = %d\n", i, i) }, minMacroReusePercent: 15},
		{name: "kconfig", lang: grammars.KconfigLanguage, line: func(i int) string { return fmt.Sprintf("config VALUE_%06d\n\tint\n\tdefault %d\n", i, i) }, minMacroReuseBytes: 16},
		{name: "odin", lang: grammars.OdinLanguage, line: func(i int) string {
			prefix := ""
			if i == 0 {
				prefix = "package main\n"
			}
			return fmt.Sprintf("%svalue_%06d :: %d\n", prefix, i, i)
		}, minMacroReusePercent: 10},
		{name: "ron", lang: grammars.RonLanguage, prefix: "(\n", suffix: ")\n", line: func(i int) string { return fmt.Sprintf("value_%06d: %d,\n", i, i) }, minMacroReusePercent: 70},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plans := []struct {
				name      string
				size      int
				positions []string
				classes   []statelessScannerEditClass
			}{
				{name: "4KiB", size: 4 << 10, positions: []string{"start", "middle", "end"}, classes: []statelessScannerEditClass{statelessScannerInsert, statelessScannerDelete, statelessScannerReplace}},
				{name: "137KiB", size: 137 << 10, positions: []string{"middle"}, classes: []statelessScannerEditClass{statelessScannerReplace}},
			}
			for _, plan := range plans {
				source, sites := makeStatelessScannerCertificationSource(plan.size, tc.prefix, tc.suffix, tc.line)
				for _, position := range plan.positions {
					var site int
					switch position {
					case "start":
						site = sites[1]
					case "middle":
						site = sites[len(sites)/2]
					case "end":
						site = sites[len(sites)-2]
					}
					for _, class := range plan.classes {
						t.Run(fmt.Sprintf("%s/%s/%s", plan.name, class, position), func(t *testing.T) {
							edited, edit := applyStatelessScannerCertificationEdit(source, site, class)
							profile := runStatelessScannerCertificationSample(t, tc.lang(), source, edited, edit)
							if plan.size >= 137<<10 && profile.ReusedBytes*100 < uint64(len(edited))*tc.minMacroReusePercent {
								t.Fatalf("stateless scanner reused only %d/%d bytes (<%d%%): %+v", profile.ReusedBytes, len(edited), tc.minMacroReusePercent, profile)
							}
							if plan.size >= 137<<10 && profile.ReusedBytes < tc.minMacroReuseBytes {
								t.Fatalf("stateless scanner reused only %d bytes (<%d): %+v", profile.ReusedBytes, tc.minMacroReuseBytes, profile)
							}
						})
					}
				}
			}
		})
	}
}

type statelessScannerEditClass uint8

const (
	statelessScannerInsert statelessScannerEditClass = iota
	statelessScannerDelete
	statelessScannerReplace
)

func (c statelessScannerEditClass) String() string {
	switch c {
	case statelessScannerInsert:
		return "insert"
	case statelessScannerDelete:
		return "delete"
	case statelessScannerReplace:
		return "replace"
	default:
		return "unknown"
	}
}

func makeStatelessScannerCertificationSource(target int, prefix, suffix string, line func(int) string) ([]byte, []int) {
	var source strings.Builder
	source.Grow(target + 128)
	source.WriteString(prefix)
	sites := make([]int, 0, target/24)
	for i := 0; source.Len() < target || len(sites) < 4; i++ {
		entry := line(i)
		underscore := strings.IndexByte(entry, '_')
		if underscore < 0 {
			panic("stateless scanner certification line lacks marker")
		}
		marker := underscore + 1
		sites = append(sites, source.Len()+marker)
		source.WriteString(entry)
	}
	source.WriteString(suffix)
	return []byte(source.String()), sites
}

func applyStatelessScannerCertificationEdit(source []byte, offset int, class statelessScannerEditClass) ([]byte, gts.InputEdit) {
	oldEnd := offset
	replacement := []byte{'8'}
	switch class {
	case statelessScannerDelete:
		oldEnd = offset + 1
		replacement = nil
	case statelessScannerReplace:
		oldEnd = offset + 1
		replacement = []byte("88")
	}
	edited := make([]byte, 0, len(source)-(oldEnd-offset)+len(replacement))
	edited = append(edited, source[:offset]...)
	edited = append(edited, replacement...)
	edited = append(edited, source[oldEnd:]...)
	return edited, gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(offset + len(replacement)),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, oldEnd),
		NewEndPoint: pointForOffset(edited, offset+len(replacement)),
	}
}

func runStatelessScannerCertificationSample(t *testing.T, lang *gts.Language, source, edited []byte, edit gts.InputEdit) gts.IncrementalParseProfile {
	t.Helper()
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	oldTree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	oldRoot := requireCompleteParse(t, oldTree, source, lang, "initial stateless-scanner certification")
	if oldRoot.HasError() {
		t.Fatalf("initial fixture produced ERROR: %s", oldRoot.SExpr(lang))
	}
	oldTree.Edit(edit)

	incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer incremental.Release()
	requireCompleteParse(t, incremental, edited, lang, "incremental stateless-scanner certification")
	if profile.ReuseUnsupported || !profile.OldTreeReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
		t.Fatalf("stateless scanner did not enter useful reuse: %+v", profile)
	}

	fresh, err := gts.NewParser(lang).Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer fresh.Release()
	requireCompleteParse(t, fresh, edited, lang, "fresh stateless-scanner certification")
	requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
	return profile
}

// TestExternalScannerCertification discovers scanner contracts through the
// registry. The implementation lives below the root layout ratchet.
func TestExternalScannerCertification(t *testing.T) {
	scannercert.Run(t, scannerCertificationLexerAPI())
}

func TestExternalScannerCertificationDetectsContractFaults(t *testing.T) {
	scannercert.RunContractFaults(t, scannerCertificationLexerAPI())
}

func scannerCertificationLexerAPI() scannercert.LexerAPI {
	return scannercert.LexerAPI{
		New:   gts.NewExternalScannerLexerForTest,
		Clone: gts.CloneExternalScannerLexerForTest,
		Input: gts.ExternalScannerInputForTest,
		Observe: func(lexer *gts.ExternalLexer) scannercert.LexerObservation {
			return scannercert.LexerObservation(gts.ObserveExternalScannerLexerForTest(lexer))
		},
		SerializationCapacity: gts.ExternalScannerSerializationCapacityForTest(),
	}
}

func TestCheckpointScannerReuseWitness(t *testing.T) {
	for _, tc := range []struct {
		name string
		lang func() *gts.Language
		line func(int) string
	}{
		{"bash", grammars.BashLanguage, func(i int) string { return fmt.Sprintf("echo value_%06d\n", i) }},
		{"blade", grammars.BladeLanguage, func(i int) string { return fmt.Sprintf("<div>value_%06d</div>\n", i) }},
		{"properties", grammars.PropertiesLanguage, func(i int) string { return fmt.Sprintf("value_%06d=%d\n", i, i) }},
		{"c_sharp", grammars.CSharpLanguage, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []int{4096, 137 << 10, 1 << 20} {
				t.Run(fmt.Sprint(size), func(t *testing.T) {
					var source []byte
					var sites []int
					if tc.line == nil {
						var err error
						source, _, err = benchfixtures.GeneratedSource(tc.name, size)
						if err != nil {
							t.Fatal(err)
						}
						sites = []int{strings.Index(string(source), "x0")}
					} else {
						source, sites = makeStatelessScannerCertificationSource(size, "", "", tc.line)
					}
					for _, route := range []bool{false, true} {
						t.Run(fmt.Sprintf("compact=%t", route), func(t *testing.T) {
							lang := tc.lang()
							p := gts.NewParser(lang)
							p.SetAdmissionCandidateRoute(route)
							old, err := p.Parse(source)
							if err != nil {
								t.Fatal(err)
							}
							defer old.Release()
							requireCompleteParse(t, old, source, lang, "old checkpoint witness")
							at := sites[len(sites)/2]
							edited := append([]byte(nil), source...)
							edited[at] = 'y'
							edit := gts.InputEdit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1), StartPoint: pointForOffset(source, at), OldEndPoint: pointForOffset(source, at+1), NewEndPoint: pointForOffset(source, at+1)}
							old.Edit(edit)
							next, profile, err := p.ParseIncrementalProfiled(edited, old)
							if err != nil {
								t.Fatal(err)
							}
							defer next.Release()
							requireCompleteParse(t, next, edited, lang, "incremental checkpoint witness")
							fresh, err := p.Parse(edited)
							if err != nil {
								t.Fatal(err)
							}
							defer fresh.Release()
							requireIncrementalDeepTreeMatchesFresh(t, next, fresh, lang)
							if tc.name == "properties" && (profile.ReusedBytes != uint64(len(source)) ||
								profile.TokenInvariantDependencyChecks != 1 || profile.TokensConsumed != 0 ||
								profile.NewNodesAllocated != 0 || profile.ReparseNanos != 0) {
								t.Fatalf("complete scanner substitution proof did not reuse the tree: %+v", profile)
							}
							t.Logf("COUNTERS bytes=%d profile=%+v", len(source), profile)
						})
					}
				})
			}
		})
	}
}

func TestPropertiesScannerASCIIProof(t *testing.T) {
	scanner := grammars.PropertiesLanguage().ExternalScanner
	classes := scanner.(gts.ASCIIEquivalenceExternalScanner)
	for _, initial := range []byte{0, 1} {
		for _, valid := range []bool{false, true} {
			for b := 1; b < 128; b++ {
				if classes.ExternalScannerASCIIEquivalenceClass(byte(b)) != classes.ExternalScannerASCIIEquivalenceClass('a') {
					t.Fatalf("byte %d lost nonzero class", b)
				}
				var observations [2]gts.ExternalScannerObservationForTest
				var outcomes [2]bool
				var snapshots [2]byte
				for i, ch := range []byte{'a', byte(b)} {
					payload := scanner.Create()
					scanner.Deserialize(payload, []byte{initial})
					lexer := gts.NewExternalScannerLexerForTest([]byte{ch}, 0)
					outcomes[i] = scanner.Scan(payload, lexer, []bool{valid})
					observations[i] = gts.ObserveExternalScannerLexerForTest(lexer)
					buf := make([]byte, 1)
					if n := scanner.Serialize(payload, buf); n != 1 {
						t.Fatalf("snapshot length=%d", n)
					}
					snapshots[i] = buf[0]
					scanner.Destroy(payload)
				}
				if outcomes[0] != outcomes[1] || observations[0] != observations[1] || snapshots[0] != snapshots[1] {
					t.Fatalf("byte=%d initial=%d valid=%t violates ASCII proof", b, initial, valid)
				}
			}
		}
	}
	for _, b := range []byte{0, 128, 255} {
		if classes.ExternalScannerASCIIEquivalenceClass(b) != 0 {
			t.Fatalf("byte %d admitted", b)
		}
	}
}
