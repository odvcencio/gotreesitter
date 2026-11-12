package bench_test

// Copies of helpers from root test files (benchmark_recovery_test.go).
// The root package keeps its own copies for the tests that remain there.

import (
	"fmt"
	"strings"
)

// from benchmark_recovery_test.go
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
