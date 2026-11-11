package lexexact

import (
	"fmt"
	"regexp"
	"strings"
)

// directiveShapes are the preprocessor directives that tree-sitter's generator
// writes into parser.c (white space collapsed). Any other directive is refused,
// because a directive can change what the lexer macros, set_contains, true,
// false or a variable mean.
var directiveShapes = []*regexp.Regexp{
	regexp.MustCompile(`^#endif$`),
	regexp.MustCompile(`^#else$`),
	regexp.MustCompile(`^#if defined\(__GNUC__\) \|\| defined\(__clang__\)$`),
	regexp.MustCompile(`^#ifdef (_MSC_VER|__cplusplus|_WIN32|TREE_SITTER_HIDE_SYMBOLS)$`),
	regexp.MustCompile(`^#elif defined\(__(clang|GNUC)__\)$`),
	regexp.MustCompile(`^#elif defined\(_WIN32\)$`),
	regexp.MustCompile(`^#pragma GCC diagnostic (push|ignored "-Wmissing-field-initializers")$`),
	regexp.MustCompile(`^#pragma optimize\("", off\)$`),
	regexp.MustCompile(`^#pragma clang optimize off$`),
	regexp.MustCompile(`^#pragma GCC optimize \("O0"(, "jump-tables")?\)$`),
	regexp.MustCompile(`^#define extern __declspec\(dllexport\)$`),
	regexp.MustCompile(`^#define TS_PUBLIC( __declspec\(dllexport\)| __attribute__\(\(visibility\("default"\)\)\))?$`),
	regexp.MustCompile(`^#include (<tree_sitter/parser\.h>|"tree_sitter/parser\.h")$`),
}

// countMacroRE matches the definition of a count macro; its value is an
// unsigned decimal literal.
var countMacroRE = regexp.MustCompile(`^#define (LANGUAGE_VERSION|STATE_COUNT|LARGE_STATE_COUNT|SYMBOL_COUNT|ALIAS_COUNT|TOKEN_COUNT|EXTERNAL_TOKEN_COUNT|FIELD_COUNT|MAX_ALIAS_SEQUENCE_LENGTH|PRODUCTION_ID_COUNT|MAX_RESERVED_WORD_SET_SIZE|SUPERTYPE_COUNT) (0|[1-9][0-9]*)$`)

// checkDirective refuses a directive that is not one of the shapes above and
// keeps the open conditionals in *cond: #if and #ifdef open one, #else and #elif
// need an open one, #endif closes one. A count macro may be defined once and
// only if no earlier item took its name; the name is then taken in defined.
func checkDirective(t token, cond *[]condFrame, defined map[string]bool) error {
	ok := false
	for _, re := range directiveShapes {
		if re.MatchString(t.text) {
			ok = true
			break
		}
	}
	if m := countMacroRE.FindStringSubmatch(t.text); m != nil {
		ok = true
		if defined[m[1]] {
			return fmt.Errorf("parser.c: #define %s at offset %d takes a name that is already defined", m[1], t.off)
		}
		defined[m[1]] = true
	}
	if !ok {
		return fmt.Errorf("parser.c: directive %q at offset %d is not one the generator writes", t.text, t.off)
	}
	switch {
	case t.text == "#endif":
		if len(*cond) == 0 {
			return fmt.Errorf("parser.c: #endif without #if at offset %d", t.off)
		}
		*cond = (*cond)[:len(*cond)-1]
	case strings.HasPrefix(t.text, "#if"):
		*cond = append(*cond, condFrame{text: t.text})
	case strings.HasPrefix(t.text, "#elif"), t.text == "#else":
		if len(*cond) == 0 {
			return fmt.Errorf("parser.c: %s without #if at offset %d", t.text, t.off)
		}
		top := &(*cond)[len(*cond)-1]
		if top.elseSeen {
			return fmt.Errorf("parser.c: %s after #else at offset %d", t.text, t.off)
		}
		if t.text == "#else" {
			top.elseSeen = true
		}
	}
	return nil
}

// condFrame is one open conditional.
type condFrame struct {
	text     string
	elseSeen bool
}
