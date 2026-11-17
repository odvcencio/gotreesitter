package benchfixtures

import (
	"reflect"
	"testing"
)

func TestCleanCandidatesAndBookkeeping(t *testing.T) {
	src := []byte("alpha 1234\nbeta word\n")
	var g CleanEditGenerator
	for _, kind := range []string{"space+", "same", "type", "del"} {
		a := g.Candidates(src, kind, 40, 99)
		if len(a) == 0 || !reflect.DeepEqual(a, g.Candidates(src, kind, 40, 99)) {
			t.Fatalf("%s candidates: %v", kind, a)
		}
		for _, c := range a {
			if _, err := g.Apply(src, c); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := g.Candidates(src, "space-", 40, 0); len(got) != 0 {
		t.Fatalf("original space eligible: %v", got)
	}
	plus := g.Candidates(src, "space+", 40, 0)[0]
	step, err := g.Apply(src, plus)
	if err != nil {
		t.Fatal(err)
	}
	g.Accept(plus)
	before := CleanCandidate{Kind: "type", At: 1}
	step2, err := g.Apply(step.Source, before)
	if err != nil {
		t.Fatal(err)
	}
	g.Accept(before)
	minus := g.Candidates(step2.Source, "space-", 40, 0)
	if len(minus) != 1 || minus[0].At != plus.At+1 {
		t.Fatalf("tracked space: %v", minus)
	}
	restored, err := g.Apply(step2.Source, minus[0])
	if err != nil {
		t.Fatal(err)
	}
	g.Accept(minus[0])
	if len(restored.Source) != len(src)+1 || len(g.Candidates(restored.Source, "space-", 40, 0)) != 0 {
		t.Fatal("space removal failed")
	}
}

func TestCleanCandidateRules(t *testing.T) {
	src := []byte("a ab abc abcd 89\n")
	var gen CleanEditGenerator
	positions := func(kind string) map[int]bool {
		out := map[int]bool{}
		for _, c := range gen.Candidates(src, kind, 100, 0) {
			out[c.At] = true
		}
		return out
	}
	same, typed, deleted := positions("same"), positions("type"), positions("del")
	for i, b := range src {
		if (b == '8' || b == '9') && !same[i] {
			t.Fatalf("digit at %d omitted", i)
		}
		if (i == 0 || i == 2 || i == 3) && (same[i] || typed[i] || deleted[i]) {
			t.Fatalf("short identifier accepted at %d", i)
		}
	}
	if len(typed) != 3 || len(deleted) != 1 {
		t.Fatalf("typed=%v deleted=%v", typed, deleted)
	}
	for _, kind := range []string{"space+", "same", "type", "del"} {
		if n := len(gen.Candidates(src, kind, 2, 3)); n > 2 {
			t.Fatalf("%s returned %d", kind, n)
		}
	}
}

func TestCleanEditConstruction(t *testing.T) {
	src := []byte("abc1\n")
	for _, tc := range []struct {
		kind           string
		at             int
		want           string
		oldEnd, newEnd uint32
	}{
		{"space+", 4, "abc1 \n", 4, 5},
		{"same", 3, "abc2\n", 4, 4},
		{"type", 1, "axbc1\n", 1, 2},
		{"del", 1, "ac1\n", 2, 1},
	} {
		var gen CleanEditGenerator
		step, err := gen.Apply(src, CleanCandidate{Kind: tc.kind, At: tc.at})
		if err != nil {
			t.Fatal(err)
		}
		if string(step.Source) != tc.want || step.Edit.StartByte != uint32(tc.at) || step.Edit.OldEndByte != tc.oldEnd || step.Edit.NewEndByte != tc.newEnd || step.Edit.StartPoint.Row != 0 || step.Edit.StartPoint.Column != uint32(tc.at) {
			t.Fatalf("%s: %#v, source %q", tc.kind, step.Edit, step.Source)
		}
	}
}
