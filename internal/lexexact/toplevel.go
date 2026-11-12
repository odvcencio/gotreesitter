package lexexact

import (
	"fmt"
	"regexp"
	"strings"
)

// Parse reads parserC, the text of a generated parser.c, and parserH, the text
// of the grammar's own tree_sitter/parser.h. The header must be one of the known
// headers (see header.go).
//
// Parse accepts a file only if all of it is made of the directives of
// directiveShapes and of the top-level items that the generator writes, each
// matched whole: the enums ts_symbol_identifiers, ts_field_identifiers,
// ts_external_scanner_symbol_identifiers and anonymous enums; the data and parse
// tables with their fixed names; arrays of TSCharacterRange; helper functions
// `static inline bool NAME(int32_t c) { return EXPR; }`; the start arrays of an
// own-range record; the functions ts_lex and ts_lex_keywords; the external
// scanner prototypes; the language function `tree_sitter_NAME`; and the
// `extern "C" {` block inside `#ifdef __cplusplus`. Anything else, anywhere in the
// file (a prototype of another shape, a typedef, a function, an attribute, a
// second definition of a name, a directive of another shape), is refused with its
// offset. No item or macro may take a name that is already taken by an
// enumerator, a count macro, a set, a helper, a lex function, a table, or a name
// that the header's macros use. Each name a lex function reads must be defined
// once, before its first use.
//
// Before that, the whole file is split into tokens: comments are removed, string
// and character literal bodies are opaque, and a NUL byte, a lone carriage
// return, a trigraph, a digraph, or a backslash (followed by white space or not)
// before a line end is refused.
func Parse(parserC, parserH string) (*Source, error) {
	return parseWith(parserC, parserH, knownHeaders)
}

// parseWith is Parse with its own table of known headers.
func parseWith(parserC, parserH string, table map[string]header) (*Source, error) {
	hdr, err := parseHeaderIn(parserH, table)
	if err != nil {
		return nil, err
	}
	toks, err := tokenize(parserC)
	if err != nil {
		return nil, err
	}
	s := &Source{hdr: hdr, sets: map[string][]charRange{}, funcs: map[string]*charFunc{}, starts: map[string][]uint32{}}
	if err := s.readTop(toks); err != nil {
		return nil, err
	}
	if err := s.checkCalls(); err != nil {
		return nil, err
	}
	return s, nil
}

// dataTables lists the fixed-name tables of a generated parser.c that precede the
// lexer: name, element type, and whether the element is a pointer.
var dataTables = map[string]struct {
	types []string
	ptr   bool
}{
	"ts_symbol_metadata":             {[]string{"TSSymbolMetadata"}, false},
	"ts_symbol_names":                {[]string{"char"}, true},
	"ts_symbol_map":                  {[]string{"TSSymbol"}, false},
	"ts_alias_sequences":             {[]string{"TSSymbol"}, false},
	"ts_non_terminal_alias_map":      {[]string{"uint16_t"}, false},
	"ts_primary_state_ids":           {[]string{"TSStateId"}, false},
	"ts_field_map_entries":           {[]string{"TSFieldMapEntry"}, false},
	"ts_field_names":                 {[]string{"char"}, true},
	"ts_field_map_slices":            {[]string{"TSMapSlice", "TSFieldMapSlice"}, false},
	"ts_supertype_symbols":           {[]string{"TSSymbol"}, false},
	"ts_supertype_map_slices":        {[]string{"TSMapSlice"}, false},
	"ts_supertype_map_entries":       {[]string{"TSSymbol"}, false},
	"ts_lex_modes":                   {[]string{"TSLexMode", "TSLexerMode"}, false},
	"ts_parse_table":                 {[]string{"uint16_t"}, false},
	"ts_small_parse_table":           {[]string{"uint16_t"}, false},
	"ts_small_parse_table_map":       {[]string{"uint32_t"}, false},
	"ts_parse_actions":               {[]string{"TSParseActionEntry"}, false},
	"ts_external_scanner_symbol_map": {[]string{"TSSymbol"}, false},
	"ts_external_scanner_states":     {[]string{"bool"}, false},
	"ts_reserved_words":              {[]string{"TSSymbol"}, false},
}

