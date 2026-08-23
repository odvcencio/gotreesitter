//go:build p17_attribution

package gotreesitter

// This file is a temporary P17 attribution harness. It must not remain in the
// worktree after the evidence packet is frozen.

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"testing"
	"time"
	"unsafe"
)

type p17Manifest struct {
	Schema string        `json:"schema"`
	Edits  []p17EditSpec `json:"edits"`
}

type p17EditSpec struct {
	Name         string `json:"name"`
	Language     string `json:"language"`
	Fixture      string `json:"fixture"`
	Role         string `json:"role"`
	StartByte    int    `json:"start_byte"`
	OldEndByte   int    `json:"old_end_byte"`
	OldText      string `json:"old_text"`
	NewText      string `json:"new_text"`
	SourceSHA256 string `json:"source_sha256"`
	EditedSHA256 string `json:"edited_sha256"`
}

type p17Case struct {
	Spec   p17EditSpec
	Source []byte
	Edited []byte
}

type p17ArenaSample struct {
	CaseName       string          `json:"case"`
	Direction      string          `json:"direction"`
	Mode           string          `json:"mode"`
	Repeat         int             `json:"repeat"`
	SourceLen      int             `json:"source_len"`
	TargetLen      int             `json:"target_len"`
	HintBefore     int             `json:"incremental_arena_hint_before"`
	HintAfter      int             `json:"incremental_arena_hint_after"`
	CapacityTarget int             `json:"parse_incremental_arena_node_capacity_target"`
	InitialPrimary int             `json:"initial_primary_arena_capacity"`
	ArenaClass     string          `json:"returned_arena_class"`
	PrimaryLen     int             `json:"primary_arena_len"`
	PrimaryCap     int             `json:"primary_arena_cap"`
	ArenaUsed      int             `json:"arena_used"`
	PrimaryBytes   int64           `json:"primary_arena_bytes"`
	OverflowCaps   []int           `json:"overflow_slab_capacities"`
	OverflowBytes  int64           `json:"overflow_node_bytes"`
	TotalNodeBytes int64           `json:"total_node_bytes"`
	OldTarget      int             `json:"old_geometric_target"`
	OldCrossesUsed bool            `json:"old_geometric_target_crosses_used"`
	ExactCrosses   bool            `json:"exact_target_crosses_used"`
	BytesPerOp     uint64          `json:"bytes_per_op"`
	AllocsPerOp    uint64          `json:"allocs_per_op"`
	NewNodes       uint64          `json:"new_nodes_allocated"`
	ReusedSubtrees uint64          `json:"reused_subtrees"`
	ReusedBytes    uint64          `json:"reused_bytes"`
	TokensConsumed uint64          `json:"tokens_consumed"`
	MaxStacksSeen  int             `json:"max_stacks_seen"`
	EntryPeak      uint64          `json:"entry_scratch_peak"`
	StopReason     string          `json:"stop_reason"`
	ArenaBytes     int64           `json:"arena_bytes_allocated"`
	ScratchBytes   int64           `json:"scratch_bytes_allocated"`
	EntryBytes     int64           `json:"entry_scratch_bytes_allocated"`
	GSSBytes       int64           `json:"gss_bytes_allocated"`
	ParserLoopNs   int64           `json:"parser_loop_nanos"`
	DispatchNs     int64           `json:"action_dispatch_nanos"`
	MergeNs        int64           `json:"glr_merge_nanos"`
	ProfilePath    string          `json:"profile_path,omitempty"`
	ArenaEvents    []p17ArenaEvent `json:"arena_events,omitempty"`
}

