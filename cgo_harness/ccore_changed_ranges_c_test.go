//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestCcoreChangedRangesAgainstC compares both an incremental new tree and a
// fresh new tree against the corresponding pinned C trees, using the edited
// old tree in both comparisons. It tests ccore directly, without a public route.
func TestCcoreChangedRangesAgainstC(t *testing.T) {
	set := os.Getenv("CCORE_INC_SET")
	if set == "" {
		t.Skip("set CCORE_INC_SET to a supported changed-ranges session")
	}
	var lang string
	switch set {
	case "json-session":
		lang = "json"
	case "python-session":
		lang = "python"
	case "typescript-session":
		lang = "typescript"
	default:
		t.Fatal("set CCORE_INC_SET to json-session, python-session, or typescript-session")
	}
	gl, cl := ccoreLanguages(t, lang)
	cfg, err := bind.Config(gl)
	if err != nil {
		t.Fatal(err)
	}
	core, err := ccore.NewParser(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	checked, incrementalChecks, freshChecks, nonempty, mutated := 0, 0, 0, 0, false
	fmtCore := func(rs []ccore.Range) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = fmt.Sprintf("%d-%d(%d,%d)-(%d,%d)", r.StartByte, r.EndByte, r.StartRow, r.StartCol, r.EndRow, r.EndCol)
		}
		return out
	}
	fmtC := func(rs []sitter.Range) []string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = fmt.Sprintf("%d-%d(%d,%d)-(%d,%d)", r.StartByte, r.EndByte, r.StartPoint.Row, r.StartPoint.Column, r.EndPoint.Row, r.EndPoint.Column)
		}
		return out
	}
	check := func(id, mode string, old, next *ccore.Tree, co, cn *sitter.Tree) {
		got, ok := ccore.ChangedRanges(old, next, cfg.Tables, nil, nil)
		if !ok {
			t.Fatalf("%s %s: core declined", id, mode)
		}
		if len(got) > 0 {
			nonempty++
		}
		if os.Getenv("CCORE_MUTATE_RANGES") != "" && !mutated && len(got) > 0 {
			got[0].EndByte++
			mutated = true
		}
		want := co.ChangedRanges(cn)
		if !reflect.DeepEqual(fmtCore(got), fmtC(want)) {
			t.Errorf("%s %s: core %v != C %v", id, mode, fmtCore(got), fmtC(want))
		}
		checked++
		if mode == "incremental" {
			incrementalChecks++
		} else if mode == "fresh" {
			freshChecks++
		}
	}
	chains := ccoreIncChains(t, set)
	if len(chains) == 0 {
		t.Fatal("no edit chains")
	}
	for _, chain := range chains {
		var old *ccore.Tree
		var co *sitter.Tree
		for _, step := range chain {
			if step.edit == nil {
				var stop string
				old, stop = core.ParseWith(step.src, nil)
				co = cp.Parse(step.src, nil)
				if stop != "" || old == nil || co == nil {
					t.Fatalf("%s: fresh parse: %s", step.id, stop)
				}
				continue
			}
			old.Edit(ccore.Edit(*step.edit))
			co.Edit(ccoreSitterEdit(step.edit))
			inc, stop := core.ParseWith(step.src, old)
			if stop != "" || inc == nil {
				t.Fatalf("%s: incremental parse: %s", step.id, stop)
			}
			ci := cp.Parse(step.src, co)
			if ci == nil {
				t.Fatalf("%s: C incremental parse returned nil", step.id)
			}
			check(step.id, "incremental", old, inc, co, ci)
			fresh, stop := core.ParseWith(step.src, nil)
			cf := cp.Parse(step.src, nil)
			if stop != "" || fresh == nil || cf == nil {
				t.Fatalf("%s: fresh parse: %s", step.id, stop)
			}
			check(step.id, "fresh", old, fresh, co, cf)
			fresh.Release()
			cf.Close()
			old.Release()
			co.Close()
			old, co = inc, ci
		}
		if old != nil {
			old.Release()
		}
		if co != nil {
			co.Close()
		}
	}
	fmt.Printf("CCORECHANGEDRANGES set=%s comparisons=%d nonempty=%d mutated=%t\n", set, checked, nonempty, mutated)
	ccoreCount("TestCcoreChangedRangesAgainstC", set+":incremental", incrementalChecks)
	ccoreCount("TestCcoreChangedRangesAgainstC", set+":fresh", freshChecks)
	if checked == 0 || incrementalChecks == 0 || freshChecks == 0 || nonempty == 0 {
		t.Fatal("changed-ranges comparison did not exercise a nonempty result")
	}
	if os.Getenv("CCORE_MUTATE_RANGES") != "" && !mutated {
		t.Fatal("mutation was requested but did not run")
	}
}

