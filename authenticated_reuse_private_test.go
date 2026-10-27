//go:build gts_parsercorephase0 || gts_workcount || gts_diag

package gotreesitter

import (
	"bytes"
	"fmt"
	"testing"
)

// RunAuthenticatedNativeReuseWitness exercises the real default Go producer,
// then compares the bypassed tree with a separate fresh parse.
func RunAuthenticatedNativeReuseWitness(t *testing.T, language *Language) {
	var b bytes.Buffer
	b.WriteString("package main\n\nimport \"fmt\"\n\n")
	for i := 0; b.Len() < 137*1024; i++ {
		fmt.Fprintf(&b, "func f%d(a int, b int) int {\n\tx := a + b\n\tfmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i)
	}
	source := b.Bytes()
	at := bytes.Index(source, []byte("x := a + b"))
	if at < 0 {
		t.Fatal("identifier fixture missing")
	}
	point := Point{}
	for _, c := range source[:at] {
		if c == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	after := append(append(append([]byte(nil), source[:at]...), 'x'), source[at:]...)
	parser := NewParser(language)
	old, err := parser.Parse(source)
	if err != nil || old == nil || old.RootNode().HasError() {
		t.Fatalf("initial parse: %v", err)
	}
	defer old.Release()
	key := old.arena.legacyReuseReads.DefaultProducerSchedule(len(source))
	if key == 0 {
		t.Fatal("default Go producer lacks authority")
	}
	old.Edit(InputEdit{StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1), StartPoint: point, OldEndPoint: point, NewEndPoint: Point{Row: point.Row, Column: point.Column + 1}})
	bypasses, verifiers := 0, 0
	trustedGoReuseShadowObserver.Lock()
	trustedGoReuseShadowObserver.call = func(event trustedGoReuseShadowEvent) {
		if event.parser == parser {
			if event.bypassed {
				bypasses++
			} else {
				verifiers++
			}
		}
	}
	trustedGoReuseShadowObserver.Unlock()
	defer func() {
		trustedGoReuseShadowObserver.Lock()
		trustedGoReuseShadowObserver.call = nil
		trustedGoReuseShadowObserver.Unlock()
	}()
	result, err := parser.ParseIncremental(after, old)
	if err != nil || result == nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer result.Release()
	if bypasses != 1 || verifiers != 0 || !result.hasCertifiedIncrementalReuse() || result.incrementalFreshVerified() {
		t.Fatalf("bypasses=%d verifiers=%d certified=%t fresh=%t", bypasses, verifiers, result.hasCertifiedIncrementalReuse(), result.incrementalFreshVerified())
	}
	fresh, err := NewParser(language).Parse(after)
	if err != nil || fresh == nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer fresh.Release()
	if !incrementalTreesStructurallyEqual(result, fresh, language) {
		t.Fatal("bypassed tree differs from fresh parse")
	}
	type pair struct{ a, b *Node }
	stack := []pair{{result.RootNode(), fresh.RootNode()}}
	for len(stack) > 0 {
		q := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if q.a.ParseState() != q.b.ParseState() || q.a.PreGotoState() != q.b.PreGotoState() || q.a.dynamicPrecedence != q.b.dynamicPrecedence {
			t.Fatalf("state mismatch at %d", q.a.StartByte())
		}
		for i := 0; i < q.a.ChildCount(); i++ {
			stack = append(stack, pair{q.a.Child(i), q.b.Child(i)})
		}
	}
}