type p17Aggregate struct {
	CaseName       string           `json:"case"`
	Direction      string           `json:"direction"`
	Mode           string           `json:"mode"`
	SourceLen      int              `json:"source_len"`
	TargetLen      int              `json:"target_len"`
	HintBefore     int              `json:"incremental_arena_hint_before"`
	HintAfter      int              `json:"incremental_arena_hint_after"`
	CapacityTarget int              `json:"parse_incremental_arena_node_capacity_target"`
	InitialPrimary int              `json:"initial_primary_arena_capacity"`
	ArenaClass     string           `json:"returned_arena_class"`
	PrimaryLen     int              `json:"primary_arena_len"`
	PrimaryCap     int              `json:"primary_arena_cap"`
	ArenaUsed      int              `json:"arena_used"`
	PrimaryBytes   int64            `json:"primary_arena_bytes"`
	OverflowCaps   []int            `json:"overflow_slab_capacities"`
	OverflowBytes  int64            `json:"overflow_node_bytes"`
	TotalNodeBytes int64            `json:"total_node_bytes"`
	OldTarget      int              `json:"old_geometric_target"`
	OldCrossesUsed bool             `json:"old_geometric_target_crosses_used"`
	ExactCrosses   bool             `json:"exact_target_crosses_used"`
	ArenaBytes     int64            `json:"arena_bytes_allocated"`
	ScratchBytes   int64            `json:"scratch_bytes_allocated"`
	EntryBytes     int64            `json:"entry_scratch_bytes_allocated"`
	GSSBytes       int64            `json:"gss_bytes_allocated"`
	MedianBytes    uint64           `json:"median_bytes_per_op"`
	MedianAllocs   uint64           `json:"median_allocs_per_op"`
	MinBytes       uint64           `json:"min_bytes_per_op"`
	MaxBytes       uint64           `json:"max_bytes_per_op"`
	MinAllocs      uint64           `json:"min_allocs_per_op"`
	MaxAllocs      uint64           `json:"max_allocs_per_op"`
	Samples        []p17ArenaSample `json:"samples"`
}

type p17Receipt struct {
	Schema       string               `json:"schema"`
	GeneratedUTC string               `json:"generated_utc"`
	GoVersion    string               `json:"go_version"`
	NodeSize     int                  `json:"node_size"`
	ManifestSHA  string               `json:"manifest_sha256"`
	SourceSHA    string               `json:"tracked_source_sha256,omitempty"`
	Samples      []p17Aggregate       `json:"aggregates"`
	Sequences    []p17SequenceReceipt `json:"sequences,omitempty"`
}

type p17SequenceStep struct {
	Iteration      int             `json:"iteration"`
	Direction      string          `json:"direction"`
	SourceLen      int             `json:"source_len"`
	TargetLen      int             `json:"target_len"`
	HintBefore     int             `json:"incremental_arena_hint_before"`
	HintAfter      int             `json:"incremental_arena_hint_after"`
	CapacityTarget int             `json:"parse_incremental_arena_node_capacity_target"`
	InitialPrimary int             `json:"initial_primary_arena_capacity"`
	ArenaClass     string          `json:"returned_arena_class"`
	PrimaryLen     int             `json:"primary_arena_len"`
	ArenaUsed      int             `json:"arena_used"`
	OverflowCaps   []int           `json:"overflow_slab_capacities"`
	OverflowBytes  int64           `json:"overflow_node_bytes"`
	OldTarget      int             `json:"old_geometric_target"`
	OldCrossesUsed bool            `json:"old_geometric_target_crosses_used"`
	ExactCrosses   bool            `json:"exact_target_crosses_used"`
	BytesPerOp     uint64          `json:"bytes_per_op"`
	AllocsPerOp    uint64          `json:"allocs_per_op"`
	NewNodes       uint64          `json:"new_nodes_allocated"`
	ArenaBytes     int64           `json:"arena_bytes_allocated"`
	StopReason     string          `json:"stop_reason"`
	ArenaEvents    []p17ArenaEvent `json:"arena_events,omitempty"`
}

type p17SequenceReceipt struct {
	CaseName    string            `json:"case"`
	Cycles      int               `json:"cycles"`
	ProfilePath string            `json:"profile_path,omitempty"`
	Steps       []p17SequenceStep `json:"steps"`
}

type p17ArenaEvent struct {
	Kind      string `json:"kind"`
	Class     string `json:"arena_class"`
	Before    int    `json:"before"`
	Requested int    `json:"requested"`
	After     int    `json:"after"`
	Used      int    `json:"used"`
}

type p17ArenaTrace struct {
	Events []p17ArenaEvent
	Count  int
}

