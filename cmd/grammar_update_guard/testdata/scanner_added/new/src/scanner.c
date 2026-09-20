// Synthetic fixture: a grammar that gained a hand-written external scanner
// between the old and new ref. src/scanner.c did not exist at the old ref.
bool scan_new_external_token(TSLexer *lexer) {
  return advance_new_external_token(lexer);
}
