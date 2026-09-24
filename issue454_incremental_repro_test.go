package gotreesitter_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func issue454JS() []byte {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "function fn%d(a, b) {\n\tvar x%d = a + b;\n\treturn x%d;\n}\n\n", i, i, i)
	}
	return []byte(b.String())
}

func issue454Diff() []byte {
	var b strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&b, "diff --git a/f%d.txt b/f%d.txt\n--- a/f%d.txt\n+++ b/f%d.txt\n@@ -1,2 +1,2 @@\n-old\n+new\n", i, i, i, i)
	}
	return []byte(b.String())
}

func issue454Less() []byte {
	var b strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&b, ".rule%d {\n  padding: 10px;\n  color: red;\n}\n", i)
	}
	return []byte(b.String())
}

func issue454Step(t *testing.T, p *gts.Parser, lang *gts.Language, old *gts.Tree, before, after []byte) *gts.Tree {
	t.Helper()
	start := 0
	for start < len(before) && start < len(after) && before[start] == after[start] {
		start++
	}
	suffix := 0
	for suffix < len(before)-start && suffix < len(after)-start && before[len(before)-suffix-1] == after[len(after)-suffix-1] {
		suffix++
	}
	endOld, endNew := len(before)-suffix, len(after)-suffix
	old.Edit(gts.InputEdit{StartByte: uint32(start), OldEndByte: uint32(endOld), NewEndByte: uint32(endNew), StartPoint: incrementalEditPoint(before, uint32(start)), OldEndPoint: incrementalEditPoint(before, uint32(endOld)), NewEndPoint: incrementalEditPoint(after, uint32(endNew))})
	inc, profile, err := p.ParseIncrementalProfiled(after, old)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := p.Parse(after)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	if d := issue454FirstDivergence(lang, fresh.RootNode(), inc.RootNode()); d != nil {
		t.Errorf("edit at %d: divergence=%v freshError=%v incError=%v reused=%d bytes=%d", start, d, fresh.RootNode().HasError(), inc.RootNode().HasError(), profile.ReusedSubtrees, profile.ReusedBytes)
		t.Logf("profile: stop=%s reason=%s tokens=%d reused=%d", profile.StopReason, profile.ReuseUnsupportedReason, profile.TokensConsumed, profile.ReusedSubtrees)
	}
	return inc
}

func TestIssue454TransientErrorSequence(t *testing.T) {
	lang := grammars.JavascriptLanguage()
	src := issue454JS()
	at := strings.Index(string(src), "x0")
	quoted := append(append([]byte{}, src[:at]...), append([]byte{'"'}, src[at:]...)...)
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%v", compact), func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			old, err := p.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			middle := issue454Step(t, p, lang, old, src, quoted)
			if !middle.RootNode().HasError() {
				t.Fatal("quote insertion must create a transient error")
			}
			old.Release()
			last := issue454Step(t, p, lang, middle, quoted, src)
			if last.RootNode().HasError() {
				t.Fatal("quote deletion must repair the transient error")
			}
			middle.Release()
			last.Release()
		})
	}
}

func TestIssue454DiffQuoteDelete(t *testing.T) {
	lang := grammars.DiffLanguage()
	src := issue454Diff()
	at := strings.Index(string(src), "--- a/f76.txt")
	quoted := append(append([]byte{}, src[:at+2]...), append([]byte{'"'}, src[at+2:]...)...)
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%v", compact), func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			old, err := p.Parse(quoted)
			if err != nil {
				t.Fatal(err)
			}
			if old.RootNode().HasError() {
				t.Fatal("quoted diff must parse without an error")
			}
			next := issue454Step(t, p, lang, old, quoted, src)
			if next.RootNode().HasError() {
				t.Fatal("repaired diff must parse without an error")
			}
			next.Release()
			old.Release()
		})
	}
}

func TestIssue454LessSlash(t *testing.T) {
	lang := grammars.LessLanguage()
	src := issue454Less()
	at := strings.Index(string(src), "padding:") + len("padding:")
	slashed := append(append([]byte{}, src[:at]...), append([]byte{'/'}, src[at:]...)...)
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%v", compact), func(t *testing.T) {
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			old, err := p.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			next := issue454Step(t, p, lang, old, src, slashed)
			if !next.RootNode().HasError() {
				t.Fatal("slash insertion must create an error")
			}
			next.Release()
			old.Release()
		})
	}
}

