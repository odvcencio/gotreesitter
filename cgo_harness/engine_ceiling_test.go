//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// This opt-in measurement suite changes no parser defaults. Run one language
// per process. C runs in a native batch against COracleLanguage: no source
// marshalling, input callback, Go tree wrapper, or cgo transition per parse.
// Its primary timer includes tree edit, parse, and release, as the Go timer
// does; c-parse-ns/op isolates ts_parser_parse_string. Native B/op counts
// allocation requests (including realloc's requested size), not live bytes.
// It is deliberately measured separately with allocator hooks and no timing.
type ceilingInput struct {
	language, size, mode string
	source               [2][]byte
	edit                 [2]gts.InputEdit
}

func ceilingLanguage(tb testing.TB) string {
	tb.Helper()
	lang := os.Getenv("GTS_CEILING_LANGUAGE")
	if lang == "" {
		tb.Fatal("set GTS_CEILING_LANGUAGE; measure one grammar per process")
	}
	return lang
}

func ceilingInputs(tb testing.TB) []ceilingInput {
	tb.Helper()
	lang := ceilingLanguage(tb)
	var inputs []ceilingInput
	for _, size := range []struct {
		name  string
		bytes int
	}{{"32k", 32 * 1024}, {"137k", 137 * 1024}, {"1m", 1024 * 1024}} {
		src, marker, err := benchfixtures.GeneratedSource(lang, size.bytes)
		if err != nil {
			tb.Fatal(err)
		}
		// Authenticate the R4 generator before any edit or timing.
		manifestBytes, err := os.ReadFile("../internal/benchfixtures/generated.json")
		if err != nil {
			tb.Fatal(err)
		}
		var manifest struct {
			Entries []struct {
				Language string `json:"language"`
				Target   int    `json:"target_bytes"`
				SHA      string `json:"sha256"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
			tb.Fatal(err)
		}
		found := false
		for _, entry := range manifest.Entries {
			if entry.Language == lang && entry.Target == size.bytes {
				found = true
				if fmt.Sprintf("%x", sha256.Sum256(src)) != entry.SHA {
					tb.Fatal("R4 generator digest drift")
				}
			}
		}
		if !found {
			tb.Fatal("R4 fixture identity missing")
		}
		at := bytes.Index(src, []byte(marker))
		if at < 0 {
			tb.Fatal("edit marker missing")
		}
		line := bytes.LastIndexByte(src[:at], '\n') + 1
		for _, mode := range []string{"fresh", "byte", "edit100", "splice"} {
			input := ceilingInput{language: lang, size: size.name, mode: mode, source: [2][]byte{src, src}}
			switch mode {
			case "byte":
				input.source[1] = bytes.Clone(src)
				input.source[1][at] = 'y'
				if src[at] == 'y' {
					input.source[1][at] = 'x'
				}
				input.edit[0] = ceilingEdit(src, input.source[1], at, at+1, at+1)
			case "edit100", "splice":
				// A 100-byte comment insertion, and a 4 KiB comment splice near
				// the start, each alternating with its inverse deletion. Both
				// preserve valid syntax without changing the pinned base file.
				n := 100
				if mode == "splice" {
					n = 4096
				}
				comment := append([]byte("/*"), bytes.Repeat([]byte{' '}, n-4)...)
				comment = append(comment, '*', '/')
				if lang == "python" {
					comment = append(append([]byte{'#'}, bytes.Repeat([]byte{' '}, n-2)...), '\n')
				}
				input.source[1] = append(append(append([]byte{}, src[:line]...), comment...), src[line:]...)
				input.edit[0] = ceilingEdit(src, input.source[1], line, line, line+n)
			}
			e := input.edit[0]
			input.edit[1] = ceilingEdit(input.source[1], src, int(e.StartByte), int(e.NewEndByte), int(e.OldEndByte))
			inputs = append(inputs, input)
		}
	}
	return inputs
}

func ceilingEdit(a, b []byte, start, oldEnd, newEnd int) gts.InputEdit {
	return gts.InputEdit{StartByte: uint32(start), OldEndByte: uint32(oldEnd), NewEndByte: uint32(newEnd), StartPoint: pointAtOffset(a, start), OldEndPoint: pointAtOffset(a, oldEnd), NewEndPoint: pointAtOffset(b, newEnd)}
}

func ceilingGoTree(tb testing.TB, tree *gts.Tree, src []byte, err error) {
	tb.Helper()
	if err != nil || tree == nil || tree.RootNode() == nil {
		tb.Fatalf("parse failed: %v", err)
	}
	if tree.RootNode().EndByte() != uint32(len(src)) || tree.ParseStoppedEarly() {
		tb.Fatalf("incomplete parse: end=%d want=%d stop=%s", tree.RootNode().EndByte(), len(src), tree.ParseRuntime().StopReason)
	}
	if tree.RootNode().IsError() && !tree.RootNode().HasError() {
		tb.Fatal("ERROR root without HasError")
	}
}

func ceilingEngines() []string {
	if value := os.Getenv("GTS_CEILING_ENGINES"); value != "" {
		return strings.Split(value, ",")
	}
	return []string{"C", "legacy", "compact"}
}

func BenchmarkEngineCeiling(b *testing.B) {
	inputs := ceilingInputs(b)
	entry := grammars.DetectLanguageByName(ceilingLanguage(b))
	if entry == nil {
		b.Fatal("unknown grammar")
	}
	type cell struct {
		input  ceilingInput
		engine string
	}
	var cells []cell
	for _, input := range inputs {
		for _, engine := range ceilingEngines() {
			cells = append(cells, cell{input, engine})
		}
	}
	seed := int64(1)
	if value := flag.Lookup("test.shuffle"); value != nil {
		if n, err := strconv.ParseInt(value.Value.String(), 10, 64); err == nil {
			seed = n
		}
	}
	rand.New(rand.NewSource(seed)).Shuffle(len(cells), func(i, j int) { cells[i], cells[j] = cells[j], cells[i] })
	for _, cell := range cells {
		input, engine := cell.input, cell.engine
		b.Run(input.language+"/"+input.size+"/"+input.mode+"/"+engine, func(b *testing.B) {
			if engine == "C" {
				raw, err := cOracleRawLanguage(input.language)
				if err != nil {
					b.Fatal(err)
				}
				identity, err := COracleIdentity(input.language)
				if err != nil {
					b.Fatal(err)
				}
				encoded, _ := json.Marshal(identity)
				b.Logf("locked C identity: %s", encoded)
				parser := newCeilingCParser(raw, input.source[0], input.source[1], input.edit[0], input.edit[1], input.mode != "fresh")
				if parser == nil {
					b.Fatal("native C setup failed")
				}
				defer parser.close()
				// The separate counting batch warms both directions, then counts
				// native allocator requests. Never time the hooks.
				parser.batch(2, false)
				allocation := parser.batch(2, true)
				b.SetBytes(int64(len(input.source[0])))
				b.ResetTimer()
				sample := parser.batch(b.N, false)
				b.StopTimer()
				if sample.failed || allocation.failed {
					b.Fatal("native C parse failed")
				}
				b.ReportMetric(float64(sample.nanos)/float64(b.N), "ns/op")
				b.ReportMetric(float64(sample.parseNanos)/float64(b.N), "c-parse-ns/op")
				b.ReportMetric(float64(allocation.bytes)/2, "B/op")
				b.ReportMetric(float64(allocation.allocs)/2, "allocs/op")
				return
			}
			parser := gts.NewParser(entry.Language())
			parser.SetAdmissionCandidateRoute(engine == "compact")
			gts.ResetAdmissionCandidateCounters()
			tree, err := parser.Parse(input.source[0])
			ceilingGoTree(b, tree, input.source[0], err)
			defer func() {
				if tree != nil {
					tree.Release()
				}
			}()
			if engine == "compact" {
				served, declined := gts.AdmissionCandidateCounters()
				if served != 1 || declined != 0 {
					b.Skipf("compact declines: %s", gts.AdmissionCandidateLastFallbackReason())
				}
				if input.mode != "fresh" {
					tree.Edit(input.edit[0])
					next, err := parser.ParseIncremental(input.source[1], tree)
					ceilingGoTree(b, next, input.source[1], err)
					if next != tree {
						tree.Release()
					}
					tree = next
					if !next.ParseRuntime().CompactIncrementalReuseRoute && !next.ParseRuntime().CompactIncrementalFullRecoveryRoute {
						b.Skip("compact incremental route declined")
					}
					tree.Edit(input.edit[1])
					next, err = parser.ParseIncremental(input.source[0], tree)
					ceilingGoTree(b, next, input.source[0], err)
					if next != tree {
						tree.Release()
					}
					tree = next
					if !next.ParseRuntime().CompactIncrementalReuseRoute && !next.ParseRuntime().CompactIncrementalFullRecoveryRoute {
						b.Skip("compact inverse incremental route declined")
					}
				}
			}
			if input.mode == "fresh" {
				tree.Release()
				tree = nil
			}
			gts.ResetAdmissionCandidateCounters()
			b.ReportAllocs()
			b.SetBytes(int64(len(input.source[0])))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if input.mode == "fresh" {
					next, err := parser.Parse(input.source[0])
					if err != nil || next == nil {
						b.Fatal(err)
					}
					next.Release()
				} else {
					direction := i % 2
					tree.Edit(input.edit[direction])
					next, err := parser.ParseIncremental(input.source[1-direction], tree)
					if err != nil || next == nil {
						b.Fatal(err)
					}
					if next != tree {
						tree.Release()
					}
					tree = next
				}
			}
			b.StopTimer()
			if engine == "compact" && input.mode != "fresh" {
				ceilingGoTree(b, tree, input.source[b.N%2], nil)
				rt := tree.ParseRuntime()
				if !rt.CompactIncrementalReuseRoute && !rt.CompactIncrementalFullRecoveryRoute {
					b.Fatal("compact incremental changed route during timing")
				}
			}
			if input.mode == "fresh" && engine == "compact" {
				routed, declined := gts.AdmissionCandidateCounters()
				if routed != uint64(b.N) || declined != 0 {
					b.Fatal("compact changed route during timing")
				}
			}
		})
	}
}

func BenchmarkEngineCeilingCGOCall(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if ceilingCGONoop(uint64(i)) != uint64(i) {
			b.Fatal("cgo canary")
		}
	}
}

// TestEngineCeilingAudit emits observations, not graduation receipts. Existing
// engine divergences stay visible in its output; this is not a parity gate.
// It checks both directions of each edit, with deterministic counters before
// the randomized campaign. TestEngineCeilingContract validates the tooling.
func TestEngineCeilingAudit(t *testing.T) {
	inputs := ceilingInputs(t)
	entry := grammars.DetectLanguageByName(ceilingLanguage(t))
	if entry == nil {
		t.Fatal("unknown grammar")
	}
	lang := entry.Language()
	cLanguage, err := COracleLanguage(ceilingLanguage(t))
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	for _, input := range inputs {
		for _, engine := range []string{"legacy", "compact"} {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(engine == "compact")
			gts.ResetAdmissionCandidateCounters()
			old, err := p.Parse(input.source[0])
			ceilingGoTree(t, old, input.source[0], err)
			served, declined := gts.AdmissionCandidateCounters()
			for direction := 0; direction < 2; direction++ {
				src := input.source[1-direction]
				var profile gts.IncrementalParseProfile
				var tree *gts.Tree
				if input.mode == "fresh" {
					tree = old
					src = input.source[0]
				} else {
					old.Edit(input.edit[direction])
					tree, profile, err = p.ParseIncrementalProfiled(src, old)
					ceilingGoTree(t, tree, src, err)
					if tree != old {
						old.Release()
					}
					old = tree
				}
				freshParser := gts.NewParser(lang)
				freshParser.SetAdmissionCandidateRoute(engine == "compact")
				fresh, err := freshParser.Parse(src)
				ceilingGoTree(t, fresh, src, err)
				cTree := cp.Parse(src, nil)
				if cTree == nil {
					t.Fatal("C parse failed")
				}
				goDigest, err := benchfixtures.InspectGoTree(tree.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				if err != nil {
					t.Fatal(err)
				}
				cDigest, err := COracleDeepDigest(cTree)
				if err != nil {
					t.Fatal(err)
				}
				row := map[string]any{"language": input.language, "size": input.size, "mode": input.mode, "engine": engine, "direction": direction,
					"bytes": len(src), "sha256": fmt.Sprintf("%x", sha256.Sum256(src)), "served": served, "declined": declined, "decline_reason": gts.AdmissionCandidateLastFallbackReason(),
					"matches_C": goDigest.SHA256 == cDigest, "incremental_matches_fresh": goDigest.SHA256 == freshDigest.SHA256,
					"go_digest": goDigest.SHA256, "C_digest": cDigest, "fresh_digest": freshDigest.SHA256,
					"go_error": tree.RootNode().HasError(), "C_error": cTree.RootNode().HasError(), "runtime": tree.ParseRuntime(), "profile": profile}
				encoded, _ := json.Marshal(row)
				fmt.Printf("CEILING_AUDIT %s\n", encoded)
				fresh.Release()
				cTree.Close()
				if input.mode == "fresh" {
					break
				}
			}
			old.Release()
		}
	}
}

func TestEngineCeilingContract(t *testing.T) {
	for _, input := range ceilingInputs(t) {
		for direction, edit := range input.edit {
			a, b := input.source[direction], input.source[1-direction]
			got := append(append(append([]byte{}, a[:edit.StartByte]...), b[edit.StartByte:edit.NewEndByte]...), a[edit.OldEndByte:]...)
			if !bytes.Equal(got, b) {
				t.Fatalf("%s/%s direction=%d: edit does not produce target", input.size, input.mode, direction)
			}
		}
	}
	raw, err := cOracleRawLanguage(ceilingLanguage(t))
	if err != nil {
		t.Fatal(err)
	}
	input := ceilingInputs(t)[0]
	p := newCeilingCParser(raw, input.source[0], input.source[1], input.edit[0], input.edit[1], false)
	if p == nil {
		t.Fatal("C setup failed")
	}
	defer p.close()
	allocation := p.batch(2, true)
	timed := p.batch(2, false)
	if allocation.failed || timed.failed || allocation.bytes == 0 || allocation.allocs == 0 || timed.nanos < timed.parseNanos || timed.bytes != 0 {
		t.Fatalf("invalid native sample: allocations=%+v timing=%+v", allocation, timed)
	}
}

// Optional external fixtures augment the admission census. Sources are local
// and their digests are printed; authenticate their manifest before this run.
func TestEngineCeilingCensus(t *testing.T) {
	langName := ceilingLanguage(t)
	entry := grammars.DetectLanguageByName(langName)
	if entry == nil {
		t.Fatal("unknown grammar")
	}
	lang := entry.Language()
	paths := []string{filepath.Join("..", "internal", "benchfixtures", "testdata", "real", langName)}
	if root := os.Getenv("GTS_CEILING_CORPUS"); root != "" {
		entries, err := os.ReadDir(filepath.Join(root, langName))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				paths = append(paths, filepath.Join(root, langName, entry.Name()))
			}
		}
	}
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		p := gts.NewParser(lang)
		p.SetAdmissionCandidateRoute(true)
		gts.ResetAdmissionCandidateCounters()
		tree, err := p.Parse(src)
		served, declined := gts.AdmissionCandidateCounters()
		row := map[string]any{"language": langName, "file": filepath.Base(path), "bytes": len(src), "sha256": fmt.Sprintf("%x", sha256.Sum256(src)), "served": served, "declined": declined, "decline_reason": gts.AdmissionCandidateLastFallbackReason(), "error": fmt.Sprint(err)}
		if tree != nil {
			row["runtime"] = tree.ParseRuntime()
			row["root_end"] = tree.RootNode().EndByte()
			row["has_error"] = tree.RootNode().HasError()
			tree.Release()
		}
		encoded, _ := json.Marshal(row)
		fmt.Printf("CEILING_CENSUS %s\n", encoded)
	}
}
