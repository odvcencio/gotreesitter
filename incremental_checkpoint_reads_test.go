package gotreesitter

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/incr"
)

type checkpointReadProbeScanner struct {
	tokenInvariantRollbackScanner
	probe func(*ExternalLexer)
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