var p17CurrentArenaTrace *p17ArenaTrace

func p17TraceArenaEvent(kind string, class arenaClass, before, requested, after, used int) {
	trace := p17CurrentArenaTrace
	if trace == nil || trace.Count >= len(trace.Events) {
		return
	}
	trace.Events[trace.Count] = p17ArenaEvent{
		Kind: kind, Class: p17ArenaClassName(class), Before: before,
		Requested: requested, After: after, Used: used,
	}
	trace.Count++
}

func p17ArenaTraceSnapshot(trace *p17ArenaTrace) []p17ArenaEvent {
	if trace == nil || trace.Count == 0 {
		return nil
	}
	out := make([]p17ArenaEvent, trace.Count)
	copy(out, trace.Events[:trace.Count])
	return out
}

func TestP17CanonicalArenaAttribution(t *testing.T) {
	if os.Getenv("P17_OUT_DIR") == "" {
		t.Skip("P17_OUT_DIR is required")
	}
	manifestBytes, err := os.ReadFile("cgo_harness/testdata/canonical_go_incremental_edits.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest p17Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	fixtureByID := make(map[string][]byte)
	var cases []p17Case
	for _, spec := range manifest.Edits {
		if spec.Language != "go" || spec.Role != "representative" {
			continue
		}
		source, ok := fixtureByID[spec.Fixture]
		if !ok {
			asset := map[string]string{
				"query_compile": "internal/benchfixtures/testdata/query_compile.go.gz",
				"language":      "internal/benchfixtures/testdata/language.go.gz",
				"grammargen_lr": "internal/benchfixtures/testdata/grammargen_lr.go.gz",
				"rewrite":       "internal/benchfixtures/testdata/rewrite.go.gz",
			}[spec.Fixture]
			if asset == "" {
				t.Fatalf("fixture %q not found", spec.Fixture)
			}
			var loadErr error
			source, loadErr = p17LoadGzip(asset)
			if loadErr != nil {
				t.Fatalf("load fixture %s: %v", spec.Fixture, loadErr)
			}
			fixtureByID[spec.Fixture] = source
		}
		if got := sha256HexP17(source); got != spec.SourceSHA256 {
			t.Fatalf("%s source sha=%s want=%s", spec.Name, got, spec.SourceSHA256)
		}
		if spec.StartByte < 0 || spec.OldEndByte < spec.StartByte || spec.OldEndByte > len(source) {
			t.Fatalf("%s invalid edit range", spec.Name)
		}
		if got := string(source[spec.StartByte:spec.OldEndByte]); got != spec.OldText {
			t.Fatalf("%s old text=%q want=%q", spec.Name, got, spec.OldText)
		}
		edited := make([]byte, 0, len(source)-(spec.OldEndByte-spec.StartByte)+len(spec.NewText))
		edited = append(edited, source[:spec.StartByte]...)
		edited = append(edited, spec.NewText...)
		edited = append(edited, source[spec.OldEndByte:]...)
		if got := sha256HexP17(edited); got != spec.EditedSHA256 {
			t.Fatalf("%s edited sha=%s want=%s", spec.Name, got, spec.EditedSHA256)
		}
		cases = append(cases, p17Case{Spec: spec, Source: source, Edited: edited})
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Spec.Name < cases[j].Spec.Name })
	if len(cases) != 4 {
		t.Fatalf("representative Go cases=%d want=4", len(cases))
	}

	selectedCase := os.Getenv("P17_CASE")
	selectedDirection := os.Getenv("P17_DIRECTION")
	selectedMode := os.Getenv("P17_MODE")
	profile := strings.TrimSpace(os.Getenv("P17_PROFILE")) != ""
	if selectedMode == "alternating" {
		sequences := make([]p17SequenceReceipt, 0, len(cases))
		for _, tc := range cases {
			if selectedCase != "" && selectedCase != tc.Spec.Name {
				continue
			}
			sequences = append(sequences, p17RunSequence(t, tc, profile, os.Getenv("P17_OUT_DIR")))
		}
		if len(sequences) == 0 {
			t.Fatal("P17 alternating selection matched no cases")
		}
		receipt := p17Receipt{
			Schema: "p17-arena-attribution-v1", GeneratedUTC: time.Now().UTC().Format(time.RFC3339Nano),
			GoVersion: runtime.Version(), NodeSize: int(unsafe.Sizeof(Node{})),
			ManifestSHA: sha256HexP17(manifestBytes), Sequences: sequences,
		}
		name := "p17-sequence"
		if len(sequences) == 1 {
			name += "-" + sequences[0].CaseName
		}
		p17WriteReceipt(t, receipt, os.Getenv("P17_OUT_DIR"), name)
		return
	}
	repeats := 3
	if profile {
		repeats = 1
	}
	outDir := os.Getenv("P17_OUT_DIR")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var aggregates []p17Aggregate
	for _, tc := range cases {
		if selectedCase != "" && selectedCase != tc.Spec.Name {
			continue
		}
		for _, direction := range []string{"forward", "reverse"} {
			if selectedDirection != "" && selectedDirection != direction {
				continue
			}
			for _, mode := range []string{"first_call", "warmed_parser"} {
				if selectedMode != "" && selectedMode != mode {
					continue
				}
				from, to, edit := p17Direction(tc, direction)
				aggregate := p17Measure(t, tc.Spec.Name, direction, mode, from, to, edit, repeats, profile, outDir)
				aggregates = append(aggregates, aggregate)
			}
		}
	}
	if len(aggregates) == 0 {
		t.Fatal("P17 selection matched no cases")
	}
	receipt := p17Receipt{
		Schema:       "p17-arena-attribution-v1",
		GeneratedUTC: time.Now().UTC().Format(time.RFC3339Nano),
		GoVersion:    runtime.Version(),
		NodeSize:     int(unsafe.Sizeof(Node{})),
		ManifestSHA:  sha256HexP17(manifestBytes),
		Samples:      aggregates,
	}
	name := "p17-arena-attribution"
	if selectedCase != "" {
		name += "-" + selectedCase
	}
	if selectedDirection != "" {
		name += "-" + selectedDirection
	}
	if selectedMode != "" {
		name += "-" + selectedMode
	}
	p17WriteReceipt(t, receipt, outDir, name)
}

