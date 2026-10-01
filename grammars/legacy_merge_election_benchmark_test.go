package grammars_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Run one grammar per process with GTS_LEGACY_ELECTION_BENCH_LANG.
// The input and first edit are the same authenticated sample used by the
// deterministic counter ledger. Correctness is gated separately against C.
func BenchmarkLegacyMergeElection(b *testing.B) {
	name := os.Getenv("GTS_LEGACY_ELECTION_BENCH_LANG")
	if name == "" {
		b.Skip("set GTS_LEGACY_ELECTION_BENCH_LANG")
	}
	data, err := os.ReadFile("../internal/benchfixtures/real_corpus.json")
	if err != nil {
		b.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			Language string `json:"language"`
			Path     string `json:"committed_path"`
			SHA      string `json:"sha256"`
			Role     string `json:"role"`
		} `json:"entries"`
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		b.Fatal(err)
	}
	var source []byte
	for _, entry := range manifest.Entries {
		if entry.Language != name || entry.Role != "sample" {
			continue
		}
		source, err = os.ReadFile(filepath.Join("..", "internal", "benchfixtures", entry.Path))
		if err != nil {
			b.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(source)) != entry.SHA {
			b.Fatal("fixture digest changed")
		}
		break
	}
	if len(source) == 0 {
		b.Fatalf("no committed sample for %s", name)
	}
	entry := grammars.DetectLanguageByName(name)
	lang := entry.Language()
	if lang == nil {
		b.Fatal("missing grammar")
	}
	step := benchfixtures.EditingSession(source)[0]
	b.Run(name, func(b *testing.B) {
		b.Run("full", func(b *testing.B) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree, err := p.Parse(source)
				if err != nil || tree == nil {
					b.Fatalf("parse: %v", err)
				}
				tree.Release()
			}
		})
		b.Run("edit", func(b *testing.B) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			b.SetBytes(int64(len(step.Source)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				old, err := p.Parse(source)
				if err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				old.Edit(step.Edit)
				tree, err := p.ParseIncremental(step.Source, old)
				if err != nil || tree == nil {
					b.Fatalf("incremental parse: %v", err)
				}
				tree.Release()
				b.StopTimer()
				old.Release()
				b.StartTimer()
			}
		})
		b.Run("no_edit", func(b *testing.B) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(false)
			old, err := p.Parse(source)
			if err != nil {
				b.Fatal(err)
			}
			defer old.Release()
			b.SetBytes(int64(len(source)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tree, err := p.ParseIncremental(source, old)
				if err != nil || tree == nil {
					b.Fatalf("no-edit parse: %v", err)
				}
				tree.Release()
			}
		})
	})
}
