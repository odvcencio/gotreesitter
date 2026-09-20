// Synthetic fixture standing in for tree-sitter-yaml's scanner.c at
// a1c4812a73ec (the 2026-09-20 lock-update run cleared this without
// review). Not vendored upstream source. Mirrors the real upstream diff's
// shape: a bug fix with no new external tokens (51,508 to 51,548 bytes).
bool scan_block_scalar(TSLexer *lexer, Scanner *scanner) {
  if (scanner->indent_length < 0) {
    return false;
  }
  return read_block_scalar_header(lexer, scanner);
}
