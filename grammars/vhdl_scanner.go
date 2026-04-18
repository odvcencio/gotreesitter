//go:build !grammar_subset || grammar_subset_vhdl

package grammars

import (
	"strings"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// ---------------------------------------------------------------------------
// Token type enum (mirrors C TokenType from jpt13653903/tree-sitter-vhdl)
// ---------------------------------------------------------------------------
// Internal-only values (LINE_COMMENT_START, BLOCK_COMMENT_START,
// STRING_LITERAL_STD_LOGIC_START, BASE_SPECIFIER_*, IDENTIFIER_EXPECTING_LETTER)
// are only used within the scanner and never emitted as result symbols.
const (
	vhdlIDENTIFIER = iota

	vhdlRESERVED_ABS
	vhdlRESERVED_ACCESS
	vhdlRESERVED_AFTER
	vhdlRESERVED_ALIAS
	vhdlRESERVED_ALL
	vhdlRESERVED_AND
	vhdlRESERVED_ARCHITECTURE
	vhdlRESERVED_ARRAY
	vhdlRESERVED_ASSERT
	vhdlRESERVED_ASSUME
	vhdlRESERVED_ATTRIBUTE
	vhdlRESERVED_BEGIN
	vhdlRESERVED_BLOCK
	vhdlRESERVED_BODY
	vhdlRESERVED_BUFFER
	vhdlRESERVED_BUS
	vhdlRESERVED_CASE
	vhdlRESERVED_COMPONENT
	vhdlRESERVED_CONFIGURATION
	vhdlRESERVED_CONSTANT
	vhdlRESERVED_CONTEXT
	vhdlRESERVED_COVER
	vhdlRESERVED_DEFAULT
	vhdlRESERVED_DISCONNECT
	vhdlRESERVED_DOWNTO
	vhdlRESERVED_ELSE
	vhdlRESERVED_ELSIF
	vhdlRESERVED_END
	vhdlRESERVED_ENTITY
	vhdlRESERVED_EXIT
	vhdlRESERVED_FAIRNESS
	vhdlRESERVED_FILE
	vhdlRESERVED_FOR
	vhdlRESERVED_FORCE
	vhdlRESERVED_FUNCTION
	vhdlRESERVED_GENERATE
	vhdlRESERVED_GENERIC
	vhdlRESERVED_GROUP
	vhdlRESERVED_GUARDED
	vhdlRESERVED_IF
	vhdlRESERVED_IMPURE
	vhdlRESERVED_IN
	vhdlRESERVED_INERTIAL
	vhdlRESERVED_INOUT
	vhdlRESERVED_IS
	vhdlRESERVED_LABEL
	vhdlRESERVED_LIBRARY
	vhdlRESERVED_LINKAGE
	vhdlRESERVED_LITERAL
	vhdlRESERVED_LOOP
	vhdlRESERVED_MAP
	vhdlRESERVED_MOD
	vhdlRESERVED_NAND
	vhdlRESERVED_NEW
	vhdlRESERVED_NEXT
	vhdlRESERVED_NOR
	vhdlRESERVED_NOT
	vhdlRESERVED_NULL
	vhdlRESERVED_OF
	vhdlRESERVED_ON
	vhdlRESERVED_OPEN
	vhdlRESERVED_OR
	vhdlRESERVED_OTHERS
	vhdlRESERVED_OUT
	vhdlRESERVED_PACKAGE
	vhdlRESERVED_PARAMETER
	vhdlRESERVED_PORT
	vhdlRESERVED_POSTPONED
	vhdlRESERVED_PROCEDURE
	vhdlRESERVED_PROCESS
	vhdlRESERVED_PROPERTY
	vhdlRESERVED_PROTECTED
	vhdlRESERVED_PRIVATE
	vhdlRESERVED_PURE
	vhdlRESERVED_RANGE
	vhdlRESERVED_RECORD
	vhdlRESERVED_REGISTER
	vhdlRESERVED_REJECT
	vhdlRESERVED_RELEASE
	vhdlRESERVED_REM
	vhdlRESERVED_REPORT
	vhdlRESERVED_RESTRICT
	vhdlRESERVED_RETURN
	vhdlRESERVED_ROL
	vhdlRESERVED_ROR
	vhdlRESERVED_SELECT
	vhdlRESERVED_SEQUENCE
	vhdlRESERVED_SEVERITY
	vhdlRESERVED_SIGNAL
	vhdlRESERVED_SHARED
	vhdlRESERVED_SLA
	vhdlRESERVED_SLL
	vhdlRESERVED_SRA
	vhdlRESERVED_SRL
	vhdlRESERVED_STRONG
	vhdlRESERVED_SUBTYPE
	vhdlRESERVED_THEN
	vhdlRESERVED_TO
	vhdlRESERVED_TRANSPORT
	vhdlRESERVED_TYPE
	vhdlRESERVED_UNAFFECTED
	vhdlRESERVED_UNITS
	vhdlRESERVED_UNTIL
	vhdlRESERVED_USE
	vhdlRESERVED_VARIABLE
	vhdlRESERVED_VIEW
	vhdlRESERVED_VMODE
	vhdlRESERVED_VPKG
	vhdlRESERVED_VPROP
	vhdlRESERVED_VUNIT
	vhdlRESERVED_WAIT
	vhdlRESERVED_WHEN
	vhdlRESERVED_WHILE
	vhdlRESERVED_WITH
	vhdlRESERVED_XNOR
	vhdlRESERVED_XOR

	vhdlRESERVED_END_MARKER // internal use only

	vhdlDIRECTIVE_BODY
	vhdlDIRECTIVE_CONSTANT_BUILTIN
	vhdlDIRECTIVE_ERROR
	vhdlDIRECTIVE_PROTECT
	vhdlDIRECTIVE_WARNING
	vhdlDIRECTIVE_NEWLINE

	vhdlDELIMITER_GRAVE_ACCENT
	vhdlDELIMITER_BOX

	_ // delimiter end marker; preserves mirrored token numbering

	vhdlTOKEN_DECIMAL_INTEGER
	vhdlTOKEN_DECIMAL_FLOAT
	vhdlTOKEN_BASED_BASE
	vhdlTOKEN_BASED_INTEGER
	vhdlTOKEN_BASED_FLOAT
	vhdlTOKEN_CHARACTER_LITERAL
	vhdlTOKEN_STRING_LITERAL
	vhdlTOKEN_STRING_LITERAL_STD_LOGIC
	vhdlTOKEN_BIT_STRING_LENGTH
	vhdlTOKEN_BIT_STRING_BASE
	vhdlTOKEN_BIT_STRING_VALUE
	vhdlTOKEN_OPERATOR_SYMBOL
	vhdlTOKEN_LINE_COMMENT_START
	vhdlTOKEN_BLOCK_COMMENT_START
	vhdlTOKEN_BLOCK_COMMENT_END
	vhdlTOKEN_COMMENT_CONTENT

	vhdlTOKEN_END_MARKER // internal use only

	vhdlATTRIBUTE_FUNCTION
	vhdlATTRIBUTE_IMPURE_FUNCTION
	vhdlATTRIBUTE_MODE_VIEW
	vhdlATTRIBUTE_PURE_FUNCTION
	vhdlATTRIBUTE_RANGE
	vhdlATTRIBUTE_SIGNAL
	vhdlATTRIBUTE_SUBTYPE
	vhdlATTRIBUTE_TYPE
	vhdlATTRIBUTE_VALUE

	vhdlLIBRARY_ATTRIBUTE
	vhdlLIBRARY_CONSTANT
	vhdlLIBRARY_CONSTANT_BOOLEAN
	vhdlLIBRARY_CONSTANT_CHARACTER
	vhdlLIBRARY_CONSTANT_DEBUG
	vhdlLIBRARY_CONSTANT_ENV
	vhdlLIBRARY_CONSTANT_STANDARD
	vhdlLIBRARY_CONSTANT_STD_LOGIC
	vhdlLIBRARY_CONSTANT_UNIT
	vhdlLIBRARY_FUNCTION
	vhdlLIBRARY_NAMESPACE
	vhdlLIBRARY_TYPE

	vhdlEND_OF_FILE

	vhdlERROR_SENTINEL

	// Internal-only types (not emitted as result symbols):
	vhdlLINE_COMMENT_START
	vhdlBLOCK_COMMENT_START
	vhdlSTRING_LITERAL_STD_LOGIC_START
	vhdlBASE_SPECIFIER_BINARY
	vhdlBASE_SPECIFIER_OCTAL
	vhdlBASE_SPECIFIER_DECIMAL
	vhdlBASE_SPECIFIER_HEX
	vhdlIDENTIFIER_EXPECTING_LETTER
)

// ---------------------------------------------------------------------------
// Concrete symbol IDs mapping tok[i] -> i+37
// ---------------------------------------------------------------------------

func vhdlSymbolForTok(tok int) gotreesitter.Symbol {
	return gotreesitter.Symbol(tok + 37)
}

// ---------------------------------------------------------------------------
// Trie for greedy token matching (replaces the C balanced BST TokenTree)
// ---------------------------------------------------------------------------

type vhdlTrieNode struct {
	children map[rune]*vhdlTrieNode
	types    []int // token types at this match point (may have multiple)
}

func newVhdlTrieNode() *vhdlTrieNode {
	return &vhdlTrieNode{children: make(map[rune]*vhdlTrieNode)}
}

type vhdlTrie struct {
	root *vhdlTrieNode
}

func newVhdlTrie() *vhdlTrie {
	return &vhdlTrie{root: newVhdlTrieNode()}
}

// insert adds a pattern->type mapping. The pattern characters are stored
// as-is (caller must pass lowercase for case-insensitive keywords).
// If the pattern contains underscores, intermediate nodes get the
// IDENTIFIER_EXPECTING_LETTER type injected (matching the C node_setup).
func (t *vhdlTrie) insert(pattern string, tokenType int) {
	node := t.root
	for i, ch := range pattern {
		if ch == '_' {
			// If no types on this node yet, add IDENTIFIER_EXPECTING_LETTER
			if len(node.types) == 0 {
				node.types = append(node.types, vhdlIDENTIFIER_EXPECTING_LETTER)
			} else {
				hasIDExpecting := false
				for _, tt := range node.types {
					if tt == vhdlIDENTIFIER_EXPECTING_LETTER {
						hasIDExpecting = true
						break
					}
				}
				if !hasIDExpecting {
					node.types = append(node.types, vhdlIDENTIFIER_EXPECTING_LETTER)
				}
			}
		}
		child, ok := node.children[ch]
		if !ok {
			child = newVhdlTrieNode()
			node.children[ch] = child
		}
		node = child
		_ = i
	}
	// Add type to terminal node (avoid duplicates)
	for _, tt := range node.types {
		if tt == tokenType {
			return
		}
	}
	node.types = append(node.types, tokenType)
}

// match performs a greedy match on the lexer, advancing it character by
// character and returning the type list of the longest match.
func (t *vhdlTrie) match(lexer *gotreesitter.ExternalLexer) []int {
	lookahead := vhdlLowercase(lexer.Lookahead())
	var types []int
	node := t.root

	for {
		if lexer.Lookahead() == 0 {
			break
		}
		child, ok := node.children[lookahead]
		if !ok {
			break
		}
		lexer.Advance(false)
		lookahead = vhdlLowercase(lexer.Lookahead())
		if len(child.types) > 0 {
			lexer.MarkEnd()
		}
		types = child.types
		node = child
	}
	return types
}

// ---------------------------------------------------------------------------
// Global trie (built once, like the C token_tree)
// ---------------------------------------------------------------------------

var vhdlTokenTrie *vhdlTrie

func init() {
	vhdlTokenTrie = newVhdlTrie()

	// Reserved words (all lowercase in the trie; lexer lowercases on lookup)
	vhdlRegisterReserved(vhdlTokenTrie)
	vhdlRegisterDirectives(vhdlTokenTrie)
	vhdlRegisterDelimiters(vhdlTokenTrie)
	vhdlRegisterOperatorSymbols(vhdlTokenTrie)
	vhdlRegisterAttributes(vhdlTokenTrie)
	vhdlRegisterBaseSpecifiers(vhdlTokenTrie)

	// Library registrations
	vhdlRegisterStdEnv(vhdlTokenTrie)
	vhdlRegisterStdStandard(vhdlTokenTrie)
	vhdlRegisterStdTextio(vhdlTokenTrie)
	vhdlRegisterIeeeStdLogic1164(vhdlTokenTrie)
	vhdlRegisterIeeeNumericStd(vhdlTokenTrie)
	vhdlRegisterIeeeFixedPkg(vhdlTokenTrie)
	vhdlRegisterIeeeFloatPkg(vhdlTokenTrie)
	vhdlRegisterIeeeMathReal(vhdlTokenTrie)
	vhdlRegisterIeeeMathComplex(vhdlTokenTrie)
}

func vhdlRegisterReserved(t *vhdlTrie) {
	keywords := []struct {
		kw  string
		tok int
	}{
		{"abs", vhdlRESERVED_ABS},
		{"access", vhdlRESERVED_ACCESS},
		{"after", vhdlRESERVED_AFTER},
		{"alias", vhdlRESERVED_ALIAS},
		{"all", vhdlRESERVED_ALL},
		{"and", vhdlRESERVED_AND},
		{"architecture", vhdlRESERVED_ARCHITECTURE},
		{"array", vhdlRESERVED_ARRAY},
		{"assert", vhdlRESERVED_ASSERT},
		{"assume", vhdlRESERVED_ASSUME},
		{"attribute", vhdlRESERVED_ATTRIBUTE},
		{"begin", vhdlRESERVED_BEGIN},
		{"block", vhdlRESERVED_BLOCK},
		{"body", vhdlRESERVED_BODY},
		{"buffer", vhdlRESERVED_BUFFER},
		{"bus", vhdlRESERVED_BUS},
		{"case", vhdlRESERVED_CASE},
		{"component", vhdlRESERVED_COMPONENT},
		{"configuration", vhdlRESERVED_CONFIGURATION},
		{"constant", vhdlRESERVED_CONSTANT},
		{"context", vhdlRESERVED_CONTEXT},
		{"cover", vhdlRESERVED_COVER},
		{"default", vhdlRESERVED_DEFAULT},
		{"disconnect", vhdlRESERVED_DISCONNECT},
		{"downto", vhdlRESERVED_DOWNTO},
		{"else", vhdlRESERVED_ELSE},
		{"elsif", vhdlRESERVED_ELSIF},
		{"end", vhdlRESERVED_END},
		{"entity", vhdlRESERVED_ENTITY},
		{"exit", vhdlRESERVED_EXIT},
		{"fairness", vhdlRESERVED_FAIRNESS},
		{"file", vhdlRESERVED_FILE},
		{"for", vhdlRESERVED_FOR},
		{"force", vhdlRESERVED_FORCE},
		{"function", vhdlRESERVED_FUNCTION},
		{"generate", vhdlRESERVED_GENERATE},
		{"generic", vhdlRESERVED_GENERIC},
		{"group", vhdlRESERVED_GROUP},
		{"guarded", vhdlRESERVED_GUARDED},
		{"if", vhdlRESERVED_IF},
		{"impure", vhdlRESERVED_IMPURE},
		{"in", vhdlRESERVED_IN},
		{"inertial", vhdlRESERVED_INERTIAL},
		{"inout", vhdlRESERVED_INOUT},
		{"is", vhdlRESERVED_IS},
		{"label", vhdlRESERVED_LABEL},
		{"library", vhdlRESERVED_LIBRARY},
		{"linkage", vhdlRESERVED_LINKAGE},
		{"literal", vhdlRESERVED_LITERAL},
		{"loop", vhdlRESERVED_LOOP},
		{"map", vhdlRESERVED_MAP},
		{"mod", vhdlRESERVED_MOD},
		{"nand", vhdlRESERVED_NAND},
		{"new", vhdlRESERVED_NEW},
		{"next", vhdlRESERVED_NEXT},
		{"nor", vhdlRESERVED_NOR},
		{"not", vhdlRESERVED_NOT},
		{"null", vhdlRESERVED_NULL},
		{"of", vhdlRESERVED_OF},
		{"on", vhdlRESERVED_ON},
		{"open", vhdlRESERVED_OPEN},
		{"or", vhdlRESERVED_OR},
		{"others", vhdlRESERVED_OTHERS},
		{"out", vhdlRESERVED_OUT},
		{"package", vhdlRESERVED_PACKAGE},
		{"parameter", vhdlRESERVED_PARAMETER},
		{"port", vhdlRESERVED_PORT},
		{"postponed", vhdlRESERVED_POSTPONED},
		{"procedure", vhdlRESERVED_PROCEDURE},
		{"process", vhdlRESERVED_PROCESS},
		{"property", vhdlRESERVED_PROPERTY},
		{"protected", vhdlRESERVED_PROTECTED},
		{"private", vhdlRESERVED_PRIVATE},
		{"pure", vhdlRESERVED_PURE},
		{"range", vhdlRESERVED_RANGE},
		{"record", vhdlRESERVED_RECORD},
		{"register", vhdlRESERVED_REGISTER},
		{"reject", vhdlRESERVED_REJECT},
		{"release", vhdlRESERVED_RELEASE},
		{"rem", vhdlRESERVED_REM},
		{"report", vhdlRESERVED_REPORT},
		{"restrict", vhdlRESERVED_RESTRICT},
		{"return", vhdlRESERVED_RETURN},
		{"rol", vhdlRESERVED_ROL},
		{"ror", vhdlRESERVED_ROR},
		{"select", vhdlRESERVED_SELECT},
		{"sequence", vhdlRESERVED_SEQUENCE},
		{"severity", vhdlRESERVED_SEVERITY},
		{"signal", vhdlRESERVED_SIGNAL},
		{"shared", vhdlRESERVED_SHARED},
		{"sla", vhdlRESERVED_SLA},
		{"sll", vhdlRESERVED_SLL},
		{"sra", vhdlRESERVED_SRA},
		{"srl", vhdlRESERVED_SRL},
		{"strong", vhdlRESERVED_STRONG},
		{"subtype", vhdlRESERVED_SUBTYPE},
		{"then", vhdlRESERVED_THEN},
		{"to", vhdlRESERVED_TO},
		{"transport", vhdlRESERVED_TRANSPORT},
		{"type", vhdlRESERVED_TYPE},
		{"unaffected", vhdlRESERVED_UNAFFECTED},
		{"units", vhdlRESERVED_UNITS},
		{"until", vhdlRESERVED_UNTIL},
		{"use", vhdlRESERVED_USE},
		{"variable", vhdlRESERVED_VARIABLE},
		{"view", vhdlRESERVED_VIEW},
		{"vmode", vhdlRESERVED_VMODE},
		{"vpkg", vhdlRESERVED_VPKG},
		{"vprop", vhdlRESERVED_VPROP},
		{"vunit", vhdlRESERVED_VUNIT},
		{"wait", vhdlRESERVED_WAIT},
		{"when", vhdlRESERVED_WHEN},
		{"while", vhdlRESERVED_WHILE},
		{"with", vhdlRESERVED_WITH},
		{"xnor", vhdlRESERVED_XNOR},
		{"xor", vhdlRESERVED_XOR},
	}
	for _, kw := range keywords {
		t.insert(kw.kw, kw.tok)
	}
}

func vhdlRegisterDirectives(t *vhdlTrie) {
	t.insert("protect", vhdlDIRECTIVE_PROTECT)
	t.insert("warning", vhdlDIRECTIVE_WARNING)
	t.insert("error", vhdlDIRECTIVE_ERROR)

	// Case-sensitive directive constants (inserted as-is)
	for _, name := range []string{
		"VHDL_VERSION", "TOOL_TYPE", "TOOL_VENDOR",
		"TOOL_NAME", "TOOL_EDITION", "TOOL_VERSION",
	} {
		t.insert(strings.ToLower(name), vhdlDIRECTIVE_CONSTANT_BUILTIN)
	}

	t.insert("\r", vhdlDIRECTIVE_NEWLINE)
	t.insert("\n", vhdlDIRECTIVE_NEWLINE)
	t.insert("\r\n", vhdlDIRECTIVE_NEWLINE)
	t.insert("\n\r", vhdlDIRECTIVE_NEWLINE)
}

func vhdlRegisterDelimiters(t *vhdlTrie) {
	t.insert("`", vhdlDELIMITER_GRAVE_ACCENT)
	t.insert("<>", vhdlDELIMITER_BOX)
	t.insert("--", vhdlLINE_COMMENT_START)
	t.insert("/*", vhdlBLOCK_COMMENT_START)
}

func vhdlRegisterOperatorSymbols(t *vhdlTrie) {
	ops := []string{
		"\"??\"", "\"and\"", "\"or\"", "\"nand\"", "\"nor\"",
		"\"xor\"", "\"xnor\"", "\"=\"", "\"/=\"", "\"<\"",
		"\"<=\"", "\">\"", "\">=\"", "\"?=\"", "\"?/=\"",
		"\"?<\"", "\"?<=\"", "\"?>\"", "\"?>=\"", "\"sll\"",
		"\"srl\"", "\"sla\"", "\"sra\"", "\"rol\"", "\"ror\"",
		"\"+\"", "\"-\"", "\"&\"", "\"*\"", "\"/\"",
		"\"mod\"", "\"rem\"", "\"**\"", "\"abs\"", "\"not\"",
	}
	for _, op := range ops {
		t.insert(op, vhdlTOKEN_OPERATOR_SYMBOL)
	}
}

func vhdlRegisterAttributes(t *vhdlTrie) {
	attrDefs := []struct {
		name string
		tok  int
	}{
		{"base", vhdlATTRIBUTE_TYPE},
		{"left", vhdlATTRIBUTE_VALUE},
		{"right", vhdlATTRIBUTE_VALUE},
		{"high", vhdlATTRIBUTE_VALUE},
		{"low", vhdlATTRIBUTE_VALUE},
		{"ascending", vhdlATTRIBUTE_VALUE},
		{"length", vhdlATTRIBUTE_PURE_FUNCTION},
		{"range", vhdlATTRIBUTE_RANGE},
		{"reverse_range", vhdlATTRIBUTE_RANGE},
		{"subtype", vhdlATTRIBUTE_SUBTYPE},
		{"image", vhdlATTRIBUTE_PURE_FUNCTION},
		{"pos", vhdlATTRIBUTE_PURE_FUNCTION},
		{"succ", vhdlATTRIBUTE_PURE_FUNCTION},
		{"pred", vhdlATTRIBUTE_PURE_FUNCTION},
		{"leftof", vhdlATTRIBUTE_PURE_FUNCTION},
		{"rightof", vhdlATTRIBUTE_PURE_FUNCTION},
		{"value", vhdlATTRIBUTE_PURE_FUNCTION},
		{"val", vhdlATTRIBUTE_PURE_FUNCTION},
		{"designated_subtype", vhdlATTRIBUTE_SUBTYPE},
		{"reflect", vhdlATTRIBUTE_IMPURE_FUNCTION},
		{"left", vhdlATTRIBUTE_FUNCTION},
		{"right", vhdlATTRIBUTE_FUNCTION},
		{"high", vhdlATTRIBUTE_FUNCTION},
		{"low", vhdlATTRIBUTE_FUNCTION},
		{"length", vhdlATTRIBUTE_FUNCTION},
		{"ascending", vhdlATTRIBUTE_FUNCTION},
		{"index", vhdlATTRIBUTE_SUBTYPE},
		{"element", vhdlATTRIBUTE_SUBTYPE},
		{"delayed", vhdlATTRIBUTE_SIGNAL},
		{"stable", vhdlATTRIBUTE_SIGNAL},
		{"quiet", vhdlATTRIBUTE_SIGNAL},
		{"transaction", vhdlATTRIBUTE_SIGNAL},
		{"event", vhdlATTRIBUTE_FUNCTION},
		{"active", vhdlATTRIBUTE_FUNCTION},
		{"last_event", vhdlATTRIBUTE_FUNCTION},
		{"last_active", vhdlATTRIBUTE_FUNCTION},
		{"last_value", vhdlATTRIBUTE_FUNCTION},
		{"driving", vhdlATTRIBUTE_FUNCTION},
		{"driving_value", vhdlATTRIBUTE_FUNCTION},
		{"simple_name", vhdlATTRIBUTE_VALUE},
		{"instance_name", vhdlATTRIBUTE_VALUE},
		{"path_name", vhdlATTRIBUTE_VALUE},
		{"record", vhdlATTRIBUTE_TYPE},
		{"value", vhdlATTRIBUTE_VALUE},
		{"signal", vhdlATTRIBUTE_SIGNAL},
		{"converse", vhdlATTRIBUTE_MODE_VIEW},
	}
	for _, a := range attrDefs {
		t.insert(a.name, a.tok)
	}
}

func vhdlRegisterBaseSpecifiers(t *vhdlTrie) {
	t.insert("b", vhdlBASE_SPECIFIER_BINARY)
	t.insert("o", vhdlBASE_SPECIFIER_OCTAL)
	t.insert("x", vhdlBASE_SPECIFIER_HEX)
	t.insert("ub", vhdlBASE_SPECIFIER_BINARY)
	t.insert("uo", vhdlBASE_SPECIFIER_OCTAL)
	t.insert("ux", vhdlBASE_SPECIFIER_HEX)
	t.insert("sb", vhdlBASE_SPECIFIER_BINARY)
	t.insert("so", vhdlBASE_SPECIFIER_OCTAL)
	t.insert("sx", vhdlBASE_SPECIFIER_HEX)
	t.insert("d", vhdlBASE_SPECIFIER_DECIMAL)
}

func vhdlRegisterStdEnv(t *vhdlTrie) {
	// Types
	for _, name := range []string{
		"dayofweek", "time_record", "directory_items", "directory",
		"dir_open_status", "dir_open_status_range_record",
		"dir_create_status", "dir_create_status_range_record",
		"dir_delete_status", "dir_delete_status_range_record",
		"file_delete_status", "file_delete_status_range_record",
		"call_path_element", "call_path_vector", "call_path_vector_ptr",
	} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
	// Constants
	for _, name := range []string{
		"sunday", "monday", "tuesday", "wednesday", "thursday",
		"friday", "saturday",
		"status_ok", "status_not_found", "status_no_directory",
		"status_access_denied", "status_item_exists", "status_not_empty",
		"status_no_file", "status_error",
		"dir_separator",
	} {
		t.insert(name, vhdlLIBRARY_CONSTANT_ENV)
	}
	// Functions
	for _, name := range []string{
		"stop", "finish", "resolution_limit", "localtime", "gmtime",
		"epoch", "time_to_seconds", "seconds_to_time", "to_string",
		"file_name", "file_path", "file_line", "minimum", "maximum",
		"dir_open", "dir_close", "dir_itemexists", "dir_itemisdir",
		"dir_itemisfile", "dir_workingdir", "dir_createdir",
		"dir_deletedir", "dir_deletefile", "getenv",
		"vhdl_version", "tool_type", "tool_vendor", "tool_name",
		"tool_edition", "tool_version", "deallocate", "get_call_path",
		"pslassertfailed", "psliscovered", "setpslcoverassert",
		"getpslcoverassert", "pslisassertcovered", "clearpslstate",
		"isvhdlassertfailed", "getvhdlassertcount", "clearvhdlassert",
		"setvhdlassertenable", "getvhdlassertenable",
		"setvhdlassertformat", "getvhdlassertformat",
		"setvhdlreadseverity", "getvhdlreadseverity",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
	// Namespaces
	t.insert("work", vhdlLIBRARY_NAMESPACE)
	t.insert("std", vhdlLIBRARY_NAMESPACE)
	t.insert("ieee", vhdlLIBRARY_NAMESPACE)
}

func vhdlRegisterStdStandard(t *vhdlTrie) {
	// Types
	for _, name := range []string{
		"range_direction", "range_direction_range_record",
		"boolean", "boolean_range_record",
		"bit", "bit_range_record",
		"character", "character_range_record",
		"severity_level", "severity_level_range_record",
		"universal_integer_range_record",
		"universal_real", "universal_real_range_record",
		"integer", "integer_range_record",
		"real", "real_range_record",
		"time", "time_range_record",
		"delay_length", "natural", "positive", "string",
		"boolean_vector", "bit_vector", "integer_vector",
		"real_vector", "time_vector",
		"file_open_kind", "file_open_kind_range_record",
		"file_open_status", "file_open_status_range_record",
		"file_open_state", "file_open_state_range_record",
		"file_origin_kind", "file_origin_kind_range_record",
	} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
	// Constants
	t.insert("true", vhdlLIBRARY_CONSTANT_BOOLEAN)
	t.insert("false", vhdlLIBRARY_CONSTANT_BOOLEAN)

	for _, name := range []string{
		"nul", "soh", "stx", "etx", "eot", "enq", "ack", "bel",
		"bs", "ht", "lf", "vt", "ff", "cr", "so", "si",
		"dle", "dc1", "dc2", "dc3", "dc4", "nak", "syn", "etb",
		"can", "em", "sub", "esc", "fsp", "gsp", "rsp", "usp", "del",
	} {
		t.insert(name, vhdlLIBRARY_CONSTANT_CHARACTER)
	}
	for i := 128; i <= 159; i++ {
		name := "c" + itoa(i)
		t.insert(name, vhdlLIBRARY_CONSTANT_CHARACTER)
	}

	for _, name := range []string{"note", "warning", "error", "failure"} {
		t.insert(name, vhdlLIBRARY_CONSTANT_DEBUG)
	}
	for _, name := range []string{
		"ascending", "descending",
		"read_mode", "write_mode", "append_mode", "read_write_mode",
		"open_ok", "name_error", "mode_error",
		"state_open", "state_closed",
		"file_origin_begin", "file_origin_current", "file_origin_end",
	} {
		t.insert(name, vhdlLIBRARY_CONSTANT_STANDARD)
	}
	for _, name := range []string{
		"fs", "ps", "ns", "us", "ms", "sec", "min", "hr",
	} {
		t.insert(name, vhdlLIBRARY_CONSTANT_UNIT)
	}
	// Functions
	for _, name := range []string{
		"rising_edge", "falling_edge", "now", "to_ostring", "to_hstring",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
	// Aliases
	for _, name := range []string{
		"to_bstring", "to_binary_string", "to_octal_string", "to_hex_string",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
	// Attributes
	t.insert("foreign", vhdlLIBRARY_ATTRIBUTE)
}

func vhdlRegisterStdTextio(t *vhdlTrie) {
	for _, name := range []string{
		"line", "line_vector", "text", "side", "side_range_record", "width",
	} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
	for _, name := range []string{"right", "left", "input", "output"} {
		t.insert(name, vhdlLIBRARY_CONSTANT)
	}
	for _, name := range []string{
		"file_open", "file_close", "file_rewind", "file_seek",
		"file_truncate", "file_state", "file_mode", "file_position",
		"file_size", "file_canseek", "read", "write", "flush",
		"endfile", "justify", "readline", "sread", "oread", "hread",
		"writeline", "tee", "owrite", "hwrite",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
	for _, name := range []string{
		"string_read", "bread", "binary_read", "octal_read", "hex_read",
		"swrite", "string_write", "bwrite", "binary_write", "octal_write",
		"hex_write",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
}

func vhdlRegisterIeeeStdLogic1164(t *vhdlTrie) {
	for _, name := range []string{
		"std_ulogic", "std_ulogic_vector", "std_logic", "std_logic_vector",
		"x01", "x01z", "ux01", "ux01z",
	} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
	// STRING_LITERAL_STD_LOGIC_START patterns
	for _, ch := range []string{"0", "1", "u", "x", "z", "w", "l", "h", "-"} {
		t.insert("\""+ch, vhdlSTRING_LITERAL_STD_LOGIC_START)
	}
	// TOKEN_STRING_LITERAL_STD_LOGIC patterns
	for _, ch := range []string{"0", "1", "u", "x", "z", "w", "l", "h", "-"} {
		t.insert("\""+ch+"\"", vhdlTOKEN_STRING_LITERAL_STD_LOGIC)
	}
	for _, name := range []string{
		"resolved", "to_bit", "to_bitvector", "to_stdulogic",
		"to_stdlogicvector", "to_stdulogicvector", "to_01",
		"to_x01", "to_x01z", "to_ux01", "is_x",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
	for _, name := range []string{
		"to_bit_vector", "to_bv", "to_std_logic_vector", "to_slv",
		"to_std_ulogic_vector", "to_sulv",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
}

func vhdlRegisterIeeeNumericStd(t *vhdlTrie) {
	for _, name := range []string{
		"unresolved_unsigned", "unresolved_signed", "unsigned", "signed",
	} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
	t.insert("copyrightnotice", vhdlLIBRARY_CONSTANT)
	for _, name := range []string{
		"find_leftmost", "find_rightmost", "shift_left", "shift_right",
		"rotate_left", "rotate_right", "resize", "to_integer",
		"to_unsigned", "to_signed", "std_match",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
	for _, name := range []string{"u_unsigned", "u_signed"} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
}

func vhdlRegisterIeeeFixedPkg(t *vhdlTrie) {
	for _, name := range []string{
		"fixed_round_style_type", "fixed_overflow_style_type",
		"round_type", "unresolved_ufixed", "unresolved_sfixed",
		"ufixed", "sfixed",
	} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
	for _, name := range []string{
		"fixed_round", "fixed_truncate", "fixed_saturate", "fixed_wrap",
		"round_nearest", "round_inf", "round_neginf", "round_zero",
	} {
		t.insert(name, vhdlLIBRARY_CONSTANT)
	}
	for _, name := range []string{
		"divide", "reciprocal", "remainder", "modulo", "add_carry",
		"scalb", "is_negative", "to_ufixed", "to_real", "to_sfixed",
		"ufixed_high", "ufixed_low", "sfixed_high", "sfixed_low",
		"saturate", "to_ufix", "to_sfix", "ufix_high", "ufix_low",
		"sfix_high", "sfix_low", "from_string", "from_ostring",
		"from_hstring",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
	for _, name := range []string{"u_ufixed", "u_sfixed"} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
	for _, name := range []string{
		"from_bstring", "from_binary_string", "from_octal_string",
		"from_hex_string",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
}

func vhdlRegisterIeeeFloatPkg(t *vhdlTrie) {
	for _, name := range []string{
		"unresolved_float", "float", "unresolved_float32", "float32",
		"unresolved_float64", "float64", "unresolved_float128", "float128",
		"valid_fpstate",
	} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
	for _, name := range []string{
		"nan", "quiet_nan", "neg_inf", "neg_normal", "neg_denormal",
		"neg_zero", "pos_zero", "pos_denormal", "pos_normal", "pos_inf",
		"isx", "fphdlsynth_or_real",
	} {
		t.insert(name, vhdlLIBRARY_CONSTANT)
	}
	for _, name := range []string{
		"classfp", "add", "subtract", "multiply", "dividebyp2", "mac",
		"eq", "ne", "lt", "gt", "le", "ge",
		"to_float32", "to_float64", "to_float128", "to_float",
		"realtobits", "bitstoreal", "break_number", "normalize",
		"copysign", "logb", "nextafter", "unordered", "finite",
		"isnan", "zerofp", "nanfp", "qnanfp", "pos_inffp",
		"neg_inffp", "neg_zerofp",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
	for _, name := range []string{
		"u_float", "u_float32", "u_float64", "u_float128",
	} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
}

func vhdlRegisterIeeeMathReal(t *vhdlTrie) {
	for _, name := range []string{
		"math_e", "math_1_over_e", "math_pi", "math_2_pi",
		"math_1_over_pi", "math_pi_over_2", "math_pi_over_3",
		"math_pi_over_4", "math_3_pi_over_2", "math_log_of_2",
		"math_log_of_10", "math_log2_of_e", "math_log10_of_e",
		"math_sqrt_2", "math_1_over_sqrt_2", "math_sqrt_pi",
		"math_deg_to_rad", "math_rad_to_deg",
	} {
		t.insert(name, vhdlLIBRARY_CONSTANT)
	}
	for _, name := range []string{
		"sign", "ceil", "floor", "round", "trunc", "realmax",
		"realmin", "uniform", "sqrt", "cbrt", "exp", "log",
		"log2", "log10", "sin", "cos", "tan", "arcsin", "arccos",
		"arctan", "sinh", "cosh", "tanh", "arcsinh", "arccosh",
		"arctanh",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
}

func vhdlRegisterIeeeMathComplex(t *vhdlTrie) {
	for _, name := range []string{
		"complex", "positive_real", "principal_value", "complex_polar",
	} {
		t.insert(name, vhdlLIBRARY_TYPE)
	}
	for _, name := range []string{
		"math_cbase_1", "math_cbase_j", "math_czero",
	} {
		t.insert(name, vhdlLIBRARY_CONSTANT)
	}
	for _, name := range []string{
		"cmplx", "get_principal_value", "complex_to_polar",
		"polar_to_complex", "arg", "conj",
	} {
		t.insert(name, vhdlLIBRARY_FUNCTION)
	}
}

// ---------------------------------------------------------------------------
// Helper: int-to-string without fmt
// ---------------------------------------------------------------------------

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ---------------------------------------------------------------------------
// Scanner state (serialized/deserialized)
// ---------------------------------------------------------------------------

type vhdlScannerState struct {
	isInDirective  bool
	isBlockComment bool
	bitStringBase  int
	base           int
}

// ---------------------------------------------------------------------------
// VhdlExternalScanner
// ---------------------------------------------------------------------------

// VhdlExternalScanner implements gotreesitter.ExternalScanner for
// tree-sitter-vhdl (jpt13653903). It handles identifier/keyword matching,
// number/string literals, comments, directives, and library tokens.
type VhdlExternalScanner struct{}

func (VhdlExternalScanner) Create() any {
	return &vhdlScannerState{}
}

func (VhdlExternalScanner) Destroy(payload any) {}

func (VhdlExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*vhdlScannerState)
	if len(buf) < 4 {
		return 0
	}
	var flags byte
	if s.isInDirective {
		flags |= 1
	}
	if s.isBlockComment {
		flags |= 2
	}
	buf[0] = flags
	buf[1] = byte(s.bitStringBase)
	buf[2] = byte(s.base >> 8)
	buf[3] = byte(s.base)
	return 4
}

func (VhdlExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*vhdlScannerState)
	*s = vhdlScannerState{}
	if len(buf) >= 4 {
		s.isInDirective = buf[0]&1 != 0
		s.isBlockComment = buf[0]&2 != 0
		s.bitStringBase = int(buf[1])
		s.base = int(buf[2])<<8 | int(buf[3])
	}
}

func (VhdlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	s := payload.(*vhdlScannerState)

	// Comment content / block comment end — highest priority
	if !vhdlValid(validSymbols, vhdlERROR_SENTINEL) && vhdlValid(validSymbols, vhdlTOKEN_COMMENT_CONTENT) {
		vhdlFinishCommentContent(lexer, s.isBlockComment)
		lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_COMMENT_CONTENT))
		return true
	}
	if !vhdlValid(validSymbols, vhdlERROR_SENTINEL) && vhdlValid(validSymbols, vhdlTOKEN_BLOCK_COMMENT_END) {
		if vhdlFinishBlockCommentEnd(lexer) {
			lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_BLOCK_COMMENT_END))
			return true
		}
		return false
	}

	// Skip whitespace (skip newlines unless inside a directive)
	vhdlSkipWhitespace(lexer, !s.isInDirective, true)

	// EOF
	if vhdlValid(validSymbols, vhdlEND_OF_FILE) && lexer.Lookahead() == 0 {
		lexer.SetResultSymbol(vhdlSymbolForTok(vhdlEND_OF_FILE))
		return true
	}

	// Extended identifier: \...\
	if vhdlValid(validSymbols, vhdlIDENTIFIER) && lexer.Lookahead() == '\\' {
		lexer.Advance(false)
		if !vhdlBoundedToken(lexer, '\\') {
			return false
		}
		lexer.SetResultSymbol(vhdlSymbolForTok(vhdlIDENTIFIER))
		return true
	}

	// Character literal: 'x'
	if (vhdlValid(validSymbols, vhdlTOKEN_CHARACTER_LITERAL) ||
		vhdlValid(validSymbols, vhdlLIBRARY_CONSTANT_STD_LOGIC)) && lexer.Lookahead() == '\'' {
		lexer.Advance(false)
		la := lexer.Lookahead()
		resultTok := vhdlTOKEN_CHARACTER_LITERAL
		if la == '0' || la == '1' || vhdlIsSpecialValue(la) {
			if vhdlValid(validSymbols, vhdlLIBRARY_CONSTANT_STD_LOGIC) {
				resultTok = vhdlLIBRARY_CONSTANT_STD_LOGIC
			}
		}
		if lexer.Lookahead() == 0 {
			return false
		}
		lexer.Advance(false)
		if lexer.Lookahead() != '\'' {
			return false
		}
		lexer.Advance(false)
		lexer.SetResultSymbol(vhdlSymbolForTok(resultTok))
		return true
	}

	// Digit-based literals
	if lexer.Lookahead() >= '0' && lexer.Lookahead() <= '9' {
		if !vhdlMayStartWithDigit(validSymbols) {
			return false
		}
		return vhdlParseDigitBasedLiteral(s, lexer)
	}

	// Bit string value (after bit_string_base was matched)
	if !vhdlValid(validSymbols, vhdlERROR_SENTINEL) && vhdlValid(validSymbols, vhdlTOKEN_BIT_STRING_VALUE) {
		if lexer.Lookahead() == '"' {
			lexer.Advance(false)
			if vhdlFinishStringLiteral(lexer, s.bitStringBase) {
				lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_BIT_STRING_VALUE))
				return true
			}
		}
		return false
	}

	// Based integer/float (after # delimiter)
	if !vhdlValid(validSymbols, vhdlERROR_SENTINEL) &&
		(vhdlValid(validSymbols, vhdlTOKEN_BASED_INTEGER) || vhdlValid(validSymbols, vhdlTOKEN_BASED_FLOAT)) {
		if lexer.Lookahead() == '#' {
			return vhdlParseBaseLiteral(lexer, s.base)
		}
		return false
	}

	// Directive body
	if !vhdlValid(validSymbols, vhdlERROR_SENTINEL) && vhdlValid(validSymbols, vhdlDIRECTIVE_BODY) &&
		vhdlGraphicCharacters(lexer) {
		lexer.SetResultSymbol(vhdlSymbolForTok(vhdlDIRECTIVE_BODY))
		return true
	}

	// Trie-based token matching
	firstCharIsLetter := (lexer.Lookahead() >= 'a' && lexer.Lookahead() <= 'z') ||
		(lexer.Lookahead() >= 'A' && lexer.Lookahead() <= 'Z')
	firstCharIsDoubleQuote := lexer.Lookahead() == '"'

	types := vhdlTokenTrie.match(lexer)

	// If trie didn't match and first char was a letter, it's an identifier
	if len(types) == 0 && firstCharIsLetter {
		lexer.MarkEnd()
		vhdlFinishIdentifier(lexer, false)
		lexer.SetResultSymbol(vhdlSymbolForTok(vhdlIDENTIFIER))
		return vhdlValid(validSymbols, vhdlIDENTIFIER)
	}

	// If trie didn't match and first char was '"', it's a string literal
	if len(types) == 0 && firstCharIsDoubleQuote {
		if !vhdlBoundedToken(lexer, '"') {
			return false
		}
		lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_STRING_LITERAL))
		return vhdlValid(validSymbols, vhdlTOKEN_STRING_LITERAL)
	}

	// Process the matched type list (iterate like the C linked list)
	foundCanBeIdentifier := false
	foundCannotBeIdentifier := false

	for idx := 0; idx < len(types); idx++ {
		tt := types[idx]

		if tt == vhdlLINE_COMMENT_START {
			if vhdlValid(validSymbols, vhdlTOKEN_LINE_COMMENT_START) {
				s.isBlockComment = false
				vhdlSkipWhitespace(lexer, false, false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_LINE_COMMENT_START))
				return true
			}
			return false
		}

		if tt == vhdlBLOCK_COMMENT_START {
			if vhdlValid(validSymbols, vhdlTOKEN_BLOCK_COMMENT_START) {
				s.isBlockComment = true
				vhdlSkipWhitespace(lexer, true, false)
				lexer.MarkEnd()
				lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_BLOCK_COMMENT_START))
				return true
			}
			return false
		}

		if vhdlCanStartIdentifier(tt) &&
			vhdlFinishIdentifier(lexer, tt == vhdlIDENTIFIER_EXPECTING_LETTER) {
			lexer.SetResultSymbol(vhdlSymbolForTok(vhdlIDENTIFIER))
			return true
		}

		if vhdlIsBaseSpecifier(tt) {
			if lexer.Lookahead() == '"' {
				s.bitStringBase = tt
				lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_BIT_STRING_BASE))
				return true
			}
			if idx == len(types)-1 {
				vhdlFinishIdentifier(lexer, false)
				lexer.SetResultSymbol(vhdlSymbolForTok(vhdlIDENTIFIER))
				return true
			}
			continue
		}

		if tt == vhdlLIBRARY_CONSTANT_CHARACTER && lexer.Lookahead() == '"' {
			// Could be a bit-string base, let the loop continue
			continue
		}

		if tt == vhdlTOKEN_OPERATOR_SYMBOL || tt == vhdlTOKEN_STRING_LITERAL_STD_LOGIC {
			if lexer.Lookahead() == '"' {
				if vhdlValid(validSymbols, vhdlTOKEN_STRING_LITERAL) {
					lexer.Advance(false)
					if !vhdlBoundedToken(lexer, '"') {
						return false
					}
					lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_STRING_LITERAL))
					return true
				}
				return false
			}
			if vhdlValid(validSymbols, tt) {
				lexer.SetResultSymbol(vhdlSymbolForTok(tt))
				return true
			}
			if idx == len(types)-1 && vhdlValid(validSymbols, vhdlTOKEN_STRING_LITERAL) {
				lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_STRING_LITERAL))
				return true
			}
			continue
		}

		if tt == vhdlSTRING_LITERAL_STD_LOGIC_START {
			if vhdlValid(validSymbols, vhdlTOKEN_STRING_LITERAL_STD_LOGIC) && vhdlBinaryStringLiteral(lexer) {
				if lexer.Lookahead() != '"' {
					lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_STRING_LITERAL_STD_LOGIC))
					return vhdlValid(validSymbols, vhdlTOKEN_STRING_LITERAL_STD_LOGIC)
				}
				lexer.Advance(false)
				// drop through to string literal parsing below
			}
			if vhdlValid(validSymbols, vhdlTOKEN_STRING_LITERAL) && vhdlBoundedToken(lexer, '"') {
				lexer.SetResultSymbol(vhdlSymbolForTok(vhdlTOKEN_STRING_LITERAL))
				return vhdlValid(validSymbols, vhdlTOKEN_STRING_LITERAL)
			}
			return false
		}

		if tt < vhdlERROR_SENTINEL && vhdlValid(validSymbols, tt) {
			lexer.SetResultSymbol(vhdlSymbolForTok(tt))
			if s.isInDirective {
				s.isInDirective = (tt != vhdlDIRECTIVE_NEWLINE)
			} else {
				s.isInDirective = (tt == vhdlDELIMITER_GRAVE_ACCENT)
			}
			return true
		}

		if vhdlCanBeIdentifier(s, tt) {
			foundCanBeIdentifier = true
		} else {
			foundCannotBeIdentifier = true
		}
	}

	if vhdlValid(validSymbols, vhdlIDENTIFIER) && foundCanBeIdentifier && !foundCannotBeIdentifier {
		lexer.SetResultSymbol(vhdlSymbolForTok(vhdlIDENTIFIER))
		return true
	}

	return false
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func vhdlValid(validSymbols []bool, tok int) bool {
	return tok < len(validSymbols) && validSymbols[tok]
}

