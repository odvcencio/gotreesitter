//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Run paired Go-C-C-Go cycles on the issue's locked real input. Edit timing
// includes Tree.Edit and ParseIncremental; copying the unchanged starting tree
// and releasing the two trees are outside the timed region. errors/op records
// the old bug when this benchmark is run on the baseline revision.
func BenchmarkAWKIssue1358(b *testing.B) {
	source, err := os.ReadFile("../internal/benchfixtures/testdata/real/awk")
	if err != nil {
		b.Fatal(err)
	}
	step := benchfixtures.EditingSession(source)[0]
	language := grammars.AwkLanguage()
	cLanguage, err := COracleLanguage("awk")
	if err != nil {
		b.Fatal(err)
	}
	for _, mode := range []string{"Full", "Insert", "NoEdit"} {
		b.Run(mode, func(b *testing.B) {
			for _, engine := range []string{"GoFirst", "CFirst", "CSecond", "GoSecond"} {
				b.Run(engine, func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(source)))
					if engine == "GoFirst" || engine == "GoSecond" {
						parser := gts.NewParser(language)
						initial, err := parser.Parse(source)
						if err != nil {
							b.Fatal(err)
						}
						defer initial.Release()
						errors := 0
						b.ResetTimer()
						for i := 0; i < b.N; i++ {
							var tree *gts.Tree
							switch mode {
							case "Full":
								tree, err = parser.Parse(source)
							case "Insert":
								b.StopTimer()
								old := initial.Copy()
								b.StartTimer()
								old.Edit(step.Edit)
								tree, err = parser.ParseIncremental(step.Source, old)
								b.StopTimer()
								old.Release()
							case "NoEdit":
								tree, err = parser.ParseIncremental(source, initial)
							}
							if err != nil || tree == nil {
								b.Fatalf("parse: %v", err)
							}
							if tree.RootNode().HasError() {
								errors++
							}
							tree.Release()
							if mode == "Insert" {
								b.StartTimer()
							}
						}
						b.StopTimer()
						b.ReportMetric(float64(errors)/float64(b.N), "errors/op")
						return
					}
					parser := sitter.NewParser()
					defer parser.Close()
					if err := parser.SetLanguage(cLanguage); err != nil {
						b.Fatal(err)
					}
					initial := parser.Parse(source, nil)
					if initial == nil {
						b.Fatal("C initial parse returned no tree")
					}
					defer initial.Close()
					edit := realCorpusCInputEdit(step.Edit)
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						var tree *sitter.Tree
						switch mode {
						case "Full":
							tree = parser.Parse(source, nil)
						case "Insert":
							b.StopTimer()
							old := initial.Clone()
							b.StartTimer()
							old.Edit(&edit)
							tree = parser.Parse(step.Source, old)
							b.StopTimer()
							old.Close()
						case "NoEdit":
							tree = parser.Parse(source, initial)
						}
						if tree == nil {
							b.Fatal("C parse returned no tree")
						}
						tree.Close()
						if mode == "Insert" {
							b.StartTimer()
						}
					}
					b.StopTimer()
				})
			}
		})
	}
}
