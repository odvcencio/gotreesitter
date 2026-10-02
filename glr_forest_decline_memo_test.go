package gotreesitter

import (
	"bytes"
	"reflect"
	"testing"
)

func TestForestDeclineMemoExactSourceAndDefensiveCopy(t *testing.T) {
	parser := &Parser{language: &Language{Name: "memo-test"}}
	source := []byte("same-length source A")
	original := bytes.Clone(source)
	if parser.forestDeclineMemo != nil {
		t.Fatal("memo sidecar allocated before the first cacheable decline")
	}
	parser.rememberForestDecline(source, forestDeclineEOFRecoveryConflict)
	if parser.forestDeclineMemo == nil {
		t.Fatal("memo sidecar was not allocated for a cacheable decline")
	}
	if !parser.forestDeclineMemoHit(source) || !parser.forestDeclineMemoHit(original) {
		t.Fatal("memo missed exact source content")
	}

	source[0] ^= 0xff
	if parser.forestDeclineMemoHit(source) {
		t.Fatal("memo followed an in-place caller mutation")
	}
	if !parser.forestDeclineMemoHit(original) {
		t.Fatal("memo did not retain a defensive copy of the original source")
	}
	if bytes.Equal(parser.forestDeclineMemo.entries[0].source, source) {
		t.Fatal("memo retained the caller's source slice")
	}
}

func TestForestDeclineMemoHashIsOnlyPrefilter(t *testing.T) {
	parser := &Parser{language: &Language{Name: "memo-test"}}
	stored := []byte("same-length source A")
	collision := []byte("same-length source B")
	parser.rememberForestDecline(stored, forestDeclineEOFRecoveryConflict)
	parser.forestDeclineMemo.entries[0].sourceHash = forestDeclineSourceHash(collision)
	if parser.forestDeclineMemoHit(collision) {
		t.Fatal("equal length and forced equal hash bypassed exact source comparison")
	}
}

func TestForestDeclineMemoSeparatesSemanticModes(t *testing.T) {
	parser := &Parser{language: &Language{Name: "memo-test"}}
	source := []byte("mode source")
	previousRecover := glrForestRecover
	defer func() { glrForestRecover = previousRecover }()
	glrForestRecover = false
	parser.rememberForestDecline(source, forestDeclineEOFRecoveryConflict)

	glrForestRecover = true
	if parser.forestDeclineMemoHit(source) {
		t.Fatal("memo matched after forest recovery mode changed")
	}
	glrForestRecover = false
	if !parser.forestDeclineMemoHit(source) {
		t.Fatal("memo missed after forest recovery mode was restored")
	}

	parser.errorCostCompetition = true
	if parser.forestDeclineMemoHit(source) {
		t.Fatal("memo matched after C-recovery cost competition was enabled")
	}
	parser.errorCostCompetition = false
	if !parser.forestDeclineMemoHit(source) {
		t.Fatal("memo missed after C-recovery mode was restored")
	}

	parser.noTreeBenchmarkOnly = true
	if parser.forestDeclineMemoHit(source) {
		t.Fatal("memo matched in no-tree mode")
	}
	parser.noTreeBenchmarkOnly = false
	parser.noTreeCheckpointBenchmarkOnly = true
	if parser.forestDeclineMemoHit(source) {
		t.Fatal("memo matched in no-tree checkpoint mode")
	}
	parser.noTreeCheckpointBenchmarkOnly = false
	if !parser.forestDeclineMemoHit(source) {
		t.Fatal("memo missed after no-tree modes were restored")
	}
}