func vhdlLowercase(ch rune) rune {
	return unicode.ToLower(ch)
}

func vhdlIsWhitespace(ch rune) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func vhdlSkipWhitespace(lexer *gotreesitter.ExternalLexer, skipNewline bool, discard bool) {
	if skipNewline {
		for vhdlIsWhitespace(lexer.Lookahead()) {
			lexer.Advance(discard)
		}
	} else {
		for lexer.Lookahead() == ' ' || lexer.Lookahead() == '\t' {
			lexer.Advance(discard)
		}
	}
}

func vhdlBoundedToken(lexer *gotreesitter.ExternalLexer, bound rune) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == bound {
			lexer.Advance(false)
			lexer.MarkEnd()
			if lexer.Lookahead() != bound {
				return true
			}
		}
		if lexer.Lookahead() == '\r' || lexer.Lookahead() == '\n' {
			return false
		}
		lexer.Advance(false)
	}
	return false
}

func vhdlIsLetterOrDigit(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
}

func vhdlFinishIdentifier(lexer *gotreesitter.ExternalLexer, expectLetter bool) bool {
	lookahead := vhdlLowercase(lexer.Lookahead())
	result := false

	if expectLetter {
		if !vhdlIsLetterOrDigit(lookahead) {
			return false
		}
	}

	for lexer.Lookahead() != 0 {
		lexer.MarkEnd()
		if lookahead == '_' {
			lexer.Advance(false)
			lookahead = vhdlLowercase(lexer.Lookahead())
		}
		if !vhdlIsLetterOrDigit(lookahead) {
			return result
		}
		lexer.Advance(false)
		lookahead = vhdlLowercase(lexer.Lookahead())
		result = true
	}
	return result
}

