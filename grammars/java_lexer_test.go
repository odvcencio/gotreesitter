//go:build !grammar_subset || grammar_subset_java

package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type countedJavaRebuilder struct {
	*JavaTokenSource
	rebuilds *int
}

func (ts *countedJavaRebuilder) RebuildTokenSource(source []byte, lang *gotreesitter.Language) (gotreesitter.TokenSource, error) {
	(*ts.rebuilds)++
	return ts.JavaTokenSource.RebuildTokenSource(source, lang)
}

func TestJavaEOFCommentAppendUsesEditedFreshWitness(t *testing.T) {
	lang := JavaLanguage()
	for _, candidate := range []bool{false, true} {
		for _, profiled := range []bool{false, true} {
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(candidate)
			source := append([]byte("class Main {\n"), bytes.Repeat([]byte("int x = 2;\n"), 40)...)
			source = append(source, []byte("}\n// ")...)
			old, err := parser.ParseWithTokenSource(source, NewJavaTokenSourceOrEOF(source, lang))
			if err != nil || old == nil || old.RootNode().HasError() {
				t.Fatalf("initial parse: %v", err)
			}
			column := uint32(3)
			for _, b := range []byte("w5_typing_0123456\n") {
				edited := append(bytes.Clone(source), b)
				old.Edit(gotreesitter.InputEdit{StartByte: uint32(len(source)), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(edited)),
					StartPoint: gotreesitter.Point{Row: 42, Column: column}, OldEndPoint: gotreesitter.Point{Row: 42, Column: column},
					NewEndPoint: func() gotreesitter.Point {
						if b == '\n' {
							return gotreesitter.Point{Row: 43}
						}
						return gotreesitter.Point{Row: 42, Column: column + 1}
					}()})
				base, _ := NewJavaTokenSource(edited, lang)
				rebuilds := 0
				tokens := &countedJavaRebuilder{base, &rebuilds}
				var next *gotreesitter.Tree
				if profiled {
					next, _, err = parser.ParseIncrementalWithTokenSourceProfiled(edited, old, tokens)
				} else {
					next, err = parser.ParseIncrementalWithTokenSource(edited, old, tokens)
				}
				old.Release()
				if err != nil {
					t.Fatal(err)
				}
				fresh, err := gotreesitter.NewParser(lang).ParseWithTokenSource(edited, NewJavaTokenSourceOrEOF(edited, lang))
				if err != nil {
					t.Fatal(err)
				}
				got, gotErr := benchfixtures.InspectGoTree(next.RootNode(), lang)
				want, wantErr := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
				fresh.Release()
				if gotErr != nil || wantErr != nil || got.SHA256 != want.SHA256 || next.RootNode().EndByte() != uint32(len(edited)) {
					t.Fatalf("append %q differs from fresh parsing: %v %v", b, gotErr, wantErr)
				}
				if b != '\n' && rebuilds != 0 {
					t.Fatalf("certified append %q rebuilt a fresh stream %d times", b, rebuilds)
				}
				if b == '\n' && rebuilds == 0 {
					t.Fatal("newline bypassed fresh verification")
				}
				old, source = next, edited
				column++
			}
			old.Release()
		}
	}
}