func p17WriteReceipt(t *testing.T, receipt p17Receipt, outDir, name string) {
	t.Helper()
	receiptBytes, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(outDir, name+".json")
	if err := os.WriteFile(path, append(receiptBytes, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("P17 receipt=%s sha256=%s", path, sha256HexP17(append(receiptBytes, '\n')))
}

func p17Direction(tc p17Case, direction string) ([]byte, []byte, InputEdit) {
	if direction == "forward" {
		return tc.Source, tc.Edited, p17InputEdit(tc.Source, tc.Edited, tc.Spec.StartByte, tc.Spec.OldEndByte, tc.Spec.StartByte+len(tc.Spec.NewText))
	}
	return tc.Edited, tc.Source, p17InputEdit(tc.Edited, tc.Source, tc.Spec.StartByte, tc.Spec.StartByte+len(tc.Spec.NewText), tc.Spec.StartByte+len(tc.Spec.OldText))
}

func p17InputEdit(oldSource, newSource []byte, start, oldEnd, newEnd int) InputEdit {
	return InputEdit{
		StartByte:   uint32(start),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(newEnd),
		StartPoint:  p17PointAt(oldSource, start),
		OldEndPoint: p17PointAt(oldSource, oldEnd),
		NewEndPoint: p17PointAt(newSource, newEnd),
	}
}

func p17PointAt(source []byte, offset int) Point {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	row, col := uint32(0), uint32(0)
	for _, b := range source[:offset] {
		if b == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return Point{Row: row, Column: col}
}

func p17Measure(t *testing.T, caseName, direction, mode string, from, to []byte, edit InputEdit, repeats int, profile bool, outDir string) p17Aggregate {
	t.Helper()
	lang, err := LoadLanguage(parserCoreCertifiedGoBlob)
	if err != nil {
		t.Fatalf("load Go language: %v", err)
	}
	samples := make([]p17ArenaSample, 0, repeats)
	for repeat := 0; repeat < repeats; repeat++ {
		DrainArenaPools()
		runtime.GC()
		parser := NewParser(lang)
		initialPrimary := nodeCapacityForClass(arenaClassIncremental)
		if mode == "warmed_parser" {
			warmOld, err := parser.Parse(from)
			if err != nil {
				t.Fatalf("%s/%s warm old parse: %v", caseName, direction, err)
			}
			warmOld.Edit(edit)
			warmNew, _, err := parser.ParseIncrementalProfiled(to, warmOld)
			if err != nil {
				t.Fatalf("%s/%s warm incremental: %v", caseName, direction, err)
			}
			if warmNew != warmOld {
				warmOld.Release()
			}
			if warmNew.arena != nil {
				initialPrimary = len(warmNew.arena.nodes)
			}
			warmNew.Release()
			runtime.GC()
		}
		oldTree, err := parser.Parse(from)
		if err != nil {
			t.Fatalf("%s/%s old parse: %v", caseName, direction, err)
		}
		oldTree.Edit(edit)
		hintBefore := parser.incrementalArenaHintCapacity()
		target := parseIncrementalArenaNodeCapacity(len(to), hintBefore)
		// The full-parse old tree owns a different arena pool. The incremental
		// pool is cold for first_call and retains the warm parse for warmed_parser.
		// The old tree owns a full-parse arena. Draining the pool is safe here,
		// and the incremental parse acquires a fresh incremental arena below.
		trace := &p17ArenaTrace{Events: make([]p17ArenaEvent, 128)}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		if profile {
			runtime.MemProfileRate = 1
		}
		p17CurrentArenaTrace = trace
		newTree, prof, err := parser.ParseIncrementalProfiled(to, oldTree)
		p17CurrentArenaTrace = nil
		if profile {
			runtime.MemProfileRate = 0
		}
		if err != nil {
			t.Fatalf("%s/%s %s incremental: %v", caseName, direction, mode, err)
		}
		runtime.ReadMemStats(&after)
		if newTree == nil || newTree.arena == nil {
			t.Fatalf("%s/%s %s returned tree without arena", caseName, direction, mode)
		}
		arena := newTree.arena
		hintAfter := parser.incrementalArenaHintCapacity()
		caps := make([]int, len(arena.nodeSlabs))
		var overflowBytes int64
		for i, slab := range arena.nodeSlabs {
			caps[i] = len(slab.data)
			overflowBytes += int64(len(slab.data)) * int64(unsafe.Sizeof(Node{}))
		}
		primaryBytes := int64(len(arena.nodes)) * int64(unsafe.Sizeof(Node{}))
		used := arena.used
		oldTarget := p17GeometricCapacity(initialPrimary, target)
		profilePath := ""
		if profile {
			profilePath = filepath.Join(outDir, fmt.Sprintf("p17-alloc-%s-%s-%s-%d.pprof", caseName, direction, mode, repeat))
			f, openErr := os.Create(profilePath)
			if openErr != nil {
				t.Fatalf("create allocation profile: %v", openErr)
			}
			if writeErr := pprof.Lookup("allocs").WriteTo(f, 0); writeErr != nil {
				_ = f.Close()
				t.Fatalf("write allocation profile: %v", writeErr)
			}
			if closeErr := f.Close(); closeErr != nil {
				t.Fatalf("close allocation profile: %v", closeErr)
			}
		}
		sample := p17ArenaSample{
			CaseName: caseName, Direction: direction, Mode: mode, Repeat: repeat,
			SourceLen: len(from), TargetLen: len(to), HintBefore: hintBefore, HintAfter: hintAfter,
			CapacityTarget: target, InitialPrimary: initialPrimary,
			ArenaClass: p17ArenaClassName(arena.class),
			PrimaryLen: len(arena.nodes), PrimaryCap: cap(arena.nodes), ArenaUsed: used,
			PrimaryBytes: primaryBytes, OverflowCaps: caps, OverflowBytes: overflowBytes,
			TotalNodeBytes: primaryBytes + overflowBytes, OldTarget: oldTarget,
			OldCrossesUsed: used > oldTarget, ExactCrosses: used > target,
			BytesPerOp: after.TotalAlloc - before.TotalAlloc, AllocsPerOp: after.Mallocs - before.Mallocs,
			NewNodes: prof.NewNodesAllocated, ReusedSubtrees: prof.ReusedSubtrees, ReusedBytes: prof.ReusedBytes,
			TokensConsumed: prof.TokensConsumed, MaxStacksSeen: prof.MaxStacksSeen, EntryPeak: prof.EntryScratchPeak,
			StopReason: string(prof.StopReason), ParserLoopNs: prof.ParserLoopNanos,
			DispatchNs: prof.ActionDispatchNanos, MergeNs: prof.GLRMergeNanos, ProfilePath: profilePath,
			ArenaBytes: prof.ArenaBytesAllocated, ScratchBytes: prof.ScratchBytesAllocated,
			EntryBytes: prof.EntryScratchBytesAllocated, GSSBytes: prof.GSSBytesAllocated,
			ArenaEvents: p17ArenaTraceSnapshot(trace),
		}
		samples = append(samples, sample)
		if newTree != oldTree {
			oldTree.Release()
		}
		newTree.Release()
		DrainArenaPools()
		runtime.GC()
	}
	bytes := make([]uint64, len(samples))
	allocs := make([]uint64, len(samples))
	for i, sample := range samples {
		bytes[i] = sample.BytesPerOp
		allocs[i] = sample.AllocsPerOp
	}
	sort.Slice(bytes, func(i, j int) bool { return bytes[i] < bytes[j] })
	sort.Slice(allocs, func(i, j int) bool { return allocs[i] < allocs[j] })
	median := func(values []uint64) uint64 { return values[len(values)/2] }
	first := samples[0]
	return p17Aggregate{
		CaseName: caseName, Direction: direction, Mode: mode,
		SourceLen: first.SourceLen, TargetLen: first.TargetLen,
		HintBefore: first.HintBefore, HintAfter: first.HintAfter, CapacityTarget: first.CapacityTarget,
		InitialPrimary: first.InitialPrimary, PrimaryLen: first.PrimaryLen, PrimaryCap: first.PrimaryCap,
		ArenaClass: first.ArenaClass,
		ArenaUsed:  first.ArenaUsed, PrimaryBytes: first.PrimaryBytes, OverflowCaps: first.OverflowCaps,
		OverflowBytes: first.OverflowBytes, TotalNodeBytes: first.TotalNodeBytes, OldTarget: first.OldTarget,
		OldCrossesUsed: first.OldCrossesUsed, ExactCrosses: first.ExactCrosses,
		ArenaBytes: first.ArenaBytes, ScratchBytes: first.ScratchBytes,
		EntryBytes: first.EntryBytes, GSSBytes: first.GSSBytes,
		MedianBytes: median(bytes), MedianAllocs: median(allocs), MinBytes: bytes[0], MaxBytes: bytes[len(bytes)-1],
		MinAllocs: allocs[0], MaxAllocs: allocs[len(allocs)-1], Samples: samples,
	}
}

func p17RunSequence(t *testing.T, tc p17Case, profile bool, outDir string) p17SequenceReceipt {
	t.Helper()
	lang, err := LoadLanguage(parserCoreCertifiedGoBlob)
	if err != nil {
		t.Fatalf("load Go language: %v", err)
	}
	parser := NewParser(lang)
	tree, err := parser.Parse(tc.Source)
	if err != nil {
		t.Fatalf("%s initial parse: %v", tc.Spec.Name, err)
	}
	const cycles = 12
	steps := make([]p17SequenceStep, 0, cycles)
	var lastIncrementalPrimary int
	for i := 0; i < cycles; i++ {
		direction := "forward"
		from, to, edit := tc.Source, tc.Edited, p17InputEdit(tc.Source, tc.Edited, tc.Spec.StartByte, tc.Spec.OldEndByte, tc.Spec.StartByte+len(tc.Spec.NewText))
		if i%2 == 1 {
			direction = "reverse"
			from, to, edit = tc.Edited, tc.Source, p17InputEdit(tc.Edited, tc.Source, tc.Spec.StartByte, tc.Spec.StartByte+len(tc.Spec.NewText), tc.Spec.StartByte+len(tc.Spec.OldText))
		}
		if tree.Source() == nil {
			t.Fatalf("%s sequence tree has nil source before iteration %d", tc.Spec.Name, i)
		}
		tree.Edit(edit)
		hintBefore := parser.incrementalArenaHintCapacity()
		target := parseIncrementalArenaNodeCapacity(len(to), hintBefore)
		initialPrimary := nodeCapacityForClass(arenaClassIncremental)
		if i > 0 && lastIncrementalPrimary > 0 {
			initialPrimary = lastIncrementalPrimary
		}
		trace := &p17ArenaTrace{Events: make([]p17ArenaEvent, 128)}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		if profile && i == 0 {
			runtime.MemProfileRate = 1
		}
		p17CurrentArenaTrace = trace
		newTree, prof, err := parser.ParseIncrementalProfiled(to, tree)
		p17CurrentArenaTrace = nil
		if err != nil {
			t.Fatalf("%s sequence iteration %d: %v", tc.Spec.Name, i, err)
		}
		runtime.ReadMemStats(&after)
		if newTree == nil || newTree.arena == nil {
			t.Fatalf("%s sequence iteration %d returned nil arena", tc.Spec.Name, i)
		}
		arena := newTree.arena
		caps := make([]int, len(arena.nodeSlabs))
		var overflowBytes int64
		for j, slab := range arena.nodeSlabs {
			caps[j] = len(slab.data)
			overflowBytes += int64(len(slab.data)) * int64(unsafe.Sizeof(Node{}))
		}
		used := arena.used
		oldTarget := p17GeometricCapacity(initialPrimary, target)
		steps = append(steps, p17SequenceStep{
			Iteration: i, Direction: direction, SourceLen: len(from), TargetLen: len(to),
			HintBefore: hintBefore, HintAfter: parser.incrementalArenaHintCapacity(), CapacityTarget: target,
			InitialPrimary: initialPrimary, ArenaClass: p17ArenaClassName(arena.class),
			PrimaryLen: len(arena.nodes), ArenaUsed: used, OverflowCaps: caps, OverflowBytes: overflowBytes,
			OldTarget: oldTarget, OldCrossesUsed: used > oldTarget, ExactCrosses: used > target,
			BytesPerOp: after.TotalAlloc - before.TotalAlloc, AllocsPerOp: after.Mallocs - before.Mallocs,
			NewNodes: prof.NewNodesAllocated, ArenaBytes: prof.ArenaBytesAllocated, StopReason: string(prof.StopReason),
			ArenaEvents: p17ArenaTraceSnapshot(trace),
		})
		if newTree != tree {
			if tree.arena != nil && tree.arena.class == arenaClassIncremental {
				lastIncrementalPrimary = len(tree.arena.nodes)
			}
			tree.Release()
		}
		tree = newTree
	}
	if profile {
		runtime.MemProfileRate = 0
	}
	profilePath := ""
	if profile {
		profilePath = filepath.Join(outDir, "p17-sequence-"+tc.Spec.Name+".pprof")
		f, createErr := os.Create(profilePath)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if writeErr := pprof.Lookup("allocs").WriteTo(f, 0); writeErr != nil {
			_ = f.Close()
			t.Fatal(writeErr)
		}
		if closeErr := f.Close(); closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	tree.Release()
	DrainArenaPools()
	runtime.GC()
	return p17SequenceReceipt{CaseName: tc.Spec.Name, Cycles: cycles, ProfilePath: profilePath, Steps: steps}
}

func p17GeometricCapacity(initial, target int) int {
	if initial < minArenaNodeCap {
		initial = minArenaNodeCap
	}
	if target <= initial {
		return initial
	}
	for initial < target {
		initial *= 2
	}
	return initial
}

func p17ArenaClassName(class arenaClass) string {
	if class == arenaClassFull {
		return "full"
	}
	return "incremental"
}

func sha256HexP17(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func p17LoadGzip(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(zr)
	closeErr := zr.Close()
	if readErr != nil {
		return nil, readErr
	}
	return data, closeErr
}
