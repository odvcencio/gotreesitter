//go:build cgo && treesitter_c_parity && gts_parsercorephase0 && !gts_no_parsercorephase0

package cgoharness

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestKDLAbsorbedLeafErrorsLockedC(t *testing.T) {
	for _, size := range [][2]int{{3, 3}, {120, 300}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) { testKDLAbsorbedLeafErrorsLockedC(t, size[0], size[1]) })
	}
}

func testKDLAbsorbedLeafErrorsLockedC(t *testing.T, nodes, repeats int) {
	var body strings.Builder
	for i := 0; i < nodes; i++ {
		fmt.Fprintf(&body, "node%d \"arg%d\" key=%d {\n  child%d \"x\"\n}\n", i, i, i, i)
	}
	valid := body.String()
	var out strings.Builder
	out.WriteString(valid[:int(float64(len(valid))*0.7)])
	for i := 0; i < repeats; i++ {
		fmt.Fprintf(&out, " }} \"unterminated garbage ][ %d==%d<<>>", i, i*7)
	}
	source := []byte(out.String())
	cl, err := ParityCLanguage("kdl")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	oracle := cp.Parse(source, nil)
	if oracle == nil {
		t.Fatal("nil C tree")
	}
	defer oracle.Close()
	lang := grammars.KdlLanguage()
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprint(compact), func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			tree, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			if !tree.ParseRuntime().CRecoveryEnteredErrorState {
				t.Fatal("parse did not enter C recovery")
			}
			assertG18LockedCExact(t, "KDL garbage", tree, lang, oracle)
		})
	}
}
