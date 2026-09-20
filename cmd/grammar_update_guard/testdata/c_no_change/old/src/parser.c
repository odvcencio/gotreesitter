// Synthetic fixture: tree-sitter's generated parser.c at ae19b676, standing
// in for a real no-scanner-change lock update. The guard does not read
// parser.c at all; only src/scanner.c, src/scanner.cc, and src/grammar.json
// externals are scanner-facing.
static const uint32_t ts_parser_version = 1;
