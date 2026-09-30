//go:build cgo && treesitter_c_parity

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// Regression: downstream validation used to have no complete fixture matrix.
// A new or missing shape/size must fail instead of silently shrinking coverage.
func TestDownstreamFixtureMatrix(t *testing.T) {
	o := options{root: "../../.."}
	jobs, err := fixturePlan(o)
	if err != nil {
		t.Fatal(err)
	}
	shapes := benchfixtures.GeneratedLanguages()
	if len(jobs) != len(shapes)*3 {
		t.Fatalf("got %d jobs, want %d shapes at three sizes", len(jobs), len(shapes))
	}
	sizes := map[string]map[int]bool{}
	for _, j := range jobs {
		if sizes[j.Shape] == nil {
			sizes[j.Shape] = map[int]bool{}
		}
		if sizes[j.Shape][j.TargetBytes] {
			t.Fatalf("duplicate %s/%d", j.Shape, j.TargetBytes)
		}
		sizes[j.Shape][j.TargetBytes] = true
	}
	for _, shape := range shapes {
		if !reflect.DeepEqual(sizes[shape], map[int]bool{32768: true, 140288: true, 1048576: true}) {
			t.Fatalf("%s sizes=%v", shape, sizes[shape])
		}
	}
}

func TestDownstreamTypingSession(t *testing.T) {
	initial := []byte("αβγ\nsecond line\n")
	first := typingEdits(initial, 200)
	second := typingEdits(initial, 200)
	if len(first) != 200 || !reflect.DeepEqual(first, second) {
		t.Fatal("typing must replay exactly 200 edits")
	}
	current := initial
	positions := [3]int{}
	for i, s := range first {
		e := s
		if e.OldEndByte != e.StartByte || e.NewEndByte != e.StartByte+1 {
			t.Fatalf("edit %d is not one-byte insertion", i)
		}
		expected := append(append(append([]byte(nil), current[:e.StartByte]...), 'x'), current[e.StartByte:]...)
		if e.StartPoint != point(current, int(e.StartByte)) || e.NewEndPoint != point(expected, int(e.NewEndByte)) {
			t.Fatalf("edit %d coordinates/source differ", i)
		}
		positions[i%3]++
		current = expected
	}
	if positions != [3]int{67, 67, 66} {
		t.Fatal(positions)
	}
	if len(current) != len(initial)+200 {
		t.Fatal("lost inserted bytes")
	}
	if first[0].StartByte != 0 || first[2].StartByte != uint32(len(initial)+2) {
		t.Fatal("start/end typing cursors moved incorrectly")
	}
}

func TestDownstreamCorpusLockRejectsMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(path, []byte("go https://example.org/repo wrong . .go\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCorpusLock(options{lock: path}); err == nil {
		t.Fatal("modified external corpus lock was accepted")
	}
}

func TestDownstreamCapturesKeepNamesSpansAndText(t *testing.T) {
	a := []capture{{Name: "name", Start: 1, End: 2, Text: "a"}, {Name: "definition.function", Start: 0, End: 3, Text: "abc"}}
	b := []capture{a[1], a[0]}
	sortCaptures(a)
	sortCaptures(b)
	if captureDigest(a) != captureDigest(b) {
		t.Fatal("query iteration order changed symbol identity")
	}
	b[1].Text = "b"
	if captureDigest(a) == captureDigest(b) {
		t.Fatal("different symbol text has the same output")
	}
	v := validation{}
	v.add(witness{Kind: "d8-incremental-fresh", Step: 1, GoDigest: "old", FreshDigest: "new"})
	v.add(witness{Kind: "d8-incremental-fresh", Step: 2, GoDigest: "old2", FreshDigest: "new2"})
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if v.Mismatches["d8-incremental-fresh"] != 2 || len(v.Witnesses) != 1 || !bytes.Contains(encoded, []byte(`"step":1`)) {
		t.Fatalf("lost first reproducible mismatch: %s", encoded)
	}
}

