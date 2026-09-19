package grammarruntime

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// reservedWordsAttachFixture builds a minimal decoded Language plus a
// matching ReservedWordTable, so each guard test can mutate exactly one
// field away from "matches" and confirm the attach declines.
func reservedWordsAttachFixture() (*gotreesitter.Language, ReservedWordTable) {
	lang := &gotreesitter.Language{
		Name:        "reserved_words_attach_test",
		SymbolNames: []string{"end", "identifier", "if", "stmt"},
		LexModes: []gotreesitter.LexMode{
			{ReservedWordSetID: 0},
			{ReservedWordSetID: 1},
		},
	}
	table := ReservedWordTable{
		MaxSetSize:  2,
		Words:       []gotreesitter.Symbol{0, 0, 2, 0},
		SymbolCount: 4,
		SymbolNames: map[gotreesitter.Symbol]string{2: "if"},
	}
	return lang, table
}

func TestAttachRegisteredReservedWordsMatch(t *testing.T) {
	lang, table := reservedWordsAttachFixture()
	reservedWordsRegistry["reserved_words_attach_test"] = table
	t.Cleanup(func() { delete(reservedWordsRegistry, "reserved_words_attach_test") })

	if ok := attachRegisteredReservedWords("reserved_words_attach_test", lang); !ok {
		t.Fatal("attach declined on a matching table; want attach")
	}
	if lang.MaxReservedWordSetSize != 2 {
		t.Fatalf("MaxReservedWordSetSize = %d, want 2", lang.MaxReservedWordSetSize)
	}
	if len(lang.ReservedWords) != 4 || lang.ReservedWords[2] != 2 {
		t.Fatalf("ReservedWords = %v, want [0 0 2 0]", lang.ReservedWords)
	}
}

func TestAttachRegisteredReservedWordsNoTableRegistered(t *testing.T) {
	lang, _ := reservedWordsAttachFixture()
	if ok := attachRegisteredReservedWords("reserved_words_attach_test_absent", lang); ok {
		t.Fatal("attach succeeded with no table registered; want decline")
	}
	if len(lang.ReservedWords) != 0 || lang.MaxReservedWordSetSize != 0 {
		t.Fatal("attach wrote reserved-word data with no table registered")
	}
}

func TestAttachRegisteredReservedWordsAlreadyPresent(t *testing.T) {
	lang, table := reservedWordsAttachFixture()
	lang.ReservedWords = []gotreesitter.Symbol{0, 0, 2, 0}
	lang.MaxReservedWordSetSize = 2
	reservedWordsRegistry["reserved_words_attach_test"] = table
	t.Cleanup(func() { delete(reservedWordsRegistry, "reserved_words_attach_test") })

	if ok := attachRegisteredReservedWords("reserved_words_attach_test", lang); ok {
		t.Fatal("attach overwrote a language that already carries reserved-word data")
	}
}

func TestAttachRegisteredReservedWordsSymbolCountMismatch(t *testing.T) {
	lang, table := reservedWordsAttachFixture()
	table.SymbolCount = 5 // lang has 4 SymbolNames entries
	reservedWordsRegistry["reserved_words_attach_test"] = table
	t.Cleanup(func() { delete(reservedWordsRegistry, "reserved_words_attach_test") })

	if ok := attachRegisteredReservedWords("reserved_words_attach_test", lang); ok {
		t.Fatal("attach succeeded despite a symbol count mismatch; want decline")
	}
	if len(lang.ReservedWords) != 0 || lang.MaxReservedWordSetSize != 0 {
		t.Fatal("attach wrote reserved-word data despite a symbol count mismatch")
	}
}

func TestAttachRegisteredReservedWordsSymbolNameMismatch(t *testing.T) {
	lang, table := reservedWordsAttachFixture()
	lang.SymbolNames[2] = "unless" // table recorded "if" for symbol 2
	reservedWordsRegistry["reserved_words_attach_test"] = table
	t.Cleanup(func() { delete(reservedWordsRegistry, "reserved_words_attach_test") })

	if ok := attachRegisteredReservedWords("reserved_words_attach_test", lang); ok {
		t.Fatal("attach succeeded despite a symbol name mismatch; want decline")
	}
	if len(lang.ReservedWords) != 0 || lang.MaxReservedWordSetSize != 0 {
		t.Fatal("attach wrote reserved-word data despite a symbol name mismatch")
	}
}

func TestAttachRegisteredReservedWordsSetIDOutOfRange(t *testing.T) {
	lang, table := reservedWordsAttachFixture()
	lang.LexModes = append(lang.LexModes, gotreesitter.LexMode{ReservedWordSetID: 7})
	reservedWordsRegistry["reserved_words_attach_test"] = table
	t.Cleanup(func() { delete(reservedWordsRegistry, "reserved_words_attach_test") })

	if ok := attachRegisteredReservedWords("reserved_words_attach_test", lang); ok {
		t.Fatal("attach succeeded despite an out-of-range ReservedWordSetID; want decline")
	}
	if len(lang.ReservedWords) != 0 || lang.MaxReservedWordSetSize != 0 {
		t.Fatal("attach wrote reserved-word data despite an out-of-range ReservedWordSetID")
	}
}

func TestAttachRegisteredReservedWordsNilLanguage(t *testing.T) {
	if ok := attachRegisteredReservedWords("reserved_words_attach_test", nil); ok {
		t.Fatal("attach succeeded on a nil language; want decline")
	}
}
