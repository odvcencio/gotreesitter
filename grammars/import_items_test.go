package grammars

import (
	"reflect"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestImportItems(t *testing.T) {
	type item struct {
		kind, text, name, binding string
		typeOnly                  bool
	}
	tests := []struct {
		file, src string
		want      []item
	}{
		{"a.py", "from .π import (f as α,\r\n  # note\r\n  f as β, z)\n", []item{{"named", "f as α", "f", "α", false}, {"named", "f as β", "f", "β", false}, {"named", "z", "z", "z", false}}},
		{"a.py", "import a.b, c.d as e", []item{{"module", "a.b", "a.b", "a", false}, {"module", "c.d as e", "c.d", "e", false}}},
		{"a.py", "from __future__ import annotations", []item{{"named", "annotations", "annotations", "annotations", false}}},
		{"a.py", "from . import *", []item{{"wildcard", "*", "*", "", false}}},
		{"a.js", "import d, * as ns from 'lib'", []item{{"default", "d", "", "d", false}, {"namespace", "* as ns", "*", "ns", false}}},
		{"a.ts", "import Foo = require('foo');", []item{{"module", "Foo = require('foo')", "", "Foo", false}}},
		{"a.tsx", "import type Foo = require('foo');", []item{{"module", "Foo = require('foo')", "", "Foo", true}}},
		{"a.ts", "import F = NS.Foo;", nil},
		{"a.ts", "import { type T as U, type as value, X } from 'm'", []item{{"named", "type T as U", "T", "U", true}, {"named", "type as value", "type", "value", false}, {"named", "X", "X", "X", false}}},
		{"a.tsx", "export type {T as U} from 'lib'", []item{{"named", "T as U", "T", "U", true}}},
		{"a.ts", "export type * as ns from 'lib'", []item{{"namespace", "* as ns", "*", "ns", true}}},
		{"a.js", "export * from 'm'", []item{{"wildcard", "*", "*", "", false}}},
		{"a.js", "import 'lib'", nil},
		{"a.ts", "export {T as U}", nil},
	}
	for _, tc := range tests {
		t.Run(tc.src, func(t *testing.T) {
			tree, err := ParseFile(tc.file, []byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			stmt := tree.RootNode().NamedChild(0)
			var got []item
			var prev uint32
			text := func(r gts.Range) string { return tc.src[r.StartByte:r.EndByte] }
			for _, v := range ImportItems(stmt, tree.Language()) {
				if v.Range.StartByte < prev {
					t.Fatalf("overlapping items: %+v", v)
				}
				prev = v.Range.EndByte
				for _, r := range []gts.Range{v.NameRange, v.BindingRange} {
					if r != (gts.Range{}) && (r.StartByte < v.Range.StartByte || r.EndByte > v.Range.EndByte) {
						t.Fatalf("name outside its item: %+v", v)
					}
				}
				got = append(got, item{v.Kind, text(v.Range), text(v.NameRange), text(v.BindingRange), v.TypeOnly})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