func TestDownstreamCompleteGoOperation(t *testing.T) {
	// A full source plus all three edits and a highlight query are measured by
	// both engines. This small smoke test checks the operation isn't parse-only.
	j := job{Workflow: "fixtures", Language: "go", Shape: "go", TargetBytes: 512}
	t.Chdir("../../..")
	o := options{root: "."}
	for _, engine := range []string{"go", "c"} {
		r := measure(o, j, engine)
		if r.Error != "" {
			t.Fatalf("%s: %s", engine, r.Error)
		}
		if r.Operations != 4 || r.Captures == 0 || r.WallNS <= 0 || r.P95NS <= 0 || r.OutputSHA256 == "" {
			t.Fatalf("%s incomplete operation: %+v", engine, r)
		}
	}
	v := validate(o, j)
	if v.Error != "" || v.Checks != 10 || len(v.Mismatches) != 0 {
		t.Fatalf("correctness didn't replay every edit against fresh Go/C: %+v", v)
	}
}

// Regression: loading the Go grammar in a C timing child adds its retained
// tables to C peak RSS, even though the C parser never uses them.
func TestDownstreamCTimingDoesNotLoadGoGrammar(t *testing.T) {
	t.Chdir("../../..")
	saved := *grammars.DetectLanguageByName("json")
	replaced := saved
	replaced.Language = func() *ts.Language { t.Fatal("C timing loaded the Go grammar"); return nil }
	grammars.Register(replaced)
	t.Cleanup(func() { grammars.Register(saved) })
	j := job{Workflow: "fixtures", Language: "json", Shape: "json", TargetBytes: 128, Query: "(_) @variable"}
	r := measure(options{root: "."}, j, "c")
	if r.Error != "" || r.Operations != 4 || r.Captures == 0 {
		t.Fatalf("C operation failed: %+v", r)
	}
}

func TestDownstreamQueryCompilationDoesNotHideTreeChecks(t *testing.T) {
	t.Chdir("../../..")
	j := job{Workflow: "fixtures", Language: "go", Shape: "go", TargetBytes: 512, Query: "(not_a_node) @variable"}
	v := validate(options{root: "."}, j)
	if v.Error != "" || v.Checks != 10 {
		t.Fatalf("query compilation hid fresh/incremental tree checks: %+v", v)
	}
	if v.Mismatches["go-query-compile"] != 1 || v.Mismatches["c-query-compile"] != 1 {
		t.Fatalf("query compilation failures were not witnesses: %+v", v)
	}
}

func TestDownstreamCrashKeepsLastInput(t *testing.T) {
	tail := stderrTail{}
	for i := 0; i < 10000; i++ {
		_, _ = tail.Write([]byte("input previous-file\n"))
	}
	_, _ = tail.Write([]byte("input failing-file step 17\n"))
	if len(tail.data) > 4096 || !bytes.HasSuffix(tail.data, []byte("input failing-file step 17\n")) {
		t.Fatal("lost the crash witness or retained an unbounded trace")
	}
	_, _ = tail.Write(bytes.Repeat([]byte{'x'}, 10000))
	if len(tail.data) != 4096 {
		t.Fatal("large compiler diagnostic exceeded the bound")
	}
}

func TestDownstreamEmptyQueryStillChecksTrees(t *testing.T) {
	t.Chdir("../../..")
	j := job{Workflow: "fixtures", Language: "go", Shape: "go", TargetBytes: 512, Query: " "}
	v := validate(options{root: "."}, j)
	if v.Error != "" || v.Checks != 10 || v.Mismatches["empty-query"] != 1 {
		t.Fatalf("empty query didn't fail independently of trees: %+v", v)
	}
	if r := measure(options{root: "."}, j, "go"); r.Error == "" {
		t.Fatal("empty-query timing was reported as a complete operation")
	}
}
