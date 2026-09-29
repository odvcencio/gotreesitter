//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"sync"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestAdmissionRunnerPoolConcurrentScannerLanguages(t *testing.T) {
	for _, fixture := range []struct{ language, source string }{
		{"go", "package main\nfunc main() {}\n"},
		{"python", "def f(x):\n    return x + 1\n"},
		{"html", "<p>hello</p>\n"},
		{"markdown", "# title\n\nbody\n"},
		{"elixir", "defmodule M do\n  def f(x), do: x + 1\nend\n"},
	} {
		t.Run(fixture.language, func(t *testing.T) {
			lang := grammars.DetectLanguageByName(fixture.language).Language()
			source := []byte(fixture.source)
			var workers sync.WaitGroup
			for worker := 0; worker < 8; worker++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					for attempt := 0; attempt < 10; attempt++ {
						parser := gts.NewParser(lang)
						parser.SetAdmissionCandidateRoute(true)
						tree, err := parser.Parse(source)
						if err != nil || tree == nil {
							t.Errorf("concurrent parse: %v", err)
							return
						}
						if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint32(len(source)) {
							t.Error("concurrent parser returned an invalid tree")
						}
						tree.Release()
					}
				}()
			}
			workers.Wait()
		})
	}
}
