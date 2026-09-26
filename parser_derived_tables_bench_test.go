package gotreesitter_test

import (
	"runtime"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// BenchmarkNewParserGoWarm pins the steady-state cost of constructing a Parser
// for a language that has already been used once -- the per-file cost in a
// consumer that parses many files of the same language.
//
// It exists because that cost used to be the dominant one. NewParser rebuilt
// every derived per-language table on every call; memoizing them on the
// Language cut this benchmark from about 1.30 ms to about 39 microseconds, and
// from 995,728 to 48,752 bytes. Guard the win: a regression here means some
// table went back to being rebuilt per parser.
func BenchmarkNewParserGoWarm(b *testing.B) {
	benchmarkNewParserWarm(b, grammars.GoLanguage)
}

func BenchmarkNewParserCSharpWarm(b *testing.B) {
	benchmarkNewParserWarm(b, grammars.CSharpLanguage)
}

func BenchmarkNewParserCppWarm(b *testing.B) {
	benchmarkNewParserWarm(b, grammars.CppLanguage)
}

func BenchmarkNewParserKotlinWarm(b *testing.B) {
	benchmarkNewParserWarm(b, grammars.KotlinLanguage)
}

func BenchmarkNewParserSqlWarm(b *testing.B) {
	benchmarkNewParserWarm(b, grammars.SqlLanguage)
}

func benchmarkNewParserWarm(b *testing.B, load func() *gotreesitter.Language) {
	b.Helper()
	lang := load()
	if lang == nil {
		b.Fatal("nil language")
	}
	warm := gotreesitter.NewParser(lang)
	runtime.KeepAlive(warm)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := gotreesitter.NewParser(lang)
		runtime.KeepAlive(p)
	}
}
