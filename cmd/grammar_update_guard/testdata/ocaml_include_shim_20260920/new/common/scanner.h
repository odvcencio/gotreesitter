// Synthetic fixture standing in for tree-sitter-ocaml's shared
// common/scanner.h after an upstream scanner-logic change that renumbers
// which external token nestable-comment scanning reports. Not vendored
// upstream source.
bool tree_sitter_ocaml_external_scanner_scan(void *payload, TSLexer *lexer, const bool *valid_symbols) {
  return scan_nestable_comment_v2(lexer, valid_symbols);
}
