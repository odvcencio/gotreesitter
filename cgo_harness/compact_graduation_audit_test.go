//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"unicode/utf8"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// These opt-in audits use one grammar per process. They report the actual
// serving route: a legacy fallback never counts as compact certification.
func compactAuditLanguage(t *testing.T) (string, *gts.Language, *sitter.Language) {
	t.Helper()
	name := os.Getenv("GTS_COMPACT_AUDIT_LANGUAGE")
	if name == "" {
		t.Skip("set GTS_COMPACT_AUDIT_LANGUAGE to one generated grammar")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatalf("unknown grammar %q", name)
	}
	cl, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("grammar=%s C_runtime=%s", name, COracleRuntimeCommit)
	return name, entry.Language(), cl
}

func compactAuditDigest(t *testing.T, tree *gts.Tree, lang *gts.Language) string {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Go parse returned no root")
	}
	inspection, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	return inspection.SHA256
}

func compactAuditCDigest(t *testing.T, tree *sitter.Tree) string {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C parse returned no root")
	}
	digest, err := COracleDeepDigest(tree)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestCompactGraduationFreshAudit(t *testing.T) {
	name, lang, cl := compactAuditLanguage(t)
	sources := map[string][]byte{}
	if path := os.Getenv("GTS_COMPACT_AUDIT_SOURCE"); path != "" {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sources = map[string][]byte{"witness": source}
	} else {
		for _, size := range []int{32 * 1024, 137 * 1024, 1024 * 1024} {
			source, _, err := benchfixtures.GeneratedSource(name, size)
			if err != nil {
				t.Fatal(err)
			}
			sources[fmt.Sprintf("generated_%dKiB", size/1024)] = source
		}
	}
	// Optional R4 manifest. Authenticate every selected file before parsing.
	if root := os.Getenv("GTS_COMPACT_AUDIT_CORPUS_ROOT"); root != "" {
		var manifest struct {
			Entries []struct {
				Language, Role, Path, SHA256 string
				SourceKey                    string `json:"source_key"`
				Bytes                        int
			} `json:"entries"`
		}
		data, err := os.ReadFile(filepath.Join("..", "internal", "benchfixtures", "real_corpus.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		for _, entry := range manifest.Entries {
			if entry.Language != name || entry.Role == "sample" {
				continue
			}
			source, err := os.ReadFile(filepath.Join(root, entry.SourceKey, entry.Path))
			if err != nil {
				t.Fatal(err)
			}
			if len(source) != entry.Bytes || fmt.Sprintf("%x", sha256.Sum256(source)) != entry.SHA256 {
				t.Fatalf("corpus identity changed: %s", entry.Path)
			}
			sources["real_"+entry.Role] = source
		}
	}
	fixtures := make([]string, 0, len(sources))
	for fixture := range sources {
		fixtures = append(fixtures, fixture)
	}
	sort.Strings(fixtures)
	for _, fixture := range fixtures {
		source := sources[fixture]
		t.Run(fixture, func(t *testing.T) {
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse(source, nil)
			defer ct.Close()
			cd := compactAuditCDigest(t, ct)
			for _, candidate := range []bool{false, true} {
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(candidate)
				p.SetCompactCertificationTelemetry(candidate)
				gts.ResetAdmissionCandidateCounters()
				gt, err := p.Parse(source)
				if err != nil {
					t.Fatal(err)
				}
				digest := compactAuditDigest(t, gt, lang)
				runtime := gt.ParseRuntime()
				routed, declines := gts.AdmissionCandidateCounters()
				peak := uint64(runtime.MaxStacksSeen)
				if candidate && routed > 0 {
					peak = runtime.CompactPeakHeaders
				}
				t.Logf("AUDIT fresh candidate=%t bytes=%d sha=%x routed=%d declines=%d tokens=%d nodes=%d peak=%d stop=%s end=%d error=%t C_error=%t equal_C=%t reason=%q", candidate, len(source), sha256.Sum256(source), routed, declines, runtime.TokensConsumed, runtime.NodesAllocated, peak, runtime.StopReason, gt.RootNode().EndByte(), gt.RootNode().HasError(), ct.RootNode().HasError(), digest == cd, gts.AdmissionCandidateLastFallbackReason())
				if digest != cd {
					t.Errorf("candidate=%t differs from locked C: Go=%s C=%s", candidate, digest, cd)
				}
				if gt.RootNode().IsError() && !gt.RootNode().HasError() {
					t.Error("ERROR root without HasError")
				}
				if gt.RootNode().EndByte() < uint32(len(source)) && runtime.StopReason == gts.ParseStopAccepted {
					t.Error("accepted root does not cover input")
				}
				gt.Release()
			}
		})
	}
}

func TestCompactGraduationSessionAudit(t *testing.T) {
	name, lang, cl := compactAuditLanguage(t)
	source, _, err := benchfixtures.GeneratedSource(name, 137*1024)
	if err != nil {
		t.Fatal(err)
	}
	if path := os.Getenv("GTS_COMPACT_AUDIT_SESSION_SOURCE"); path != "" {
		source, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	old, err := p.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	ct := cp.Parse(source, nil)
	defer func() { old.Release(); ct.Close() }()
	var noEditError error
	allocs := testing.AllocsPerRun(10, func() {
		next, err := p.ParseIncremental(source, old)
		if err != nil {
			noEditError = err
			return
		}
		if next != old {
			next.Release()
			noEditError = fmt.Errorf("no-edit reparse changed tree identity")
		}
	})
	t.Logf("AUDIT no_edit allocs=%g error=%v bytes=%d sha=%x", allocs, noEditError, len(source), sha256.Sum256(source))
	if allocs != 0 || noEditError != nil {
		t.Errorf("no-edit invariant: allocs=%g error=%v", allocs, noEditError)
	}
	stepIndex := 0
	for _, kind := range []string{"insert", "replace", "delete"} {
		for step := 0; step < 24; step++ {
			site := step % 16
			start := site * len(source) / 16
			for start > 0 && start < len(source) && !utf8.RuneStart(source[start]) {
				start--
			}
			end := start
			replacement := []byte{'x'}
			if kind != "insert" && start < len(source) {
				_, width := utf8.DecodeRune(source[start:])
				end += width
			}
			if kind == "delete" {
				replacement = nil
			}
			if kind == "replace" && start < len(source) && source[start] == 'x' {
				replacement = []byte{'y'}
			}
			nextSource := append(append(append([]byte(nil), source[:start]...), replacement...), source[end:]...)
			point := func(input []byte, at int) gts.Point {
				var p gts.Point
				for _, ch := range input[:at] {
					if ch == '\n' {
						p.Row++
						p.Column = 0
					} else {
						p.Column++
					}
				}
				return p
			}
			edit := gts.InputEdit{StartByte: uint32(start), OldEndByte: uint32(end), NewEndByte: uint32(start + len(replacement)), StartPoint: point(source, start), OldEndPoint: point(source, end), NewEndPoint: point(nextSource, start+len(replacement))}
			old.Edit(edit)
			ct.Edit(&sitter.InputEdit{StartByte: uint(start), OldEndByte: uint(end), NewEndByte: uint(start + len(replacement)), StartPosition: sitter.Point{Row: uint(edit.StartPoint.Row), Column: uint(edit.StartPoint.Column)}, OldEndPosition: sitter.Point{Row: uint(edit.OldEndPoint.Row), Column: uint(edit.OldEndPoint.Column)}, NewEndPosition: sitter.Point{Row: uint(edit.NewEndPoint.Row), Column: uint(edit.NewEndPoint.Column)}})
			inc, profile, err := p.ParseIncrementalProfiled(nextSource, old)
			if err != nil {
				t.Fatal(err)
			}
			if inc != old {
				old.Release()
			}
			old = inc
			fresh, err := p.Parse(nextSource)
			if err != nil {
				t.Fatal(err)
			}
			ci := cp.Parse(nextSource, ct)
			cf := cp.Parse(nextSource, nil)
			id, fd, cid, cfd := compactAuditDigest(t, inc, lang), compactAuditDigest(t, fresh, lang), compactAuditCDigest(t, ci), compactAuditCDigest(t, cf)
			stepIndex++
			t.Logf("AUDIT step=%d kind=%s site=%d offset=%d equal_fresh=%t equal_C=%t C_incremental_equal=%t reuse_bytes=%d nodes=%d tokens=%d compact_reuse=%t compact_recovery=%t fallback=%q", stepIndex, kind, site, start, id == fd, id == cfd, cid == cfd, profile.ReusedBytes, profile.NewNodesAllocated, profile.TokensConsumed, inc.ParseRuntime().CompactIncrementalReuseRoute, inc.ParseRuntime().CompactIncrementalFullRecoveryRoute, inc.ParseRuntime().CompactIncrementalFallbackReason)
			if id != fd || id != cfd || cid != cfd {
				t.Errorf("step %d parity: incremental=%s fresh=%s C_incremental=%s C_fresh=%s", stepIndex, id, fd, cid, cfd)
			}
			root := inc.RootNode()
			if root.IsError() && !root.HasError() {
				t.Errorf("step %d ERROR root without HasError", stepIndex)
			}
			if (root.StartByte() != 0 || root.EndByte() < uint32(len(nextSource))) && inc.ParseStopReason() == gts.ParseStopAccepted {
				t.Errorf("step %d accepted root does not cover input", stepIndex)
			}
			fresh.Release()
			ct.Close()
			cf.Close()
			ct = ci
			source = nextSource
		}
	}
}
