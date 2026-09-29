//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"

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
	if path := os.Getenv("GTS_CEILING_SOURCE"); path != "" {
		src, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		if strings.HasSuffix(path, ".gz") {
			r, err := gzip.NewReader(bytes.NewReader(src))
			if err != nil {
				tb.Fatal(err)
			}
			src, err = io.ReadAll(r)
			closeErr := r.Close()
			if err != nil {
				tb.Fatal(err)
			}
			if closeErr != nil {
				tb.Fatal(closeErr)
			}
		}
		expected := os.Getenv("GTS_CEILING_SOURCE_SHA256")
		if expected == "" || fmt.Sprintf("%x", sha256.Sum256(src)) != expected {
			tb.Fatal("external source identity missing or mismatched")
		}
		if len(src) == 0 {
			tb.Fatal("external source is empty")
		}
		label := os.Getenv("GTS_CEILING_SOURCE_LABEL")
		if label == "" {
			label = filepath.Base(path)
		}
		if label == "" || strings.ContainsAny(label, "/\\") {
			tb.Fatal("external source label must be one benchmark path segment")
		}
		return []ceilingInput{{language: lang, size: label, mode: "fresh", source: [2][]byte{src, src}}}
	}
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

// ceilingUnserved reads a prior audit to avoid repeatedly paying for a declined
// compact attempt during calibration. It excludes the same cells as the live
// admission checks, and never affects a correctness test. The original source
// digest must match each input; incomplete observations are rejected.
func ceilingUnserved(path string, inputs []ceilingInput) (map[string]bool, error) {
	result := make(map[string]bool)
	if path == "" {
		return result, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	type observation struct {
		Language, Size, Mode, Engine, SHA256 string
		Direction                            int
		Served                               uint64
		Runtime                              gts.ParseRuntime
	}
	rows := make(map[string]observation)
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if !bytes.HasPrefix(line, []byte("CEILING_AUDIT ")) {
			continue
		}
		var row observation
		if err := json.Unmarshal(line[len("CEILING_AUDIT "):], &row); err != nil {
			return nil, err
		}
		if row.Engine == "compact" {
			rows[row.Language+"/"+row.Size+"/"+row.Mode+"/"+strconv.Itoa(row.Direction)] = row
		}
	}
	for _, input := range inputs {
		prefix := input.language + "/" + input.size + "/"
		base, ok := rows[prefix+"fresh/0"]
		if !ok || base.SHA256 != fmt.Sprintf("%x", sha256.Sum256(input.source[0])) {
			return nil, fmt.Errorf("audit source identity missing or mismatched: %s", prefix)
		}
		unserved := base.Served != 1
		if input.mode != "fresh" {
			for direction := 0; direction < 2; direction++ {
				row, ok := rows[prefix+input.mode+"/"+strconv.Itoa(direction)]
				if !ok || row.SHA256 != fmt.Sprintf("%x", sha256.Sum256(input.source[1-direction])) {
					return nil, fmt.Errorf("audit edit identity missing or mismatched: %s%s/%d", prefix, input.mode, direction)
				}
				unserved = unserved || (!row.Runtime.CompactIncrementalReuseRoute && !row.Runtime.CompactIncrementalFullRecoveryRoute)
			}
		}
		result[prefix+input.mode] = unserved
	}
	return result, nil
}

func BenchmarkEngineCeiling(b *testing.B) {
	inputs := ceilingInputs(b)
	unserved, err := ceilingUnserved(os.Getenv("GTS_CEILING_ADMISSION_AUDIT"), inputs)
	if err != nil {
		b.Fatal(err)
	}
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
			if engine == "compact" && unserved[input.language+"/"+input.size+"/"+input.mode] {
				b.Skip("prior source-authenticated audit: compact did not serve this cell")
			}
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
	repeated := p.batch(2, true)
	timed := p.batch(2, false)
	if allocation.failed || timed.failed || allocation.bytes == 0 || allocation.allocs == 0 || timed.nanos < timed.parseNanos || timed.bytes != 0 {
		t.Fatalf("invalid native sample: allocations=%+v timing=%+v", allocation, timed)
	}
	if allocation.bytes != repeated.bytes || allocation.allocs != repeated.allocs {
		t.Fatalf("native allocation counts changed after warming: first=%+v second=%+v", allocation, repeated)
	}
}