func vhdlIsSpecialValue(ch rune) bool {
	ch = unicode.ToLower(ch)
	switch ch {
	case 'u', 'x', 'z', 'w', 'l', 'h', '-':
		return true
	}
	return false
}

func vhdlBinaryStringLiteral(lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '_' {
			lexer.Advance(false)
		}
		la := lexer.Lookahead()
		if (la < '0' || la > '1') && !vhdlIsSpecialValue(la) {
			break
		}
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	return true
}

func vhdlOctalStringLiteral(lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '_' {
			lexer.Advance(false)
		}
		la := lexer.Lookahead()
		if (la < '0' || la > '7') && !vhdlIsSpecialValue(la) {
			break
		}
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	return true
}

func vhdlDecimalStringLiteral(lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '_' {
			lexer.Advance(false)
		}
		la := lexer.Lookahead()
		if (la < '0' || la > '9') && !vhdlIsSpecialValue(la) {
			break
		}
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	return true
}

func vhdlIsHexDigit(ch rune) bool {
	return (ch >= '0' && ch <= '9') ||
		(ch >= 'a' && ch <= 'f') ||
		(ch >= 'A' && ch <= 'F')
}

func vhdlHexStringLiteral(lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '_' {
			lexer.Advance(false)
		}
		la := lexer.Lookahead()
		if !vhdlIsHexDigit(la) && !vhdlIsSpecialValue(la) {
			break
		}
		lexer.Advance(false)
	}
	if lexer.Lookahead() != '"' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	return true
}