func TestForestDeclineMemoBoundsAndFIFO(t *testing.T) {
	if forestDeclineMemoMaxSourceBytes > forestDeclineMemoMaxRetainedSourceBytes {
		t.Fatalf("maximum source bytes = %d, exceeds total retained-source bound %d", forestDeclineMemoMaxSourceBytes, forestDeclineMemoMaxRetainedSourceBytes)
	}
	if forestDeclineMemoMaxSourceBytes < 129612 {
		t.Fatalf("maximum source bytes = %d, does not cover the 129,612-byte Make witness", forestDeclineMemoMaxSourceBytes)
	}
	if forestDeclineMemoMaxRetainedSourceBytes < 129612+18407 {
		t.Fatalf("retained source bytes = %d, do not cover both exact Make witnesses", forestDeclineMemoMaxRetainedSourceBytes)
	}

	parser := &Parser{language: &Language{Name: "memo-test"}}
	oversized := make([]byte, forestDeclineMemoMaxSourceBytes+1)
	parser.rememberForestDecline(oversized, forestDeclineEOFRecoveryConflict)
	if parser.forestDeclineMemo != nil {
		t.Fatal("oversized source allocated or populated the lazy memo")
	}
	parser.rememberForestDecline(make([]byte, forestDeclineMemoMaxRetainedSourceBytes+1), forestDeclineEOFRecoveryConflict)
	if parser.forestDeclineMemo != nil {
		t.Fatal("source larger than total retention allocated or populated the lazy memo")
	}

	parser.forestDeclineMemo = &parserColdState{
		forestDeclineMemoState: forestDeclineMemoState{
			retainedSourceBytes: forestDeclineMemoMaxRetainedSourceBytes,
		},
	}
	parser.rememberForestDecline([]byte("cannot fit"), forestDeclineEOFRecoveryConflict)
	if parser.forestDeclineMemo.count != 0 || parser.forestDeclineMemo.retainedSourceBytes != forestDeclineMemoMaxRetainedSourceBytes {
		t.Fatalf("unsatisfiable empty memo changed during rejected insert: count=%d retained=%d", parser.forestDeclineMemo.count, parser.forestDeclineMemo.retainedSourceBytes)
	}

	parser = &Parser{language: &Language{Name: "memo-test"}}
	sources := make([][]byte, forestDeclineMemoSlots+1)
	for i := range sources {
		sources[i] = []byte{byte(i), byte(i + 1), byte(i + 2)}
		parser.rememberForestDecline(sources[i], forestDeclineEOFRecoveryConflict)
	}
	if parser.forestDeclineMemoHit(sources[0]) {
		t.Fatal("slot bound did not evict the oldest source")
	}
	for i, source := range sources[1:] {
		if !parser.forestDeclineMemoHit(source) {
			t.Fatalf("slot-bound FIFO missed retained source %d", i+1)
		}
	}
	if got := parser.forestDeclineMemo.count; got != forestDeclineMemoSlots {
		t.Fatalf("slot-bound count = %d, want %d", got, forestDeclineMemoSlots)
	}

	parser = &Parser{language: &Language{Name: "memo-test"}}
	chunkSize := forestDeclineMemoMaxRetainedSourceBytes / 3
	large := make([][]byte, 4)
	for i := range large {
		large[i] = make([]byte, chunkSize)
		large[i][0] = byte(i + 1)
		parser.rememberForestDecline(large[i], forestDeclineEOFRecoveryConflict)
		if parser.forestDeclineMemo.retainedSourceBytes > forestDeclineMemoMaxRetainedSourceBytes {
			t.Fatalf("retained bytes = %d, cap %d", parser.forestDeclineMemo.retainedSourceBytes, forestDeclineMemoMaxRetainedSourceBytes)
		}
		retainedCapacity := 0
		for offset := 0; offset < int(parser.forestDeclineMemo.count); offset++ {
			index := (int(parser.forestDeclineMemo.head) + offset) % forestDeclineMemoSlots
			retainedCapacity += cap(parser.forestDeclineMemo.entries[index].source)
		}
		if retainedCapacity != parser.forestDeclineMemo.retainedSourceBytes {
			t.Fatalf("retained source capacity = %d, accounted bytes %d", retainedCapacity, parser.forestDeclineMemo.retainedSourceBytes)
		}
	}
	if parser.forestDeclineMemoHit(large[0]) {
		t.Fatal("byte bound did not evict the oldest source")
	}
	for i, source := range large[1:] {
		if !parser.forestDeclineMemoHit(source) {
			t.Fatalf("byte-bound FIFO missed retained source %d", i+1)
		}
	}
	if got := parser.forestDeclineMemo.count; got != 3 {
		t.Fatalf("byte-bound count = %d, want 3", got)
	}
	if got, want := parser.forestDeclineMemo.retainedSourceBytes, 3*chunkSize; got != want {
		t.Fatalf("retained bytes = %d, want %d", got, want)
	}
}

func TestForestDeclineMemoRejectsTransientReasons(t *testing.T) {
	parser := &Parser{language: &Language{Name: "memo-test"}}
	source := []byte("transient source")
	for _, reason := range []string{
		"budget",
		"timeout",
		"cancelled",
		"dead_end",
		"worklist-cap",
		"eof_no_root",
		"noncontiguous_root",
		"nolook_runaway",
	} {
		if forestDeclineReasonIsMemoizable(reason) {
			t.Fatalf("transient reason %q is allowlisted", reason)
		}
		parser.rememberForestDecline(source, reason)
	}
	if parser.forestDeclineMemo != nil {
		t.Fatal("transient reasons allocated or populated the lazy memo")
	}
}

func TestForestDeclineMemoDoesNotBypassExplicitForest(t *testing.T) {
	lang := loadBlobForDecode(t, "json")
	parser := NewParser(lang)
	source := []byte(`[1, 2, 3]`)
	parser.rememberForestDecline(source, forestDeclineEOFRecoveryConflict)
	if !parser.forestDeclineMemoHit(source) {
		t.Fatal("test setup did not populate automatic-dispatch memo")
	}
	tree, ok := parser.ParseForestExperimental(source)
	if !ok || tree == nil || tree.RootNode() == nil {
		t.Fatalf("explicit forest was bypassed by memo: ok=%t tree_nil=%t", ok, tree == nil)
	}
	tree.Release()
}

