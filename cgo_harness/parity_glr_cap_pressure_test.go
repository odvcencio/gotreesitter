//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func makeJavaGLRPressureSource(callCount int) []byte {
	var b strings.Builder
	b.Grow(callCount * 16)
	b.WriteString("class A { void f() {\n")
	for i := 0; i < callCount; i++ {
		b.WriteString("obj.method();\n")
	}
	b.WriteString("} }\n")
	return []byte(b.String())
}

func makeCGLRPressureSource(stmtCount int) []byte {
	var b strings.Builder
	b.Grow(stmtCount * 8)
	b.WriteString("int f() {\n")
	for i := 0; i < stmtCount; i++ {
		b.WriteString("T(x);\n")
	}
	b.WriteString("return 0;\n}\n")
	return []byte(b.String())
}

func makeDartGLRPressureSource(callCount int) []byte {
	var b strings.Builder
	b.Grow(callCount * 16)
	// The certified C conflict order converges plain member calls at three
	// stacks. A generic call with a typed map preserves the six-stack pressure
	// required by this gate, while keeping all original repeated calls.
	b.WriteString("void g(){ f<void>('x', <String,dynamic>{'width':size.width,'height':size.height}); }\n")
	b.WriteString("void f() {\n")
	for i := 0; i < callCount; i++ {
		b.WriteString("obj.method();\n")
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

func assertGLRCapPressureRuntime(t *testing.T, tc parityCase, src []byte, minStacks int) {
	t.Helper()

	goTree, goLang, err := parseWithGo(tc, src, nil)
	if err != nil {
		t.Fatalf("[%s/glr-cap-pressure] gotreesitter parse error: %v", tc.name, err)
	}
	defer goTree.Release()

	root := goTree.RootNode()
	if root == nil {
		t.Fatalf("[%s/glr-cap-pressure] nil root", tc.name)
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("[%s/glr-cap-pressure] root.EndByte=%d want=%d", tc.name, got, want)
	}

	rt := goTree.ParseRuntime()
	if rt.Truncated || goTree.ParseStoppedEarly() {
		t.Fatalf("[%s/glr-cap-pressure] unexpected early stop: %s", tc.name, rt.Summary())
	}
	if rt.StopReason != gotreesitter.ParseStopAccepted {
		t.Fatalf("[%s/glr-cap-pressure] stop reason=%s want=%s (%s)",
			tc.name, rt.StopReason, gotreesitter.ParseStopAccepted, rt.Summary())
	}
	if root.HasError() {
		t.Fatalf("[%s/glr-cap-pressure] root has error: type=%q %s", tc.name, root.Type(goLang), rt.Summary())
	}

	if minStacks < 2 {
		t.Fatalf("[%s/glr-cap-pressure] invalid minStacks=%d", tc.name, minStacks)
	}
	if rt.MaxStacksSeen < minStacks {
		t.Fatalf("[%s/glr-cap-pressure] insufficient GLR pressure: maxStacks=%d want>=%d %s",
			tc.name, rt.MaxStacksSeen, minStacks, rt.Summary())
	}
}

// TestParityGLRCapPressureTopLanguages ensures we keep at least one
// conflict-heavy structural parity case in top languages that still exercise
// production GLR branching. It deliberately disables forest dispatch because
// forest-accepted parses report forest telemetry, not production stack pressure.
func TestParityGLRCapPressureTopLanguages(t *testing.T) {
	parityRequireExhaustive(t, "TestParityGLRCapPressureTopLanguages")
	gotreesitter.SetGLRForestEnabled(false)
	t.Cleanup(func() {
		gotreesitter.SetGLRForestEnabled(os.Getenv("GOT_GLR_FOREST") != "0")
	})

	cases := []struct {
		lang      string
		name      string
		source    []byte
		minStacks int
	}{
		{
			lang:      "java",
			name:      "obj-method-100",
			source:    normalizedSource("java", string(makeJavaGLRPressureSource(100))),
			minStacks: 3,
		},
		{
			lang:      "c",
			name:      "decl-vs-call-200",
			source:    normalizedSource("c", string(makeCGLRPressureSource(200))),
			minStacks: 3,
		},
		{
			lang:      "cpp",
			name:      "decl-vs-call-200",
			source:    normalizedSource("cpp", string(makeCGLRPressureSource(200))),
			minStacks: 3,
		},
		{
			lang:      "dart",
			name:      "obj-method-200",
			source:    normalizedSource("dart", string(makeDartGLRPressureSource(200))),
			minStacks: 6,
		},
	}

	for _, cc := range cases {
		cc := cc
		t.Run(fmt.Sprintf("%s/%s", cc.lang, cc.name), func(t *testing.T) {
			tc := parityCase{name: cc.lang, source: string(cc.source)}
			runParityCase(t, tc, "glr-cap-pressure-"+cc.name, cc.source)
			assertGLRCapPressureRuntime(t, tc, cc.source, cc.minStacks)
		})
	}
}