var countMacros = map[string]bool{
	"STATE_COUNT": true, "LARGE_STATE_COUNT": true, "SYMBOL_COUNT": true, "ALIAS_COUNT": true,
	"TOKEN_COUNT": true, "EXTERNAL_TOKEN_COUNT": true, "FIELD_COUNT": true,
	"MAX_ALIAS_SEQUENCE_LENGTH": true, "PRODUCTION_ID_COUNT": true,
	"MAX_RESERVED_WORD_SET_SIZE": true, "SUPERTYPE_COUNT": true,
}

func texts(ts []token) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.text
	}
	return out
}

func eq(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// itemEnd returns the index just after the top-level item that starts at toks[i]:
// a function definition ends at the brace that closes its body, any other item at
// its first ';' outside brackets. Brackets must match, nest at most maxNesting
// deep, and no directive may appear inside.
func itemEnd(toks []token, i int) (int, error) {
	var stack []byte
	firstBrace := -1
	for j := i; j < len(toks); j++ {
		t := toks[j]
		if t.kind == tokDir {
			return 0, fmt.Errorf("parser.c: directive %q at offset %d inside a declaration", t.text, t.off)
		}
		if t.kind != tokPunct {
			continue
		}
		switch t.text {
		case "(", "[", "{":
			if t.text == "{" && len(stack) == 0 && firstBrace < 0 {
				firstBrace = j
			}
			stack = append(stack, t.text[0])
			if len(stack) > maxNesting {
				return 0, fmt.Errorf("parser.c: nesting deeper than %d at offset %d", maxNesting, t.off)
			}
		case ")", "]", "}":
			want := map[string]byte{")": '(', "]": '[', "}": '{'}[t.text]
			if len(stack) == 0 || stack[len(stack)-1] != want {
				return 0, fmt.Errorf("parser.c: unbalanced %q at offset %d", t.text, t.off)
			}
			stack = stack[:len(stack)-1]
			if t.text == "}" && len(stack) == 0 && firstBrace >= 0 && firstBrace > i && toks[firstBrace-1].text == ")" {
				return j + 1, nil
			}
		case ";":
			if len(stack) == 0 {
				return j + 1, nil
			}
		}
	}
	return 0, fmt.Errorf("parser.c: declaration at offset %d does not end", toks[i].off)
}

func isLexHead(h []string, name string) bool {
	return eq(h, "static", "bool", name, "(", "TSLexer", "*", "lexer", ",", "TSStateId", "state", ")", "{")
}

// readTop walks the top-level items of the whole file. See Parse.
func (s *Source) readTop(toks []token) error {
	enum := map[string]int64{"ts_builtin_sym_end": 0}
	defined := map[string]bool{"ts_builtin_sym_end": true}
	tags := map[string]bool{}
	var cond []condFrame
	externOpen := false
	haveMain, haveKw := false, false
	sawInclude := false
	i := 0
	for i < len(toks) {
		t := toks[i]
		if t.kind == tokDir {
			if err := checkDirective(t, &cond, defined); err != nil {
				return err
			}
			if strings.HasPrefix(t.text, "#include") {
				if len(cond) > 0 {
					return fmt.Errorf("parser.c: include at offset %d is inside a conditional", t.off)
				}
				sawInclude = true
			}
			i++
			continue
		}
		inCpp := len(cond) == 1 && cond[0].text == "#ifdef __cplusplus"
		if t.text == "extern" && i+2 < len(toks) && toks[i+1].kind == tokString && toks[i+1].text == `"C"` && toks[i+2].text == "{" {
			if !inCpp || externOpen {
				return fmt.Errorf("parser.c: extern \"C\" at offset %d is not directly inside #ifdef __cplusplus", t.off)
			}
			externOpen = true
			i += 3
			continue
		}
		if t.kind == tokPunct && t.text == "}" {
			if !inCpp || !externOpen {
				return fmt.Errorf("parser.c: '}' at offset %d closes nothing the generator opens", t.off)
			}
			externOpen = false
			i++
			continue
		}
		if len(cond) > 0 {
			return fmt.Errorf("parser.c: text at offset %d inside a conditional", t.off)
		}
		if !sawInclude {
			return fmt.Errorf("parser.c: item at offset %d is not one the generator writes before the include of tree_sitter/parser.h", t.off)
		}
		end, err := itemEnd(toks, i)
		if err != nil {
			return err
		}
		item := toks[i:end]
		kind, err := s.readItem(item, enum, defined, tags)
		if err != nil {
			return err
		}
		switch kind {
		case "lex":
			haveMain = true
		case "kw":
			haveKw = true
		case "unknown":
			return fmt.Errorf("parser.c: item at offset %d is not one the generator writes: %s", t.off, headSummary(item))
		}
		i = end
	}
	if len(cond) != 0 || externOpen {
		return fmt.Errorf("parser.c: #if or extern \"C\" without its end at the end of the file")
	}
	if !haveMain {
		return fmt.Errorf("ts_lex not found")
	}
	_ = haveKw
	return nil
}

func headSummary(item []token) string {
	n := len(item)
	if n > 8 {
		n = 8
	}
	return fmt.Sprint(texts(item[:n]))
}

// readItem matches one item and records what it defines. It returns "lex" or
// "kw" for the lex functions, "unknown" for an item of no accepted kind.
func (s *Source) readItem(item []token, enum map[string]int64, defined map[string]bool, tags map[string]bool) (string, error) {
	define := func(name string, at token) error {
		if reservedName(name) {
			return fmt.Errorf("parser.c: %s at offset %d is a name that the lexer macros or the header use", name, at.off)
		}
		if defined[name] {
			return fmt.Errorf("parser.c: %s is defined twice (offset %d)", name, at.off)
		}
		defined[name] = true
		return nil
	}
	// defineFixed is define for the names the generator fixes (the lex functions
	// and the start arrays), which reservedName lists.
	defineFixed := func(name string, at token) error {
		if defined[name] {
			return fmt.Errorf("parser.c: %s is defined twice (offset %d)", name, at.off)
		}
		defined[name] = true
		return nil
	}
	last := item[len(item)-1]
	switch {
	case item[0].text == "enum":
		if len(item) < 4 || last.text != ";" || item[len(item)-2].text != "}" {
			return "unknown", nil
		}
		k := 1
		tag := ""
		if item[1].kind == tokIdent {
			tag = item[1].text
			k = 2
			if tag != "ts_symbol_identifiers" && tag != "ts_field_identifiers" && tag != "ts_external_scanner_symbol_identifiers" {
				return "unknown", nil
			}
		}
		if k >= len(item) || item[k].text != "{" {
			return "unknown", nil
		}
		if tag != "" {
			if tags[tag] {
				return "", fmt.Errorf("parser.c: enum %s is defined twice (offset %d)", tag, item[0].off)
			}
			tags[tag] = true
		}
		sc := newScanner(item, k+1)
		names, err := readEnumBody(sc, enum)
		if err != nil {
			return "", err
		}
		if sc.i != len(item)-1 {
			return "", fmt.Errorf("parser.c: text after the enum at offset %d", item[0].off)
		}
		for _, n := range names {
			if err := define(n, item[0]); err != nil {
				return "", err
			}
		}
		return "enum", nil

	case isLexHead(texts(item[:min(len(item), 12)]), "ts_lex"), isLexHead(texts(item[:min(len(item), 12)]), "ts_lex_keywords"):
		name := item[2].text
		if err := defineFixed(name, item[2]); err != nil {
			return "", err
		}
		sc := newScanner(item, 12)
		l, err := s.readLexer(sc, name, enum)
		if err != nil {
			return "", err
		}
		if sc.i != len(item) {
			return "", fmt.Errorf("parser.c: text after %s at offset %d", name, item[0].off)
		}
		if name == "ts_lex" {
			s.Main = l
			return "lex", nil
		}
		s.Keyword = l
		return "kw", nil

	case len(item) > 10 && eq(texts(item[:3]), "static", "inline", "bool") && item[3].kind == tokIdent && item[4].text == "(" &&
		item[5].text == "int32_t" && item[7].text == ")" && item[8].text == "{" && item[9].text == "return":
		name, param := item[3].text, item[6].text
		if param != "c" && param != "lookahead" {
			return "unknown", nil
		}
		if err := define(name, item[3]); err != nil {
			return "", err
		}
		sc := newScanner(item, 10)
		p := &exprParser{sc: sc, src: s, look: param, allowSet: s.hdr.hasSet}
		if err := p.advance(); err != nil {
			return "", fmt.Errorf("function %s: %v", name, err)
		}
		e, err := p.ternary()
		if err != nil {
			return "", fmt.Errorf("function %s: %v", name, err)
		}
		if err := p.want(";"); err != nil {
			return "", fmt.Errorf("function %s: %v", name, err)
		}
		if p.tok.text != "}" || sc.i != len(item) {
			return "", fmt.Errorf("function %s: text after the return statement", name)
		}
		s.funcs[name] = &charFunc{param: param, body: e}
		s.funcOrder = append(s.funcOrder, name)
		return "helper", nil
	}

	if k := externalPrototype(texts(item)); k >= 0 {
		if err := define(item[k].text, item[k]); err != nil {
			return "", err
		}
		return "prototype", nil
	}
	if name, ok := languageFunction(item); ok {
		if err := define(name.text, name); err != nil {
			return "", err
		}
		return "language", nil
	}

	// Declarations with an initializer: head, '=', '{' ... '}', ';'.
	eqAt := -1
	for j, t := range item {
		if t.text == "=" {
			eqAt = j
			break
		}
		if t.text == "{" || t.text == ";" {
			break
		}
	}
	if eqAt < 0 || eqAt+2 >= len(item) || item[eqAt+1].text != "{" || last.text != ";" || item[len(item)-2].text != "}" {
		return "unknown", nil
	}
	// The brace that opens the initializer must be closed by the brace just
	// before the final ';': no second declarator may follow it.
	depth, matchAt := 0, -1
	for m := eqAt + 1; m < len(item); m++ {
		if item[m].kind != tokPunct {
			continue
		}
		switch item[m].text {
		case "{", "(", "[":
			depth++
		case "}", ")", "]":
			depth--
		}
		if depth == 0 {
			matchAt = m
			break
		}
	}
	if matchAt != len(item)-2 {
		return "unknown", nil
	}
	head := texts(item[:eqAt])
	body := item[eqAt+2 : len(item)-2] // between the braces
	// start array of an own-range record
	if len(head) == 6 && eq(head[:3], "static", "const", "uint16_t") && (head[3] == "ts_lex_start" || head[3] == "ts_lex_keywords_start") && eq(head[4:], "[", "]") {
		name := head[3]
		if err := defineFixed(name, item[3]); err != nil {
			return "", err
		}
		var vs []uint32
		for _, t := range body {
			if t.text == "," {
				continue
			}
			if t.kind != tokNumber || t.num > maxState {
				return "", fmt.Errorf("%s: unexpected %v", name, t)
			}
			vs = append(vs, uint32(t.num))
		}
		if len(vs) == 0 {
			return "", fmt.Errorf("%s is empty", name)
		}
		s.starts[name] = vs
		return "start", nil
	}
	// character-range array
	if k := setHead(head); k > 0 {
		name := head[k]
		if err := define(name, item[k]); err != nil {
			return "", err
		}
		rs, err := readSetBody(name, body)
		if err != nil {
			return "", err
		}
		s.sets[name] = rs
		return "set", nil
	}
	// fixed-name data table
	if k := tableHead(head); k > 0 {
		if err := defineFixed(head[k], item[k]); err != nil {
			return "", err
		}
		return "table", nil
	}
	return "unknown", nil
}

// setHead returns the index of the name in `[static] [const] TSCharacterRange NAME [ ]`, or 0.
func setHead(h []string) int {
	k := 0
	if k < len(h) && h[k] == "static" {
		k++
	}
	if k < len(h) && h[k] == "const" {
		k++
	}
	if k+4 == len(h) && h[k] == "TSCharacterRange" && eq(h[k+2:], "[", "]") && !reservedName(h[k+1]) {
		return k + 1
	}
	return 0
}

// reservedName reports names that the lexer macros or the header use, which a
// set or table must not take.
var stdIntLimit = regexp.MustCompile(`^U?INT[A-Z0-9_]*_(MAX|MIN|C)$`)

func reservedName(n string) bool {
	switch n {
	case "ts_lex", "ts_lex_keywords", "ts_lex_start", "ts_lex_keywords_start", "set_contains", "eof", "true", "false",
		"lookahead", "state", "result", "skip", "lexer", "map", "i", "TS_PUBLIC", "extern", "language", "NULL", "bool", "ts_builtin_sym_error", "ts_builtin_sym_end", "TREE_SITTER_SERIALIZATION_BUFFER_SIZE", "TREE_SITTER_PARSER_H_", "TREE_SITTER_API_H_", "UNUSED", "ACCEPT_INPUT", "ACTIONS", "STATE", "SMALL_STATE", "SHIFT", "SHIFT_REPEAT", "SHIFT_EXTRA", "REDUCE", "RECOVER", "__bool_true_false_are_defined", "SIZE_MAX", "RAND_MAX", "EXIT_SUCCESS", "EXIT_FAILURE", "ADVANCE", "SKIP", "ACCEPT_TOKEN", "END_STATE", "START_LEXER", "ADVANCE_MAP":
		return true
	}
	if stdIntLimit.MatchString(n) {
		return true
	}
	_, isTable := dataTables[n]
	return isTable
}

// tableHead returns the index of the name in `static [const] TYPE [* [const]] NAME dims`, or 0.
func tableHead(h []string) int {
	if len(h) < 5 || h[0] != "static" {
		return 0
	}
	k := 1
	if h[k] == "const" {
		k++
	}
	if k >= len(h) {
		return 0
	}
	typ := h[k]
	k++
	ptr := false
	if k < len(h) && h[k] == "*" {
		ptr = true
		k++
		if k < len(h) && h[k] == "const" {
			k++
		}
	}
	if k >= len(h) {
		return 0
	}
	name := h[k]
	d, ok := dataTables[name]
	if !ok || d.ptr != ptr {
		return 0
	}
	good := false
	for _, ty := range d.types {
		if ty == typ {
			good = true
		}
	}
	if !good {
		return 0
	}
	rest := h[k+1:]
	dims := 0
	for len(rest) >= 2 && rest[0] == "[" {
		if rest[1] == "]" {
			rest = rest[2:]
		} else if len(rest) >= 3 && rest[2] == "]" && (countMacros[rest[1]] || isNumberText(rest[1])) {
			rest = rest[3:]
		} else {
			return 0
		}
		dims++
	}
	if len(rest) != 0 || dims == 0 || dims > 2 {
		return 0
	}
	return k
}

func isNumberText(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

func readSetBody(name string, body []token) ([]charRange, error) {
	sc := newScanner(body, 0)
	var rs []charRange
	for {
		t, err := sc.next()
		if err != nil {
			return nil, fmt.Errorf("character set %s: %v", name, err)
		}
		if t.kind == tokEOF {
			return rs, nil
		}
		if t.text == "," {
			continue
		}
		if t.text != "{" {
			return nil, fmt.Errorf("character set %s: unexpected %v", name, t)
		}
		lo, err := sc.number()
		if err != nil {
			return nil, fmt.Errorf("character set %s: %v", name, err)
		}
		if err := sc.expect(","); err != nil {
			return nil, fmt.Errorf("character set %s: %v", name, err)
		}
		hi, err := sc.number()
		if err != nil {
			return nil, fmt.Errorf("character set %s: %v", name, err)
		}
		if err := sc.expect("}"); err != nil {
			return nil, fmt.Errorf("character set %s: %v", name, err)
		}
		rs = append(rs, charRange{lo, hi})
	}
}

// readEnumBody reads the enumerators of an enum, from just after its '{' to its '}', into enum.
func readEnumBody(sc *scanner, enum map[string]int64) ([]string, error) {
	var names []string
	next := int64(0)
	for {
		t, err := sc.next()
		if err != nil {
			return nil, fmt.Errorf("enum: %v", err)
		}
		if t.text == "}" && t.kind == tokPunct {
			return names, nil
		}
		if t.text == "," && t.kind == tokPunct {
			continue
		}
		if t.kind != tokIdent {
			return nil, fmt.Errorf("enum: unexpected %v", t)
		}
		name := t.text
		save := *sc
		t2, err := sc.next()
		if err != nil {
			return nil, fmt.Errorf("enum: %v", err)
		}
		if t2.kind == tokPunct && t2.text == "=" {
			v, err := sc.number()
			if err != nil {
				return nil, fmt.Errorf("enum %s: %v", name, err)
			}
			next = v
		} else {
			*sc = save
		}
		if _, dup := enum[name]; dup {
			return nil, fmt.Errorf("enum name %s is defined twice (line %d)", name, t.line)
		}
		enum[name] = next
		names = append(names, name)
		next++
	}
}

// externalPrototype matches the five prototypes of the external scanner and
// returns the index of the function name, or -1.
func externalPrototype(h []string) int {
	forms := []struct {
		suffix string
		name   int
		shape  []string // the tokens, with "" for the name
	}{
		{"_external_scanner_create", 2, []string{"void", "*", "", "(", "void", ")", ";"}},
		{"_external_scanner_destroy", 1, []string{"void", "", "(", "void", "*", ")", ";"}},
		{"_external_scanner_scan", 1, []string{"bool", "", "(", "void", "*", ",", "TSLexer", "*", ",", "const", "bool", "*", ")", ";"}},
		{"_external_scanner_serialize", 1, []string{"unsigned", "", "(", "void", "*", ",", "char", "*", ")", ";"}},
		{"_external_scanner_deserialize", 1, []string{"void", "", "(", "void", "*", ",", "const", "char", "*", ",", "unsigned", ")", ";"}},
	}
	for _, f := range forms {
		if len(h) != len(f.shape) {
			continue
		}
		ok := true
		for i, w := range f.shape {
			if w != "" && h[i] != w {
				ok = false
				break
			}
		}
		n := h[f.name]
		if ok && strings.HasPrefix(n, "tree_sitter_") && strings.HasSuffix(n, f.suffix) && isIdentText(n) {
			return f.name
		}
	}
	return -1
}

func isIdentText(s string) bool {
	if s == "" || !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdentPart(s[i]) {
			return false
		}
	}
	return true
}

// languageFunction matches
//
//	[TS_PUBLIC|extern] const TSLanguage *tree_sitter_NAME(void) {
//	  static [const] TSLanguage language = { ... };
//	  return &language;
//	}
//
// and returns the name token.
func languageFunction(item []token) (token, bool) {
	h := texts(item)
	k := 0
	if len(h) > 0 && (h[0] == "TS_PUBLIC" || h[0] == "extern") {
		k = 1
	}
	if len(h) < k+8 || !eq(h[k:k+3], "const", "TSLanguage", "*") || !strings.HasPrefix(h[k+3], "tree_sitter_") || !isIdentText(h[k+3]) {
		return token{}, false
	}
	name := item[k+3]
	j := k + 4
	if !(j < len(h) && h[j] == "(") {
		return token{}, false
	}
	j++
	if j < len(h) && h[j] == "void" {
		j++
	}
	if !(j+1 < len(h) && h[j] == ")" && h[j+1] == "{") {
		return token{}, false
	}
	j += 2
	if !(j < len(h) && h[j] == "static") {
		return token{}, false
	}
	j++
	if j < len(h) && h[j] == "const" {
		j++
	}
	if !(j+3 < len(h) && eq(h[j:j+4], "TSLanguage", "language", "=", "{")) {
		return token{}, false
	}
	open := j + 3
	depth := 0
	closeAt := -1
	for m := open; m < len(h); m++ {
		if item[m].kind != tokPunct {
			continue
		}
		switch h[m] {
		case "{", "(", "[":
			depth++
		case "}", ")", "]":
			depth--
			if depth == 0 {
				closeAt = m
			}
		}
		if closeAt >= 0 {
			break
		}
	}
	if closeAt < 0 || closeAt != len(h)-7 || !eq(h[closeAt:], "}", ";", "return", "&", "language", ";", "}") {
		return token{}, false
	}
	return name, true
}
