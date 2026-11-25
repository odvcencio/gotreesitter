// Excerpt of tree-sitter-fish src/parser.c at fa2143f5d66a9eb6c007ba9173525ea7aaafe788
// (The Unlicense). The helper functions and the lexer states below are copied
// verbatim; the other enum values, helpers and states are left out. The
// generator wrote a large negated character class as !<helper>(lookahead) with
// NUL in the helper's set, so the class does not match at the end of input.

#include <tree_sitter/parser.h>

enum ts_symbol_identifiers {
  sym_word = 1,
  anon_sym_BANG = 14,
  sym_integer = 21,
};

static inline bool sym_word_character_set_1(int32_t c) {
  return (c < 8192
    ? (c < 160
      ? (c < 133
        ? (c >= 11 && c <= '\f')
        : c <= 133)
      : (c <= 160 || c == 5760))
    : (c <= 8202 || (c < 8287
      ? (c < 8239
        ? (c >= 8232 && c <= 8233)
        : c <= 8239)
      : (c <= 8287 || c == 12288))));
}

static inline bool sym_word_character_set_2(int32_t c) {
  return (c < '$'
    ? (c < '\r'
      ? (c < '\t'
        ? c == 0
        : c <= '\n')
      : (c <= '\r' || (c < '"'
        ? c == ' '
        : c <= '"')))
    : (c <= '$' || (c < '>'
      ? (c < ';'
        ? (c >= '&' && c <= '*')
        : c <= '<')
      : (c <= '>' || (c < '{'
        ? (c >= '[' && c <= ']')
        : c <= '~')))));
}

static inline bool sym_word_character_set_3(int32_t c) {
  return (c < '$'
    ? (c < '\r'
      ? (c < '\t'
        ? c == 0
        : c <= '\n')
      : (c <= '\r' || (c < '"'
        ? c == ' '
        : c <= '"')))
    : (c <= '$' || (c < '['
      ? (c < ';'
        ? (c >= '&' && c <= '*')
        : c <= ';')
      : (c <= ']' || (c >= '{' && c <= '~')))));
}

static bool ts_lex(TSLexer *lexer, TSStateId state) {
  START_LEXER();
  eof = lexer->eof(lexer);
  switch (state) {
    case 30:
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(65);
      END_STATE();
    case 31:
      if (eof) ADVANCE(33);
      if (lookahead == '\n') ADVANCE(42);
      if (lookahead == '\r') ADVANCE(48);
      if (lookahead == '!') ADVANCE(54);
      if (lookahead == '"') ADVANCE(86);
      if (lookahead == '#') ADVANCE(77);
      if (lookahead == '$') ADVANCE(55);
      if (lookahead == '&') ADVANCE(40);
      if (lookahead == '\'') ADVANCE(90);
      if (lookahead == '(') ADVANCE(56);
      if (lookahead == ')') ADVANCE(58);
      if (lookahead == '*') ADVANCE(117);
      if (lookahead == '+' ||
          lookahead == '-') ADVANCE(140);
      if (lookahead == ';') ADVANCE(39);
      if (lookahead == '[') ADVANCE(81);
      if (lookahead == '\\') ADVANCE(19);
      if (lookahead == ']') ADVANCE(82);
      if (lookahead == 'i') ADVANCE(135);
      if (lookahead == '{') ADVANCE(83);
      if (lookahead == '}') ADVANCE(76);
      if (lookahead == '~') ADVANCE(116);
      if (lookahead == '\t' ||
          lookahead == ' ') SKIP(31)
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(63);
      if (sym_word_character_set_1(lookahead)) ADVANCE(118);
      if (lookahead != 0 &&
          lookahead != '<' &&
          lookahead != '>' &&
          lookahead != '|') ADVANCE(142);
      END_STATE();
    case 33:
      ACCEPT_TOKEN(ts_builtin_sym_end);
      END_STATE();
    case 54:
      ACCEPT_TOKEN(anon_sym_BANG);
      if (!sym_word_character_set_2(lookahead)) ADVANCE(142);
      END_STATE();
    case 61:
      ACCEPT_TOKEN(sym_integer);
      if (lookahead == '.') ADVANCE(141);
      if (lookahead == '<') ADVANCE(108);
      if (lookahead == '>') ADVANCE(109);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(61);
      if (!sym_word_character_set_3(lookahead)) ADVANCE(142);
      END_STATE();
    case 63:
      ACCEPT_TOKEN(sym_integer);
      if (lookahead == '.') ADVANCE(141);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(63);
      if (!sym_word_character_set_2(lookahead)) ADVANCE(142);
      END_STATE();
    case 141:
      ACCEPT_TOKEN(sym_word);
      if (('0' <= lookahead && lookahead <= '9')) ADVANCE(66);
      if (!sym_word_character_set_2(lookahead)) ADVANCE(142);
      END_STATE();
    case 142:
      ACCEPT_TOKEN(sym_word);
      if (!sym_word_character_set_2(lookahead)) ADVANCE(142);
      END_STATE();
    default:
      return false;
  }
}
