package ccore_test

import (
	"fmt"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

func TestForeignTableIdentityFallback(t *testing.T) {
	lang := grammars.DetectLanguageByName("json").Language()
	cfg, e := bind.Config(lang)
	if e != nil {
		t.Fatal(e)
	}
	same, e := bind.Config(lang)
	if e != nil {
		t.Fatal(e)
	}
	if cfg.Tables != same.Tables {
		t.Fatal("expected same cached table")
	}
	p, e := ccore.NewParser(cfg)
	if e != nil {
		t.Fatal(e)
	}
	old, stop := p.Parse([]byte(`{"a":1,"b":2}`))
	if stop != "" {
		t.Fatal(stop)
	}
	defer old.Release()
	copy := old.Copy()
	defer copy.Release()
	old.Edit(ccore.Edit{StartByte: 5, OldEndByte: 6, NewEndByte: 6, StartCol: 5, OldEndCol: 6, NewEndCol: 6})
	src := []byte(`{"a":9,"b":2}`)
	other := *cfg.Tables
	different := cfg
	different.Tables = &other
	for _, tc := range []struct {
		name  string
		cfg   ccore.Config
		reuse bool
	}{{"shared_same_language", same, true}, {"equal_but_distinct_tables", different, false}} {
		t.Run(tc.name, func(t *testing.T) {
			q, e := ccore.NewParser(tc.cfg)
			if e != nil {
				t.Fatal(e)
			}
			next, stop := q.ParseWith(src, old)
			if stop != "" || next == nil {
				t.Fatal(stop)
			}
			defer next.Release()
			n, z, _ := q.ReuseStats()
			d := q.DigestOf(next, false)
			fresh, stop := q.Parse(src)
			if stop != "" {
				t.Fatal(stop)
			}
			defer fresh.Release()
			eq := d == q.DigestOf(fresh, false)
			fmt.Printf("TABLE case=%s equal=%t offers=%d bytes=%d\n", tc.name, eq, n, z)
			if !eq || (n > 0) != tc.reuse {
				t.Fatal("unexpected binding route")
			}
		})
	}

	old.Release()
	q, e := ccore.NewParser(same)
	if e != nil {
		t.Fatal(e)
	}
	n, stop := q.ParseWith(src, old)
	if n != nil || stop != "old tree released" {
		t.Fatal("released tree accepted")
	}
	if copy.Root() == 0 {
		t.Fatal("copy was invalidated")
	}
}
