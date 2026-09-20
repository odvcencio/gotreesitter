package grammars

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func TestLookupExternalScanner(t *testing.T) {
	// Registered scanners should be found.
	for _, name := range []string{"python", "css", "javascript", "html", "yaml"} {
		if s := LookupExternalScanner(name); s == nil {
			t.Errorf("LookupExternalScanner(%q) = nil, want non-nil", name)
		}
	}

	// Non-existent scanner should return nil.
	if s := LookupExternalScanner("nonexistent_language_xyz"); s != nil {
		t.Errorf("LookupExternalScanner(%q) = %v, want nil", "nonexistent_language_xyz", s)
	}
}

func TestLookupExternalLexStates(t *testing.T) {
	// scss, yaml, python, and generated sidecars have registered external lex states.
	for _, name := range []string{"python", "scss", "yaml", "wgsl", "angular", "jsonnet", "caddy", "cooklang", "kconfig"} {
		if els := LookupExternalLexStates(name); els == nil {
			t.Errorf("LookupExternalLexStates(%q) = nil, want non-nil", name)
		}
	}

	if els := LookupExternalLexStates("nonexistent_language_xyz"); els != nil {
		t.Errorf("LookupExternalLexStates(%q) = %v, want nil", "nonexistent_language_xyz", els)
	}
}

func TestGeneratedExternalLexStatesAttachToExternalScannerLanguages(t *testing.T) {
	tests := []struct {
		name       string
		load       func() *gotreesitter.Language
		wantRows   int
		wantCols   int
		checkState int
		checkCols  []int
	}{
		{
			name:       "wgsl",
			load:       WgslLanguage,
			wantRows:   2,
			wantCols:   1,
			checkState: 1,
			checkCols:  []int{0},
		},
		{
			name:       "angular",
			load:       AngularLanguage,
			wantRows:   14,
			wantCols:   13,
			checkState: 1,
			checkCols:  []int{0, 1, 2, 3, 4, 6, 7, 8, 9, 10, 11, 12},
		},
		{
			name:       "jsonnet",
			load:       JsonnetLanguage,
			wantRows:   5,
			wantCols:   3,
			checkState: 1,
			checkCols:  []int{0, 1, 2},
		},
		{
			name:       "caddy",
			load:       CaddyLanguage,
			wantRows:   3,
			wantCols:   3,
			checkState: 1,
			checkCols:  []int{0, 1, 2},
		},
		{
			name:       "cooklang",
			load:       CooklangLanguage,
			wantRows:   2,
			wantCols:   1,
			checkState: 1,
			checkCols:  []int{0},
		},
		{
			name:       "kconfig",
			load:       KconfigLanguage,
			wantRows:   2,
			wantCols:   1,
			checkState: 1,
			checkCols:  []int{0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registered := LookupExternalLexStates(tt.name)
			if len(registered) != tt.wantRows {
				t.Fatalf("registered rows = %d, want %d", len(registered), tt.wantRows)
			}
			if got := len(registered[0]); got != tt.wantCols {
				t.Fatalf("registered columns = %d, want %d", got, tt.wantCols)
			}
			for _, col := range tt.checkCols {
				if !registered[tt.checkState][col] {
					t.Fatalf("registered[%d][%d] = false, want true; row=%v", tt.checkState, col, registered[tt.checkState])
				}
			}

			lang := tt.load()
			if len(lang.ExternalSymbols) != tt.wantCols {
				t.Fatalf("ExternalSymbols = %d, want %d", len(lang.ExternalSymbols), tt.wantCols)
			}
			if len(lang.ExternalLexStates) != tt.wantRows {
				t.Fatalf("attached rows = %d, want %d", len(lang.ExternalLexStates), tt.wantRows)
			}
			if got := len(lang.ExternalLexStates[0]); got != tt.wantCols {
				t.Fatalf("attached columns = %d, want %d", got, tt.wantCols)
			}
			for state := 0; state < int(lang.StateCount) && state < len(lang.LexModes); state++ {
				if int(lang.LexModes[state].ExternalLexState) >= len(lang.ExternalLexStates) {
					t.Fatalf("LexModes[%d].ExternalLexState = %d beyond %d rows",
						state, lang.LexModes[state].ExternalLexState, len(lang.ExternalLexStates))
				}
			}
		})
	}
}

