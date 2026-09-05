//go:build gts_parsercorephase0 && !gts_no_parsercorephase0

package gotreesitter_test

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// This target uses fresh legacy Go parsing, not locked C, as its oracle.
func FuzzTokenInvariantTypeScriptNumericFreshParity(f *testing.F) {
	contexts := [][2]string{
		{"const n = ", ";\n"},
		{"type N = Box<Box<", ">>;\n"},
		{"foo<", ">(2);\n"},
		{"const n = <number>", ";\n"},
		{"const n = ", " >> 2;\n"},
		{"class C { get x() { return ", "; } set x(v: number) {} }\n"},
		{"const n = (", " as number);\n"},
		{"type N = ", ";\n"},
	}
	language := grammars.TypescriptLanguage()
	for _, seed := range []struct {
		spelling                     string
		context, offset, replacement uint8
	}{
		{"1", 0, 0, '2'}, {"1.5", 1, 2, '6'}, {"1e1", 2, 2, '2'},
		{"0x1", 3, 2, '2'}, {"8", 4, 0, '9'}, {"0.5", 5, 2, '6'},
		{"1_000", 6, 4, '1'}, {"0b1", 7, 2, '0'},
		{"0b1", 0, 2, '2'}, {"1e1", 0, 2, '+'},
	} {
		f.Add([]byte(seed.spelling), seed.context, seed.offset, seed.replacement)
	}
	f.Fuzz(func(t *testing.T, spelling []byte, context, offset, replacement uint8) {
		if len(spelling) == 0 || len(spelling) > 16 || int(offset) >= len(spelling) {
			t.Skip("outside the bounded edit domain")
		}
		frame := contexts[int(context)%len(contexts)]
		source := []byte(frame[0] + string(spelling) + frame[1])
		position := len(frame[0]) + int(offset)
		edited := append([]byte(nil), source...)
		edited[position] = replacement
		oracleParser := gts.NewParser(language)
		oracleParser.SetAdmissionCandidateRoute(false)
		initial, err := oracleParser.Parse(source)
		if err != nil || initial == nil {
			t.Fatalf("initial oracle: %v", err)
		}
		malformed := initial.RootNode().HasError()
		initial.Release()
		if malformed {
			t.Skip("initial source is malformed")
		}
		fresh, err := oracleParser.Parse(edited)
		if err != nil || fresh == nil {
			t.Fatalf("fresh oracle: %v", err)
		}
		defer fresh.Release()
		want, err := benchfixtures.InspectGoTree(fresh.RootNode(), language)
		if err != nil {
			t.Fatal(err)
		}
		edit := gts.InputEdit{
			StartByte: uint32(position), OldEndByte: uint32(position + 1), NewEndByte: uint32(position + 1),
			StartPoint: pointForOffset(source, position), OldEndPoint: pointForOffset(source, position+1), NewEndPoint: pointForOffset(edited, position+1),
		}
		for _, compact := range []bool{false, true} {
			func() {
				parser := gts.NewParser(language)
				parser.SetAdmissionCandidateRoute(compact)
				old, err := parser.Parse(source)
				if err != nil || old == nil {
					t.Fatalf("initial compact=%t: %v", compact, err)
				}
				defer old.Release()
				if old.RootNode().HasError() {
					t.Fatalf("compact=%t rejected clean initial source %q", compact, source)
				}
				root := old.RootNode()
				old.Edit(edit)
				next, profile, err := parser.ParseIncrementalProfiled(edited, old)
				if err != nil || next == nil {
					t.Fatalf("incremental compact=%t: %v", compact, err)
				}
				if next != old {
					defer next.Release()
				}
				if source[position] != replacement && next.RootNode() == root && profile.ReparseNanos == 0 && profile.NewNodesAllocated == 0 && profile.TokenInvariantDependencyChecks == 0 {
					t.Fatalf("whole-root shortcut omitted dependency proof: %+v", profile)
				}
				actual, err := benchfixtures.InspectGoTree(next.RootNode(), language)
				if err != nil {
					t.Fatal(err)
				}
				if actual.SHA256 != want.SHA256 {
					t.Fatalf("compact=%t source=%q edited=%q incremental=%s fresh=%s", compact, source, edited, next.RootNode().SExpr(language), fresh.RootNode().SExpr(language))
				}
			}()
		}
	})
}
