//go:build linux && cgo && treesitter_c_parity

package cgoharness

import (
	"flag"
	"math/rand"
	"os"
	"strconv"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// BenchmarkQ6Parse measures the same pinned witness against the locked fresh
// C runtime. Run one grammar per process with GTS_Q6_BENCH_LANGUAGE set, using
// scripts/run_randomized_benchmarks.sh for before/after comparisons. B/op and
// allocs/op cover Go allocations; measure native C memory separately with RSS.
func BenchmarkQ6Parse(b *testing.B) {
	selected := os.Getenv("GTS_Q6_BENCH_LANGUAGE")
	if selected == "" {
		b.Skip("set GTS_Q6_BENCH_LANGUAGE to one pinned cliff witness")
	}
	source, grammar := q6BenchmarkSource(b, selected)
	language := grammars.DetectLanguageByName(grammar).Language()
	cLanguage, err := COracleLanguage(grammar)
	if err != nil {
		b.Fatal(err)
	}
	routes := []struct {
		name      string
		candidate bool
	}{{"legacy", false}, {"compact", true}}
	seed := int64(1)
	if shuffle := flag.Lookup("test.shuffle"); shuffle != nil {
		if parsed, err := strconv.ParseInt(shuffle.Value.String(), 10, 64); err == nil {
			seed = parsed
		}
	}
	rand.New(rand.NewSource(seed)).Shuffle(len(routes), func(i, j int) { routes[i], routes[j] = routes[j], routes[i] })
	for _, route := range routes {
		b.Run(route.name, func(b *testing.B) {
			// Each cycle brackets two C samples with Go samples. Shuffle the
			// route order, while preserving the required Go-C-C-Go cycle.
			for _, sample := range []string{"GoFirst", "CFirst", "CSecond", "GoSecond"} {
				b.Run(sample, func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(source)))
					if sample == "CFirst" || sample == "CSecond" {
						parser := sitter.NewParser()
						defer parser.Close()
						if err := parser.SetLanguage(cLanguage); err != nil {
							b.Fatal(err)
						}
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							tree := parser.Parse(source, nil)
							if tree == nil {
								b.Fatal("C returned no tree")
							}
							tree.Close()
						}
						return
					}
					parser := gts.NewParser(language)
					parser.SetAdmissionCandidateRoute(route.candidate)
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						tree, err := parser.Parse(source)
						if err != nil {
							b.Fatal(err)
						}
						tree.Release()
					}
				})
			}
		})
	}
}

func q6BenchmarkSource(t testing.TB, selected string) ([]byte, string) {
	t.Helper()
	var source []byte
	grammar := selected
	if selected == "python_issue454" {
		grammar = "python"
		source = q6PythonIssue454Source()
	} else {
		var fixture *cliffFixture
		for i := range cliffFixtures {
			if cliffFixtures[i].Name == selected {
				fixture = &cliffFixtures[i]
				break
			}
		}
		if fixture == nil {
			t.Fatalf("unknown Q6 witness %q", selected)
		}
		source = loadCliffSource(t, *fixture)
		grammar = fixture.Grammar
	}
	return source, grammar
}

// TestQ6ParsePeakRSSProbe runs ten fresh parses in an isolated process.
// Invoke a compiled harness with /usr/bin/time -v, one grammar and route per
// process. Native C memory is outside Go's B/op accounting.
func TestQ6ParsePeakRSSProbe(t *testing.T) {
	name, route := os.Getenv("GTS_Q6_BENCH_LANGUAGE"), os.Getenv("GTS_Q6_MEMORY_ROUTE")
	if name == "" || route == "" {
		t.Skip("set one Q6 grammar and memory route")
	}
	if route != "C" && route != "legacy" && route != "compact" {
		t.Fatalf("unknown Q6 memory route %q", route)
	}
	source, grammar := q6BenchmarkSource(t, name)
	if route == "C" {
		language, err := COracleLanguage(grammar)
		if err != nil {
			t.Fatal(err)
		}
		parser := sitter.NewParser()
		defer parser.Close()
		if err := parser.SetLanguage(language); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 10; i++ {
			tree := parser.Parse(source, nil)
			if tree == nil {
				t.Fatal("no C tree")
			}
			tree.Close()
		}
	} else {
		parser := gts.NewParser(grammars.DetectLanguageByName(grammar).Language())
		parser.SetAdmissionCandidateRoute(route == "compact")
		for i := 0; i < 10; i++ {
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			tree.Release()
		}
	}
}
