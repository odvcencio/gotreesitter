package gotreesitter

import (
	"github.com/odvcencio/gotreesitter/internal/incr"
	"testing"
)

type tokenInvariantRollbackScanner struct{ accept rune }

func (tokenInvariantRollbackScanner) Create() any                      { return nil }
func (tokenInvariantRollbackScanner) Destroy(any)                      {}
func (tokenInvariantRollbackScanner) Serialize(any, []byte) int        { return 0 }
func (tokenInvariantRollbackScanner) Deserialize(any, []byte)          {}
func (tokenInvariantRollbackScanner) ExternalScannerIsStateless() bool { return true }
func (s tokenInvariantRollbackScanner) Scan(_ any, lexer *ExternalLexer, valid []bool) bool {
	if len(valid) == 0 || !valid[0] || lexer.Lookahead() != 'a' {
		return false
	}
	saved := *lexer
	for i := 0; i < 5; i++ {
		lexer.Advance(false)
	}
	want := s.accept
	if want == 0 {
		want = 'd'
	}
	accept := lexer.Lookahead() == want
	*lexer = saved
	if !accept {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(0)
	return true
}

func TestTokenInvariantExternalRollbackUTF8Continuation(t *testing.T) {
	oldSource, newSource := []byte("abbbb😁;"), []byte("abbbb😀;")
	d := tokenInvariantRollbackSource(oldSource)
	d.language.ExternalScanner = tokenInvariantRollbackScanner{accept: '😁'}
	defer d.Close()
	lexer := newExternalLexer(oldSource, 0, 0, 0)
	if !d.runExternalScannerWithRetry(lexer, []bool{true}) {
		t.Fatal("old scanner did not accept")
	}
	if lexer.lookaheadEndByte != 6 || d.tokenInvariantReadSpan() != 9 {
		t.Fatalf("C frontier=%d examined span=%d, want 6 and 9", lexer.lookaheadEndByte, d.tokenInvariantReadSpan())
	}
	edit := InputEdit{StartByte: 8, OldEndByte: 9, NewEndByte: 9,
		StartPoint: Point{Column: 8}, OldEndPoint: Point{Column: 9}, NewEndPoint: Point{Column: 9}}
	if _, equal := d.tokenInvariantPrimitiveEditsEquivalent(oldSource, newSource, edit, d.tokenInvariantReadSpan()); equal {
		t.Fatal("proof ignored the edited UTF-8 continuation byte")
	}
}

func TestTokenInvariantExternalObserverPoolOwnership(t *testing.T) {
	d := tokenInvariantRollbackSource([]byte("abbbbd;"))
	d.noPool = true
	d.externalLexer.reset(d.lexer.source, 0, 0, 0)
	if !d.runExternalScannerWithRetry(&d.externalLexer, []bool{true}) {
		t.Fatal("scanner did not accept")
	}
	observer := d.externalLexer.readFrontier
	d.Close()
	if len(d.externalLexer.source) != 0 || d.externalLexer.readFrontier != observer || *observer != (externalReadFrontier{}) {
		t.Fatal("Close retained source data or lost reusable observer storage")
	}
	resetPooledDFATokenSource(d)
	if d.externalLexer.readFrontier != observer {
		t.Fatal("pool reset discarded observer storage")
	}
}

func tokenInvariantRollbackSource(source []byte) *dfaTokenSource {
	lang := readSpanTestLanguage()
	lang.ExternalScanner = tokenInvariantRollbackScanner{}
	lang.ExternalSymbols = []Symbol{1}
	lang.ExternalLexStates = [][]bool{{true}}
	return newDFATokenSourceDirectWithCRecovery(NewLexer(lang.LexStates, source), lang, nil, nil, nil, nil, false)
}

func TestTokenInvariantExternalRollbackRetainsReads(t *testing.T) {
	for _, source := range []string{"abbbbd;", "abbbbc;"} {
		t.Run(source, func(t *testing.T) {
			d := tokenInvariantRollbackSource([]byte(source))
			defer d.Close()
			lexer := newExternalLexer(d.lexer.source, 0, 0, 0)
			accepted := d.runExternalScannerWithRetry(lexer, []bool{true})
			if accepted != (source[5] == 'd') {
				t.Fatal("scanner fixture selected the wrong outcome")
			}
			if got := d.tokenInvariantReadSpan(); got != 6 {
				t.Fatalf("scanner rollback lost its examined suffix: span=%d, want 6", got)
			}
		})
	}
}

func TestTokenInvariantExternalRollbackRejectsChangedDependency(t *testing.T) {
	oldSource, newSource := []byte("abbbbd;"), []byte("abbbbc;")
	d := tokenInvariantRollbackSource(oldSource)
	defer d.Close()
	lexer := newExternalLexer(oldSource, 0, 0, 0)
	if !d.runExternalScannerWithRetry(lexer, []bool{true}) {
		t.Fatal("old scanner fixture did not accept its short token")
	}
	// The scanner returns to byte 1 after examining byte 5. The proof must
	// compare its distant decision, not only its restored cursor and token.
	edit := InputEdit{StartByte: 5, OldEndByte: 6, NewEndByte: 6,
		StartPoint: Point{Column: 5}, OldEndPoint: Point{Column: 6}, NewEndPoint: Point{Column: 6}}
	if _, equal := d.tokenInvariantPrimitiveEditsEquivalent(oldSource, newSource, edit, d.tokenInvariantReadSpan()); equal {
		t.Fatal("primitive proof ignored a changed scanner decision after rollback")
	}
}

type checkpointReadProbeScanner struct {
	tokenInvariantRollbackScanner
	probe func(*ExternalLexer)
}

type checkpointOnlyReadScanner struct{ ExternalScanner }

func (checkpointOnlyReadScanner) UsesExternalScannerCheckpoints() bool { return true }

type certifiedCheckpointReadScanner struct {
	checkpointOnlyReadScanner
	reads, nonLeaf bool
}

func (s certifiedCheckpointReadScanner) SupportsCheckpointReadDependencies() bool { return s.reads }
func (s certifiedCheckpointReadScanner) SupportsCheckpointedNonLeafReuse() bool   { return s.nonLeaf }

func TestIncrementalCheckpointReadsRequireIndependentCertificate(t *testing.T) {
	base := checkpointOnlyReadScanner{tokenInvariantRollbackScanner{}}
	for _, tc := range []struct {
		name    string
		scanner ExternalScanner
		want    bool
	}{
		{"checkpoints only", base, false},
		{"explicit decline", certifiedCheckpointReadScanner{base, false, true}, false},
		{"leaf checkpoints only", certifiedCheckpointReadScanner{base, true, false}, false},
		{"native read certificate", certifiedCheckpointReadScanner{base, true, true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte("abbbbd;")
			d := tokenInvariantRollbackSource(source)
			defer d.Close()
			d.language.ExternalScanner = tc.scanner
			if got := legacyReuseReadsEligible(d, source); got != tc.want {
				t.Fatalf("native reuse eligibility=%t, want %t", got, tc.want)
			}
		})
	}
}

func (s checkpointReadProbeScanner) Scan(payload any, lexer *ExternalLexer, valid []bool) bool {
	saved := *lexer
	s.probe(lexer)
	*lexer = saved
	return s.tokenInvariantRollbackScanner.Scan(payload, lexer, valid)
}

func TestIncrementalCheckpointReadsDeclineNonlocalScans(t *testing.T) {
	for name, probe := range map[string]func(*ExternalLexer){
		"previous": func(l *ExternalLexer) { l.Previous() },
		"prefix":   func(l *ExternalLexer) { l.HasPreviousBytes("a") },
		"column":   func(l *ExternalLexer) { l.Column() },
	} {
		t.Run(name, func(t *testing.T) {
			for _, source := range []string{"abbbbd;", "abbbbc;"} {
				d := tokenInvariantRollbackSource([]byte(source))
				d.language.ExternalScanner = checkpointReadProbeScanner{probe: probe}
				d.lexer.reuseReads = incr.NewReads(len(source))
				lexer := newExternalLexer(d.lexer.source, 0, 0, 0)
				accepted := d.runExternalScannerWithRetry(lexer, []bool{true})
				if accepted != (source[5] == 'd') {
					t.Fatal("scanner selected the wrong result")
				}
				if d.lexer.reuseReads.Recording() {
					t.Fatal("rollback erased a nonlocal dependency")
				}
				d.Close()
			}
		})
	}
}

func TestIncrementalCheckpointReadsRetainForwardRollback(t *testing.T) {
	for _, source := range []string{"abbbbd;", "abbbbc;"} {
		d := tokenInvariantRollbackSource([]byte(source))
		d.lexer.reuseReads = incr.NewReads(len(source))
		lexer := newExternalLexer(d.lexer.source, 0, 0, 0)
		d.runExternalScannerWithRetry(lexer, []bool{true})
		d.lexer.reuseReads.Seal()
		if count, ok := d.lexer.reuseReads.Lookahead(1); !ok || count != 5 {
			t.Fatalf("forward rollback dependency=%d known=%t, want 5", count, ok)
		}
		d.Close()
	}
}
