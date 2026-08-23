package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type p25cOperationReceipt struct {
	Label             string                               `json:"label"`
	Bytes             int                                  `json:"bytes"`
	SourceSHA256      string                               `json:"source_sha256"`
	WallNanos         int64                                `json:"wall_nanos"`
	TreeEditNanos     int64                                `json:"tree_edit_nanos,omitempty"`
	HeapAllocDelta    int64                                `json:"heap_alloc_delta"`
	TotalAllocDelta   uint64                               `json:"total_alloc_delta"`
	MallocsDelta      uint64                               `json:"mallocs_delta"`
	TreeDigest        string                               `json:"tree_digest"`
	FreshDigest       string                               `json:"fresh_digest,omitempty"`
	IncrementalDigest string                               `json:"incremental_digest,omitempty"`
	FreshEqual        bool                                 `json:"fresh_equal,omitempty"`
	Profile           gotreesitter.IncrementalParseProfile `json:"profile,omitempty"`
	Runtime           gotreesitter.ParseRuntime            `json:"runtime"`
	Recovery          gotreesitter.RecoveryRuntimeStats    `json:"recovery"`
	Attempts          gotreesitter.RecoveryRuntimeAttempts `json:"attempts,omitempty"`
	Error             string                               `json:"error,omitempty"`
}

func TestP25CObjcProbe(t *testing.T) {
	target := p25cTargetBytes(t)
	source, editOffset := p25cObjcSource(target)
	if len(source) != target {
		t.Fatalf("source bytes=%d want=%d", len(source), target)
	}
	if editOffset <= 0 || editOffset >= len(source) || source[editOffset] != 'x' {
		t.Fatalf("edit offset=%d does not identify x", editOffset)
	}
	gotreesitter.EnableRecoveryRuntimeTelemetry(true)
	defer gotreesitter.EnableRecoveryRuntimeTelemetry(false)

	t.Logf("P25C_FIXTURE bytes=%d edit_offset=%d source_sha256=%x", len(source), editOffset, sha256.Sum256(source))
	base, baseReceipt := p25cFresh("full", source, editOffset, nil)
	if baseReceipt.Error != "" {
		t.Fatal(baseReceipt.Error)
	}
	if base == nil || base.RootNode() == nil {
		t.Fatal("base parse returned no root")
	}
	base.Release()

	for _, kind := range []string{"insert", "delete"} {
		t.Run(kind, func(t *testing.T) {
			edited, edit := p25cApplyEdit(source, editOffset, kind)
			fresh, freshReceipt := p25cFresh("fresh_"+kind, edited, editOffset, nil)
			if freshReceipt.Error != "" {
				t.Fatal(freshReceipt.Error)
			}
			if fresh == nil || fresh.RootNode() == nil {
				t.Fatal("fresh edited parse returned no root")
			}
			freshDigest := p25cTreeDigest(fresh, grammars.ObjcLanguage())
			fresh.Release()
			old, oldReceipt := p25cFresh("old_"+kind, source, editOffset, nil)
			if oldReceipt.Error != "" {
				t.Fatal(oldReceipt.Error)
			}
			if old == nil || old.RootNode() == nil {
				t.Fatal("old parse returned no root")
			}
			t.Cleanup(old.Release)
			beforeEdit := time.Now()
			old.Edit(edit)
			treeEditNanos := time.Since(beforeEdit).Nanoseconds()
			parser := gotreesitter.NewParser(grammars.ObjcLanguage())
			parser.SetAdmissionCandidateRoute(false)
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			started := time.Now()
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, old)
			wall := time.Since(started).Nanoseconds()
			runtime.ReadMemStats(&after)
			receipt := p25cOperationReceipt{
				Label:           "incremental_" + kind,
				Bytes:           len(edited),
				SourceSHA256:    fmt.Sprintf("%x", sha256.Sum256(edited)),
				WallNanos:       wall,
				TreeEditNanos:   treeEditNanos,
				HeapAllocDelta:  int64(after.HeapAlloc) - int64(before.HeapAlloc),
				TotalAllocDelta: after.TotalAlloc - before.TotalAlloc,
				MallocsDelta:    after.Mallocs - before.Mallocs,
				Profile:         profile,
			}
			if err != nil {
				receipt.Error = err.Error()
			}
			if incremental != nil && incremental.RootNode() != nil {
				receipt.TreeDigest = p25cTreeDigest(incremental, grammars.ObjcLanguage())
				receipt.Runtime = incremental.ParseRuntime()
				receipt.Recovery = parser.DebugRecoveryRuntimeStats()
				receipt.Attempts = parser.DebugRecoveryRuntimeAttempts()
			}
			receipt.FreshDigest = freshDigest
			receipt.IncrementalDigest = receipt.TreeDigest
			receipt.FreshEqual = receipt.FreshDigest != "" && receipt.FreshDigest == receipt.IncrementalDigest
			p25cLogReceipt(t, receipt)
			if incremental != nil && incremental != old {
				t.Cleanup(incremental.Release)
			}
		})
	}
}

