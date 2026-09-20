// Synthetic fixture standing in for tree-sitter-yaml's scanner.c at
// 4463985dfccc. Not vendored upstream source.
bool scan_block_scalar(TSLexer *lexer, Scanner *scanner) {
  return read_block_scalar_header(lexer, scanner);
}
