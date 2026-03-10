package gotreesitter

// ExternalSymbolResolver maps external token names to their concrete Symbol IDs
// in a specific Language. This allows external scanners to resolve symbol IDs at
// runtime rather than using hardcoded constants, making them compatible with any
// Language that defines the same external tokens (whether from ts2go extraction
// or grammargen).
type ExternalSymbolResolver struct {
	byName  map[string]Symbol
	byIndex []Symbol // external token index → Symbol ID
}

// NewExternalSymbolResolver builds a resolver from a Language's external symbol
// definitions. Returns nil if the Language has no external symbols.
func NewExternalSymbolResolver(lang *Language) *ExternalSymbolResolver {
	if lang == nil || len(lang.ExternalSymbols) == 0 {
		return nil
	}
	r := &ExternalSymbolResolver{
		byName:  make(map[string]Symbol, len(lang.ExternalSymbols)),
		byIndex: make([]Symbol, len(lang.ExternalSymbols)),
	}
	for i, sym := range lang.ExternalSymbols {
		r.byIndex[i] = sym
		if int(sym) < len(lang.SymbolNames) {
			r.byName[lang.SymbolNames[sym]] = sym
		}
	}
	return r
}

// ByName returns the Symbol ID for the given external token name.
// Returns 0, false if the name is not found.
func (r *ExternalSymbolResolver) ByName(name string) (Symbol, bool) {
	if r == nil {
		return 0, false
	}
	sym, ok := r.byName[name]
	return sym, ok
}

// ByIndex returns the Symbol ID for the given external token index
// (position in the grammar's externals array).
// Returns 0, false if the index is out of range.
func (r *ExternalSymbolResolver) ByIndex(idx int) (Symbol, bool) {
	if r == nil || idx < 0 || idx >= len(r.byIndex) {
		return 0, false
	}
	return r.byIndex[idx], true
}

// Count returns the number of external tokens.
func (r *ExternalSymbolResolver) Count() int {
	if r == nil {
		return 0
	}
	return len(r.byIndex)
}
