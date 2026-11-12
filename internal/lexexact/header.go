package lexexact

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// header holds what the extractor needs to know about the lexer macros of a
// grammar's tree_sitter/parser.h.
type header struct {
	eofDeclared bool // START_LEXER declares `bool eof`
	eofSet      bool // START_LEXER sets eof from lexer->eof() at every step
	hasMap      bool // ADVANCE_MAP is defined
	hasSet      bool // set_contains is defined
}

// knownHeaders lists the accepted tree_sitter/parser.h files by the SHA-256 of
// their bytes with each "\r\n" replaced by "\n". Each entry was written after reading the
// lexer macros and set_contains of that file and checking that they mean what
// parse.go and eval.go assume: START_LEXER sets up result, skip, lookahead (and
// eof where the entry says so), ADVANCE and SKIP assign state and jump to
// next_state, ACCEPT_TOKEN sets result and the result symbol, END_STATE returns
// result, ADVANCE_MAP compares the uint16_t keys with lookahead, and set_contains
// is a binary search over the first len ranges. The files hold no other
// directive that changes these (only include guards, <stdbool.h>, <stdint.h>,
// <stdlib.h>, the __cplusplus wrapper, and _MSC_VER for UNUSED).
//
// Adding a header is a reviewed change to this list.
var knownHeaders = map[string]header{
	// 109 grammars; standard, set_contains takes const pointers
	"180b893c8734778fd32f372dfbc27bd6ad1cd2221f26150b31256ff6716320d2": {eofDeclared: true, eofSet: false, hasMap: true, hasSet: true},
	// 54 grammars; standard, set_contains takes non-const pointers
	"a1f6ef161fbaf48a0e10fca90ef5290a062462b307b3898aa562993853b9f80a": {eofDeclared: true, eofSet: false, hasMap: true, hasSet: true},
	// 24 grammars; standard, set_contains takes non-const pointers
	"a3eb18ef034b3f4255b965a26caa276f9cfe13a79573b402f1a12dc5018052aa": {eofDeclared: true, eofSet: false, hasMap: true, hasSet: true},
	// 19 grammars; no UNUSED, no ADVANCE_MAP, no set_contains
	"ab104936984904469572a4e868149f7a22fb2929347f837ae6a1f9b790f1b173": {eofDeclared: true, eofSet: false, hasMap: false, hasSet: false},
	// 13 grammars; standard, set_contains takes const pointers
	"11b654fbba7770b1c710cc14b169be2deb5d89c0f87233d4605037c26c1ff657": {eofDeclared: true, eofSet: false, hasMap: true, hasSet: true},
	// 6 grammars; no ADVANCE_MAP, no set_contains
	"8e819abdeba5866bf1aaf6e7b97522241c903d94a18395c6a75ed02984270ee8": {eofDeclared: true, eofSet: false, hasMap: false, hasSet: false},
	// 5 grammars; no UNUSED, no ADVANCE_MAP, no set_contains
	"c569eeb34b7ed816b3a0a43c4cb8c06b36a6ef1164bc82ebed5047284cd73036": {eofDeclared: true, eofSet: false, hasMap: false, hasSet: false},
	// 3 grammars; eof is set inside START_LEXER
	"83a7aebb8067b898fa0975e4c627148ec2407617865b42edc91537f6bf651591": {eofDeclared: true, eofSet: true, hasMap: false, hasSet: false},
	// 3 grammars; no UNUSED, no ADVANCE_MAP, no set_contains
	"05729ff0c43ba3aebd8429a71e13c25a5dc7944e0eb725438bc98fda44573285": {eofDeclared: true, eofSet: false, hasMap: false, hasSet: false},
	// 1 grammar; standard, set_contains takes non-const pointers
	"64720a93d46ad7eb27cac63d142e1c7575c3b7207c15e4899cf0243aaff0c033": {eofDeclared: true, eofSet: false, hasMap: true, hasSet: true},
	// 1 grammar; no UNUSED, no ADVANCE_MAP, no set_contains
	"8f5ea110cbeb7bd849322bddc9815ad18dc550611b5331317da774c2f4047a92": {eofDeclared: true, eofSet: false, hasMap: false, hasSet: false},
	// 1 grammar; no ADVANCE_MAP, no set_contains
	"5a51af1cca99044757923e22ae1f8597dd7a2c7313248b40cfb6947a9fdf3e55": {eofDeclared: true, eofSet: false, hasMap: false, hasSet: false},
	// 1 grammar; eof is not declared
	"ed3abc20269b10767556429deb6fab6ec91e3cbc5476218b871e802e2a1a70c5": {eofDeclared: false, eofSet: false, hasMap: false, hasSet: false},
}

// headerHash is the lookup key of a header: the SHA-256 of its bytes with each
// "\r\n" replaced by "\n", in lower-case hexadecimal.
func headerHash(parserH string) string {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(parserH, "\r\n", "\n")))
	return hex.EncodeToString(sum[:])
}

// parseHeader accepts parserH only if it is one of the known headers.
func parseHeader(parserH string) (header, error) { return parseHeaderIn(parserH, knownHeaders) }

// parseHeaderIn looks parserH up in table. The header is required: it decides
// when `eof` is set and which macros exist. A carriage return that is not part of
// "\r\n" refuses the header.
func parseHeaderIn(parserH string, table map[string]header) (header, error) {
	if strings.TrimSpace(parserH) == "" {
		return header{}, fmt.Errorf("header: the grammar's own tree_sitter/parser.h is required")
	}
	if strings.Contains(strings.ReplaceAll(parserH, "\r\n", "\n"), "\r") {
		return header{}, fmt.Errorf("header: tree_sitter/parser.h holds a carriage return that is not part of a line end")
	}
	sum := headerHash(parserH)
	if h, ok := table[sum]; ok {
		return h, nil
	}
	return header{}, fmt.Errorf("header: tree_sitter/parser.h with SHA-256 %s (CRLF line ends read as LF) is not in the list of known headers (knownHeaders in internal/lexexact/header.go); adding a header is a reviewed change to that list", sum)
}
