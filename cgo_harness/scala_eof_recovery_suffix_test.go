//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestScalaEOFRecoveryNativeVersionTrace(t *testing.T) {
	cLang, err := ParityCLanguage("scala")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	var trace []string
	cParser.SetLogger(func(_ sitter.LogType, message string) {
		if strings.HasPrefix(message, "process version:") || strings.HasPrefix(message, "recover_with_missing") || message == "detect_error" || message == "condense" {
			trace = append(trace, message)
		}
	})
	tree := cParser.Parse([]byte("((y)->"), nil)
	if tree == nil {
		t.Fatal("locked C returned no tree")
	}
	defer tree.Close()
	start := -1
	for i, line := range trace {
		if line == "recover_with_missing symbol:), state:5364" {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("locked C did not insert the expected missing parenthesis: %v", trace)
	}
	var got []string
	for _, line := range trace[start:] {
		if strings.HasPrefix(line, "process version:") {
			got = append(got, line)
		}
	}
	want := []string{
		"process version:1, version_count:8, state:5364, row:0, col:6",
		"process version:2, version_count:8, state:5381, row:0, col:6",
		"process version:3, version_count:8, state:4304, row:0, col:6",
		"process version:4, version_count:8, state:5952, row:0, col:6",
		"process version:5, version_count:8, state:12937, row:0, col:6",
		"process version:6, version_count:8, state:14397, row:0, col:6",
		"process version:7, version_count:8, state:1, row:0, col:6",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("locked C physical version order:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	t.Logf("locked C retained eight versions; later states: 5364, 5381, 4304, 5952, 12937, 14397, 1")
}

// TestScalaEOFRecoverySuffixDifferential checks 57 recovery continuations
// against the locked C parser. Keep this comparison independent of Go recovery.
func TestScalaEOFRecoverySuffixDifferential(t *testing.T) {
	cLang, err := ParityCLanguage("scala")
	if err != nil {
		t.Fatal(err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	goParser := gotreesitter.NewParser(grammars.ScalaLanguage())
	goParser.SetAdmissionCandidateRoute(false)
	prefixes := []string{"((y)->", "(y;", "((y)->; "}
	suffixes := []string{"", " ", "\n", ")", ");", ";", "x", " x", "\n)", "}", "\n}", "; ", "\n;", ") ", ")\n", "  ", "\n\n", "\t", ";\n"}
	baseline, err := os.ReadFile("testdata/scala_eof_recovery_production_baseline.tsv")
	if err != nil {
		t.Fatal(err)
	}
	baselineRows := strings.Split(strings.TrimSuffix(string(baseline), "\n"), "\n")
	if len(baselineRows) != 57 {
		t.Fatalf("production baseline has %d rows, want 57", len(baselineRows))
	}
	knownGaps := map[string]bool{
		"((y)->\n;":   true,
		"((y)->; ;":   true,
		"((y)->; }":   true,
		"((y)->; \n}": true,
		"((y)->; ; ":  true,
		"((y)->; \n;": true,
		"((y)->; ;\n": true,
	}
	count, exact, baselineGaps := 0, 0, 0
	var maxCandidateAttempts, maxMissingTrials, ceilingHits, eofFallbacks uint64
	var maxStacks int
	var maxArenaBytes, maxScratchBytes int64
	for _, prefix := range prefixes {
		for _, suffix := range suffixes {
			source := []byte(prefix + suffix)
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatalf("locked C returned no root for %q", source)
			}
			goTree, err := goParser.Parse(source)
			if err != nil {
				cTree.Close()
				t.Fatalf("Go parse %q: %v", source, err)
			}
			fields := strings.Split(baselineRows[count], "\t")
			if len(fields) != 4 {
				t.Fatalf("baseline row %d has %d fields, want four", count, len(fields))
			}
			baselineSource, err := strconv.Unquote(fields[0])
			if err != nil || baselineSource != string(source) {
				t.Fatalf("baseline row %d source = %q, error %v; want %q", count, baselineSource, err, source)
			}
			inspection, err := benchfixtures.InspectGoTree(goTree.RootNode(), grammars.ScalaLanguage())
			if err != nil {
				t.Fatalf("inspect production tree %q: %v", source, err)
			}
			if inspection.SHA256 != fields[1] || goTree.RootNode().Type(grammars.ScalaLanguage()) != fields[2] || strconv.FormatBool(goTree.RootNode().HasError()) != fields[3] {
				t.Errorf("source %q: production tree changed from main: digest=%s type=%s error=%t; baseline=%v", source, inspection.SHA256, goTree.RootNode().Type(grammars.ScalaLanguage()), goTree.RootNode().HasError(), fields[1:])
			}
			runtime := goTree.ParseRuntime()
			if runtime.CRecoverReductionCandidateAttemptsPeak > maxCandidateAttempts {
				maxCandidateAttempts = runtime.CRecoverReductionCandidateAttemptsPeak
			}
			if runtime.CRecoverMissingTokenTrialAttemptsPeak > maxMissingTrials {
				maxMissingTrials = runtime.CRecoverMissingTokenTrialAttemptsPeak
			}
			if runtime.MaxStacksSeen > maxStacks {
				maxStacks = runtime.MaxStacksSeen
			}
			if runtime.ArenaBytesAllocated > maxArenaBytes {
				maxArenaBytes = runtime.ArenaBytesAllocated
			}
			if runtime.ScratchBytesAllocated > maxScratchBytes {
				maxScratchBytes = runtime.ScratchBytesAllocated
			}
			ceilingHits += runtime.CRecoverReductionCandidateCeilingHits + runtime.CRecoverMissingTokenCeilingHits
			eofFallbacks += runtime.CRecoverEOFFallbacks
			if diff := FirstDivergenceDumpV1(goTree.RootNode(), grammars.ScalaLanguage(), cTree.RootNode()); diff != nil {
				if !knownGaps[string(source)] || diff.Path != "/compilation_unit" || diff.Category != "type" || diff.CValue != "ERROR" {
					t.Errorf("source %q: new tree divergence: %+v", source, *diff)
				} else {
					if !goTree.RootNode().HasError() || !cTree.RootNode().HasError() {
						t.Errorf("source %q: known root gap lost an error flag", source)
					}
					baselineGaps++
					t.Logf("known main gap %q: %s versus %s", source, diff.GoValue, diff.CValue)
				}
			} else if diff := firstLockedCTreeFlagDivergence(goTree.RootNode(), grammars.ScalaLanguage(), cTree.RootNode(), "/"+goTree.RootNode().Type(grammars.ScalaLanguage())); diff != nil {
				t.Errorf("source %q: flag divergence: %v", source, diff)
			} else {
				exact++
			}
			count++
			goTree.Release()
			cTree.Close()
		}
	}
	t.Logf("Scala EOF suffix differential: %d cases, %d exact, %d unchanged main gaps", count, exact, baselineGaps)
	t.Logf("Scala recovery counters: candidate_peak=%d missing_trial_peak=%d max_stacks=%d max_arena_bytes=%d max_scratch_bytes=%d work_ceiling_hits=%d eof_fallbacks=%d", maxCandidateAttempts, maxMissingTrials, maxStacks, maxArenaBytes, maxScratchBytes, ceilingHits, eofFallbacks)
	if count != 57 || exact != 50 || baselineGaps != 7 {
		t.Fatalf("Scala suffix table = %d/%d/%d, want 57/50/7", count, exact, baselineGaps)
	}
	if eofFallbacks != 0 {
		t.Fatalf("Scala EOF recovery used %d exact-row fallbacks", eofFallbacks)
	}
}
