//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestPythonGraduationDeclinedEditKeepsUnexpectedToken(t *testing.T) {
	lang := grammars.PythonLanguage()
	cl, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	source := []byte("def f():\n    return 1\n")
	changed := append(bytes.Clone(source[:7]), '/')
	changed = append(changed, source[7:]...)
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(true)
	old, err := p.Parse(source)
	ceilingGoTree(t, old, source, err)
	defer func() { old.Release() }()
	for direction, to := range [][]byte{changed, source} {
		from, oldEnd, newEnd := source, 7, 8
		if direction == 1 {
			from, oldEnd, newEnd = changed, 8, 7
		}
		old.Edit(canonicalGoInputEdit(from, to, 7, oldEnd, newEnd))
		next, err := p.ParseIncremental(to, old)
		ceilingGoTree(t, next, to, err)
		freshParser := gts.NewParser(lang)
		freshParser.SetAdmissionCandidateRoute(true)
		fresh, err := freshParser.Parse(to)
		ceilingGoTree(t, fresh, to, err)
		ct := cp.Parse(to, nil)
		if ct == nil {
			t.Fatal("C returned no tree")
		}
		if direction == 0 && !ct.RootNode().HasError() {
			t.Fatal("unexpected token witness lost its C error")
		}
		assertLockedCTreeExactWithErrors(t, "declined edit", next, lang, ct)
		assertLockedCTreeExactWithErrors(t, "declined edit fresh", fresh, lang, ct)
		ct.Close()
		fresh.Release()
		if next.RootNode().EndByte() != uint32(len(to)) {
			t.Fatal("declined edit lost source coverage")
		}
		if allocations := testing.AllocsPerRun(1, func() {
			same, err := p.ParseIncremental(to, next)
			if err != nil || same != next {
				t.Fatalf("no-edit: %v", err)
			}
			same.Release()
		}); allocations != 0 {
			t.Fatalf("no-edit allocations=%g", allocations)
		}
		old.Release()
		old = next
	}
}

func TestPythonGraduationRecoveryEditSession(t *testing.T) {
	lang := grammars.PythonLanguage()
	cl, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err = cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	sources := []string{
		"lazy import example\na,b = first, second\nf(*map(g, items))\n",
		"def f():\n    lazy from example import thing\n    a,b = first, second\n    return f(*map(g, items))\n",
		"lazy import example\na,b = first, second\nlazy from example import thing\nf(*map(g, items))\n",
	}
	steps := 0
	for fixture, text := range sources {
		t.Run(fmt.Sprint(fixture), func(t *testing.T) {
			source := []byte(text)
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			old, err := p.Parse(source)
			ceilingGoTree(t, old, source, err)
			defer func() { old.Release() }()
			ct := cp.Parse(source, nil)
			if ct == nil {
				t.Fatal("C returned no tree")
			}
			assertLockedCTreeExactWithErrors(t, "initial recovery", old, lang, ct)
			ct.Close()
			for _, site := range []string{"first", "second", "items", "example"} {
				at := bytes.Index(source, []byte(site))
				if at < 0 {
					t.Fatal("missing edit site")
				}
				for _, class := range []string{"insert", "delete", "replace"} {
					end, added := at+1, []byte("z")
					if class == "insert" {
						end = at
					}
					if class == "delete" {
						added = nil
					}
					changed := append(bytes.Clone(source[:at]), added...)
					changed = append(changed, source[end:]...)
					for direction, to := range [][]byte{changed, source} {
						from, oldEnd, newEnd := source, end, at+len(added)
						if direction != 0 {
							from, oldEnd, newEnd = changed, newEnd, oldEnd
						}
						old.Edit(canonicalGoInputEdit(from, to, at, oldEnd, newEnd))
						next, _, err := p.ParseIncrementalProfiled(to, old)
						ceilingGoTree(t, next, to, err)
						freshParser := gts.NewParser(lang)
						freshParser.SetAdmissionCandidateRoute(true)
						fresh, err := freshParser.Parse(to)
						ceilingGoTree(t, fresh, to, err)
						ct := cp.Parse(to, nil)
						if ct == nil {
							t.Fatal("C returned no tree")
						}
						assertLockedCTreeExactWithErrors(t, "fresh recovery", fresh, lang, ct)
						assertLockedCTreeExactWithErrors(t, "incremental recovery", next, lang, ct)
						got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
						if err != nil {
							t.Fatal(err)
						}
						want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
						if err != nil {
							t.Fatal(err)
						}
						if got.SHA256 != want.SHA256 {
							t.Fatal("incremental recovery differs from fresh")
						}
						if !next.ParseRuntime().CompactIncrementalFullRecoveryRoute {
							t.Fatal("edited recovery tree did not use the certified fresh route")
						}
						root := next.RootNode()
						if root.IsError() && !root.HasError() {
							t.Fatal("ERROR root lost HasError")
						}
						if root.EndByte() != uint32(len(to)) {
							t.Fatal("recovery root does not cover input")
						}
						if allocations := testing.AllocsPerRun(1, func() {
							same, err := p.ParseIncremental(to, next)
							if err != nil || same != next {
								t.Fatalf("no-edit: %v", err)
							}
							same.Release()
						}); allocations != 0 {
							t.Fatalf("no-edit allocations=%g", allocations)
						}
						ct.Close()
						fresh.Release()
						old.Release()
						old = next
						steps++
					}
				}
			}
		})
	}
	t.Logf("PYTHON_RECOVERY_SESSION_TOTAL steps=%d", steps)
}
