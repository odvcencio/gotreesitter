//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCcoreCppScannerChangedRangesMatchC(t *testing.T) {
	gl, cl := ccoreLanguages(t, "cpp")
	if gl.ExternalScanner == nil {
		t.Fatal("expected external scanner")
	}
	cfg, e := bind.Config(gl)
	if e != nil {
		t.Fatal(e)
	}
	p, e := ccore.NewParser(cfg)
	if e != nil {
		t.Fatal(e)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if e = cp.SetLanguage(cl); e != nil {
		t.Fatal(e)
	}
	for _, c := range []struct{ name, old, marker, replacement string }{{"raw_string", "const char *s = R\"tag(hi)tag\";", "hi", "hello"}, {"structural", "int x = 1;", "1", "\"abc\""}} {
		t.Run(c.name, func(t *testing.T) {
			at := strings.Index(c.old, c.marker)
			if at < 0 {
				t.Fatal("missing marker")
			}
			next := c.old[:at] + c.replacement + c.old[at+len(c.marker):]
			old, stop := p.ParseWith([]byte(c.old), nil)
			co := cp.Parse([]byte(c.old), nil)
			if stop != "" || old == nil || co == nil {
				t.Fatalf("old stop=%s", stop)
			}
			defer old.Release()
			defer co.Close()
			if p.DigestOf(old, false) != ccoreCDigest(co) {
				t.Fatal("initial tree differs from C")
			}
			ed := ccore.Edit{StartByte: uint32(at), OldEndByte: uint32(at + len(c.marker)), NewEndByte: uint32(at + len(c.replacement)), StartCol: uint32(at), OldEndCol: uint32(at + len(c.marker)), NewEndCol: uint32(at + len(c.replacement))}
			old.Edit(ed)
			ce := sitter.InputEdit{StartByte: uint(at), OldEndByte: uint(at + len(c.marker)), NewEndByte: uint(at + len(c.replacement)), StartPosition: sitter.Point{Column: uint(at)}, OldEndPosition: sitter.Point{Column: uint(at + len(c.marker))}, NewEndPosition: sitter.Point{Column: uint(at + len(c.replacement))}}
			co.Edit(&ce)
			ni, stop := p.ParseWith([]byte(next), old)
			ci := cp.Parse([]byte(next), co)
			if stop != "" || ni == nil || ci == nil {
				t.Fatalf("incremental stop=%s", stop)
			}
			defer ni.Release()
			defer ci.Close()
			if p.DigestOf(ni, false) != ccoreCDigest(ci) {
				t.Fatal("incremental tree differs from C")
			}
			nf, stop := p.ParseWith([]byte(next), nil)
			cf := cp.Parse([]byte(next), nil)
			if stop != "" || nf == nil || cf == nil {
				t.Fatalf("fresh stop=%s", stop)
			}
			defer nf.Release()
			defer cf.Close()
			if p.DigestOf(nf, false) != ccoreCDigest(cf) {
				t.Fatal("fresh tree differs from C")
			}
			check := func(mode string, a *ccore.Tree, b *sitter.Tree) {
				got, ok := ccore.ChangedRanges(old, a, cfg.Tables, nil, nil)
				if !ok {
					t.Fatalf("%s declined", mode)
				}
				var gr, cr []string
				for _, r := range got {
					gr = append(gr, fmt.Sprintf("%d-%d(%d,%d)-(%d,%d)", r.StartByte, r.EndByte, r.StartRow, r.StartCol, r.EndRow, r.EndCol))
				}
				for _, r := range co.ChangedRanges(b) {
					cr = append(cr, fmt.Sprintf("%d-%d(%d,%d)-(%d,%d)", r.StartByte, r.EndByte, r.StartPoint.Row, r.StartPoint.Column, r.EndPoint.Row, r.EndPoint.Column))
				}
				t.Logf("%s core=%v C=%v", mode, gr, cr)
				if !reflect.DeepEqual(gr, cr) {
					t.Fatal("ranges differ")
				}
			}
			check("incremental", ni, ci)
			check("fresh", nf, cf)
		})
	}
}