func TestAdaptScannerForLanguageNilTarget(t *testing.T) {
	if AdaptScannerForLanguage("css", nil) {
		t.Fatal("expected false for nil target language")
	}
}

func TestAdaptScannerForLanguagePreservesExistingExternalLexStates(t *testing.T) {
	ref := YamlLanguage()
	target := &gotreesitter.Language{
		ExternalSymbols: append([]gotreesitter.Symbol(nil), ref.ExternalSymbols...),
		ExternalLexStates: [][]bool{
			{false, false},
			{true, false},
		},
	}

	if !AdaptScannerForLanguage("yaml", target) {
		t.Fatal("AdaptScannerForLanguage(yaml) returned false")
	}
	if target.ExternalScanner == nil {
		t.Fatal("expected external scanner to be attached")
	}
	if len(target.ExternalLexStates) != 2 {
		t.Fatalf("len(ExternalLexStates) = %d, want 2", len(target.ExternalLexStates))
	}
	if !target.ExternalLexStates[1][0] || target.ExternalLexStates[1][1] {
		t.Fatalf("ExternalLexStates was overwritten: %+v", target.ExternalLexStates)
	}
}

func TestAdaptScannerForGeneratedLanguagePreservesGeneratedExternalLexStates(t *testing.T) {
	ref := YamlLanguage()
	symbolCount := maxSymbolForTest(ref.ExternalSymbols) + 1
	externalRow := make([]bool, len(ref.ExternalSymbols))
	externalRow[0] = true
	target := &gotreesitter.Language{
		GeneratedByGrammargen:                    true,
		CRecoveryCostCompetitionCapable:          true,
		CRecoveryCostCompetitionEnabledByDefault: false,
		InitialState:                             1,
		StateCount:                               2,
		SymbolCount:                              uint32(symbolCount),
		TokenCount:                               uint32(symbolCount),
		SymbolNames:                              make([]string, symbolCount),
		SymbolMetadata:                           make([]gotreesitter.SymbolMetadata, symbolCount),
		ParseTable: [][]uint16{
			make([]uint16, symbolCount),
			make([]uint16, symbolCount),
		},
		ParseActions: []gotreesitter.ParseActionEntry{
			{},
			{Actions: []gotreesitter.ParseAction{{Type: gotreesitter.ParseActionRecover, State: 0}}},
		},
		LexModes:        []gotreesitter.LexMode{{LexState: 0}, {LexState: 0}},
		LexStates:       []gotreesitter.LexState{{Default: -1, EOF: -1}},
		ExternalSymbols: append([]gotreesitter.Symbol(nil), ref.ExternalSymbols...),
		ExternalLexStates: [][]bool{
			make([]bool, len(ref.ExternalSymbols)),
			externalRow,
		},
	}
	registered := LookupExternalLexStates("yaml")
	if len(registered) == 0 {
		t.Fatal("yaml registered ExternalLexStates missing")
	}

	if !AdaptScannerForLanguage("yaml", target) {
		t.Fatal("AdaptScannerForLanguage(yaml) returned false")
	}
	if len(target.ExternalLexStates) == len(registered) {
		t.Fatalf("len(ExternalLexStates) = registered len %d, want existing generated rows preserved", len(registered))
	}
	if len(target.ExternalLexStates) != 2 {
		t.Fatalf("len(ExternalLexStates) = %d, want existing generated len 2", len(target.ExternalLexStates))
	}
	if !target.ExternalLexStates[1][0] || target.ExternalLexStates[1][1] {
		t.Fatalf("ExternalLexStates was overwritten: %+v", target.ExternalLexStates)
	}
	if !target.CRecoveryCostCompetitionEnabledByDefault {
		t.Fatal("generated capable language was not default-certified after ExternalLexStates attachment")
	}
}

func maxSymbolForTest(symbols []gotreesitter.Symbol) int {
	max := 0
	for _, sym := range symbols {
		if int(sym) > max {
			max = int(sym)
		}
	}
	return max
}
