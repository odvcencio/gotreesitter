//go:build gts_parsercorephase0

package gotreesitter

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/incr"
)

func TestCompactReadHistoryDropsCapacityForSmallerInput(t *testing.T) {
	reads := incr.NewReads(100000)
	for i := 0; i < 100000; i++ {
		reads.Record(i, uint32(i+1))
	}
	before := reads.Bytes()
	if before < 100000 {
		t.Fatal("large control history was not allocated")
	}
	d := compactReuseDependencies{reads: reads}
	s := diagnosticParserCoreGenericScheduler{reuseDependencies: d.reset(), tokenSource: &dfaTokenSource{language: &Language{}, lexer: NewLexer(nil, []byte("a"))}}
	s.beginCompactCReads()
	if !s.reuseDependencies.reads.Recording() || s.reuseDependencies.reads.Bytes() >= before {
		t.Fatal("the smaller parse retained oversized history")
	}
}
