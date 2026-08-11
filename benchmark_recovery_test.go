package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestKDLRecoveryGarbageSuffixExact(t *testing.T) {
	lang := grammars.KdlLanguage()
	src := makeKDLRecoveryGarbageSource(120, 300)
	tree, err := gotreesitter.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("parse recovery garbage suffix: %v", err)
	}
	t.Cleanup(tree.Release)

	root := tree.RootNode()
	if root == nil {
		t.Fatal("recovery garbage-suffix parse returned nil root")
	}
	if got, want := root.Type(lang), "document"; got != want {
		t.Fatalf("root type = %q, want %q", got, want)
	}
	if !root.HasError() {
		t.Fatal("recovery garbage-suffix workload parsed without error")
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("root.EndByte = %d, want %d", got, want)
	}

	wantDigest := [sha256.Size]byte{
		0xcb, 0xe5, 0x33, 0x87, 0x5b, 0xb7, 0x90, 0xc7,
		0x7d, 0x7c, 0x7a, 0xe4, 0x7d, 0x36, 0xb2, 0xd4,
		0x32, 0x05, 0xc3, 0x97, 0x88, 0x46, 0xa9, 0x8b,
		0xd5, 0xd0, 0x04, 0x63, 0xb4, 0x5b, 0x12, 0x12,
	}
	if got := sha256.Sum256([]byte(root.SExpr(lang))); got != wantDigest {
		t.Fatalf("selected tree digest = %x, want %x", got, wantDigest)
	}

	runtime := tree.ParseRuntime()
	if got, want := runtime.StopReason, gotreesitter.ParseStopAccepted; got != want {
		t.Fatalf("stop reason = %q, want %q", got, want)
	}
	if runtime.Truncated {
		t.Fatal("parse runtime is truncated")
	}
	if !runtime.CRecoveryEnteredErrorState {
		t.Fatal("parse did not enter C recovery")
	}
	if runtime.CRecoveryDroppedErrorForClean {
		t.Fatal("selected tree carries the dropped-error marker")
	}
	if runtime.CRecoverySwallowedErrorFallbackAttempted {
		t.Fatal("parse attempted the swallowed-error fallback")
	}
}

func TestRecoveryRuntimeTelemetry(t *testing.T) {
	gotreesitter.EnableRecoveryRuntimeTelemetry(true)
	defer gotreesitter.EnableRecoveryRuntimeTelemetry(false)

	recoveryParser := gotreesitter.NewParser(grammars.KdlLanguage())
	source := makeKDLRecoveryGarbageSource(12, 24)
	tree, err := recoveryParser.Parse(source)
	if err != nil {
		t.Fatalf("parse recovery telemetry witness: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("recovery telemetry witness returned a nil tree")
	}
	stats := recoveryParser.DebugRecoveryRuntimeStats()
	tree.Release()
	if !stats.Enabled || !stats.Completed {
		t.Fatalf("telemetry status = enabled:%v completed:%v, want enabled and completed", stats.Enabled, stats.Completed)
	}
	if stats.RecoveryEntryCount == 0 {
		t.Fatal("recovery entry count = 0, want a recovery witness")
	}
	if stats.ErrorNodeCount == 0 || stats.ErrorSpanBytes == 0 {
		t.Fatalf("error shape = count:%d span:%d, want both non-zero", stats.ErrorNodeCount, stats.ErrorSpanBytes)
	}
	if stats.PeakLiveVersionCount == 0 {
		t.Fatal("peak live version count = 0, want a recovery witness")
	}

	cleanParser := gotreesitter.NewParser(grammars.GoLanguage())
	cleanTree, err := cleanParser.Parse([]byte("package p\n"))
	if err != nil {
		t.Fatalf("parse clean telemetry control: %v", err)
	}
	cleanStats := cleanParser.DebugRecoveryRuntimeStats()
	cleanTree.Release()
	if cleanStats.RecoveryEntryCount != 0 || cleanStats.ErrorNodeCount != 0 || cleanStats.ErrorSpanBytes != 0 {
		t.Fatalf("clean control recorded recovery facts: %+v", cleanStats)
	}
}

// makeKDLRecoveryGarbageSource synthesizes a KDL document that reliably
// drives the C-recovery cost-competition machinery: a valid, node-per-line
// KDL prefix truncated 70% of the way through, followed by a long run of
// unterminated-string / mismatched-brace garbage that the parser can never
// resynchronize against. The whole garbage tail is absorbed into one
// continuously growing ERROR region, exactly the shape that makes warm CPU
// profiles of error-bearing parses on fleet-tail languages (kdl, uxntal)
// dominated by cRecoverStrategy1Election / cNodeErrorCost / cStackPrefixAgg /
// cHandleError / cCondenseAndResume (parser_recover_c.go).
func makeKDLRecoveryGarbageSource(nodeCount, garbageRepeats int) []byte {
	var body strings.Builder
	for i := 0; i < nodeCount; i++ {
		fmt.Fprintf(&body, "node%d \"arg%d\" key=%d {\n", i, i, i)
		fmt.Fprintf(&body, "  child%d \"x\"\n", i)
		body.WriteString("}\n")
	}
	valid := body.String()
	cut := int(float64(len(valid)) * 0.7)

	var out strings.Builder
	out.Grow(cut + garbageRepeats*32)
	out.WriteString(valid[:cut])
	for i := 0; i < garbageRepeats; i++ {
		out.WriteString(" }} \"unterminated garbage ][ ")
		fmt.Fprintf(&out, "%d==%d<<>>", i, i*7)
	}
	return []byte(out.String())
}

// BenchmarkKDLRecoveryGarbageSuffix exercises the C-recovery cost-competition
// machinery end to end on a representative fleet-tail-class error-bearing
// parse (see makeKDLRecoveryGarbageSource). This is the recovery-heavy
// counterpart to the clean-parse canonical trio in BENCH.md: those
// benchmarks never enter cHandleError, so they cannot see regressions or
// wins in the recovery cost-competition path (cRecoverStrategy1Election,
// cNodeErrorCost, cStackPrefixAgg, cHandleError, cCondenseAndResume in
// parser_recover_c.go / glr.go).
func BenchmarkKDLRecoveryGarbageSuffix(b *testing.B) {
	lang := grammars.KdlLanguage()
	parser := gotreesitter.NewParser(lang)
	src := makeKDLRecoveryGarbageSource(120, 300)

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(src)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
		root := tree.RootNode()
		if root == nil {
			b.Fatalf("recovery garbage-suffix parse returned nil root")
		}
		if !root.HasError() {
			b.Fatalf("recovery garbage-suffix workload parsed without error: benchmark no longer exercises recovery")
		}
		if got, want := root.EndByte(), uint32(len(src)); got != want {
			b.Fatalf("recovery garbage-suffix parse truncated: root.EndByte=%d want=%d", got, want)
		}
		tree.Release()
	}
}