func vhdlFinishStringLiteral(lexer *gotreesitter.ExternalLexer, baseType int) bool {
	switch baseType {
	case vhdlBASE_SPECIFIER_BINARY:
		return vhdlBinaryStringLiteral(lexer)
	case vhdlBASE_SPECIFIER_OCTAL:
		return vhdlOctalStringLiteral(lexer)
	case vhdlBASE_SPECIFIER_DECIMAL:
		return vhdlDecimalStringLiteral(lexer)
	case vhdlBASE_SPECIFIER_HEX:
		return vhdlHexStringLiteral(lexer)
	default:
		return false
	}
}

func vhdlFinishLineComment(lexer *gotreesitter.ExternalLexer) {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '\r' || lexer.Lookahead() == '\n' {
			lexer.Advance(false)
			lexer.MarkEnd()
			return
		}
		lexer.Advance(false)
	}
	lexer.MarkEnd()
}

func vhdlFinishBlockComment(lexer *gotreesitter.ExternalLexer) {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '*' {
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				return
			}
			lexer.MarkEnd()
		} else if vhdlIsWhitespace(lexer.Lookahead()) {
			lexer.Advance(false)
		} else {
			lexer.Advance(false)
			lexer.MarkEnd()
		}
	}
}

func vhdlFinishBlockCommentEnd(lexer *gotreesitter.ExternalLexer) bool {
	for lexer.Lookahead() != 0 {
		if lexer.Lookahead() == '*' {
			lexer.Advance(false)
			if lexer.Lookahead() == '/' {
				lexer.Advance(false)
				lexer.MarkEnd()
				return true
			}
		} else {
			lexer.Advance(false)
		}
	}
	return false
}

