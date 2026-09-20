// Synthetic fixture standing in for tree-sitter-c-sharp's scanner.c at
// 88366631d598 (the ref grammars/languages.lock had pinned before the
// 2026-09-20 lock-update run). Not vendored upstream source.
enum TokenType {
  OPTIONAL_SEMI,
  INTERPOLATION_START,
};

bool tree_sitter_c_sharp_external_scanner_scan(void *payload, TokenizeSpan span) {
  return scan_optional_semi(span);
}