// BenchmarkRecoveryCorpusFile profiles one exact corpus file through its
// registered grammar. Set GTS_RECOVERY_CORPUS_FILE and
// GTS_RECOVERY_CORPUS_LANG to enable it. GTS_RECOVERY_CORPUS_TIMEOUT sets the
// parser timeout as a Go duration. The default timeout is 10 seconds.
// GTS_RECOVERY_CORPUS_RETRY_MODE selects the runtime, generic, skip-complete,
// or skip-fresh retry behavior.
func BenchmarkRecoveryCorpusFile(b *testing.B) {
	file := strings.TrimSpace(os.Getenv("GTS_RECOVERY_CORPUS_FILE"))
	langName := strings.TrimSpace(os.Getenv("GTS_RECOVERY_CORPUS_LANG"))
	if file == "" || langName == "" {
		b.Skip("set GTS_RECOVERY_CORPUS_FILE and GTS_RECOVERY_CORPUS_LANG")
	}
	entry := grammars.DetectLanguageByName(langName)
	if entry == nil || entry.Language == nil {
		b.Fatalf("language %q is not registered", langName)
	}
	src, err := os.ReadFile(file)
	if err != nil {
		b.Fatalf("read corpus file: %v", err)
	}
	timeout := 10 * time.Second
	if raw := strings.TrimSpace(os.Getenv("GTS_RECOVERY_CORPUS_TIMEOUT")); raw != "" {
		timeout, err = time.ParseDuration(raw)
		if err != nil || timeout <= 0 {
			b.Fatalf("invalid GTS_RECOVERY_CORPUS_TIMEOUT %q", raw)
		}
	}

	lang := entry.Language()
	originalProfile := lang.FullParseAcceptedErrorRetryProfile
	defer func() {
		lang.FullParseAcceptedErrorRetryProfile = originalProfile
	}()
	switch mode := strings.TrimSpace(os.Getenv("GTS_RECOVERY_CORPUS_RETRY_MODE")); mode {
	case "", "runtime":
	case "generic":
		lang.FullParseAcceptedErrorRetryProfile = gotreesitter.FullParseAcceptedErrorRetryProfile{}
	case "skip-complete":
		profile := lang.FullParseAcceptedErrorRetryProfile
		profile.SkipCompleteAcceptedErrorRetry = true
		lang.FullParseAcceptedErrorRetryProfile = profile
	case "skip-fresh":
		profile := lang.FullParseAcceptedErrorRetryProfile
		profile.SkipFreshCompleteAcceptedErrorRetry = true
		lang.FullParseAcceptedErrorRetryProfile = profile
	default:
		b.Fatalf("invalid GTS_RECOVERY_CORPUS_RETRY_MODE %q", mode)
	}
	parser := gotreesitter.NewParser(lang)
	parser.SetTimeoutMicros(uint64(timeout / time.Microsecond))
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	var memoEntries, memoBytes, memoCollisions uint64
	for i := 0; i < b.N; i++ {
		tree, parseErr := parser.Parse(src)
		if parseErr != nil {
			b.Fatalf("parse corpus file: %v", parseErr)
		}
		if tree == nil || tree.RootNode() == nil {
			b.Fatal("parse corpus file: nil tree")
		}
		if tree.ParseStoppedEarly() {
			reason := tree.ParseStopReason()
			tree.Release()
			b.Fatalf("parse corpus file stopped early: %s", reason)
		}
		memo := tree.RecoveryNodeMemoRuntime()
		memoEntries += uint64(memo.PeakTier.Entries())
		memoBytes += uint64(memo.PeakTier.Bytes())
		memoCollisions += uint64(memo.Collisions)
		tree.Release()
	}
	b.StopTimer()
	b.ReportMetric(float64(memoEntries)/float64(b.N), "memo_entries/op")
	b.ReportMetric(float64(memoBytes)/float64(b.N), "memo_bytes/op")
	b.ReportMetric(float64(memoCollisions)/float64(b.N), "memo_collisions/op")
}