func vhdlFinishCommentContent(lexer *gotreesitter.ExternalLexer, isBlockComment bool) {
	lexer.MarkEnd()
	if isBlockComment {
		vhdlFinishBlockComment(lexer)
	} else {
		vhdlFinishLineComment(lexer)
	}
}

func vhdlMayStartWithDigit(validSymbols []bool) bool {
	return vhdlValid(validSymbols, vhdlTOKEN_DECIMAL_INTEGER) ||
		vhdlValid(validSymbols, vhdlTOKEN_DECIMAL_FLOAT) ||
		vhdlValid(validSymbols, vhdlTOKEN_BASED_BASE) ||
		vhdlValid(validSymbols, vhdlTOKEN_BIT_STRING_LENGTH)
}

func vhdlParseInteger(lexer *gotreesitter.ExternalLexer) int {
	result := 0
	for lexer.Lookahead() != 0 {
		lexer.MarkEnd()
		if lexer.Lookahead() == '_' {
			lexer.Advance(false)
		}
		la := lexer.Lookahead()
		if la < '0' || la > '9' {
			return result
		}
		result = result*10 + int(la-'0')
		lexer.Advance(false)
	}
	return result
}

func vhdlParseDecimalExponent(lexer *gotreesitter.ExternalLexer) bool {
	lexer.Advance(false)
	la := lexer.Lookahead()
	if la == '+' || la == '-' {
		lexer.Advance(false)
	}
	la = lexer.Lookahead()
	if la < '0' || la > '9' {
		return false
	}
	vhdlParseInteger(lexer)
	return true
}