// The second C parse excludes the second key/value pair and its comma.
// ccore parses an equal-length space-masked source; digest equality is
// required before comparing because ccore cannot parse included ranges.
func TestCcoreChangedRangesIncludedJSONAgainstC(t *testing.T) {
	lang := os.Getenv("CCORE_INCLUDED_RANGE_LANG")
	if lang == "" {
		t.Skip("set CCORE_INCLUDED_RANGE_LANG=json")
	}
	if lang != "json" {
		t.Fatalf("unsupported CCORE_INCLUDED_RANGE_LANG %q", lang)
	}
	gl, cl := ccoreLanguages(t, "json")
	cfg, err := bind.Config(gl)
	if err != nil {
		t.Fatal(err)
	}
	core, err := ccore.NewParser(cfg)
	if err != nil {
		t.Fatal(err)
	}
	source := []byte(`{"a":1, "b":2}`)
	old, stop := core.ParseWith(source, nil)
	if stop != "" || old == nil {
		t.Fatalf("old core parse stop=%q", stop)
	}
	defer old.Release()
	masked := append([]byte(nil), source...)
	for i := 6; i < 13; i++ {
		masked[i] = ' '
	}
	next, stop := core.ParseWith(masked, nil)
	if stop != "" || next == nil {
		t.Fatalf("new core parse stop=%q", stop)
	}
	defer next.Release()
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	mk := func(a, b uint) sitter.Range {
		return sitter.Range{StartByte: a, EndByte: b, StartPoint: sitter.Point{Column: a}, EndPoint: sitter.Point{Column: b}}
	}
	oldRanges := []sitter.Range{mk(0, 14)}
	newRanges := []sitter.Range{mk(0, 6), mk(13, 14)}
	if err := cp.SetIncludedRanges(oldRanges); err != nil {
		t.Fatal(err)
	}
	co := cp.Parse(source, nil)
	if co == nil {
		t.Fatal("old C parse nil")
	}
	defer co.Close()
	if err := cp.SetIncludedRanges(newRanges); err != nil {
		t.Fatal(err)
	}
	cn := cp.Parse(source, nil)
	if cn == nil {
		t.Fatal("new C parse nil")
	}
	defer cn.Close()
	if got, want := core.Digest(old), ccoreCDigest(co); got != want {
		t.Fatalf("old parsed trees differ: core=%s C=%s", got, want)
	}
	if got, want := core.Digest(next), ccoreCDigest(cn); got != want {
		t.Fatalf("new parsed trees differ: core=%s C=%s", got, want)
	}
	oldInc := []ccore.Range{{StartByte: 0, EndByte: 14, StartCol: 0, EndCol: 14}}
	newInc := []ccore.Range{{StartByte: 0, EndByte: 6, StartCol: 0, EndCol: 6}, {StartByte: 13, EndByte: 14, StartCol: 13, EndCol: 14}}
	got, ok := ccore.ChangedRanges(old, next, cfg.Tables, oldInc, newInc)
	if !ok {
		t.Fatal("ccore declined")
	}
	want := co.ChangedRanges(cn)
	if len(want) == 0 {
		t.Fatal("C produced no changed range for this fixture; control is not discriminating")
	}
	wantCore := make([]ccore.Range, len(want))
	for i, r := range want {
		wantCore[i] = ccore.Range{StartByte: uint32(r.StartByte), EndByte: uint32(r.EndByte), StartRow: uint32(r.StartPoint.Row), StartCol: uint32(r.StartPoint.Column), EndRow: uint32(r.EndPoint.Row), EndCol: uint32(r.EndPoint.Column)}
	}
	if !reflect.DeepEqual(got, wantCore) {
		t.Fatalf("included changes: core=%+v C=%+v", got, wantCore)
	}
	fmt.Printf("CCINCLUDEDRANGE core=%+v C=%+v\n", got, wantCore)
	ccoreCount("TestCcoreChangedRangesIncludedJSONAgainstC", "json", 1)
}