func TestIssue454RandomSingleByteEdits(t *testing.T) {
	for _, tc := range []struct {
		name string
		lang *gts.Language
		src  []byte
	}{
		{"javascript", grammars.JavascriptLanguage(), issue454JS()},
		{"diff", grammars.DiffLanguage(), issue454Diff()},
		{"less", grammars.LessLanguage(), issue454Less()},
		{"toml", grammars.TomlLanguage(), issue454Toml()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("compact=%v", compact), func(t *testing.T) {
					rng := rand.New(rand.NewSource(454))
					p := gts.NewParser(tc.lang)
					p.SetAdmissionCandidateRoute(compact)
					old, err := p.Parse(tc.src)
					if err != nil {
						t.Fatal(err)
					}
					defer old.Release()
					src := tc.src
					for i := 0; i < 72; i++ {
						at := rng.Intn(len(src))
						var next []byte
						switch i % 3 {
						case 0:
							next = append(append([]byte{}, src[:at]...), append([]byte{'"'}, src[at:]...)...)
						case 1:
							next = append(append([]byte{}, src[:at]...), src[at+1:]...)
						case 2:
							next = append([]byte{}, src...)
							next[at] = '/'
						}
						t.Run(fmt.Sprintf("step-%d-byte-%d", i, at), func(t *testing.T) {
							newer := issue454Step(t, p, tc.lang, old, src, next)
							old.Release()
							old = newer
						})
						src = next
					}
				})
			}
		})
	}
}

// This 72-step session reconstructs the reported edit classes. The reporter's
// original TOML script was not included with the issue report.
func TestIssue454TomlEditingSession(t *testing.T) {
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%v", compact), func(t *testing.T) {
			lang := grammars.TomlLanguage()
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(compact)
			source := issue454Toml()
			old, err := p.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { old.Release() }()
			step := 0
			apply := func(after []byte) {
				t.Helper()
				step++
				t.Run(fmt.Sprintf("step-%02d", step), func(t *testing.T) {
					next := issue454Step(t, p, lang, old, source, after)
					old.Release()
					old = next
				})
				source = after
			}
			baseValue := "0"
			var lastBlock string
			for cycle := 0; cycle < 12; cycle++ {
				if cycle%2 == 0 {
					lastBlock = fmt.Sprintf("[session%d]\nvalue = %d\n", cycle, cycle)
					apply(append(append([]byte{}, source...), lastBlock...))
				} else {
					apply([]byte(strings.Replace(string(source), lastBlock, "", 1)))
				}
				clean := append([]byte{}, source...)
				marker := "x0 = " + baseValue
				at := strings.Index(string(source), marker)
				if at < 0 {
					t.Fatal("session marker not found")
				}
				switch cycle % 3 {
				case 0:
					apply(append(append([]byte{}, source[:at]...), append([]byte{'"'}, source[at:]...)...))
				case 1:
					apply(append(append([]byte{}, source[:at]...), append([]byte{'}'}, source[at:]...)...))
				case 2:
					equals := at + len("x0 ")
					apply(append(append([]byte{}, source[:equals]...), source[equals+1:]...))
				}
				apply(clean)
				nextValue := "1"
				if baseValue == "1" {
					nextValue = "0"
				}
				apply([]byte(strings.Replace(string(source), marker, "x0 = "+nextValue, 1)))
				baseValue = nextValue
				apply(append(append([]byte{}, source...), fmt.Sprintf("half%d = ", cycle)...))
				apply(append(append([]byte{}, source...), []byte("\"done\"\n")...))
			}
			if step != 72 {
				t.Fatalf("session steps = %d, want 72", step)
			}
		})
	}
}

func issue454Toml() []byte {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "[section%d]\nx0 = %d\nname = \"f%d\"\nenabled = true\n\n", i, i, i)
	}
	return []byte(b.String())
}

// issue454FirstDivergence also checks flags excluded by the clean-only corpus gate.
func issue454FirstDivergence(lang *gts.Language, fresh, inc *gts.Node) *incrGateDivergence {
	if d := incrGateFirstDivergence(lang, fresh, inc, nil); d != nil {
		return d
	}
	var check func(*gts.Node, *gts.Node) *incrGateDivergence
	check = func(a, b *gts.Node) *incrGateDivergence {
		if a.HasError() != b.HasError() || a.IsError() != b.IsError() {
			return &incrGateDivergence{kind: "errorFlags", nodeType: a.Type(lang)}
		}
		if a.StartPoint() != b.StartPoint() || a.EndPoint() != b.EndPoint() {
			return &incrGateDivergence{kind: "pointRange", nodeType: a.Type(lang)}
		}
		if a.IsExtra() != b.IsExtra() {
			return &incrGateDivergence{kind: "extraFlag", nodeType: a.Type(lang)}
		}
		for i := 0; i < a.ChildCount(); i++ {
			if d := check(a.Child(i), b.Child(i)); d != nil {
				return d
			}
		}
		return nil
	}
	return check(fresh, inc)
}