func vhdlParseDecimalFraction(lexer *gotreesitter.ExternalLexer) bool {
	lexer.Advance(false)
	la := lexer.Lookahead()
	if la < '0' || la > '9' {
		return false
	}
	vhdlParseInteger(lexer)
	la = lexer.Lookahead()
	if la == 'e' || la == 'E' {
		return vhdlParseDecimalExponent(lexer)
	}
	return true
}

func vhdlToDigit(ch rune) int {
	if ch >= '0' && ch <= '9' {
		return int(ch - '0')
	}
	if ch >= 'a' && ch <= 'z' {
		return int(ch-'a') + 10
	}
	if ch >= 'A' && ch <= 'Z' {
		return int(ch-'A') + 10
	}
	return -1
}

func vhdlBasedInteger(lexer *gotreesitter.ExternalLexer, base int) bool {
	for lexer.Lookahead() != 0 {
		lexer.MarkEnd()
		if lexer.Lookahead() == '_' {
			lexer.Advance(false)
		}
		digit := vhdlToDigit(lexer.Lookahead())
		if digit < 0 {
			return true
		}
		if digit >= base {
			return false
		}
		lexer.Advance(false)
	}
	return true
}

func vhdlParseBaseLiteral(lexer *gotreesitter.ExternalLexer, base int) bool {
	lexer.Advance(false) // consume '#'
	resultTok := vhdlTOKEN_BASED_INTEGER

	if !vhdlBasedInteger(lexer, base) {
		return false
	}
	if lexer.Lookahead() == '.' {
		lexer.Advance(false)
		resultTok = vhdlTOKEN_BASED_FLOAT
		if !vhdlBasedInteger(lexer, base) {
			return false
		}
	}
	if lexer.Lookahead() != '#' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	if lexer.Lookahead() == 'e' || lexer.Lookahead() == 'E' {
		if !vhdlParseDecimalExponent(lexer) {
			return false
		}
	}
	lexer.SetResultSymbol(vhdlSymbolForTok(resultTok))
	return true
}