func TestEngineCeilingAdmissionAudit(t *testing.T) {
	inputs := ceilingInputs(t)[:2]
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	rows := []map[string]any{
		{"language": inputs[0].language, "size": inputs[0].size, "mode": "fresh", "engine": "compact", "direction": 0, "served": 1, "sha256": fmt.Sprintf("%x", sha256.Sum256(inputs[0].source[0]))},
		{"language": inputs[1].language, "size": inputs[1].size, "mode": "byte", "engine": "compact", "direction": 0, "sha256": fmt.Sprintf("%x", sha256.Sum256(inputs[1].source[1])), "runtime": gts.ParseRuntime{CompactIncrementalReuseRoute: true}},
		{"language": inputs[1].language, "size": inputs[1].size, "mode": "byte", "engine": "compact", "direction": 1, "sha256": fmt.Sprintf("%x", sha256.Sum256(inputs[1].source[0])), "runtime": gts.ParseRuntime{CompactIncrementalReuseRoute: true}},
	}
	write := func() {
		var data []byte
		for _, row := range rows {
			encoded, _ := json.Marshal(row)
			data = append(data, append(append([]byte("CEILING_AUDIT "), encoded...), '\n')...)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	mask, err := ceilingUnserved(path, inputs)
	if err != nil || mask[inputs[1].language+"/"+inputs[1].size+"/byte"] {
		t.Fatalf("served edit excluded: mask=%v err=%v", mask, err)
	}
	rows[2]["runtime"] = gts.ParseRuntime{}
	write()
	mask, err = ceilingUnserved(path, inputs)
	if err != nil || !mask[inputs[1].language+"/"+inputs[1].size+"/byte"] {
		t.Fatalf("inverse fallback not excluded: mask=%v err=%v", mask, err)
	}
	rows[0]["sha256"] = "wrong"
	write()
	if _, err := ceilingUnserved(path, inputs); err == nil {
		t.Fatal("stale source audit accepted")
	}
	rows = rows[1:]
	write()
	if _, err := ceilingUnserved(path, inputs); err == nil {
		t.Fatal("incomplete source audit accepted")
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
	seen := make(map[[32]byte]bool)
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(src)
		if seen[digest] {
			continue
		}
		seen[digest] = true
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

// TestEngineCeilingProfile profiles the selected route, including declined
// compact attempts. Its metadata distinguishes a candidate route that falls
// back from actual compact execution. It never supplies benchmark timings.
func TestEngineCeilingProfile(t *testing.T) {
	dir := os.Getenv("GTS_CEILING_PROFILE_DIR")
	if dir == "" {
		t.Skip("set GTS_CEILING_PROFILE_DIR to collect profiles")
	}
	engine := os.Getenv("GTS_CEILING_PROFILE_ENGINE")
	if engine != "legacy" && engine != "compact" {
		t.Fatal("profile engine must be legacy or compact")
	}
	size, mode := os.Getenv("GTS_CEILING_PROFILE_SIZE"), os.Getenv("GTS_CEILING_PROFILE_MODE")
	if size == "" {
		size = "137k"
	}
	if mode == "" {
		mode = "fresh"
	}
	var input *ceilingInput
	for _, candidate := range ceilingInputs(t) {
		if candidate.size == size && candidate.mode == mode {
			copy := candidate
			input = &copy
			break
		}
	}
	if input == nil {
		t.Fatal("profile cell unavailable")
	}
	entry := grammars.DetectLanguageByName(input.language)
	if entry == nil {
		t.Fatal("unknown grammar")
	}
	p := gts.NewParser(entry.Language())
	p.SetAdmissionCandidateRoute(engine == "compact")
	tree, err := p.Parse(input.source[0])
	ceilingGoTree(t, tree, input.source[0], err)
	if mode == "fresh" {
		tree.Release()
		tree = nil
	}
	defer func() {
		if tree != nil {
			tree.Release()
		}
	}()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, input.language+"-"+size+"-"+mode+"-"+engine)
	cpu, err := os.Create(base + ".cpu")
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	gts.ResetAdmissionCandidateCounters()
	if err := pprof.StartCPUProfile(cpu); err != nil {
		cpu.Close()
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()
	defer cpu.Close()
	start := time.Now()
	var operations, incrementalServed uint64
	for time.Since(start) < 30*time.Second {
		if mode == "fresh" {
			next, err := p.Parse(input.source[0])
			if err != nil || next == nil {
				t.Fatal(err)
			}
			next.Release()
		} else {
			direction := int(operations % 2)
			tree.Edit(input.edit[direction])
			next, err := p.ParseIncremental(input.source[1-direction], tree)
			if err != nil || next == nil {
				t.Fatal(err)
			}
			if next != tree {
				tree.Release()
			}
			tree = next
			rt := tree.ParseRuntime()
			if rt.CompactIncrementalReuseRoute || rt.CompactIncrementalFullRecoveryRoute {
				incrementalServed++
			}
		}
		operations++
	}
	pprof.StopCPUProfile()
	if err := cpu.Close(); err != nil {
		t.Fatal(err)
	}
	heap, err := os.Create(base + ".mem")
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.WriteHeapProfile(heap); err != nil {
		heap.Close()
		t.Fatal(err)
	}
	if err := heap.Close(); err != nil {
		t.Fatal(err)
	}
	served, declined := gts.AdmissionCandidateCounters()
	encoded, _ := json.Marshal(map[string]any{"language": input.language, "size": size, "mode": mode, "engine": engine, "operations": operations, "candidate_served": served, "candidate_declined": declined, "incremental_served": incrementalServed, "duration_seconds": time.Since(start).Seconds()})
	fmt.Printf("CEILING_PROFILE %s\n", encoded)
}
