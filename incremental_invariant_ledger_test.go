package gotreesitter_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type invariantLedgerEntry struct {
	Steps      int `json:"steps"`
	Production int `json:"production"`
	Compact    int `json:"compact"`
	// The design's independent probe used different inputs. Keep its count for context.
	ReportedBaseline int `json:"reported_baseline,omitempty"`
}

type invariantLedger struct {
	Version  int                             `json:"version"`
	Sessions map[string]invariantLedgerEntry `json:"sessions"`
	Probes   map[string]invariantLedgerEntry `json:"probes"`
}

func readInvariantLedger(t *testing.T) invariantLedger {
	t.Helper()
	data, err := os.ReadFile("testdata/incremental_invariant_ledger.json")
	if err != nil {
		t.Fatal(err)
	}
	var ledger invariantLedger
	if err := json.Unmarshal(data, &ledger); err != nil {
		t.Fatal(err)
	}
	if ledger.Version != 1 {
		t.Fatalf("invariant ledger version = %d, want 1", ledger.Version)
	}
	return ledger
}

func checkInvariantLedger(t *testing.T, section map[string]invariantLedgerEntry, name string, compact bool, steps, mismatches int) {
	t.Helper()
	entry, ok := section[name]
	if !ok || entry.Steps != steps {
		t.Fatalf("ledger entry %q: found=%t steps=%d, want %d", name, ok, entry.Steps, steps)
	}
	limit := entry.Production
	if compact {
		limit = entry.Compact
	}
	if limit < 0 || mismatches > limit {
		t.Fatalf("invariant mismatches = %d, ledger limit = %d", mismatches, limit)
	}
	if mismatches < limit {
		t.Logf("lower the invariant ledger for %s: %d -> %d", name, limit, mismatches)
	}
	t.Logf("invariant mismatches: %d/%d", mismatches, steps)
}

func invariantLedgerStep(t *testing.T, parser *gts.Parser, lang *gts.Language, old *gts.Tree, before, after []byte) (*gts.Tree, bool) {
	t.Helper()
	old.Edit(issue454InputEdit(before, after))
	next, err := parser.ParseIncremental(after, old)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := parser.Parse(after)
	if err != nil {
		next.Release()
		t.Fatal(err)
	}
	defer fresh.Release()
	root := next.RootNode()
	if root == nil {
		next.Release()
		t.Fatal("incremental parse returned no root")
	}
	if root.IsError() && !root.HasError() {
		t.Error("ERROR root reports HasError false")
	}
	if root.StartByte() != 0 || root.EndByte() != uint32(len(after)) {
		if next.ParseStopReason() == gts.ParseStopAccepted || next.ParseStopReason() == gts.ParseStopNone {
			t.Errorf("root covers %d..%d of %d bytes with stop reason %s; fresh end=%d", root.StartByte(), root.EndByte(), len(after), next.ParseStopReason(), fresh.RootNode().EndByte())
		}
	}
	difference := issue454FirstDivergence(lang, fresh.RootNode(), root)
	if difference != nil {
		t.Logf("mismatch at %d: %v; incremental error=%t fresh error=%t", issue454InputEdit(before, after).StartByte, difference, root.HasError(), fresh.RootNode().HasError())
	}
	return next, difference != nil
}

func TestIncrementalInvariantProbeLedger(t *testing.T) {
	ledger := readInvariantLedger(t)
	fixtures := []struct {
		name   string
		lang   *gts.Language
		source []byte
		steps  int
	}{
		{"javascript", grammars.JavascriptLanguage(), issue454JS(), 96},
		{"toml", grammars.TomlLanguage(), issue454Toml(), 73},
		{"python", grammars.PythonLanguage(), invariantProbeSource("python"), 108},
		{"go", grammars.GoLanguage(), invariantProbeSource("go"), 136},
		{"typescript", grammars.TypescriptLanguage(), invariantProbeSource("typescript"), 96},
	}
	if len(ledger.Probes) != len(fixtures) {
		t.Fatalf("probe ledger has %d entries, want %d", len(ledger.Probes), len(fixtures))
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gts.NewParser(fixture.lang)
					parser.SetAdmissionCandidateRoute(compact)
					old, err := parser.Parse(fixture.source)
					if err != nil {
						t.Fatal(err)
					}
					defer func() { old.Release() }()
					source := fixture.source
					seed := uint32(1)
					mismatches := 0
					for step := 0; step < fixture.steps; step++ {
						seed = seed*1664525 + 1013904223
						at := int(seed % uint32(len(source)))
						nextSource := append([]byte{}, source...)
						switch step % 3 {
						case 0:
							nextSource = append(nextSource[:at], append([]byte{'"'}, nextSource[at:]...)...)
						case 1:
							nextSource = append(nextSource[:at], nextSource[at+1:]...)
						case 2:
							nextSource[at] = '/'
						}
						next, mismatch := invariantLedgerStep(t, parser, fixture.lang, old, source, nextSource)
						if mismatch {
							t.Logf("step=%d oldError=%t", step, old.RootNode().HasError())
							mismatches++
						}
						old.Release()
						old, source = next, nextSource
					}
					checkInvariantLedger(t, ledger.Probes, fixture.name, compact, fixture.steps, mismatches)
				})
			}
		})
	}
}

func invariantProbeSource(name string) []byte {
	var out strings.Builder
	switch name {
	case "go":
		out.WriteString("package session\n\n")
		for i := 0; i < 30; i++ {
			fmt.Fprintf(&out, "func f%d(a int) int { return a + %d }\n", i, i)
		}
	case "python":
		for i := 0; i < 30; i++ {
			fmt.Fprintf(&out, "def f%d(a):\n    return a + %d\n\n", i, i)
		}
	case "typescript":
		for i := 0; i < 30; i++ {
			fmt.Fprintf(&out, "function f%d(a: number): number { return a + %d; }\n", i, i)
		}
	}
	return []byte(out.String())
}

func TestIncrementalNoEditAllocations(t *testing.T) {
	fixtures := append(issue454SessionFixtures(), issue454SessionFixture{
		name: "python", lang: grammars.PythonLanguage(), source: invariantProbeSource("python"),
	})
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
					parser := gts.NewParser(fixture.lang)
					parser.SetAdmissionCandidateRoute(compact)
					old, err := parser.Parse(fixture.source)
					if err != nil {
						t.Fatal(err)
					}
					defer old.Release()
					allocations := testing.AllocsPerRun(100, func() {
						next, err := parser.ParseIncremental(fixture.source, old)
						if err != nil {
							panic(err)
						}
						next.Release()
					})
					if allocations != 0 {
						t.Fatalf("no-edit reparse allocated %.2f times per run", allocations)
					}
				})
			}
		})
	}
}