func vhdlParseDigitBasedLiteral(s *vhdlScannerState, lexer *gotreesitter.ExternalLexer) bool {
	resultTok := vhdlTOKEN_DECIMAL_INTEGER

	s.base = vhdlParseInteger(lexer)

	la := vhdlLowercase(lexer.Lookahead())
	switch la {
	case '.':
		resultTok = vhdlTOKEN_DECIMAL_FLOAT
		if !vhdlParseDecimalFraction(lexer) {
			return false
		}
	case 'e':
		if !vhdlParseDecimalExponent(lexer) {
			return false
		}
	case '#':
		resultTok = vhdlTOKEN_BASED_BASE
	case 'b', 'o', 'd', 'x':
		lexer.Advance(false)
		if lexer.Lookahead() != '"' {
			lexer.SetResultSymbol(vhdlSymbolForTok(resultTok))
			return true
		}
		resultTok = vhdlTOKEN_BIT_STRING_LENGTH
	case 'u', 's':
		lexer.Advance(false)
		nextLa := vhdlLowercase(lexer.Lookahead())
		switch nextLa {
		case 'b', 'o', 'x':
			// valid
		default:
			lexer.SetResultSymbol(vhdlSymbolForTok(resultTok))
			return true
		}
		lexer.Advance(false)
		if lexer.Lookahead() != '"' {
			lexer.SetResultSymbol(vhdlSymbolForTok(resultTok))
			return true
		}
		resultTok = vhdlTOKEN_BIT_STRING_LENGTH
	}

	lexer.SetResultSymbol(vhdlSymbolForTok(resultTok))
	return true
}

