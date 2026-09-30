package gotreesitter

import (
	"bytes"
	"testing"
)

func primitiveMemoTestSource(t testing.TB) (*Parser, *dfaTokenSource) {
	t.Helper()
	lang := primitiveProofDigitLanguage()
	// Model an immutable loaded producer without embedding a grammar fixture.
	lang.grammarBlobSHA256Valid = true
	p := NewParser(lang)
	d := newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, []byte("1")), lang, nil, nil, nil, nil, false)
	t.Cleanup(d.Close)
	return p, d
}

func primitiveMemoTestEdit(at uint32) InputEdit {
	return InputEdit{StartByte: at, OldEndByte: at + 1, NewEndByte: at + 1,
		StartPoint: Point{Column: at}, OldEndPoint: Point{Column: at + 1}, NewEndPoint: Point{Column: at + 1}}
}

func TestTokenInvariantMemoUsesOnlyCompletedDirectedProofs(t *testing.T) {
	p, d := primitiveMemoTestSource(t)
	edit := primitiveMemoTestEdit(0)
	for i, pair := range [][2]string{{"1", "2"}, {"1", "2"}, {"2", "1"}, {"2", "1"}, {"1", "2"}} {
		span, proven, hit := p.tokenInvariantPrimitiveProofCached(d, []byte(pair[0]), []byte(pair[1]), edit, 2, false)
		wantHit := i == 1 || i == 3 || i == 4
		if !proven || span != 2 || hit != wantHit {
			t.Fatalf("step=%d span=%d proven=%t hit=%t wantHit=%t", i, span, proven, hit, wantHit)
		}
	}
	if _, proven, hit := p.tokenInvariantPrimitiveProofCached(d, []byte("1"), []byte("x"), edit, 2, false); proven || hit {
		t.Fatal("changed token accepted through cache")
	}
	if _, proven, hit := p.tokenInvariantPrimitiveProofCached(d, []byte("1"), []byte("2"), edit, 2, false); !proven || !hit {
		t.Fatal("failed proof replaced a successful cache entry")
	}
}

func TestTokenInvariantMemoDeclinesCustomTablesAndExplicitBudgets(t *testing.T) {
	for _, name := range []string{"custom_producer", "custom_lexer", "memory_budget"} {
		t.Run(name, func(t *testing.T) {
			p, d := primitiveMemoTestSource(t)
			switch name {
			case "custom_producer":
				d.language.grammarBlobSHA256Valid = false
			case "custom_lexer":
				d.lexer.states = append([]LexState(nil), d.lexer.states...)
			case "memory_budget":
				p.SetMemoryBudgetBytes(1024)
			}
			for i := 0; i < 2; i++ {
				if _, proven, hit := p.tokenInvariantPrimitiveProofCached(d, []byte("1"), []byte("2"), primitiveMemoTestEdit(0), 2, false); !proven || hit {
					t.Fatalf("ordinary proof changed or declined cache was used: proven=%t hit=%t", proven, hit)
				}
			}
			if p.forestDeclineMemo != nil && p.forestDeclineMemo.tokenInvariantPrimitiveMemo != nil {
				t.Fatal("declined cache allocated storage")
			}
		})
	}
}

type primitiveMemoStatelessScanner struct{ parameter int }

func (primitiveMemoStatelessScanner) Create() any                           { return nil }
func (primitiveMemoStatelessScanner) Destroy(any)                           {}
func (primitiveMemoStatelessScanner) Serialize(any, []byte) int             { return 0 }
func (primitiveMemoStatelessScanner) Deserialize(any, []byte)               {}
func (primitiveMemoStatelessScanner) Scan(any, *ExternalLexer, []bool) bool { return false }
func (primitiveMemoStatelessScanner) ExternalScannerIsStateless() bool      { return true }

func TestTokenInvariantMemoAuthenticatesScannerParameters(t *testing.T) {
	p, d := primitiveMemoTestSource(t)
	d.language.ExternalScanner = primitiveMemoStatelessScanner{1}
	d.language.ExternalLexStates = [][]bool{{}}
	edit := primitiveMemoTestEdit(0)
	for i := 0; i < 2; i++ {
		_, proven, hit := p.tokenInvariantPrimitiveProofCached(d, []byte("1"), []byte("2"), edit, 2, false)
		if !proven || hit != (i == 1) {
			t.Fatalf("unchanged scanner: proven=%t hit=%t", proven, hit)
		}
	}
	d.language.ExternalScanner = primitiveMemoStatelessScanner{2}
	if _, proven, hit := p.tokenInvariantPrimitiveProofCached(d, []byte("1"), []byte("2"), edit, 2, false); !proven || hit {
		t.Fatalf("scanner replacement bypassed proof: proven=%t hit=%t", proven, hit)
	}
}

func FuzzTokenInvariantMemoMatchesPrimitiveProof(f *testing.F) {
	f.Add([]byte("1234 56\n"), uint32(2), uint32(4), uint32(5), byte('8'), false)
	f.Add([]byte("abc\n1\n"), uint32(4), uint32(2), uint32(0), byte('x'), true)
	f.Add([]byte("\xef\xbb\xbf1\xc2\xa0x"), uint32(3), uint32(2), uint32(6), byte(0xa1), false)
	f.Fuzz(func(t *testing.T, source []byte, at, span, mutateAt uint32, replacement byte, mutateOld bool) {
		if len(source) == 0 || len(source) > 2048 {
			return
		}
		p, d := primitiveMemoTestSource(t)
		at %= uint32(len(source))
		span = span%32 + 1
		edit := primitiveMemoTestEdit(at)
		edit.StartPoint = Point{}
		for _, ch := range source[:at] {
			if ch == '\n' {
				edit.StartPoint.Row++
				edit.StartPoint.Column = 0
			} else {
				edit.StartPoint.Column++
			}
		}
		edit.OldEndPoint = edit.StartPoint
		if source[at] == '\n' {
			edit.OldEndPoint.Row++
			edit.OldEndPoint.Column = 0
		} else {
			edit.OldEndPoint.Column++
		}
		edit.NewEndPoint = edit.OldEndPoint
		old, next := bytes.Clone(source), bytes.Clone(source)
		next[at] = replacement
		if _, proven, _ := p.tokenInvariantPrimitiveProofCached(d, old, next, edit, span, false); !proven {
			return
		}
		mutateAt %= uint32(len(source))
		if mutateOld {
			old[mutateAt] ^= 0x5a
		} else {
			next[mutateAt] ^= 0x5a
		}
		wantSpan, want := d.tokenInvariantPrimitiveEditsEquivalentWithScannerProof(old, next, edit, span, false)
		gotSpan, got, _ := p.tokenInvariantPrimitiveProofCached(d, old, next, edit, span, false)
		if got != want || gotSpan != wantSpan {
			t.Fatalf("memo differs: at=%d span=%d mutation=%d old=%t got=%d/%t want=%d/%t", at, span, mutateAt, mutateOld, gotSpan, got, wantSpan, want)
		}
	})
}
