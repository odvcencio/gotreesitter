//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func maxConflictWidthForLanguage(lang *gotreesitter.Language) int {
	maxWidth := 1
	for i := range lang.ParseActions {
		if n := len(lang.ParseActions[i].Actions); n > maxWidth {
			maxWidth = n
		}
	}
	return maxWidth
}

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
	b.WriteString("void f() {\n")
	for i := 0; i < callCount; i++ {
		b.WriteString("obj.method();\n")
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

func assertGLRCapPressureRuntime(t *testing.T, tc parityCase, src []byte, minWidthScale int) {
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

	width := maxConflictWidthForLanguage(goLang)
	if width < 2 {
		t.Fatalf("[%s/glr-cap-pressure] invalid max conflict width=%d", tc.name, width)
	}
	minStacks := width * minWidthScale
	if rt.MaxStacksSeen < minStacks {
		t.Fatalf("[%s/glr-cap-pressure] insufficient GLR pressure: maxStacks=%d want>=%d (width=%d scale=%d) %s",
			tc.name, rt.MaxStacksSeen, minStacks, width, minWidthScale, rt.Summary())
	}
}

// TestParityGLRCapPressureTopLanguages ensures we keep at least one
// conflict-heavy, cap-pressure structural parity case in top languages.
// These cases are intentionally shaped to drive MaxStacksSeen well above the
// grammar's conflict-width floor while still requiring exact C parity.
func TestParityGLRCapPressureTopLanguages(t *testing.T) {
	cases := []struct {
		lang      string
		name      string
		source    []byte
		widthMult int
	}{
		{
			lang:      "go",
			name:      "bench500",
			source:    normalizedSource("go", string(makeGoBenchmarkSource(500))),
			widthMult: 2,
		},
		{
			lang:      "java",
			name:      "obj-method-100",
			source:    normalizedSource("java", string(makeJavaGLRPressureSource(100))),
			widthMult: 2,
		},
		{
			lang:      "c",
			name:      "decl-vs-call-200",
			source:    normalizedSource("c", string(makeCGLRPressureSource(200))),
			widthMult: 2,
		},
		{
			lang:      "cpp",
			name:      "decl-vs-call-200",
			source:    normalizedSource("cpp", string(makeCGLRPressureSource(200))),
			widthMult: 2,
		},
		{
			lang:      "dart",
			name:      "obj-method-200",
			source:    normalizedSource("dart", string(makeDartGLRPressureSource(200))),
			widthMult: 2,
		},
	}

	for _, cc := range cases {
		cc := cc
		t.Run(fmt.Sprintf("%s/%s", cc.lang, cc.name), func(t *testing.T) {
			tc := parityCase{name: cc.lang, source: string(cc.source)}
			runParityCase(t, tc, "glr-cap-pressure-"+cc.name, cc.source)
			assertGLRCapPressureRuntime(t, tc, cc.source, cc.widthMult)
		})
	}
}