func vhdlGraphicCharacters(lexer *gotreesitter.ExternalLexer) bool {
	la := lexer.Lookahead()
	if la == '\n' || la == '\r' {
		return false
	}
	for lexer.Lookahead() != 0 {
		la = lexer.Lookahead()
		if la == ' ' || la == '\t' || la == '\n' || la == '\r' {
			return true
		}
		lexer.Advance(false)
	}
	return false
}

// Token classification helpers (matching C can_be_identifier, can_start_identifier, is_base_specifier)

func vhdlCanStartIdentifier(tt int) bool {
	return (tt >= vhdlIDENTIFIER && tt < vhdlRESERVED_END_MARKER) ||
		(tt >= vhdlDIRECTIVE_BODY && tt <= vhdlDIRECTIVE_WARNING) ||
		(tt > vhdlTOKEN_END_MARKER && tt < vhdlERROR_SENTINEL) ||
		(tt == vhdlIDENTIFIER_EXPECTING_LETTER)
}

func vhdlCanBeIdentifier(s *vhdlScannerState, tt int) bool {
	if s.isInDirective {
		return vhdlCanStartIdentifier(tt)
	}
	return (tt == vhdlIDENTIFIER) ||
		(tt >= vhdlDIRECTIVE_BODY && tt <= vhdlDIRECTIVE_WARNING) ||
		(tt > vhdlTOKEN_END_MARKER && tt < vhdlERROR_SENTINEL)
}

func vhdlIsBaseSpecifier(tt int) bool {
	return tt >= vhdlBASE_SPECIFIER_BINARY && tt <= vhdlBASE_SPECIFIER_HEX
}
