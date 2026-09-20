// Synthetic fixture standing in for tree-sitter-c-sharp's scanner.c at
// 9150f7d56bb4 (the ref the 2026-09-20 lock-update run tried to clear
// without review). Not vendored upstream source. Adds lambda-paren lookahead
// handling, matching the real upstream diff's shape (14,206 to 23,064
// bytes, a new "_lambda_paren_open" external).
enum TokenType {
  OPTIONAL_SEMI,
  INTERPOLATION_START,
  LAMBDA_PAREN_OPEN,
};

bool tree_sitter_c_sharp_external_scanner_scan(void *payload, TokenizeSpan span) {
  if (scan_lambda_paren_open(span)) {
    return true;
  }
  return scan_optional_semi(span);
}
