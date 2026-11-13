package ccore_test

import (
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

func TestCrossBindingCoreTree(t *testing.T) {
	a, e := bind.New(grammars.DetectLanguageByName("json").Language())
	if e != nil {
		t.Fatal(e)
	}
	b, e := bind.New(grammars.DetectLanguageByName("python").Language())
	if e != nil {
		t.Fatal(e)
	}
	old, stop := a.Parse([]byte(`{"a":1}`))
	if stop != "" || old == nil {
		t.Fatal(stop)
	}
	defer old.Release()
	old.Edit(ccore.Edit{StartByte: 5, OldEndByte: 6, NewEndByte: 6, StartCol: 5, OldEndCol: 6, NewEndCol: 6})
	src := []byte(`{"a":2}`)
	defer func() {
		if x := recover(); x != nil {
			t.Fatalf("cross-binding core panic: %v", x)
		}
	}()
	inc, stop := b.ParseWith(src, old)
	if stop != "" || inc == nil {
		t.Fatalf("incremental stop=%q", stop)
	}
	defer inc.Release()
	incr := b.DigestOf(inc, false)
	n, z, _ := b.ReuseStats()
	fresh, stop := b.Parse(src)
	if stop != "" || fresh == nil {
		t.Fatalf("fresh stop=%q", stop)
	}
	defer fresh.Release()
	clean := b.DigestOf(fresh, false)
	fmt.Printf("FOREIGN old=json new=python incremental=%s fresh=%s equal=%t offers=%d bytes=%d\n", incr, clean, incr == clean, n, z)
	if incr != clean || n != 0 {
		t.Fatal("foreign old tree was reused or differs from fresh target language")
	}
}
