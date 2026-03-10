package gotreesitter

import "testing"

func TestExternalSymbolResolverNil(t *testing.T) {
	r := NewExternalSymbolResolver(nil)
	if r != nil {
		t.Fatal("expected nil resolver for nil language")
	}
	// All methods must be safe on nil receiver.
	if _, ok := r.ByName("foo"); ok {
		t.Fatal("expected false for nil receiver ByName")
	}
	if _, ok := r.ByIndex(0); ok {
		t.Fatal("expected false for nil receiver ByIndex")
	}
	if r.Count() != 0 {
		t.Fatal("expected 0 count for nil receiver")
	}
}

func TestExternalSymbolResolverNoExternals(t *testing.T) {
	lang := &Language{
		SymbolNames:     []string{"end", "identifier"},
		ExternalSymbols: nil,
	}
	r := NewExternalSymbolResolver(lang)
	if r != nil {
		t.Fatal("expected nil resolver for language with no externals")
	}
}

func TestExternalSymbolResolverBasic(t *testing.T) {
	// Simulate a Language with 3 external tokens:
	//   index 0 → Symbol 5 ("_newline")
	//   index 1 → Symbol 6 ("_indent")
	//   index 2 → Symbol 7 ("_dedent")
	names := make([]string, 8)
	names[0] = "end"
	names[1] = "identifier"
	names[5] = "_newline"
	names[6] = "_indent"
	names[7] = "_dedent"
	lang := &Language{
		SymbolNames:     names,
		ExternalSymbols: []Symbol{5, 6, 7},
	}

	r := NewExternalSymbolResolver(lang)
	if r == nil {
		t.Fatal("expected non-nil resolver")
	}

	if r.Count() != 3 {
		t.Fatalf("expected 3 external tokens, got %d", r.Count())
	}

	// ByName lookup.
	sym, ok := r.ByName("_newline")
	if !ok || sym != 5 {
		t.Fatalf("ByName(_newline) = %d, %v; want 5, true", sym, ok)
	}
	sym, ok = r.ByName("_indent")
	if !ok || sym != 6 {
		t.Fatalf("ByName(_indent) = %d, %v; want 6, true", sym, ok)
	}
	sym, ok = r.ByName("_dedent")
	if !ok || sym != 7 {
		t.Fatalf("ByName(_dedent) = %d, %v; want 7, true", sym, ok)
	}
	_, ok = r.ByName("nonexistent")
	if ok {
		t.Fatal("expected false for nonexistent name")
	}

	// ByIndex lookup.
	sym, ok = r.ByIndex(0)
	if !ok || sym != 5 {
		t.Fatalf("ByIndex(0) = %d, %v; want 5, true", sym, ok)
	}
	sym, ok = r.ByIndex(2)
	if !ok || sym != 7 {
		t.Fatalf("ByIndex(2) = %d, %v; want 7, true", sym, ok)
	}
	_, ok = r.ByIndex(-1)
	if ok {
		t.Fatal("expected false for negative index")
	}
	_, ok = r.ByIndex(3)
	if ok {
		t.Fatal("expected false for out-of-range index")
	}
}
