package gotreesitter_test

import (
	"reflect"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCRecoveryAllEnablesExternalScannerGrammarsWithLexStates(t *testing.T) {
	tests := []struct {
		name string
		load func() *gotreesitter.Language
	}{
		{name: "wgsl", load: grammars.WgslLanguage},
		{name: "angular", load: grammars.AngularLanguage},
		{name: "jsonnet", load: grammars.JsonnetLanguage},
		{name: "caddy", load: grammars.CaddyLanguage},
		{name: "cooklang", load: grammars.CooklangLanguage},
		{name: "kconfig", load: grammars.KconfigLanguage},
		// Restored curated-12: hand-written scanners that regained their
		// ExternalLexStates tables (see grammars/*_external_lex_states_gen.go).
		{name: "awk", load: grammars.AwkLanguage},
		{name: "bicep", load: grammars.BicepLanguage},
		{name: "bitbake", load: grammars.BitbakeLanguage},
		{name: "css", load: grammars.CssLanguage},
		{name: "cue", load: grammars.CueLanguage},
		{name: "dtd", load: grammars.DtdLanguage},
		{name: "hack", load: grammars.HackLanguage},
		{name: "hlsl", load: grammars.HlslLanguage},
		{name: "jsdoc", load: grammars.JsdocLanguage},
		{name: "just", load: grammars.JustLanguage},
		{name: "luau", load: grammars.LuauLanguage},
		{name: "svelte", load: grammars.SvelteLanguage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOT_C_RECOVERY", "all")
			gotreesitter.ResetParseEnvConfigCacheForTests()
			t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)
			lang := tt.load()
			if len(lang.ExternalSymbols) == 0 {
				t.Fatal("ExternalSymbols is empty")
			}
			if len(lang.ExternalLexStates) == 0 {
				t.Fatal("ExternalLexStates is empty")
			}
			parser := gotreesitter.NewParser(lang)
			if !parserCRecoveryEnabledForExternalTest(parser) {
				t.Fatal("NewParser did not enable C recovery cost competition under GOT_C_RECOVERY=all")
			}

			t.Setenv("GOT_C_RECOVERY", "other,"+tt.name)
			gotreesitter.ResetParseEnvConfigCacheForTests()
			t.Cleanup(gotreesitter.ResetParseEnvConfigCacheForTests)
			parser = gotreesitter.NewParser(tt.load())
			if !parserCRecoveryEnabledForExternalTest(parser) {
				t.Fatal("NewParser did not enable C recovery cost competition under named GOT_C_RECOVERY override")
			}
		})
	}
}

func parserCRecoveryEnabledForExternalTest(parser *gotreesitter.Parser) bool {
	if parser == nil {
		return false
	}
	v := reflect.ValueOf(parser).Elem().FieldByName("errorCostCompetition")
	return v.IsValid() && v.Kind() == reflect.Bool && v.Bool()
}
