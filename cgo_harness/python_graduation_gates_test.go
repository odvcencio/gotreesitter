//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"bytes"
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

// Unlike the certification sweep, this gate also compares declined compact
// attempts' public fallback output with C. Corpus inputs remain external.
func TestPythonGraduationCorpusOutput(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("corpus_real", "python"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("Python corpus unavailable: %v", err)
	}
	lang := grammars.PythonLanguage()
	cl, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	files, served, declined, different := 0, 0, 0, 0
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		source, err := os.ReadFile(filepath.Join("corpus_real", "python", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files++
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(true)
		gts.ResetAdmissionCandidateCounters()
		tree, err := p.Parse(source)
		ceilingGoTree(t, tree, source, err)
		ct := cp.Parse(source, nil)
		if ct == nil {
			t.Fatal("locked C returned no tree")
		}
		actual, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
		if err != nil {
			t.Fatal(err)
		}
		want, err := COracleDeepDigest(ct)
		if err != nil {
			t.Fatal(err)
		}
		accepted, fallback := gts.AdmissionCandidateCounters()
		if accepted != 0 {
			served++
		} else {
			declined++
		}
		equal := actual.SHA256 == want
		if !equal {
			different++
			t.Errorf("file=%s compact-served=%d Go=%s C=%s", entry.Name(), accepted, actual.SHA256, want)
		}
		row, err := json.Marshal(map[string]any{"file": entry.Name(), "sha256": fmt.Sprintf("%x", sha256.Sum256(source)), "bytes": len(source), "served": accepted, "declined": fallback, "reason": gts.AdmissionCandidateLastFallbackReason(), "fresh_C_equal": equal, "operation_work": tree.ParseRuntime().OperationWork.Total})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("PYTHON_CORPUS_CHECK %s", row)
		tree.Release()
		ct.Close()
	}
	t.Logf("PYTHON_CORPUS_TOTAL files=%d served=%d declined=%d different=%d", files, served, declined, different)
}

// Sixteen separated identifier sites cover insertion, deletion and replacement.
// Restoring each edit makes a continuous 96-step session, exceeding the
// required 72 steps, with C parity and all public invariants at every step.
func TestPythonGraduationEditSession(t *testing.T) {
	if ceilingLanguage(t) != "python" {
		t.Fatal("Python gate requires Python inputs")
	}
	var source []byte
	for _, input := range graduationInputs(t) {
		if input.size == "32k" {
			source = input.source[0]
			break
		}
	}
	if len(source) == 0 {
		t.Fatal("32 KiB fixture missing")
	}
	var positions []int
	for at := 0; at < len(source); {
		found := bytes.Index(source[at:], []byte("x0 ="))
		if found < 0 {
			break
		}
		positions = append(positions, at+found+1)
		at += found + 4
	}
	if len(positions) < 16 {
		t.Fatal("fixture has fewer than 16 identifier sites")
	}
	lang := grammars.PythonLanguage()
	cl, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	old, err := p.Parse(source)
	ceilingGoTree(t, old, source, err)
	defer func() { old.Release() }()
	steps := 0
	for _, class := range []string{"insert", "delete", "replace"} {
		for site := 0; site < 16; site++ {
			at := positions[site*(len(positions)-1)/15]
			end, added := at+1, []byte("1")
			if class == "insert" {
				end, added = at, []byte("z")
			}
			if class == "delete" {
				added = nil
			}
			changed := append(bytes.Clone(source[:at]), added...)
			changed = append(changed, source[end:]...)
			for direction, to := range [][]byte{changed, source} {
				from, oldEnd, newEnd := source, end, at+len(added)
				if direction != 0 {
					from, oldEnd, newEnd = changed, newEnd, oldEnd
				}
				old.Edit(canonicalGoInputEdit(from, to, at, oldEnd, newEnd))
				next, profile, err := p.ParseIncrementalProfiled(to, old)
				ceilingGoTree(t, next, to, err)
				freshParser := gts.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(true)
				fresh, err := freshParser.Parse(to)
				ceilingGoTree(t, fresh, to, err)
				ct := cp.Parse(to, nil)
				if ct == nil {
					t.Fatal("locked C returned no tree")
				}
				actual, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
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
				root := next.RootNode()
				if actual.SHA256 != want.SHA256 || want.SHA256 != oracle {
					t.Fatalf("class=%s site=%d step=%d D8=%t C=%t", class, site, steps, actual.SHA256 == want.SHA256, want.SHA256 == oracle)
				}
				if root.IsError() && !root.HasError() {
					t.Fatal("ERROR root lost HasError")
				}
				if root.EndByte() != uint32(len(to)) && (next.ParseStopReason() == gts.ParseStopAccepted || next.ParseStopReason() == gts.ParseStopNone) {
					t.Fatal("unexplained input coverage gap")
				}
				allocations := testing.AllocsPerRun(1, func() {
					same, err := p.ParseIncremental(to, next)
					if err != nil || same != next {
						t.Fatalf("no-edit parse: %v", err)
					}
					same.Release()
				})
				if allocations != 0 {
					t.Fatalf("no-edit allocations=%g", allocations)
				}
				if !next.ParseRuntime().CompactIncrementalReuseRoute || profile.ReusedSubtrees == 0 || profile.ReusedBytes == 0 {
					t.Fatalf("clean class=%s site=%d step=%d lost compact reuse", class, site, steps)
				}
				t.Logf("PYTHON_SESSION class=%s site=%d step=%d compact=%t reused=%d", class, site, steps, next.ParseRuntime().CompactIncrementalReuseRoute, profile.ReusedSubtrees)
				ct.Close()
				fresh.Release()
				old.Release()
				old = next
				steps++
			}
		}
	}
	t.Logf("PYTHON_SESSION_TOTAL steps=%d sites_per_class=16", steps)
}
