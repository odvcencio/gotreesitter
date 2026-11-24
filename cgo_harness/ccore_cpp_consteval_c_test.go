//go:build cgo && treesitter_c_parity && gts_ccore_test_certify

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// C++ has no production certificate. These inputs come from issues #1633 and
// #1559. Each one is mis-parsed on the legacy route (an empty root, a wrong
// shape, or a stop with no_stacks_alive). The core route with a scratch
// certificate must equal pinned C on every one of them.
func TestCcoreCppConstevalAndSkippedBytesMatchPinnedC(t *testing.T) {
	defer ccoreRouteCertify(t)()
	gl, cl := ccoreLanguages(t, "cpp")
	ce := "constexpr int ce(int x) {\n    if consteval {\n        return 1;\n    } else {\n        return x;\n    }\n}\n"
	for _, c := range []struct{ name, src string }{
		{"if_consteval", "void f() { if consteval { } }"},
		{"if_not_consteval", "void f() { if !consteval { } }"},
		{"definitions_after_consteval", "void before() {}\n" + ce + "void after1() { f(); }\nvoid after2() {}\n"},
		{"consteval_in_namespace", "namespace n {\n" + ce + "int inside() { return 2; }\nint last() { return 3; }\n}\nvoid after() {}\n"},
		{"macro_between_strings", "void f(){ log(\"a: %\" PRIx64 \"\\n\", x); }"},
		{"close_angle_run", "i[]>>&;"},
		{"bracket_then_bar", "[c]|"},
		{"hash_after_paren", "(v)#f D#i"},
		{"hash_then_number", "X#g 1L"},
		{"brace_then_hash", "}d o#f"},
		{"string_then_backslash", ",\"\"P\\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := []byte(c.src)
			ct := receiptCTree(t, cl, src)
			defer ct.Close()
			p := gotreesitter.NewParser(gl)
			p.SetAdmissionCandidateRoute(true)
			tr, err := p.Parse(src)
			if err != nil || tr == nil {
				t.Fatalf("Parse: %v", err)
			}
			defer tr.Release()
			if got := tr.ParseStopReason(); got != gotreesitter.ParseStopAccepted {
				t.Fatalf("stop reason %v, want accepted", got)
			}
			if end := tr.RootNode().EndByte(); int(end) != len(src) {
				t.Fatalf("root ends at %d of %d", end, len(src))
			}
			if d := FirstDivergenceDumpV1(tr.RootNode(), gl, ct.RootNode()); d != nil {
				t.Fatalf("core route differs from pinned C: %+v", *d)
			}
		})
	}
}
