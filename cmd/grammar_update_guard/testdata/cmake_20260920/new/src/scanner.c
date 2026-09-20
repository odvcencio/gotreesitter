// Synthetic fixture standing in for tree-sitter-cmake's scanner.c at
// 58993af75218 (the 2026-09-20 lock-update run cleared this without
// review). Not vendored upstream source. Mirrors the real upstream diff's
// shape: a body-only fix with no new external tokens (4,738 to 4,780 bytes).
bool scan_bracket_argument(TSLexer *lexer) {
  if (lexer->eof(lexer)) {
    return false;
  }
  return advance_until_close_bracket(lexer);
}
