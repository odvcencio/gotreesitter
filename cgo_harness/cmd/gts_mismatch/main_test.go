//go:build cgo && treesitter_c_parity

package main

import (
	"bytes"
	"testing"

	cgoharness "github.com/odvcencio/gotreesitter/cgo_harness"
)

func TestDDMinFindsMinimalSubset(t *testing.T) {
	src := []byte("aaaa(bbbb)cccc[dddd]")
	// Interesting while both '(' and ']' remain.
	got := shrinkBytes(src, func(candidate []byte) bool {
		return bytes.IndexByte(candidate, '(') >= 0 && bytes.IndexByte(candidate, ']') >= 0
	})
	if string(got) != "(]" {
		t.Fatalf("shrinkBytes = %q, want %q", got, "(]")
	}
}

func TestDDMinKeepsWholeInputWhenNothingCanGo(t *testing.T) {
	units := []int{1, 2, 3}
	got := ddmin(units, func(c []int) bool { return len(c) == 3 })
	if len(got) != 3 {
		t.Fatalf("ddmin removed required units: %v", got)
	}
}

func TestDiffEdit(t *testing.T) {
	cases := []struct {
		before, after         string
		start, oldEnd, newEnd int
	}{
		{"abc", "abxc", 2, 2, 3},
		{"abc", "ac", 1, 2, 1},
		{"abc", "axc", 1, 2, 2},
		{"aaa", "aaaa", 3, 3, 4},
		{"héllo", "hxllo", 1, 3, 2},
	}
	for _, tc := range cases {
		s, oe, ne := diffEdit([]byte(tc.before), []byte(tc.after))
		if s != tc.start || oe != tc.oldEnd || ne != tc.newEnd {
			t.Errorf("diffEdit(%q, %q) = %d,%d,%d; want %d,%d,%d", tc.before, tc.after, s, oe, ne, tc.start, tc.oldEnd, tc.newEnd)
		}
		b := []byte(tc.before)
		a := []byte(tc.after)
		rebuilt := applyEdit(b, s, oe, a[s:ne])
		if !bytes.Equal(rebuilt, a) {
			t.Errorf("applyEdit(diffEdit(%q, %q)) = %q", tc.before, tc.after, rebuilt)
		}
	}
}

func TestSelectEditMatchesReceiptSites(t *testing.T) {
	src := []byte("0123456789abcdef")
	if s, oe, r := selectEdit(src, "insert", 0); s != 0 || oe != 0 || string(r) != "x" {
		t.Fatalf("insert site 0 = %d,%d,%q", s, oe, r)
	}
	if s, oe, r := selectEdit(src, "insert", 15); s != 16 || oe != 16 || string(r) != "x" {
		t.Fatalf("insert site 15 = %d,%d,%q", s, oe, r)
	}
	if s, oe, r := selectEdit(src, "replace", 4); s != 4 || oe != 5 || string(r) != "x" {
		t.Fatalf("replace site 4 = %d,%d,%q", s, oe, r)
	}
	if s, oe, r := selectEdit([]byte("xx"), "replace", 0); s != 0 || oe != 1 || string(r) != "y" {
		t.Fatalf("replace of x = %d,%d,%q", s, oe, r)
	}
	if s, oe, r := selectEdit(src, "delete", 8); s != 8 || oe != 9 || r != nil {
		t.Fatalf("delete site 8 = %d,%d,%q", s, oe, r)
	}
}

func TestDivergenceSignatureIgnoresOffsets(t *testing.T) {
	a := &cgoharness.DumpV1Divergence{Path: "/program/list[3]/pipeline[0]", Category: "range", GoValue: "0:0-1:2 @0..9", CValue: "0:0-1:3 @0..10"}
	b := &cgoharness.DumpV1Divergence{Path: "/program/pipeline[0]", Category: "range", GoValue: "0:0-0:2 @0..2", CValue: "0:0-0:3 @0..3"}
	if divergenceSignature(a) != divergenceSignature(b) {
		t.Fatalf("range signatures differ: %q vs %q", divergenceSignature(a), divergenceSignature(b))
	}
	c := &cgoharness.DumpV1Divergence{Path: "/program/pipeline[0]", Category: "type", GoValue: "pipeline", CValue: "redirected_statement"}
	if got, want := divergenceSignature(c), "type:pipeline|redirected_statement@pipeline"; got != want {
		t.Fatalf("type signature = %q, want %q", got, want)
	}
	if divergenceSignature(nil) != "digest-only" {
		t.Fatalf("nil divergence signature = %q", divergenceSignature(nil))
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name      string
		cHasError bool
		out       RouteOutcome
		want      string
	}{
		{"match", false, RouteOutcome{Match: true}, "match"},
		{"recovery go clean", true, RouteOutcome{StopReason: "accepted"}, "recovery:c-error-go-clean"},
		{"recovery both", true, RouteOutcome{StopReason: "accepted", HasError: true}, "recovery:both-error"},
		{"go error on valid", false, RouteOutcome{StopReason: "accepted", HasError: true}, "valid:go-error"},
		{"external", false, RouteOutcome{StopReason: "accepted", Leaf: &LeafDiff{GoExternal: true}}, "valid:token-external-scanner"},
		{"token type", false, RouteOutcome{StopReason: "accepted", Leaf: &LeafDiff{SameSpan: true}}, "valid:token-type"},
		{"structure", false, RouteOutcome{StopReason: "accepted", Divergence: &cgoharness.DumpV1Divergence{Category: "shape"}}, "valid:structure-shape"},
		{"timeout", false, RouteOutcome{StopReason: "timeout"}, "go-stopped:timeout"},
	}
	for _, tc := range cases {
		if got := classify(tc.cHasError, tc.out); got != tc.want {
			t.Errorf("%s: classify = %q, want %q", tc.name, got, tc.want)
		}
	}
}