func TestP25CObjcCorrectness(t *testing.T) {
	target := p25cTargetBytes(t)
	source, editOffset := p25cObjcSource(target)
	if len(source) != target {
		t.Fatalf("source bytes=%d want=%d", len(source), target)
	}
	for _, kind := range []string{"insert", "delete"} {
		t.Run(kind, func(t *testing.T) {
			edited, edit := p25cApplyEdit(source, editOffset, kind)
			fresh, err := gotreesitter.NewParser(grammars.ObjcLanguage()).Parse(edited)
			if err != nil {
				t.Fatal(err)
			}
			if fresh == nil || fresh.RootNode() == nil {
				t.Fatal("fresh parse returned no root")
			}
			t.Cleanup(fresh.Release)
			old, err := gotreesitter.NewParser(grammars.ObjcLanguage()).Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			if old == nil || old.RootNode() == nil {
				t.Fatal("old parse returned no root")
			}
			t.Cleanup(old.Release)
			old.Edit(edit)
			parser := gotreesitter.NewParser(grammars.ObjcLanguage())
			parser.SetAdmissionCandidateRoute(false)
			incremental, _, err := parser.ParseIncrementalProfiled(edited, old)
			if err != nil {
				t.Fatal(err)
			}
			if incremental == nil || incremental.RootNode() == nil {
				t.Fatal("incremental parse returned no root")
			}
			t.Cleanup(func() {
				if incremental != old {
					incremental.Release()
				}
			})
			freshRoot := fresh.RootNode()
			incrementalRoot := incremental.RootNode()
			if got, want := freshRoot.EndByte(), uint32(len(edited)); got != want {
				t.Fatalf("fresh root end=%d want=%d", got, want)
			}
			if got, want := incrementalRoot.EndByte(), uint32(len(edited)); got != want {
				t.Fatalf("incremental root end=%d want=%d", got, want)
			}
			if fresh.ParseRuntime().StopReason != gotreesitter.ParseStopAccepted ||
				incremental.ParseRuntime().StopReason != gotreesitter.ParseStopAccepted {
				t.Fatalf("stop reasons fresh=%s incremental=%s", fresh.ParseRuntime().StopReason, incremental.ParseRuntime().StopReason)
			}
			if fresh.ParseRuntime().Truncated || incremental.ParseRuntime().Truncated {
				t.Fatal("correctness parse truncated")
			}
			if got, want := freshRoot.HasError(), kind == "delete"; got != want {
				t.Fatalf("fresh has_error=%v want=%v", got, want)
			}
			freshDigest := p25cTreeDigest(fresh, grammars.ObjcLanguage())
			incrementalDigest := p25cTreeDigest(incremental, grammars.ObjcLanguage())
			if freshDigest != incrementalDigest {
				t.Fatalf("fresh/incremental digest mismatch fresh=%s incremental=%s", freshDigest, incrementalDigest)
			}
			t.Logf("P25C_CORRECTNESS kind=%s bytes=%d fresh=%s incremental=%s has_error=%v", kind, len(edited), freshDigest, incrementalDigest, freshRoot.HasError())
		})
	}
}

