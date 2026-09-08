//go:build cgo && (treesitter_c_parity || treesitter_c_bench)

package cgoharness

/*
#include <stdint.h>
#include <stdbool.h>

typedef struct TSLanguage TSLanguage;
typedef uint16_t TSSymbol;

// The tree-sitter runtime is linked into the test binary by the Go binding;
// these are its public language API entry points (tree_sitter/api.h).
extern const TSSymbol *ts_language_supertypes(const TSLanguage *self, uint32_t *length);
extern const TSSymbol *ts_language_subtypes(const TSLanguage *self, TSSymbol supertype, uint32_t *length);
extern const char *ts_language_symbol_name(const TSLanguage *self, TSSymbol symbol);
extern int ts_language_symbol_type(const TSLanguage *self, TSSymbol symbol);
extern uint32_t ts_language_abi_version(const TSLanguage *self);
*/
import "C"

import (
	"unsafe"
)

// cOracleSymbolLabel names a C symbol the way the parity boards print
// symbols: the grammar name, with "(anon)" for anonymous tokens.
func cOracleSymbolLabel(lang unsafe.Pointer, symbol uint16) string {
	l := (*C.TSLanguage)(lang)
	name := C.GoString(C.ts_language_symbol_name(l, C.TSSymbol(symbol)))
	// TSSymbolTypeRegular = 0, TSSymbolTypeAnonymous = 1, TSSymbolTypeSupertype = 2, TSSymbolTypeAuxiliary = 3.
	if C.ts_language_symbol_type(l, C.TSSymbol(symbol)) == 1 {
		return name + "(anon)"
	}
	return name
}

// cOracleABIVersion returns the ABI version the C grammar was generated at.
func cOracleABIVersion(lang unsafe.Pointer) uint32 {
	return uint32(C.ts_language_abi_version((*C.TSLanguage)(lang)))
}

// cOracleSupertypeMap returns the C runtime's supertype map: each supertype
// name mapped to its subtype labels in table order (ts_language_supertypes
// and ts_language_subtypes).
func cOracleSupertypeMap(lang unsafe.Pointer) (order []string, subtypes map[string][]string) {
	l := (*C.TSLanguage)(lang)
	var count C.uint32_t
	supers := C.ts_language_supertypes(l, &count)
	subtypes = make(map[string][]string, int(count))
	if supers == nil || count == 0 {
		return nil, subtypes
	}
	superSlice := unsafe.Slice((*uint16)(unsafe.Pointer(supers)), int(count))
	for _, super := range superSlice {
		name := cOracleSymbolLabel(lang, super)
		order = append(order, name)
		var subCount C.uint32_t
		subs := C.ts_language_subtypes(l, C.TSSymbol(super), &subCount)
		var labels []string
		if subs != nil && subCount > 0 {
			for _, sub := range unsafe.Slice((*uint16)(unsafe.Pointer(subs)), int(subCount)) {
				labels = append(labels, cOracleSymbolLabel(lang, sub))
			}
		}
		subtypes[name] = labels
	}
	return order, subtypes
}
