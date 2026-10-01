//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"runtime"
	"strconv"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func graduationInputs(tb testing.TB) []ceilingInput {
	var inputs []ceilingInput
	for _, input := range ceilingInputs(tb) {
		if input.mode == "fresh" || input.mode == "byte" {
			inputs = append(inputs, input)
		}
	}
	return inputs
}

// Each shuffled seed measures Go-C-C-Go for both Go routes on identical
// sources. The two C passes use independent parsers, native input buffers,
// and the locked runtime. Initial parsing is outside the edit timer; edit,
// parse, hidden verification, and old-tree release are inside it. Compact
// declines retain their attempted work and legacy retry in the timer.
func BenchmarkCompactGraduation(b *testing.B) {
	inputs := graduationInputs(b)
	seed := int64(1)
	if value := flag.Lookup("test.shuffle"); value != nil {
		if n, err := strconv.ParseInt(value.Value.String(), 10, 64); err == nil {
			seed = n
		}
	}
	rand.New(rand.NewSource(seed)).Shuffle(len(inputs), func(i, j int) { inputs[i], inputs[j] = inputs[j], inputs[i] })
	for _, input := range inputs {
		b.Run(input.language+"/"+input.size+"/"+input.mode, func(b *testing.B) {
			order := []string{"legacy", "compact", "C", "C", "compact", "legacy"}
			if seed%2 == 0 {
				order = []string{"compact", "legacy", "C", "C", "legacy", "compact"}
			}
			for pass, engine := range order {
				b.Run(fmt.Sprintf("%s/pass%d", engine, pass), func(b *testing.B) {
					benchmarkCeilingCell(b, input, engine, true, nil)
				})
			}
		})
	}
}

// This gate emits every completed observation before reporting a failure.
// A failed compact admission is reported separately from tree equivalence:
// a correct legacy fallback cannot certify compact. Four alternating edits
// and the no-edit check run even when an earlier C comparison differs.
func TestCompactGraduationCorrectness(t *testing.T) {
	name := ceilingLanguage(t)
	lang := grammars.DetectLanguageByName(name).Language()
	cl, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity(name)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("GRADUATION_ORACLE %s", encoded)
	for _, input := range graduationInputs(t) {
		for _, engine := range []string{"legacy", "compact"} {
			t.Run(input.size+"/"+input.mode+"/"+engine, func(t *testing.T) {
				runtime.GC()
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(engine == "compact")
				cp := sitter.NewParser()
				defer cp.Close()
				if err := cp.SetLanguage(cl); err != nil {
					t.Fatal(err)
				}
				gts.ResetAdmissionCandidateCounters()
				tree, err := p.Parse(input.source[0])
				ceilingGoTree(t, tree, input.source[0], err)
				defer func() { tree.Release() }()
				served, declined := gts.AdmissionCandidateCounters()
				initialServed := engine == "legacy" || served == 1 && declined == 0
				initialReason := gts.AdmissionCandidateLastFallbackReason()
				steps := 1
				if input.mode == "byte" {
					steps = 4
				}
				for step := 0; step < steps; step++ {
					source := input.source[0]
					var profile gts.IncrementalParseProfile
					if input.mode == "byte" {
						direction := step % 2
						source = input.source[1-direction]
						tree.Edit(input.edit[direction])
						next, observed, err := p.ParseIncrementalProfiled(source, tree)
						ceilingGoTree(t, next, source, err)
						tree.Release()
						tree, profile = next, observed
					}
					freshParser := gts.NewParser(lang)
					freshParser.SetAdmissionCandidateRoute(engine == "compact")
					fresh, err := freshParser.Parse(source)
					ceilingGoTree(t, fresh, source, err)
					ct := cp.Parse(source, nil)
					if ct == nil {
						t.Fatal("locked C returned no tree")
					}
					got, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
					if err != nil {
						t.Fatal(err)
					}
					want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
					if err != nil {
						t.Fatal(err)
					}
					oracle, err := COracleDeepDigest(ct)
					if err != nil {
						t.Fatal(err)
					}
					freshC := want.SHA256 == oracle
					d8 := got.SHA256 == want.SHA256
					errorEqual := tree.RootNode().HasError() == ct.RootNode().HasError()
					clean := !tree.RootNode().HasError() && !ct.RootNode().HasError()
					requestedServed := initialServed && (input.mode == "fresh" || engine == "legacy" || tree.ParseRuntime().CompactIncrementalReuseRoute)
					fresh.Release()
					ct.Close()
					allocations := testing.AllocsPerRun(3, func() {
						same, err := p.ParseIncremental(source, tree)
						if err != nil || same == nil {
							t.Fatalf("no-edit reparse failed: %v", err)
						}
						if same != tree {
							t.Error("no-edit reparse changed tree identity")
						}
						same.Release()
					})
					row := map[string]any{
						"language": name, "size": input.size, "mode": input.mode, "engine": engine, "step": step,
						"source_bytes": len(source), "source_sha256": fmt.Sprintf("%x", sha256.Sum256(source)),
						"initial_served": initialServed, "initial_decline_reason": initialReason, "requested_served": requestedServed,
						"fresh_C_equal": freshC, "incremental_fresh_equal": d8, "has_error_equal": errorEqual, "clean": clean,
						"no_edit_allocations": allocations, "go_digest": got.SHA256, "fresh_digest": want.SHA256, "C_digest": oracle,
						"runtime": tree.ParseRuntime(), "profile": profile,
					}
					encoded, err := json.Marshal(row)
					if err != nil {
						t.Fatal(err)
					}
					t.Logf("GRADUATION_CORRECTNESS %s", encoded)
					if !freshC || !d8 || !errorEqual || !clean || !requestedServed || allocations != 0 {
						t.Errorf("step=%d fresh-C=%t incremental-fresh=%t HasError=%t clean=%t served=%t no-edit-allocs=%g", step, freshC, d8, errorEqual, clean, requestedServed, allocations)
					}
				}
			})
		}
	}
}
