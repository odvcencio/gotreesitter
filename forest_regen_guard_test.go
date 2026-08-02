package gotreesitter_test

import (
	"bytes"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// forestRegenGuardCorpus generates N syntactically valid, repeated top-level
// items for a language known to dispatch to the GSS-forest GLR fast path by
// default (gotreesitter.LanguageWantsForest). This is the work-floor
// regression class found in the 2026-08-02 javascript table-fork regression
// (hypha spore spore.2026-08-02.pine.table-fork-regression): a table regen
// can change forest-engine coverage (not tree correctness -- production
// silently absorbs the decline) without any parity gate noticing, because
// parity floors compare trees, and forest and production converge on the
// same tree once production runs. This corpus is deliberately trivial
// (single-construct repetition, no internal ambiguity of its own) so a
// decline here isolates a forest-coverage regression from any other cause.
var forestRegenGuardCorpus = map[string]func(n int) []byte{
	"javascript": func(n int) []byte {
		var b bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "let a%d = %d;\n", i, i)
		}
		return b.Bytes()
	},
	"json5": func(n int) []byte {
		var b bytes.Buffer
		b.WriteByte('[')
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "%d", i)
		}
		b.WriteByte(']')
		return b.Bytes()
	},
	"gitignore": func(n int) []byte {
		var b bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "file%d.o\n", i)
		}
		return b.Bytes()
	},
	"gitattributes": func(n int) []byte {
		var b bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "file%d.txt text\n", i)
		}
		return b.Bytes()
	},
	"nix": func(n int) []byte {
		var b bytes.Buffer
		b.WriteString("{\n")
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "  a%d = %d;\n", i, i)
		}
		b.WriteString("}\n")
		return b.Bytes()
	},
	"squirrel": func(n int) []byte {
		var b bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "local a%d = %d;\n", i, i)
		}
		return b.Bytes()
	},
	"prisma": func(n int) []byte {
		var b bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "model M%d {\n  id Int @id\n}\n\n", i)
		}
		return b.Bytes()
	},
	"arduino": func(n int) []byte {
		var b bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "void f%d() {}\n", i)
		}
		return b.Bytes()
	},
	"awk": func(n int) []byte {
		var b bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "{ print %d }\n", i)
		}
		return b.Bytes()
	},
	"kdl": func(n int) []byte {
		var b bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "node%d %d\n", i, i)
		}
		return b.Bytes()
	},
	"uxntal": func(n int) []byte {
		var b bytes.Buffer
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "@label%d\n", i)
		}
		return b.Bytes()
	},
}

// TestForestRegenGuardRepeatedTopLevelItems is the work-floor regen guard: for
// every forest-default language, parsing N=1..20 repeated top-level items via
// Parser.Parse (the real dispatch entry point, not the diagnostic-only
// ParseForestExperimental) must keep ROUTING through the GSS-forest fast path
// -- Tree.UsedForestFastPath() true, not merely "the forest engine can handle
// this input in isolation" -- and Parse's own resulting MaxStacksSeen on the
// same corpus must stay 0. It runs against whatever blobs are currently
// embedded (grammars/grammar_blobs), so it is a plain default-on test today
// and automatically re-checks the new blob the next time a table-resync PR
// swaps one in.
//
// A generator is validated (HasError=false at a small N) before the sweep
// trusts it, but an established generator's baseline going dirty is treated
// as a hard failure, not a skip: once a language is in this map, silently
// skipping on a broken baseline would disable the guard on exactly the
// regression class it exists to catch (a table resync corrupting even the
// trivial corpus). Missing an entry entirely (a forest-default language with
// no generator yet) still skips, since no coverage claim is being made there.
func TestForestRegenGuardRepeatedTopLevelItems(t *testing.T) {
	const maxN = 20
	for _, entry := range grammars.AllLanguages() {
		lang := entry.Language()
		if !gotreesitter.LanguageWantsForest(lang) {
			continue
		}
		name := entry.Name
		t.Run(name, func(t *testing.T) {
			gen, ok := forestRegenGuardCorpus[name]
			if !ok {
				t.Skipf("forest-regen-guard: no repeated-top-level-item generator for forest-default language %q; add one to forestRegenGuardCorpus", name)
			}

			baseline := gen(2)
			baselineParser := gotreesitter.NewParser(lang)
			baselineTree, err := baselineParser.Parse(baseline)
			if err != nil || baselineTree == nil || baselineTree.RootNode() == nil || baselineTree.RootNode().HasError() {
				t.Fatalf("forest-regen-guard: generator baseline for %q is not clean (err=%v); either the fixture needs authoring or a table regen broke the trivial corpus itself -- the exact regression this guard exists to catch", name, err)
			}
			baselineTree.Release()

			for n := 1; n <= maxN; n++ {
				src := gen(n)

				routedParser := gotreesitter.NewParser(lang)
				routedTree, err := routedParser.Parse(src)
				if err != nil {
					t.Fatalf("N=%d: Parse failed: %v", n, err)
				}
				if !routedTree.UsedForestFastPath() {
					diagParser := gotreesitter.NewParser(lang)
					_, forestOK := diagParser.ParseForestExperimental(src)
					offset, sym, reason, states := diagParser.ForestDeclineInfo()
					routedTree.Release()
					t.Fatalf("N=%d: Parse did not route through the forest fast path (forest engine standalone ok=%v; decline offset=%d sym=%d reason=%q states=%v); a table regen regressed forest coverage on a trivial repeated-item corpus",
						n, forestOK, offset, sym, reason, states)
				}
				if maxStacks := routedTree.ParseRuntime().MaxStacksSeen; maxStacks != 0 {
					routedTree.Release()
					t.Fatalf("N=%d: MaxStacksSeen=%d, want 0 on this trivial repeated-item corpus (genuine multi-stack fork even while forest-routed)", n, maxStacks)
				}
				routedTree.Release()
			}
		})
	}
}
