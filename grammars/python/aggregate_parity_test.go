package python_test

import (
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	standalonepython "github.com/odvcencio/gotreesitter/grammars/python"
)

func TestRuntimeProfileMatchesAggregatePython(t *testing.T) {
	standalone := standalonepython.Language()
	aggregate := grammars.PythonLanguage()
	if standalone.Name != aggregate.Name {
		t.Fatalf("standalone name = %q, aggregate name = %q", standalone.Name, aggregate.Name)
	}
	if !reflect.DeepEqual(standalone.ExternalLexStates, aggregate.ExternalLexStates) {
		t.Fatal("standalone external lex states differ from aggregate Python")
	}
	if standalone.ExternalScannerFullParseRetryPolicy != aggregate.ExternalScannerFullParseRetryPolicy {
		t.Fatal("standalone retry policy differs from aggregate Python")
	}
	standaloneCompact := [...]bool{
		standalone.CompactConvergedReductionSplitDropsCertified,
		standalone.CompactPrimaryAcceptanceDerivationCertified,
		standalone.CompactAcceptanceStructuralElectionCertified,
		standalone.CompactMixedGSSMergeCertified,
	}
	aggregateCompact := [...]bool{
		aggregate.CompactConvergedReductionSplitDropsCertified,
		aggregate.CompactPrimaryAcceptanceDerivationCertified,
		aggregate.CompactAcceptanceStructuralElectionCertified,
		aggregate.CompactMixedGSSMergeCertified,
	}
	if standaloneCompact != aggregateCompact {
		t.Fatalf("standalone compact profile = %v, aggregate profile = %v", standaloneCompact, aggregateCompact)
	}
}
