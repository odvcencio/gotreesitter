// Synthetic fixture standing in for tree-sitter-cmake's scanner.c at
// c7b2a71e7f8e. Not vendored upstream source.
bool scan_bracket_argument(TSLexer *lexer) {
  return advance_until_close_bracket(lexer);
}
