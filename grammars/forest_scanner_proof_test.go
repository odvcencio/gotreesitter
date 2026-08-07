package grammars

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

// TestForestDownstreamScannerProofs keeps the downstream forest witnesses on a
// proven scanner route instead of silently disabling later incremental reuse.
func TestForestDownstreamScannerProofs(t *testing.T) {
	cases := []struct {
		name       string
		stateless  bool
		checkpoint bool
	}{
		{name: "toml", stateless: true},
		{name: "css", stateless: true},
		{name: "tsx", stateless: true},
		{name: "scss", stateless: true},
		{name: "typescript", stateless: true},
		{name: "cmake", checkpoint: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang := &gts.Language{Name: tc.name}
			if !attachExternalScannerForLanguage(tc.name, lang) || lang.ExternalScanner == nil {
				t.Fatal("scanner did not attach")
			}
			reusable, ok := lang.ExternalScanner.(gts.IncrementalReuseExternalScanner)
			if !ok || !reusable.SupportsIncrementalReuse() {
				t.Fatalf("scanner %T does not certify incremental reuse", lang.ExternalScanner)
			}
			_, isStateless := lang.ExternalScanner.(gts.StatelessExternalScanner)
			if isStateless != tc.stateless {
				t.Fatalf("stateless proof = %t, want %t", isStateless, tc.stateless)
			}
			_, isCheckpointed := lang.ExternalScanner.(gts.CheckpointedExternalScanner)
			if isCheckpointed != tc.checkpoint {
				t.Fatalf("checkpoint proof = %t, want %t", isCheckpointed, tc.checkpoint)
			}
		})
	}
}

func TestForestDownstreamAdmissionPolicy(t *testing.T) {
	safe := []struct {
		name string
		load func() *gts.Language
	}{
		{name: "toml", load: TomlLanguage},
		{name: "css", load: CssLanguage},
		{name: "tsx", load: TsxLanguage},
		{name: "ini", load: IniLanguage},
		{name: "scss", load: ScssLanguage},
		{name: "typescript", load: TypescriptLanguage},
		{name: "cmake", load: CmakeLanguage},
	}
	for _, tc := range safe {
		lang := *tc.load()
		lang.WantsForest = true
		if !gts.LanguageWantsForest(&lang) {
			t.Errorf("proven explicit route %q was rejected", tc.name)
		}
	}

	unsafe := []struct {
		name string
		load func() *gts.Language
	}{
		{name: "haskell", load: HaskellLanguage},
		{name: "python", load: PythonLanguage},
		{name: "c_sharp", load: CSharpLanguage},
	}
	for _, tc := range unsafe {
		lang := *tc.load()
		lang.WantsForest = true
		if gts.LanguageWantsForest(&lang) {
			t.Errorf("unproven explicit route %q reached normal admission", tc.name)
		}
	}
}