func TestForestDeclineMemoWarmSemanticDeclineMatchesProduction(t *testing.T) {
	previousEnabled, previousRecover := glrForestEnabled, glrForestRecover
	defer func() {
		glrForestEnabled = previousEnabled
		glrForestRecover = previousRecover
	}()
	glrForestRecover = false
	lang := loadBlobForDecode(t, "make")
	lang.WantsForest = true
	source := []byte("all:\n\t@echo hi\n")

	glrForestEnabled = false
	// Pin to production: this test compares the production forest decline path
	// against production, and observes production-internal decline-memo state.
	productionParser := NewParser(lang)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil || production == nil || production.RootNode() == nil {
		t.Fatalf("production parse: tree_nil=%t err=%v", production == nil, err)
	}
	defer production.Release()
	wantSExpr := production.RootNode().SExpr(lang)
	wantRuntime := production.ParseRuntime()

	glrForestEnabled = true
	parser := NewParser(lang)
	// Pin to production: this test observes production-internal forest decline
	// reason and memo state, which the compact candidate route does not produce.
	parser.SetAdmissionCandidateRoute(false)
	first, err := parser.Parse(source)
	if err != nil || first == nil || first.RootNode() == nil {
		t.Fatalf("first automatic parse: tree_nil=%t err=%v", first == nil, err)
	}
	defer first.Release()
	if parser.forestDeclineReason != forestDeclineEOFRecoveryConflict {
		t.Fatalf("first automatic decline reason = %q, want %q", parser.forestDeclineReason, forestDeclineEOFRecoveryConflict)
	}
	if !parser.forestDeclineMemoHit(source) {
		t.Fatal("first semantic decline did not populate the memo")
	}

	parser.forestDeclineReason = ""
	warm, err := parser.Parse(bytes.Clone(source))
	if err != nil || warm == nil || warm.RootNode() == nil {
		t.Fatalf("warm automatic parse: tree_nil=%t err=%v", warm == nil, err)
	}
	defer warm.Release()
	if parser.forestDeclineReason != "" {
		t.Fatalf("warm parse entered forest: parser_reason=%q", parser.forestDeclineReason)
	}
	if got := warm.RootNode().SExpr(lang); got != wantSExpr {
		t.Fatalf("warm tree differs from production\n got: %s\nwant: %s", got, wantSExpr)
	}
	// Arena and checkpoint slot capacities can be retained by the process-global
	// arena pool before this parser exists, so they can vary with test order
	// despite identical parse work. Compare the causal checkpoint record and
	// snapshot counters, plus every other runtime field, exactly.
	wantRuntime.ArenaBytesAllocated = 0
	wantRuntime.ArenaBaselineBytes = 0
	wantRuntime.ExternalScannerCheckpointSlotsAllocated = 0
	wantRuntime.ExternalScannerCheckpointBytesAllocated = 0
	gotRuntime := warm.ParseRuntime()
	gotRuntime.ArenaBytesAllocated = 0
	gotRuntime.ArenaBaselineBytes = 0
	gotRuntime.ExternalScannerCheckpointSlotsAllocated = 0
	gotRuntime.ExternalScannerCheckpointBytesAllocated = 0
	// Operation bytes count physical growth, so retained pool capacity can
	// change them just like the arena allocation fields above. Keep every
	// causal operation counter exact; the resource-limit tests gate bytes.
	for _, runtime := range []*ParseRuntime{&gotRuntime, &wantRuntime} {
		runtime.OperationWork.Total.Bytes = 0
		runtime.OperationWork.Initial.Bytes = 0
		runtime.OperationWork.Compact.Bytes = 0
		runtime.OperationWork.Retry.Bytes = 0
		runtime.OperationWork.Fallback.Bytes = 0
		runtime.OperationWork.Verification.Bytes = 0
		runtime.OperationWork.Recovery.Bytes = 0
		runtime.OperationWork.Forest.Bytes = 0
	}
	if !reflect.DeepEqual(gotRuntime, wantRuntime) {
		gotValue, wantValue := reflect.ValueOf(gotRuntime), reflect.ValueOf(wantRuntime)
		for i := 0; i < gotValue.NumField(); i++ {
			if !reflect.DeepEqual(gotValue.Field(i).Interface(), wantValue.Field(i).Interface()) {
				t.Errorf("warm runtime field %s = %v, production = %v", gotValue.Type().Field(i).Name, gotValue.Field(i).Interface(), wantValue.Field(i).Interface())
			}
		}
		t.FailNow()
	}
}
