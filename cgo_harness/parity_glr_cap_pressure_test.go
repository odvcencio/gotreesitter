//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// cGLRWidth parses src with pinned C and returns the largest version_count
// that the parse log reports: C's maximum GLR width on the input. The
// certified core counts the same quantity (internal/ccore/parser.go, the
// version loop of the main parse), so the two numbers compare directly.
// C/parser.c: LOG("process version:%d, version_count:%u, ...") in
// ts_parser__advance.
func cGLRWidth(t *testing.T, tc parityCase, src []byte) int {
	t.Helper()
	cLang, err := ParityCLanguage(tc.name)
	if err != nil {
		t.Fatalf("[%s/glr-cap-pressure] load C parser: %v", tc.name, err)
	}
	p := sitter.NewParser()
	defer p.Close()
	if err := p.SetLanguage(cLang); err != nil {
		t.Fatalf("[%s/glr-cap-pressure] C SetLanguage: %v", tc.name, err)
	}
	width, lines := 0, 0
	p.SetLogger(func(kind sitter.LogType, message string) {
		if kind != sitter.LogTypeParse {
			return
		}
		lines++
		if n, ok := cParseVersionCountLine(message); ok && n > width {
			width = n
		}
	})
	tree := p.Parse(src, nil)
	if tree == nil {
		t.Fatalf("[%s/glr-cap-pressure] C parse returned nil", tc.name)
	}
	tree.Close()
	if lines == 0 || width == 0 {
		t.Fatalf("[%s/glr-cap-pressure] C parse log was empty (lines=%d width=%d)", tc.name, lines, width)
	}
	return width
}

func cParseVersionCountLine(line string) (int, bool) {
	if !strings.HasPrefix(line, "process version:") {
		return 0, false
	}
	for _, field := range strings.Split(line, ", ") {
		if v, ok := strings.CutPrefix(field, "version_count:"); ok {
			n, err := strconv.Atoi(v)
			return n, err == nil
		}
	}
	return 0, false
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
	// The certified C conflict order converges plain member calls at three
	// stacks. A generic call with a typed map widens the parse to five stacks
	// in pinned C and in the core, while keeping all original repeated calls.
	b.WriteString("void g(){ f<void>('x', <String,dynamic>{'width':size.width,'height':size.height}); }\n")
	b.WriteString("void f() {\n")
	for i := 0; i < callCount; i++ {
		b.WriteString("obj.method();\n")
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

func assertGLRCapPressureRuntime(t *testing.T, tc parityCase, src []byte, wantStacks int) {
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

	t.Logf("[%s/glr-cap-pressure] max GLR width: go=%d c=%d", tc.name, rt.MaxStacksSeen, wantStacks)
	if wantStacks < 2 {
		t.Fatalf("[%s/glr-cap-pressure] witness does not branch: pinned C width=%d, want>=2", tc.name, wantStacks)
	}
	if rt.MaxStacksSeen != wantStacks {
		t.Fatalf("[%s/glr-cap-pressure] GLR width differs from pinned C: go maxStacks=%d c=%d %s",
			tc.name, rt.MaxStacksSeen, wantStacks, rt.Summary())
	}
}

// TestParityGLRCapPressureTopLanguages ensures we keep at least one
// conflict-heavy structural parity case in top languages that branch, and that
// the Go parse reaches the same maximum GLR width as pinned C on each one. It
// deliberately disables forest dispatch because forest-accepted parses report
// forest telemetry, not production stack pressure.
func TestParityGLRCapPressureTopLanguages(t *testing.T) {
	parityRequireExhaustive(t, "TestParityGLRCapPressureTopLanguages")
	gotreesitter.SetGLRForestEnabled(false)
	t.Cleanup(func() {
		gotreesitter.SetGLRForestEnabled(os.Getenv("GOT_GLR_FOREST") != "0")
	})

	cases := []struct {
		lang   string
		name   string
		source []byte
	}{
		{
			lang:   "java",
			name:   "obj-method-100",
			source: normalizedSource("java", string(makeJavaGLRPressureSource(100))),
		},
		{
			lang:   "c",
			name:   "decl-vs-call-200",
			source: normalizedSource("c", string(makeCGLRPressureSource(200))),
		},
		{
			lang:   "cpp",
			name:   "decl-vs-call-200",
			source: normalizedSource("cpp", string(makeCGLRPressureSource(200))),
		},
		{
			lang:   "dart",
			name:   "obj-method-200",
			source: normalizedSource("dart", string(makeDartGLRPressureSource(200))),
		},
	}

	for _, cc := range cases {
		cc := cc
		t.Run(fmt.Sprintf("%s/%s", cc.lang, cc.name), func(t *testing.T) {
			tc := parityCase{name: cc.lang, source: string(cc.source)}
			runParityCase(t, tc, "glr-cap-pressure-"+cc.name, cc.source)
			assertGLRCapPressureRuntime(t, tc, cc.source, cGLRWidth(t, tc, cc.source))
		})
	}
}