func TestJavaEOFCommentAppendKeepsSourceSensitiveMergeVerification(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)
	source := []byte("class Main {}\n// @interfac")
	old, err := parser.ParseWithTokenSource(source, NewJavaTokenSourceOrEOF(source, lang))
	if err != nil {
		t.Fatal(err)
	}
	defer old.Release()
	edited := append(bytes.Clone(source), 'e')
	column := uint32(len("// @interfac"))
	old.Edit(gotreesitter.InputEdit{StartByte: uint32(len(source)), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(edited)),
		StartPoint: gotreesitter.Point{Row: 1, Column: column}, OldEndPoint: gotreesitter.Point{Row: 1, Column: column},
		NewEndPoint: gotreesitter.Point{Row: 1, Column: column + 1}})
	base, err := NewJavaTokenSource(edited, lang)
	if err != nil {
		t.Fatal(err)
	}
	rebuilds := 0
	next, err := parser.ParseIncrementalWithTokenSource(edited, old, &countedJavaRebuilder{base, &rebuilds})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Release()
	if rebuilds == 0 {
		t.Fatal("a source-sensitive fresh merge-width change bypassed verification")
	}
	fresh, err := gotreesitter.NewParser(lang).ParseWithTokenSource(edited, NewJavaTokenSourceOrEOF(edited, lang))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Release()
	got, gotErr := benchfixtures.InspectGoTree(next.RootNode(), lang)
	want, wantErr := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if gotErr != nil || wantErr != nil || got.SHA256 != want.SHA256 {
		t.Fatal("merge-width change differs from fresh parsing")
	}
}

func TestJavaEOFCommentAppendKeepsExplicitStopVerification(t *testing.T) {
	var cancellation uint32
	for name, configure := range map[string]func(*gotreesitter.Parser){
		"memory":       func(p *gotreesitter.Parser) { p.SetMemoryBudgetBytes(16 << 20) },
		"timeout":      func(p *gotreesitter.Parser) { p.SetTimeoutMicros(1 << 30) },
		"cancellation": func(p *gotreesitter.Parser) { p.SetCancellationFlag(&cancellation) },
	} {
		t.Run(name, func(t *testing.T) {
			lang := JavaLanguage()
			parser := gotreesitter.NewParser(lang)
			configure(parser)
			source := []byte("class Main {}\n// x")
			old, err := parser.ParseWithTokenSource(source, NewJavaTokenSourceOrEOF(source, lang))
			if err != nil {
				t.Fatal(err)
			}
			defer old.Release()
			edited := append(bytes.Clone(source), 'y')
			old.Edit(gotreesitter.InputEdit{StartByte: uint32(len(source)), OldEndByte: uint32(len(source)), NewEndByte: uint32(len(edited)),
				StartPoint: gotreesitter.Point{Row: 1, Column: 4}, OldEndPoint: gotreesitter.Point{Row: 1, Column: 4},
				NewEndPoint: gotreesitter.Point{Row: 1, Column: 5}})
			base, err := NewJavaTokenSource(edited, lang)
			if err != nil {
				t.Fatal(err)
			}
			rebuilds := 0
			next, err := parser.ParseIncrementalWithTokenSource(edited, old, &countedJavaRebuilder{base, &rebuilds})
			if err != nil {
				t.Fatal(err)
			}
			defer next.Release()
			if rebuilds == 0 {
				t.Fatal("explicit stop controls bypassed fresh verification")
			}
			freshParser := gotreesitter.NewParser(lang)
			configure(freshParser)
			fresh, err := freshParser.ParseWithTokenSource(edited, NewJavaTokenSourceOrEOF(edited, lang))
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			got, gotErr := benchfixtures.InspectGoTree(next.RootNode(), lang)
			want, wantErr := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
			if gotErr != nil || wantErr != nil || got.SHA256 != want.SHA256 {
				t.Fatal("controlled append differs from fresh parsing")
			}
		})
	}
}
func TestNewJavaTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewJavaTokenSource([]byte("class Main { int x; }\n"), lang); err == nil {
		t.Fatal("expected error for language missing java token symbols")
	}
}

func TestNewJavaTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewJavaTokenSourceOrEOF([]byte("class Main { int x; }\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestJavaTokenSourceSkipToByte(t *testing.T) {
	lang := JavaLanguage()
	src := []byte("class Main {\n  int x = 1;\n  int y = 2;\n}\n")
	target := bytes.Index(src, []byte("y"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(target))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < target {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, target)
	}
	if tok.Text != "y" {
		t.Fatalf("expected token text %q, got %q", "y", tok.Text)
	}
}

func TestJavaTokenSourceRebuildPreservesPendingStream(t *testing.T) {
	lang := JavaLanguage()
	ts, err := NewJavaTokenSource([]byte(`"old"`), lang)
	if err != nil {
		t.Fatal(err)
	}
	ts.Next()
	rebuilt, err := ts.RebuildTokenSource([]byte("class Fresh {}"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if token := rebuilt.Next(); token.Text != "class" || token.StartByte != 0 {
		t.Fatalf("rebuilt stream did not start fresh: %+v", token)
	}
	if token := ts.Next(); token.Text != "old" {
		t.Fatalf("rebuilding changed the pending original stream: %+v", token)
	}
	if _, err := ts.RebuildTokenSource(nil, &gotreesitter.Language{}); err == nil {
		t.Fatal("rebuilding ignored the supplied language")
	}
}

func TestJavaTokenSourceIncrementalReuseHasFreshVerifier(t *testing.T) {
	lang := JavaLanguage()
	source := append([]byte("class Main { int target = 1; "), bytes.Repeat([]byte("int x = 2; "), 160)...)
	source = append(source, '}')
	offset := bytes.Index(source, []byte("= 1")) + 2
	for _, candidate := range []bool{false, true} {
		parser := gotreesitter.NewParser(lang)
		parser.SetAdmissionCandidateRoute(candidate)
		old, err := parser.ParseWithTokenSource(source, NewJavaTokenSourceOrEOF(source, lang))
		if err != nil {
			t.Fatal(err)
		}
		for _, digit := range []byte{'3', ';', '1'} {
			edited := bytes.Clone(source)
			edited[offset] = digit
			old.Edit(gotreesitter.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1),
				StartPoint: gotreesitter.Point{Column: uint32(offset)}, OldEndPoint: gotreesitter.Point{Column: uint32(offset + 1)}, NewEndPoint: gotreesitter.Point{Column: uint32(offset + 1)}})
			next, profile, err := parser.ParseIncrementalWithTokenSourceProfiled(edited, old, NewJavaTokenSourceOrEOF(edited, lang))
			old.Release()
			if err != nil {
				t.Fatal(err)
			}
			old = next
			fresh, err := gotreesitter.NewParser(lang).ParseWithTokenSource(edited, NewJavaTokenSourceOrEOF(edited, lang))
			if err != nil {
				t.Fatal(err)
			}
			if next.RootNode().HasError() != fresh.RootNode().HasError() || next.RootNode().EndByte() != uint32(len(edited)) || next.RootNode().SExpr(lang) != fresh.RootNode().SExpr(lang) {
				t.Fatal("incremental Java tree differs from fresh parsing")
			}
			fresh.Release()
			if digit == '3' && (profile.ReuseUnsupported || profile.ReusedSubtrees == 0) {
				t.Fatalf("candidate=%t declined Java reuse: %+v", candidate, profile)
			}
		}
		old.Release()
	}
}

func TestJavaTokenSourceZeroLongLiteralIsDecimal(t *testing.T) {
	lang := JavaLanguage()
	src := []byte("0L")
	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}
	want, ok := lang.SymbolByName("decimal_integer_literal")
	if !ok {
		t.Fatal("missing decimal_integer_literal symbol")
	}

	tok := ts.Next()
	if tok.Symbol != want {
		gotName := "<unknown>"
		if int(tok.Symbol) < len(lang.SymbolNames) {
			gotName = lang.SymbolNames[tok.Symbol]
		}
		t.Fatalf("token symbol = %d (%s), want %d (decimal_integer_literal)", tok.Symbol, gotName, want)
	}
	if tok.Text != "0L" {
		t.Fatalf("token text = %q, want %q", tok.Text, "0L")
	}
}

func TestParseJavaWithTokenSource(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("class Main { int x; }\n")
	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected java parse without syntax errors")
	}
}
