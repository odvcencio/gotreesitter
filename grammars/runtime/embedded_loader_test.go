package grammarruntime

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestRepairNoLookaheadLexModes(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	// The no-lookahead repair logic writes ^uint32(0) sentinel LexStateIndex
	// values into the last few LexModes entries (one per repaired state).
	// Use a tail-relative offset so the fixture survives blob regens that
	// add new states ahead of the sentinels. Negative `state` means
	// "len(LexModes) + state" — i.e. -4 is the fourth-from-last entry,
	// which is the first repaired sentinel slot for grammars that repair
	// four no-lookahead states.
	tests := []struct {
		name  string
		load  func() []gotreesitter.LexMode
		state int
	}{
		{
			name:  "scala",
			load:  func() []gotreesitter.LexMode { return ScalaLanguage().LexModes },
			state: -4,
		},
		{
			name:  "rust",
			load:  func() []gotreesitter.LexMode { return RustLanguage().LexModes },
			state: 3820,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lexModes := tc.load()
			idx := tc.state
			if idx < 0 {
				idx = len(lexModes) + idx
			}
			if idx < 0 || idx >= len(lexModes) {
				t.Fatalf("state %d (resolved %d) out of range for %s (len=%d)",
					tc.state, idx, tc.name, len(lexModes))
			}
			if got := lexModes[idx].LexStateIndex(); got != ^uint32(0) {
				t.Fatalf("LexModes[%d].LexStateIndex = %d, want %d", idx, got, ^uint32(0))
			}
		})
	}
}

func TestDecodeLanguageBlobDataDecodesEnvelopedLargeStateGotosTrailer(t *testing.T) {
	want := map[uint64]gotreesitter.StateID{
		uint64(70000)<<32 | 3: 80001,
		uint64(70001)<<32 | 9: 80007,
		uint64(70002)<<32 | 5: 80011,
	}
	trailer, err := gotreesitter.EncodeLargeStateGotosTrailer(want)
	if err != nil {
		t.Fatalf("EncodeLargeStateGotosTrailer: %v", err)
	}

	lang := &gotreesitter.Language{
		Name:        "embedded_c_sharp_like",
		SymbolNames: []string{"end", "identifier"},
		// The deterministic encoder clears this field from the gob payload.
	}
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(gzw).Encode(lang); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := gzw.Write(trailer); err != nil {
		t.Fatalf("write trailer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	enveloped, err := gotreesitter.WrapLanguageBlobEnvelope(buf.Bytes())
	if err != nil {
		t.Fatalf("WrapLanguageBlobEnvelope: %v", err)
	}
	if _, err := gzip.NewReader(bytes.NewReader(enveloped)); err == nil {
		t.Fatal("legacy gzip-first embedded loader accepted an enveloped blob")
	}

	decoded, err := decodeLanguageBlobData("c_sharp_like.bin", enveloped)
	if err != nil {
		t.Fatalf("decodeLanguageBlobData: %v", err)
	}
	if len(decoded.LargeStateGotos) != len(want) {
		t.Fatalf("LargeStateGotos has %d entries, want %d", len(decoded.LargeStateGotos), len(want))
	}
	for key, target := range want {
		if got := decoded.LargeStateGotos[key]; got != target {
			t.Fatalf("LargeStateGotos[%d] = %d, want %d", key, got, target)
		}
	}
}

func TestEmbeddedReduceChainHints(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	tests := []struct {
		name      string
		load      func() *gotreesitter.Language
		start     gotreesitter.StateID
		lookahead gotreesitter.Symbol
		maxSteps  uint16
	}{
		{
			name:      "python",
			load:      PythonLanguage,
			start:     gotreesitter.StateID(1101),
			lookahead: gotreesitter.Symbol(101),
			maxSteps:  10,
		},
		{
			name:      "rust",
			load:      RustLanguage,
			start:     gotreesitter.StateID(205),
			lookahead: gotreesitter.Symbol(5),
			maxSteps:  32,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.load()
			if len(lang.ReduceChainHints) != 1 {
				t.Fatalf("hint count = %d, want 1", len(lang.ReduceChainHints))
			}
			hint := lang.ReduceChainHints[0]
			if hint.StartState != tc.start || hint.Lookahead != tc.lookahead || hint.MaxSteps != tc.maxSteps {
				t.Fatalf("hint = %+v, want state=%d lookahead=%d maxSteps=%d", hint, tc.start, tc.lookahead, tc.maxSteps)
			}
			if hint.TerminalAction != gotreesitter.ReduceChainTerminalSingleShift {
				t.Fatalf("terminal action = %d, want single shift", hint.TerminalAction)
			}
			if len(hint.TerminalStates) == 0 {
				t.Fatal("expected terminal states")
			}
		})
	}
}

func TestDecodeLanguageBlobDataInfersGeneratedRepeatAuxMetadata(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount: 2,
		SymbolNames: []string{
			"end",
			"token_repeat1",
			"module_repeat1",
			"visible_repeat2",
			"named_repeat3",
		},
		SymbolMetadata: []gotreesitter.SymbolMetadata{
			{Name: "end", Named: true},
			{},
			{},
			{Visible: true},
			{Named: true},
		},
	}

	decoded, err := decodeLanguageBlobData("tiny.bin", encodeLanguageBlobForTest(t, lang))
	if err != nil {
		t.Fatalf("decodeLanguageBlobData: %v", err)
	}
	if !decoded.SymbolMetadata[2].GeneratedRepeatAux {
		t.Fatal("decodeLanguageBlobData did not infer GeneratedRepeatAux for invisible anonymous module_repeat1")
	}
	for _, idx := range []int{1, 3, 4} {
		if decoded.SymbolMetadata[idx].GeneratedRepeatAux {
			t.Fatalf("SymbolMetadata[%d].GeneratedRepeatAux = true, want false", idx)
		}
	}
}

func TestDhallUnicodeAnonymousSymbolNamesDecodeOnLoad(t *testing.T) {
	t.Cleanup(func() { PurgeEmbeddedLanguageCache() })

	lang := DhallLanguage()
	sym, ok := lang.SymbolByName("\u2192")
	if !ok {
		t.Fatal("DhallLanguage missing decoded anonymous arrow symbol")
	}
	if int(sym) >= len(lang.SymbolNames) {
		t.Fatalf("arrow symbol %d out of SymbolNames range %d", sym, len(lang.SymbolNames))
	}
	if got := lang.SymbolNames[sym]; got != "\u2192" {
		t.Fatalf("SymbolNames[%d] = %q, want decoded arrow", sym, got)
	}
	if int(sym) >= len(lang.SymbolMetadata) {
		t.Fatalf("arrow symbol %d out of SymbolMetadata range %d", sym, len(lang.SymbolMetadata))
	}
	meta := lang.SymbolMetadata[sym]
	if meta.Name != "\u2192" || !meta.Visible || meta.Named {
		t.Fatalf("arrow metadata = %+v, want visible anonymous decoded arrow", meta)
	}
	if _, ok := lang.SymbolByName(`\u2192`); ok {
		t.Fatal("DhallLanguage still exposes escaped arrow symbol name")
	}
}

func TestDecodeDhallAnonymousUnicodeEscapesPreservesInvalidEscapes(t *testing.T) {
	cases := []string{
		`\uZZZZ`,
		`\u219`,
		`\uD83D`,
		`\u{}`,
	}
	for _, tc := range cases {
		if got := decodeDhallAnonymousUnicodeEscapes(tc); got != tc {
			t.Fatalf("decodeDhallAnonymousUnicodeEscapes(%q) = %q, want unchanged", tc, got)
		}
	}
}
