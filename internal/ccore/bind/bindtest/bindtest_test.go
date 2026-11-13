package bindtest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/ccoretest"
)

// TestMain checks the refusal without an adapter first, then registers an
// adapter whose language is a *ccoretest.Golden (Register may be called once).
func TestMain(m *testing.M) {
	if _, err := RecordGolden("x", nil, nil); !errors.Is(err, bind.ErrNoAdapter) {
		println("RecordGolden without adapter:", err)
		os.Exit(1)
	}
	bind.Register(bind.Adapter{
		Config: func(lang any) (ccore.Config, error) {
			g, ok := lang.(*ccoretest.Golden)
			if !ok {
				return ccore.Config{}, errors.New("stub: not a golden")
			}
			return ccore.Config{Tables: g.Tables}, nil
		},
		CheckTables: func(lang any) (int, int, error) { return 7, 0, nil },
	})
	os.Exit(m.Run())
}

func golden(t *testing.T, name string) *ccoretest.Golden {
	t.Helper()
	g, err := ccoretest.Read(filepath.Join("..", "..", "testdata", "golden"), filepath.Join("..", "..", "testdata", "tables"), name+".json")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// TestNewParsesGolden: a configured parser gives the golden's digest, and the
// wrapper's incremental path gives the fresh digest after an edit to itself.
func TestNewParsesGolden(t *testing.T) {

	g := golden(t, "json-object")
	p, err := NewParser(g)
	if err != nil {
		t.Fatal(err)
	}
	if stop := p.Parse(g.Src, nil); stop != g.Stop {
		t.Fatalf("stop %q, want %q", stop, g.Stop)
	}
	if d := p.Digest(); d != g.Digest {
		t.Fatalf("digest %s, want %s", d, g.Digest)
	}
	tree := p.Tree()
	edited := p.Edit(tree, Edit{StartByte: 1, OldEndByte: 2, NewEndByte: 2, StartCol: 1, OldEndCol: 2, NewEndCol: 2})
	again, stop := p.ParseIncremental(g.Src, edited, nil)
	if stop != "" {
		t.Fatalf("incremental stop %q", stop)
	}
	if d := p.Digest(); d != g.Digest {
		t.Fatalf("incremental digest %s, want %s", d, g.Digest)
	}
	if p.DigestOf(again, false) == "" {
		t.Fatal("DigestOf returned nothing")
	}
	if h, n := p.RecordsHash(""); n == 0 || h == 0 {
		t.Fatalf("records hash %d lines %d", h, n)
	}
}

func TestRecordGoldenAndCheckTables(t *testing.T) {

	g := golden(t, "json-object")
	rec, err := RecordGolden(g.Name, g, g.Src)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Digest != g.Digest || rec.Stop != g.Stop || rec.Tables != g.Tables || rec.RecordHash != g.RecordHash || rec.RecordCount != g.RecordCount {
		t.Fatalf("recorded golden differs: digest %s stop %q", rec.Digest, rec.Stop)
	}
	if c, d, err := bind.CheckTables(g); err != nil || c != 7 || d != 0 {
		t.Fatalf("CheckTables: %d %d %v", c, d, err)
	}
}

func TestDigestRecordNormalizesNames(t *testing.T) {
	defer NormalizeNames(false)
	NormalizeNames(false)
	a := DigestRecord(nil, `a\b`, 0, 1, 0, 0, 0, 1, true, false, false, false, 0)
	NormalizeNames(true)
	b := DigestRecord(nil, `a\b`, 0, 1, 0, 0, 0, 1, true, false, false, false, 0)
	c := DigestRecord(nil, `ab`, 0, 1, 0, 0, 0, 1, true, false, false, false, 0)
	if string(a) == string(b) {
		t.Fatal("normalization changed nothing")
	}
	if string(b) != string(c) {
		t.Fatal("normalized name differs from the plain name")
	}
}

// TestTreeLifetime: the store surface of the wrapper. Trees are copied, edited
// and released through the wrapper; the integrity counters stay at zero and the
// lease is free after the last release.
func TestTreeLifetime(t *testing.T) {
	g := golden(t, "json-object")
	p, err := NewParser(g)
	if err != nil {
		t.Fatal(err)
	}
	if stop := p.Parse(g.Src, nil); stop != g.Stop {
		t.Fatalf("stop %q, want %q", stop, g.Stop)
	}
	tree := p.Tree()
	if !tree.Valid() || tree.Handle() == 0 {
		t.Fatal("no tree after a parse")
	}
	n, fold := tree.CountNodes()
	cp := p.Copy(tree)
	if n2, f2 := cp.CountNodes(); n2 != n || f2 != fold || n == 0 {
		t.Fatalf("copy: %d nodes fold %d, want %d fold %d", n2, f2, n, fold)
	}
	p.EditInPlace(cp, Edit{StartByte: 1, OldEndByte: 2, NewEndByte: 2, StartCol: 1, OldEndCol: 2, NewEndCol: 2})
	if cp.EditRecords() == 0 {
		t.Fatal("an edit created no records")
	}
	again, stop := p.ParseIncremental(g.Src, cp, nil)
	if stop != "" || !again.Valid() {
		t.Fatalf("incremental stop %q", stop)
	}
	if d := p.Digest(); d != g.Digest {
		t.Fatalf("incremental digest %s, want %s", d, g.Digest)
	}
	p.Release(again)
	p.Release(cp)
	p.Release(tree)
	p.Release(tree) // a second release does nothing
	p.ReleaseLast()
	if p.SharedInTree() != 0 || p.DoubleFree() != 0 || p.BadRetain() != 0 {
		t.Fatalf("integrity counters: shared %d double %d bad %d", p.SharedInTree(), p.DoubleFree(), p.BadRetain())
	}
	if !p.LeaseFree() {
		t.Fatal("lease held after the last release")
	}
}
