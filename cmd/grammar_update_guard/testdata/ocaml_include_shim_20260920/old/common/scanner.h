// Synthetic fixture standing in for tree-sitter-ocaml's shared
// common/scanner.h, the file grammars/ocaml/src/scanner.c reaches through
// its "../../../common/scanner.h" include (see
// cmd/grammar_update_guard/main_test.go
// TestApplyGrammarDiffFollowsScannerIncludes). Not vendored upstream source.
bool tree_sitter_ocaml_external_scanner_scan(void *payload, TSLexer *lexer, const bool *valid_symbols) {
  return scan_nestable_comment(lexer, valid_symbols);
}