func p25cFresh(label string, source []byte, editOffset int, _ *gotreesitter.Parser) (*gotreesitter.Tree, p25cOperationReceipt) {
	parser := gotreesitter.NewParser(grammars.ObjcLanguage())
	parser.SetAdmissionCandidateRoute(false)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	tree, err := parser.Parse(source)
	wall := time.Since(started).Nanoseconds()
	runtime.ReadMemStats(&after)
	receipt := p25cOperationReceipt{
		Label:           label,
		Bytes:           len(source),
		SourceSHA256:    fmt.Sprintf("%x", sha256.Sum256(source)),
		WallNanos:       wall,
		HeapAllocDelta:  int64(after.HeapAlloc) - int64(before.HeapAlloc),
		TotalAllocDelta: after.TotalAlloc - before.TotalAlloc,
		MallocsDelta:    after.Mallocs - before.Mallocs,
	}
	if err != nil {
		receipt.Error = err.Error()
	}
	if tree != nil && tree.RootNode() != nil {
		receipt.TreeDigest = p25cTreeDigest(tree, grammars.ObjcLanguage())
		receipt.Runtime = tree.ParseRuntime()
		receipt.Recovery = parser.DebugRecoveryRuntimeStats()
		receipt.Attempts = parser.DebugRecoveryRuntimeAttempts()
	}
	if os.Getenv("P25C_LOG_FRESH") != "0" {
		fmt.Printf("P25C_RECEIPT %s\n", p25cJSON(receipt))
	}
	_ = editOffset
	return tree, receipt
}

func p25cApplyEdit(source []byte, offset int, kind string) ([]byte, gotreesitter.InputEdit) {
	replacement := []byte("x")
	oldEnd := offset
	switch kind {
	case "insert":
		replacement = []byte("x")
	case "delete":
		oldEnd = offset + 1
		replacement = nil
	default:
		panic("unknown edit kind " + kind)
	}
	edited := make([]byte, 0, len(source)-oldEnd+offset+len(replacement))
	edited = append(edited, source[:offset]...)
	edited = append(edited, replacement...)
	edited = append(edited, source[oldEnd:]...)
	line := uint32(bytesLine(source, offset))
	point := gotreesitter.Point{Row: line, Column: uint32(bytesColumn(source, offset))}
	return edited, gotreesitter.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(offset + len(replacement)),
		StartPoint:  point,
		OldEndPoint: point,
		NewEndPoint: point,
	}
}

func p25cObjcSource(target int) ([]byte, int) {
	const prefix = "@interface P25CBox : NSObject\n@end\n@implementation P25CBox\n"
	const suffix = "@end\n"
	var b strings.Builder
	b.Grow(target)
	b.WriteString(prefix)
	editOffset := -1
	for i := 0; ; i++ {
		line := fmt.Sprintf("- (int)m%d:(int)arg { int x%d = %d; return x%d; }\n", i, i, i, i)
		if b.Len()+len(line)+len(suffix) > target {
			break
		}
		if editOffset < 0 {
			editOffset = b.Len() + strings.Index(line, "x0")
		}
		b.WriteString(line)
	}
	b.WriteString(suffix)
	if b.Len() < target {
		b.WriteString(strings.Repeat(" ", target-b.Len()))
	}
	return []byte(b.String()), editOffset
}

func p25cTargetBytes(t *testing.T) int {
	t.Helper()
	value := os.Getenv("P25C_BYTES")
	if value == "" {
		return 4 * 1024
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 256 {
		t.Fatalf("invalid P25C_BYTES=%q", value)
	}
	return n
}

func p25cTreeDigest(tree *gotreesitter.Tree, lang *gotreesitter.Language) string {
	if tree == nil || tree.RootNode() == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(tree.RootNode().SExpr(lang)))
	return fmt.Sprintf("%x", sum)
}

func p25cLogReceipt(t *testing.T, receipt p25cOperationReceipt) {
	t.Helper()
	line := "P25C_RECEIPT " + p25cJSON(receipt)
	t.Log(line)
	fmt.Println(line)
}

func p25cJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("json_error:%v", err)
	}
	return string(encoded)
}

func bytesLine(source []byte, offset int) int {
	return strings.Count(string(source[:offset]), "\n")
}

func bytesColumn(source []byte, offset int) int {
	last := strings.LastIndexByte(string(source[:offset]), '\n')
	if last < 0 {
		return offset
	}
	return offset - last - 1
}
