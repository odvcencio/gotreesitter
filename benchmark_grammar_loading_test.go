package gotreesitter_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

func BenchmarkGrammarLanguageWarm(b *testing.B) {
	grammars.GoLanguage()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		grammars.GoLanguage()
	}
}
