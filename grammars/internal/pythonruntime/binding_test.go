package pythonruntime

import (
	"slices"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	grammarblobs "github.com/odvcencio/gotreesitter/grammars/grammar_blobs"
)

func TestPythonCanonicalExternalSymbols(t *testing.T) {
	lang, err := gotreesitter.LoadLanguage(grammarblobs.Python())
	if err != nil {
		t.Fatal(err)
	}
	scanner := PythonExternalScanner{}.ExternalScannerForLanguage(lang).(PythonExternalScanner)
	if got, want := scanner.externalToToken, []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}; !slices.Equal(got, want) {
		t.Fatalf("Python token mapping = %v, want %v", got, want)
	}
	if scanner.symbols != pyDefaultSymTable {
		t.Fatalf("Python symbols = %v, want %v", scanner.symbols, pyDefaultSymTable)
	}
}

func TestPythonExternalScannerBindsExternalSymbolsPositionally(t *testing.T) {
	// Positional binding: external index i binds to scanner token i. The Language
	// names here are a shuffled subset; positional binding maps by position.
	lang := pythonExternalBindingTestLanguage(
		"_extension_only",
		"escape_interpolation",
		"_indent",
		"string_start",
		"_newline",
	)

	scanner, ok := PythonExternalScanner{}.ExternalScannerForLanguage(lang).(PythonExternalScanner)
	if !ok {
		t.Fatalf("PythonExternalScanner binding type = %T, want PythonExternalScanner", PythonExternalScanner{}.ExternalScannerForLanguage(lang))
	}
	if got, want := scanner.externalToToken, []int{0, 1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("python externalToToken = %v, want %v", got, want)
	}
	if got, want := scanner.externalToToken[1], pyTokIndent; got != want {
		t.Fatalf("external index 1 mapped to token %d, want %d", got, want)
	}
	if got, want := scanner.symbols[pyTokIndent], gotreesitter.Symbol(2); got != want {
		t.Fatalf("token 1 result symbol = %d, want %d", got, want)
	}

	validExternal := []bool{false, true, false, false, false}
	var semanticValid [pyTokenCount]bool
	validSemantic := scanner.remapValidSymbols(validExternal, &semanticValid)
	if !validSemantic[pyTokIndent] {
		t.Fatalf("external index 1 did not become valid semantic token 1: %v", validSemantic)
	}
}

func TestPythonExternalScannerZeroValueKeepsBuiltinSymbols(t *testing.T) {
	symbols := PythonExternalScanner{}.symbolTable()
	if got, want := symbols[pyTokNewline], pySymNewline; got != want {
		t.Fatalf("zero-value newline symbol = %d, want %d", got, want)
	}
	if got, want := symbols[pyTokStringContent], pySymStringContent; got != want {
		t.Fatalf("zero-value string-content symbol = %d, want %d", got, want)
	}
}

func pythonExternalBindingTestLanguage(names ...string) *gotreesitter.Language {
	symbolNames := make([]string, len(names)+1)
	symbols := make([]gotreesitter.Symbol, len(names))
	for i, name := range names {
		symbolNames[i+1] = name
		symbols[i] = gotreesitter.Symbol(i + 1)
	}
	return &gotreesitter.Language{
		SymbolNames:     symbolNames,
		ExternalSymbols: symbols,
	}
}
