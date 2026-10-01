//go:build cgo && treesitter_c_parity

package cgoharness

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
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// BenchmarkMergeElectionComplete times the public parse and edit operations,
// including Tree.Edit, result construction, and result release. Old-tree setup
// is outside the edit timer on both runtimes. C prepares each old tree by
// cloning an unchanged base tree; Go reparses to preserve its first-edit
// dependency fold. Each phase runs Go-C-C-Go.
func BenchmarkMergeElectionComplete(b *testing.B) {
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

	cl, err := COracleLanguage(name)
	if err != nil {
		b.Fatal(err)
	}
	for _, phase := range []string{"full", "edit"} {
		b.Run(name+"/"+phase, func(b *testing.B) {
			for _, route := range []string{"go_first", "c_first", "c_second", "go_second"} {
				b.Run(route, func(b *testing.B) {
					gp := gts.NewParser(lang)
					gp.SetAdmissionCandidateRoute(false)
					cp := sitter.NewParser()
					defer cp.Close()
					if err := cp.SetLanguage(cl); err != nil {
						b.Fatal(err)
					}
					ce := sitter.InputEdit{StartByte: uint(step.Edit.StartByte), OldEndByte: uint(step.Edit.OldEndByte), NewEndByte: uint(step.Edit.NewEndByte), StartPosition: sitter.Point{Row: uint(step.Edit.StartPoint.Row), Column: uint(step.Edit.StartPoint.Column)}, OldEndPosition: sitter.Point{Row: uint(step.Edit.OldEndPoint.Row), Column: uint(step.Edit.OldEndPoint.Column)}, NewEndPosition: sitter.Point{Row: uint(step.Edit.NewEndPoint.Row), Column: uint(step.Edit.NewEndPoint.Column)}}
					checkSource := source
					if phase == "edit" {
						checkSource = step.Source
					}
					goCheck, err := gp.Parse(checkSource)
					if err != nil {
						b.Fatal(err)
					}
					cCheck := cp.Parse(checkSource, nil)
					if cCheck == nil || goCheck.RootNode() == nil || goCheck.ParseRuntime().StopReason != gts.ParseStopAccepted || goCheck.RootNode().EndByte() != uint32(len(checkSource)) || goCheck.RootNode().HasError() != cCheck.RootNode().HasError() {
						b.Fatal("Go/C completion, coverage, or error-state disagreement")
					}
					goDigest, err := benchfixtures.InspectGoTree(goCheck.RootNode(), lang)
					if err != nil {
						b.Fatal(err)
					}
					cDigest, err := COracleDeepDigest(cCheck)
					if err != nil {
						b.Fatal(err)
					}
					b.Logf("runtime=%s@%s phase=%s canonical_fresh_match=%t", COracleRuntimeVersion, COracleRuntimeCommit, phase, goDigest.SHA256 == cDigest)
					goCheck.Release()
					cCheck.Close()
					var cBase *sitter.Tree
					if phase == "edit" && route[:2] == "c_" {
						cBase = cp.Parse(source, nil)
						if cBase == nil {
							b.Fatal("C base parse returned nil")
						}
						defer cBase.Close()
					}
					b.SetBytes(int64(len(checkSource)))
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if route[:2] == "go" {
							if phase == "full" {
								tree, err := gp.Parse(source)
								if err != nil {
									b.Fatal(err)
								}
								tree.Release()
							} else {
								b.StopTimer()
								old, err := gp.Parse(source)
								if err != nil {
									b.Fatal(err)
								}
								b.StartTimer()
								old.Edit(step.Edit)
								tree, err := gp.ParseIncremental(step.Source, old)
								if err != nil {
									b.Fatal(err)
								}
								tree.Release()
								b.StopTimer()
								old.Release()
								b.StartTimer()
							}
						} else {
							if phase == "full" {
								tree := cp.Parse(source, nil)
								if tree == nil {
									b.Fatal("C parse returned nil")
								}
								tree.Close()
							} else {
								b.StopTimer()
								old := cBase.Clone()
								if old == nil {
									b.Fatal("C parse returned nil")
								}
								b.StartTimer()
								old.Edit(&ce)
								tree := cp.Parse(step.Source, old)
								if tree == nil {
									b.Fatal("C edit returned nil")
								}
								tree.Close()
								b.StopTimer()
								old.Close()
								b.StartTimer()
							}
						}
					}
				})
			}
		})
	}
}
