package grammars

import (
	"strings"
	"sync"
)

type tagsQueryPattern struct {
	Query   string
	Symbols []string
}

var (
	inferredTagsQueryCache sync.Map // map[string]string
)

// inferredTagsQueryOverrides holds two kinds of entries, both per-language
// data, exactly like the pre-existing Go row:
//
//  1. Precision overrides: the shared inference table either misses a
//     language's conventional shapes or misnames them (a field-constrained
//     query resolves what a positional pattern cannot, e.g. the Go override
//     already does for the return-type overcapture bug). These use `name:`
//     and other field constraints verified against this repository's
//     compiled grammars, not against upstream tree-sitter, because a
//     grammargen/ts2go-compiled blob's field map is the fact that matters
//     here.
//
//  2. Golden-fixture freezes: python, javascript, typescript, tsx, java,
//     rust, c, cpp, and c_sharp each already carry a committed, exact-match
//     golden fixture in the core package (outline_golden_test.go,
//     testdata/outline/**). Those files sit outside grammars/ and are out of
//     scope for a grammars-data change, and TestOutlineGoldens compares full
//     JSON including DefinitionKinds() -- which is the compiled query's
//     static capture-name list, so even an unmatched new pattern changes it.
//     Any shared-inferredTagsQueryPatterns edit that a frozen language's
//     grammar happens to satisfy would silently move that language's golden
//     output. Snapshotting each one's pre-change resolved query here removes
//     that whole risk class: overrides short-circuit inference (see
//     inferredTagsQuery below) before the shared table is ever consulted, so
//     these nine stay byte-identical no matter what the shared table gains.
//     Each snapshot is the exact string ResolveTagsQuery produced before
//     this change; verified unchanged by TestOutlineGoldens continuing to
//     pass. A future change that also updates the goldens in the same
//     commit is free to replace any of these with a real fix.
//
//  3. Impossible-pattern fixes (2026-08-11, cgo_harness C-oracle exhaustive
//     sweep, cgo_harness/outline_differential_test.go): tree-sitter's C
//     ts_query_new statically rejects a pattern whose child node type can
//     never occur at that position in the grammar's rules ("Impossible
//     pattern"), and rejects the WHOLE multi-pattern query atomically --
//     one bad line drops every valid line for that language on the C side.
//     gotreesitter's Go NewQuery has no equivalent static check, so a dead
//     line "compiled" here while producing zero matches, silently masking
//     the C incompatibility. Because C reports only the FIRST impossible
//     pattern it hits, an initial hand-reported list (from a first C-oracle
//     pass) undercounted: fixing pattern 1 in a language's query routinely
//     exposed pattern 2 on the next compile attempt. Every row below was
//     therefore run against BOTH sides before being trusted fixed: Go side
//     via SExpr(lang) dumps of this repository's own compiled grammar
//     blobs plus an isolated single-pattern gotreesitter.NewQuery/Execute
//     firing test (0 matches = dead), C side via
//
//     CGO_ENABLED=1 GTS_PARITY_ALLOW_HOST=1 go test -tags "cgo treesitter_c_parity" \
//     ./cgo_harness -run 'TestOutlineOracleDifferential/CoreNine' -v
//
//     which now reports status=equal for all nine core languages (go,
//     python, javascript, typescript, tsx, rust, java, c, cpp) -- zero
//     query_incompatible, zero diverged. Final dead/corrected rows, all
//     confirmed 0-match on Go and "Impossible pattern" on C before the fix,
//     and firing correctly (or simply absent, superseded by a working row)
//     after it:
//     - javascript: (method_definition (identifier) @name) -- every JS
//     method_definition names through property_identifier,
//     computed_property_name, or private_property_identifier, never a
//     bare identifier.
//     - typescript, tsx: (function_declaration (type_identifier) @name);
//     (class_declaration (identifier) @name); (interface_declaration
//     (identifier) @name); (enum_declaration (type_identifier) @name).
//     The pattern is consistent: function/enum name through (identifier)
//     only, class/interface name through (type_identifier) only -- never
//     the other spelling, for any of the four.
//     - java: (class_declaration (type_identifier) @name);
//     (interface_declaration (type_identifier) @name); (enum_declaration
//     (type_identifier) @name). Java's own-name child is (identifier)
//     only for all three; type_identifier appears solely inside
//     type_parameters/superclass/super_interfaces/extends/implements
//     clauses, never as the declaration's own name.
//     - c, cpp: (function_definition (identifier) @name) and
//     (function_definition (field_identifier) @name) -- a
//     function_definition's name is always wrapped one level inside
//     function_declarator, never a bare direct child; the two wrapped
//     rows already cover both spellings. cpp's (type_definition
//     (identifier) @name) -- a typedef's target is always
//     (type_identifier) in this grammar family, never plain identifier.
//     - rust, c, cpp: (call_expression (field_identifier) @name) for a
//     method-call reference like "obj.method()" -- field_identifier is
//     always nested one level inside field_expression, never a bare
//     direct child of call_expression. Corrected in place (not deleted)
//     to (call_expression (field_expression (field_identifier) @name))
//     @reference.call, which fires correctly on the same witness.
//     Every row above is either a removal (a working alternate spelling
//     already covers the same capture name) or, for the three
//     field_identifier reference corrections, a reference.* capture:
//     OutlineTree's collectOutlineCandidates discards any match without a
//     definition.* capture before it is ever counted (outline.go), and
//     DefinitionKinds() filters CaptureNames() to the "definition." prefix
//     (isOutlineDefinitionCapture). So none of this can move Symbols, any
//     OutlineReport counter, or DefinitionKinds() for a frozen language --
//     confirmed by TestOutlineGoldens continuing to pass byte for byte
//     after every edit in this list.
//     python and c_sharp were checked against the same method (real
//     source, SExpr dump, isolated single-pattern firing test, and the
//     CoreNine C-oracle run above) and carry no structurally-impossible
//     row. c_sharp's method_declaration name/return-type collision is a
//     real, already-documented ambiguity (see the c_sharp golden fixture
//     purpose string), not a dead pattern, and is left exactly as shipped.
var inferredTagsQueryOverrides = map[string]string{
	"go": strings.Join([]string{
		"(function_declaration name: (identifier) @name) @definition.function",
		"(method_declaration name: (field_identifier) @name) @definition.method",
		"(call_expression function: (identifier) @name) @reference.call",
		"(call_expression function: (selector_expression field: (field_identifier) @name)) @reference.call",
	}, "\n"),

	// --- golden-fixture freezes (see comment above) ---

	"python": strings.Join([]string{
		"(function_definition (identifier) @name) @definition.function",
		"(class_definition (identifier) @name) @definition.class",
		"(call (identifier) @name) @reference.call",
		"(call (attribute (identifier) @name)) @reference.call",
	}, "\n"),
	"javascript": strings.Join([]string{
		"(function_declaration (identifier) @name) @definition.function",
		"(method_definition (property_identifier) @name) @definition.method",
		"(class_declaration (identifier) @name) @definition.class",
		"(call_expression (identifier) @name) @reference.call",
		"(call_expression (member_expression (property_identifier) @name)) @reference.call",
	}, "\n"),
	"typescript": strings.Join([]string{
		"(function_declaration (identifier) @name) @definition.function",
		"(method_definition (property_identifier) @name) @definition.method",
		"(class_declaration (type_identifier) @name) @definition.class",
		"(interface_declaration (type_identifier) @name) @definition.interface",
		"(enum_declaration (identifier) @name) @definition.type",
		"(call_expression (identifier) @name) @reference.call",
		"(call_expression (member_expression (property_identifier) @name)) @reference.call",
	}, "\n"),
	"tsx": strings.Join([]string{
		"(function_declaration (identifier) @name) @definition.function",
		"(method_definition (property_identifier) @name) @definition.method",
		"(class_declaration (type_identifier) @name) @definition.class",
		"(interface_declaration (type_identifier) @name) @definition.interface",
		"(enum_declaration (identifier) @name) @definition.type",
		"(call_expression (identifier) @name) @reference.call",
		"(call_expression (member_expression (property_identifier) @name)) @reference.call",
	}, "\n"),
	"java": strings.Join([]string{
		"(method_declaration (identifier) @name) @definition.method",
		"(class_declaration (identifier) @name) @definition.class",
		"(interface_declaration (identifier) @name) @definition.interface",
		"(enum_declaration (identifier) @name) @definition.type",
		"(constructor_declaration (identifier) @name) @definition.constructor",
		"(method_invocation (identifier) @name) @reference.call",
	}, "\n"),
	"rust": strings.Join([]string{
		"(function_item (identifier) @name) @definition.function",
		"(function_signature_item (identifier) @name) @definition.function",
		"(struct_item (type_identifier) @name) @definition.type",
		"(enum_item (type_identifier) @name) @definition.type",
		"(trait_item (type_identifier) @name) @definition.type",
		"(call_expression (identifier) @name) @reference.call",
		"(call_expression (field_expression (field_identifier) @name)) @reference.call",
		"(call_expression (scoped_identifier (identifier) @name)) @reference.call",
		"(macro_invocation (identifier) @name) @reference.call",
	}, "\n"),
	"c": strings.Join([]string{
		"(function_definition (function_declarator (identifier) @name)) @definition.function",
		"(function_definition (function_declarator (field_identifier) @name)) @definition.function",
		"(type_definition (type_identifier) @name) @definition.type",
		"(struct_specifier (type_identifier) @name) @definition.type",
		"(call_expression (identifier) @name) @reference.call",
		"(call_expression (field_expression (field_identifier) @name)) @reference.call",
	}, "\n"),
	"cpp": strings.Join([]string{
		"(function_definition (function_declarator (identifier) @name)) @definition.function",
		"(function_definition (function_declarator (field_identifier) @name)) @definition.function",
		"(type_definition (type_identifier) @name) @definition.type",
		"(class_specifier (type_identifier) @name) @definition.class",
		"(struct_specifier (type_identifier) @name) @definition.type",
		"(call_expression (identifier) @name) @reference.call",
		"(call_expression (field_expression (field_identifier) @name)) @reference.call",
	}, "\n"),
	"c_sharp": strings.Join([]string{
		"(method_declaration (identifier) @name) @definition.method",
		"(class_declaration (identifier) @name) @definition.class",
		"(interface_declaration (identifier) @name) @definition.interface",
		"(enum_declaration (identifier) @name) @definition.type",
		"(constructor_declaration (identifier) @name) @definition.constructor",
	}, "\n"),

	// --- stage-5 coverage growth overrides (2026-08-11) ---
	//
	// Every row below was verified against this repository's OWN compiled
	// grammar blob, not upstream tree-sitter: a real snippet was parsed,
	// Node.SExpr(lang) was read to confirm the exact child shape, and the
	// candidate pattern was run in isolation through gotreesitter.NewTagger
	// to confirm it fires exactly once per definition with the expected
	// name text and no duplicate/conflicting capture. None of these touch a
	// golden-frozen language.

	// PHP: the grammar has no plain "identifier" symbol at all (verified:
	// SymbolByName("identifier") absent) -- every name-bearing leaf is
	// "name" instead, which is why every shared-table pattern silently
	// gated PHP to an empty query. All seven shapes below are direct
	// children of their definition node in this grammar; enum_declaration
	// keeps the definition.type suffix on purpose so the existing
	// outlineKindRefinement row ("enum_declaration" -> "enum") applies
	// without a new table entry.
	"php": strings.Join([]string{
		"(function_definition (name) @name) @definition.function",
		"(method_declaration (name) @name) @definition.method",
		"(class_declaration (name) @name) @definition.class",
		"(interface_declaration (name) @name) @definition.interface",
		"(trait_declaration (name) @name) @definition.trait",
		"(enum_declaration (name) @name) @definition.type",
		"(enum_case (name) @name) @definition.enum_member",
	}, "\n"),

	// Erlang: function clauses nest the name one level inside
	// function_clause; a bare record_decl's own name is its only direct
	// atom child (field atoms sit inside their own record_field wrapper,
	// never a direct child of record_decl, so there is no ambiguity to
	// anchor against). definition.record is used directly rather than
	// definition.type plus an outlineKindRefinement row: the core
	// outline.go tables are out of scope for this change, and
	// "record_decl" is Erlang-unique (verified via SymbolByName across
	// every registered language) so a direct kind loses nothing.
	"erlang": strings.Join([]string{
		"(fun_decl (function_clause (atom) @name)) @definition.function",
		"(record_decl (atom) @name) @definition.record",
	}, "\n"),

	// OCaml: "name"/"pattern" fields resolve type_binding and let_binding
	// precisely (verified present via lang.FieldNames); module_binding and
	// class_binding do not carry a "name" field in this grammar, so those
	// two stay positional (verified: exactly one direct name-shaped child,
	// with no other candidate at that depth). let_binding covers both
	// functions and plain values -- OCaml's grammar does not distinguish
	// them structurally -- so "function" is used uniformly, matching the
	// upstream tree-sitter-ocaml tags convention.
	"ocaml": strings.Join([]string{
		"(type_definition (type_binding name: (type_constructor) @name)) @definition.type",
		"(value_definition (let_binding pattern: (value_name) @name)) @definition.function",
		"(module_definition (module_binding (module_name) @name)) @definition.module",
		"(exception_definition (constructor_declaration (constructor_name) @name)) @definition.exception",
		"(class_definition (class_binding (class_name) @name)) @definition.class",
		"(method_definition (method_name) @name) @definition.method",
	}, "\n"),

	// Bash and fish both expose a "name" field on function_definition.
	"bash": "(function_definition name: (word) @name) @definition.function",
	"fish": "(function_definition name: (word) @name) @definition.function",

	// GraphQL: no fields at all in this grammar (verified: lang.FieldNames
	// is empty), so every row is positional. Each definition node's own
	// "name" is always its first-reachable direct or one-level-nested
	// (name) node; member names (enum values) sit one level deeper still
	// and cannot collide with the parent's own name capture because the
	// parent pattern only looks at direct children.
	// enum_type_definition uses definition.enum directly (not
	// definition.type plus a refinement row): the core outline.go tables
	// are out of scope for this change, and "enum_type_definition" is
	// GraphQL-unique (verified via SymbolByName across every registered
	// language), so a direct kind loses nothing.
	"graphql": strings.Join([]string{
		"(object_type_definition (name) @name) @definition.type",
		"(interface_type_definition (name) @name) @definition.interface",
		"(enum_type_definition (name) @name) @definition.enum",
		"(enum_value_definition (enum_value (name) @name)) @definition.enum_member",
		"(input_object_type_definition (name) @name) @definition.type",
		"(union_type_definition (name) @name) @definition.union",
		"(scalar_type_definition (name) @name) @definition.scalar",
		"(directive_definition (name) @name) @definition.directive",
	}, "\n"),

	// Ruby: class/module name through the "constant" node type; the "left"
	// field on assignment excludes a plain read of a constant on the right
	// -- verified both directions with "CONST = 1" and "x = CONST" snippets
	// -- so only genuine constant bindings are captured.
	"ruby": strings.Join([]string{
		"(class (constant) @name) @definition.class",
		"(module (constant) @name) @definition.module",
		"(method (identifier) @name) @definition.method",
		"(singleton_method (identifier) @name) @definition.method",
		"(assignment left: (constant) @name) @definition.constant",
	}, "\n"),

	// Kotlin: class_declaration is reused for classes, interfaces, AND enum
	// classes, distinguished only by an anonymous keyword token or body
	// node type -- there is no per-shape node type the way Java has. The
	// three rows below are mutually exclusive by construction: "interface"
	// only ever appears on an interface; enum_class_body only ever appears
	// on an enum class; requiring (class_body), which enum classes never
	// carry, keeps the plain "class" row from also firing (and thereby
	// conflicting) on an enum class. Verified on interface+class+enum-class
	// source together with zero OmittedConflict. Function/method names use
	// "simple_identifier", never "identifier", which is why the shared
	// table's function_declaration rows never fired for Kotlin.
	"kotlin": strings.Join([]string{
		`(class_declaration "interface" (type_identifier) @name) @definition.interface`,
		"(class_declaration (type_identifier) @name (enum_class_body)) @definition.enum",
		`(class_declaration "class" (type_identifier) @name (class_body)) @definition.class`,
		"(object_declaration (type_identifier) @name) @definition.object",
		"(function_declaration (simple_identifier) @name) @definition.function",
		"(enum_entry (simple_identifier) @name) @definition.enum_member",
		"(type_alias (type_identifier) @name) @definition.type",
	}, "\n"),

	// Swift: the same class_declaration reuse as Kotlin (class, struct, AND
	// enum all parse as class_declaration), disambiguated the same way by
	// keyword token plus body node type. Constructors (init_declaration)
	// carry no name distinct from the fixed "init" keyword, so -- as with
	// Kotlin -- they are not captured; there is nothing to name that is not
	// already guessing.
	"swift": strings.Join([]string{
		"(protocol_declaration (type_identifier) @name) @definition.interface",
		`(class_declaration "class" (type_identifier) @name (class_body)) @definition.class`,
		`(class_declaration "struct" (type_identifier) @name (class_body)) @definition.struct`,
		"(class_declaration (type_identifier) @name (enum_class_body)) @definition.enum",
		"(protocol_function_declaration (simple_identifier) @name) @definition.method",
		"(function_declaration (simple_identifier) @name) @definition.function",
		"(enum_entry (simple_identifier) @name) @definition.enum_member",
	}, "\n"),

	// Scala: trait/class/object/enum/function/def all carry an explicit
	// "name:" field (verified via lang.FieldNames and firing witnesses),
	// so every row below is field-constrained and unambiguous by
	// construction; no anchor or body-shape disambiguation is needed the
	// way Kotlin and Swift require.
	"scala": strings.Join([]string{
		"(trait_definition name: (identifier) @name) @definition.interface",
		"(class_definition name: (identifier) @name) @definition.class",
		"(object_definition name: (identifier) @name) @definition.object",
		"(enum_definition name: (identifier) @name) @definition.enum",
		"(function_declaration name: (identifier) @name) @definition.function",
		"(function_definition name: (identifier) @name) @definition.function",
		"(simple_enum_case (identifier) @name) @definition.enum_member",
	}, "\n"),

	// Dart: class_definition already fires through the shared table, but
	// method/constructor/getter names live under function_signature /
	// constructor_signature / getter_signature -- none of which the shared
	// table names -- and enum_declaration's own name needs anchoring away
	// from enum_body's member identifiers (verified: enum_constant's
	// identifier sits one level deeper, inside enum_body, never a direct
	// child of enum_declaration, so no anchor is actually required, but the
	// direct-child requirement alone is what keeps this safe).
	"dart": strings.Join([]string{
		"(class_definition (identifier) @name) @definition.class",
		"(enum_declaration (identifier) @name) @definition.type",
		"(enum_constant (identifier) @name) @definition.enum_member",
		"(mixin_declaration (identifier) @name) @definition.mixin",
		"(function_signature (identifier) @name) @definition.function",
		"(constructor_signature (identifier) @name) @definition.constructor",
		"(getter_signature (identifier) @name) @definition.method",
	}, "\n"),

	// Zig: struct/enum literals carry no name of their own -- the name
	// comes from the surrounding "const X = struct {...}" variable
	// binding, so the pattern anchors on variable_declaration and checks
	// the value's shape. function_declaration needs the leading anchor
	// because a user-defined return type parses as a second, later
	// (identifier) sibling (e.g. "fn init(...) Point"); without the anchor
	// the two identifiers share one span with different names and both are
	// dropped as OmittedNameConflict -- verified both with and without the
	// anchor on that exact shape.
	"zig": strings.Join([]string{
		"(variable_declaration (identifier) @name (struct_declaration)) @definition.type",
		"(variable_declaration (identifier) @name (enum_declaration)) @definition.enum",
		"(function_declaration . (identifier) @name) @definition.function",
	}, "\n"),

	// Elixir: defmodule/def/defp/defmacro are ordinary calls, not distinct
	// node types (the grammar has no "module"/"function definition" node
	// at all) -- the same shape as Clojure. #eq?/#any-of? predicates on the
	// call's own head identifier are the only way to name the construct
	// without guessing; verified the predicate engine matches this
	// repository's compiled grammar output (anchors and #eq?/#any-of? all
	// fire as expected).
	"elixir": strings.Join([]string{
		`(call (identifier) @_head (arguments (alias) @name) (#eq? @_head "defmodule")) @definition.module`,
		`(call (identifier) @_head (arguments (call (identifier) @name)) (#any-of? @_head "def" "defp" "defmacro")) @definition.function`,
		// Carried over from the shared table's generic call reference so an
		// override does not regress reference coverage the shared inference
		// already gave Elixir (every construct, defmodule/def included, is
		// itself a "call" node in this grammar).
		"(call (identifier) @name) @reference.call",
	}, "\n"),

	// Julia: every definition wraps its name one to two levels deep
	// (module_definition direct; struct/abstract through type_head;
	// function through a call-shaped signature, mirroring how a Julia
	// function header actually parses). const_statement is anchored to the
	// assignment's first child so a constant that reads another constant
	// on its right-hand side is never mistaken for a second definition.
	"julia": strings.Join([]string{
		"(module_definition (identifier) @name) @definition.module",
		"(struct_definition (type_head (identifier) @name)) @definition.type",
		"(abstract_definition (type_head (identifier) @name)) @definition.type",
		"(function_definition (signature (call_expression (identifier) @name))) @definition.function",
		"(const_statement (assignment . (identifier) @name)) @definition.constant",
	}, "\n"),

	// Groovy: class_definition already fires through the shared table, but
	// function_definition needs the trailing anchor -- a typed method like
	// "String greet()" parses return type and name as two sibling
	// (identifier) nodes in that order, so an unanchored pattern captures
	// "String" (or drops both to OmittedNameConflict once both identifiers
	// match); anchoring the name immediately before parameter_list
	// resolves it to the actual method/function name in both the typed and
	// untyped ("def standalone()") cases.
	"groovy": strings.Join([]string{
		"(class_definition (identifier) @name) @definition.class",
		"(function_definition (identifier) @name . (parameter_list)) @definition.function",
	}, "\n"),

	// Solidity: contract/interface/library/struct/enum/function/modifier/
	// event all carry a "name:" field (verified via lang.FieldNames and a
	// firing witness covering all seven). Constructors are intentionally
	// not captured: constructor_definition has no name distinct from the
	// fixed "constructor" keyword.
	"solidity": strings.Join([]string{
		"(contract_declaration name: (identifier) @name) @definition.class",
		"(interface_declaration name: (identifier) @name) @definition.interface",
		"(library_declaration name: (identifier) @name) @definition.class",
		"(struct_declaration name: (identifier) @name) @definition.type",
		"(enum_declaration name: (identifier) @name) @definition.type",
		"(function_definition name: (identifier) @name) @definition.function",
		"(modifier_definition name: (identifier) @name) @definition.method",
		"(event_definition name: (identifier) @name) @definition.event",
	}, "\n"),

	// Nim: type_declaration wraps the symbol name in type_symbol_declaration
	// one level deep, then the type's own shape (object_declaration versus
	// enum_declaration) as a second direct child -- both are checked so an
	// object type and an enum type resolve to different kinds without
	// touching the language-neutral refinement table. proc/func/method all
	// carry a direct (identifier) name child with no return-type collision
	// (the return type sits inside its own type_expression wrapper).
	"nim": strings.Join([]string{
		"(type_declaration (type_symbol_declaration (identifier) @name) (object_declaration)) @definition.type",
		"(type_declaration (type_symbol_declaration (identifier) @name) (enum_declaration)) @definition.enum",
		"(proc_declaration (identifier) @name) @definition.function",
		"(func_declaration (identifier) @name) @definition.function",
		"(method_declaration (identifier) @name) @definition.method",
		"(const_section (variable_declaration (symbol_declaration_list (symbol_declaration (identifier) @name)))) @definition.constant",
		// Carried over from the shared table's generic call reference so an
		// override does not regress reference coverage the shared inference
		// already gave Nim (the smoke sample is a single bare call with no
		// definitions at all, so this is what makes Nim's Tagger output
		// non-empty on that fixture).
		"(call (identifier) @name) @reference.call",
	}, "\n"),

	// Crystal: module_def/class_def/struct_def/const_assign all name
	// through a direct "constant" child, and because def/class_def/
	// struct_def/enum_def are distinct sibling node types (not one node
	// reused three ways, unlike Kotlin/Swift) there is no cross-kind
	// conflict to guard against. enum_def is the one exception -- its own
	// name and every member share the flat "constant" child shape, so only
	// the leading anchor distinguishes the enum's own name; members are not
	// captured (same structural limit as Thrift's enum_definition).
	"crystal": strings.Join([]string{
		"(module_def (constant) @name) @definition.module",
		"(class_def (constant) @name) @definition.class",
		"(struct_def (constant) @name) @definition.struct",
		"(enum_def . (constant) @name) @definition.enum",
		"(method_def (identifier) @name) @definition.method",
		"(const_assign (constant) @name) @definition.constant",
	}, "\n"),

	// D: interface/class/function already fire through the shared table
	// (return types wrap in their own "type" node, never colliding with
	// the name identifier); struct_declaration is not in the shared table
	// at all, and enum_member needs its own row since the shared table has
	// no enum-member concept.
	"d": strings.Join([]string{
		"(interface_declaration (identifier) @name) @definition.interface",
		"(class_declaration (identifier) @name) @definition.class",
		"(struct_declaration (identifier) @name) @definition.struct",
		"(enum_declaration (identifier) @name) @definition.type",
		"(enum_member (identifier) @name) @definition.enum_member",
		"(function_declaration (identifier) @name) @definition.function",
	}, "\n"),

	// V: interface/function(including methods, whose receiver is a nested
	// "receiver" node and never collides with the direct-child function
	// name) already fire through the shared table. struct_declaration and
	// const_declaration are not in the shared table; const_declaration
	// nests its name one level inside const_definition.
	"v": strings.Join([]string{
		"(interface_declaration (identifier) @name) @definition.interface",
		"(struct_declaration (identifier) @name) @definition.struct",
		"(enum_declaration (identifier) @name) @definition.type",
		"(enum_field_definition (identifier) @name) @definition.enum_member",
		"(function_declaration (identifier) @name) @definition.function",
		"(const_declaration (const_definition (identifier) @name)) @definition.constant",
	}, "\n"),

	// Thrift: the shared table only ever reached the one function_definition
	// pattern (service methods), because it happens to also be shaped like
	// a generic "function_definition". struct/service/exception each name
	// through a direct, unambiguous first identifier child (body members
	// nest one level deeper and never collide); enum_definition repeats
	// its own name and every member as flat sibling identifiers with no
	// wrapping node at all, so -- as with Crystal -- only the leading
	// anchor is used and members are not captured. typedef_identifier is a
	// dedicated node type with no ambiguity.
	"thrift": strings.Join([]string{
		"(struct_definition (identifier) @name) @definition.type",
		"(enum_definition . (identifier) @name) @definition.enum",
		"(service_definition (identifier) @name) @definition.service",
		"(typedef_definition (typedef_identifier) @name) @definition.type",
		"(exception_definition (identifier) @name) @definition.exception",
		"(function_definition (identifier) @name) @definition.method",
	}, "\n"),

	// HCL / Terraform: every top-level construct is the same generic
	// "block" node (resource, variable, module, output, provider, locals,
	// terraform, data, ...); there is no per-kind node type to distinguish
	// them, and this grammar carries no fields at all. The name is
	// whichever label (a quoted string) sits immediately before
	// block_start -- always the LAST label regardless of how many labels
	// the block type takes (one for "variable", two for "resource"), which
	// is also Terraform's own addressing convention (TYPE.NAME). The
	// zero-label row (locals, terraform, ...) falls back to the block's own
	// type identifier. Recursive: nested blocks (e.g. "lifecycle {}" inside
	// a resource) get their own definition.block symbol nested as a child
	// by the outliner's ordinary byte-containment logic, with no new code.
	"hcl": strings.Join([]string{
		"(block (identifier) (string_lit (template_literal) @name) . (block_start)) @definition.block",
		"(block (identifier) @name . (block_start)) @definition.block",
	}, "\n"),

	// CMake: function()/macro() define their own name as the FIRST
	// argument in argument_list, with the parameter names following as
	// more arguments of the identical shape -- the leading anchor is
	// required, or the pattern also captures every parameter name at the
	// same span (verified: without the anchor, "greet" and "name" would
	// both bind to the definition and drop via OmittedNameConflict).
	"cmake": strings.Join([]string{
		"(function_def (function_command (function) (argument_list . (argument (unquoted_argument) @name)))) @definition.function",
		"(macro_def (macro_command (macro) (argument_list . (argument (unquoted_argument) @name)))) @definition.macro",
	}, "\n"),

	// PowerShell: function_name is a dedicated node type. class_statement's
	// own name is a direct (simple_name) child; a method's simple_name
	// sits one level inside class_method_definition instead, so the two
	// never collide. enum_statement's own name is likewise its own direct
	// (simple_name) child, distinct from each enum_member's nested one;
	// it uses definition.enum directly (not definition.type plus a
	// refinement row, since the core outline.go tables are out of scope
	// for this change). "enum_statement" is shared verbatim by Fortran,
	// PowerShell, and Smithy (verified via SymbolByName across every
	// registered language), all three for a genuine enum construct, so a
	// direct kind here loses nothing a refinement row would have added.
	"powershell": strings.Join([]string{
		"(function_statement (function_name) @name) @definition.function",
		"(class_statement (simple_name) @name) @definition.class",
		"(class_method_definition (simple_name) @name) @definition.method",
		"(enum_statement (simple_name) @name) @definition.enum",
		"(enum_member (simple_name) @name) @definition.enum_member",
	}, "\n"),

	// SQL: create_index_statement and create_trigger_statement carry a
	// "name:" field that picks out the index/trigger identifier from a
	// second, unfielded identifier (the target table); the rest have no
	// field and are positional, verified as the sole direct-child
	// identifier for each (parameter, return-type, and body identifiers
	// all nest at least one level deeper).
	"sql": strings.Join([]string{
		"(create_table_statement (identifier) @name) @definition.type",
		"(create_view_statement (identifier) @name) @definition.type",
		"(create_function_statement (identifier) @name) @definition.function",
		"(create_index_statement name: (identifier) @name) @definition.index",
		"(create_type_statement (identifier) @name) @definition.type",
		"(create_sequence (identifier) @name) @definition.sequence",
		"(create_trigger_statement name: (identifier) @name) @definition.trigger",
	}, "\n"),

	// Protobuf: message/enum/service/rpc each wrap their name in a
	// dedicated *_name node, and enum_field is a direct, unambiguous child
	// of enum_body. definition.enum is used directly (not definition.type
	// plus refinement) because the bare node type "enum" is used as an
	// anonymous keyword token by 47 other grammars in this fleet -- adding
	// it to outlineKindRefinement would be a cross-language keyword
	// collision, not the genuine convention the refinement table is for.
	"proto": strings.Join([]string{
		"(message (message_name (identifier) @name)) @definition.message",
		"(enum (enum_name (identifier) @name)) @definition.enum",
		"(enum_field (identifier) @name) @definition.enum_member",
		"(service (service_name (identifier) @name)) @definition.service",
		"(rpc (rpc_name (identifier) @name)) @definition.rpc",
	}, "\n"),

	// Clojure, Common Lisp, and Scheme: none of these grammars gives a
	// definition its own node type (defn/def/defun-with-macro-body/define
	// are just s-expression lists), except Common Lisp's "defun", which
	// does get a dedicated node with a "function_name" field. The other
	// two are matched with #eq?/#any-of? predicates on the head symbol,
	// verified against this repository's compiled grammar output.
	"clojure": strings.Join([]string{
		`(list_lit . (sym_lit (sym_name) @_head) . (sym_lit (sym_name) @name) (#any-of? @_head "defn" "defn-")) @definition.function`,
		`(list_lit . (sym_lit (sym_name) @_head) . (sym_lit (sym_name) @name) (#eq? @_head "def")) @definition.variable`,
		`(list_lit . (sym_lit (sym_name) @_head) . (sym_lit (sym_name) @name) (#eq? @_head "defmacro")) @definition.macro`,
	}, "\n"),
	"commonlisp": strings.Join([]string{
		"(defun (defun_header function_name: (sym_lit) @name)) @definition.function",
		`(list_lit . (sym_lit) @_head . (sym_lit) @name (#eq? @_head "defvar")) @definition.variable`,
		`(list_lit . (sym_lit) @_head . (sym_lit) @name (#eq? @_head "defparameter")) @definition.variable`,
	}, "\n"),
	"scheme": strings.Join([]string{
		`(list . (symbol) @_head . (list . (symbol) @name) (#eq? @_head "define")) @definition.function`,
		`(list . (symbol) @_head . (symbol) @name (#eq? @_head "define")) @definition.variable`,
	}, "\n"),

	// Lua: found via the cgo_harness C-oracle fleet census (log-only
	// FleetCensus subtest, GTS_PARITY_MODE=top50), not the CoreNine sweep --
	// Lua is not one of the core nine. The shared table's
	// "(function_definition (identifier) @name) @definition.function" row
	// gates in because Lua's grammar DEFINES a "function_definition" symbol,
	// but no production ever reaches it (verified: 0 matches on a real
	// firing witness with three function definitions), so it is an
	// impossible pattern on the C side, exactly like the core-nine dead
	// rows above. All of Lua's real function definitions -- free functions,
	// "local function", and "function table.field(...)" method syntax --
	// go through function_declaration; the dot_index_expression row picks
	// the LAST identifier (the field/method name, not the table) via the
	// trailing anchor, verified against "function obj.method(self) ... end".
	"lua": strings.Join([]string{
		"(function_declaration (identifier) @name) @definition.function",
		"(function_declaration (dot_index_expression (identifier) @name .)) @definition.method",
	}, "\n"),
}

var inferredTagsQueryPatterns = []tagsQueryPattern{
	// Common definitions.
	{Query: "(function_declaration (identifier) @name) @definition.function", Symbols: []string{"function_declaration", "identifier"}},
	{Query: "(function_declaration (type_identifier) @name) @definition.function", Symbols: []string{"function_declaration", "type_identifier"}},
	{Query: "(function_definition (identifier) @name) @definition.function", Symbols: []string{"function_definition", "identifier"}},
	{Query: "(function_definition (field_identifier) @name) @definition.function", Symbols: []string{"function_definition", "field_identifier"}},
	{Query: "(function_definition (function_declarator (identifier) @name)) @definition.function", Symbols: []string{"function_definition", "function_declarator", "identifier"}},
	{Query: "(function_definition (function_declarator (field_identifier) @name)) @definition.function", Symbols: []string{"function_definition", "function_declarator", "field_identifier"}},
	{Query: "(method_declaration (identifier) @name) @definition.method", Symbols: []string{"method_declaration", "identifier"}},
	{Query: "(method_declaration (field_identifier) @name) @definition.method", Symbols: []string{"method_declaration", "field_identifier"}},
	{Query: "(method_definition (property_identifier) @name) @definition.method", Symbols: []string{"method_definition", "property_identifier"}},
	{Query: "(method_definition (identifier) @name) @definition.method", Symbols: []string{"method_definition", "identifier"}},
	{Query: "(class_declaration (identifier) @name) @definition.class", Symbols: []string{"class_declaration", "identifier"}},
	{Query: "(class_declaration (type_identifier) @name) @definition.class", Symbols: []string{"class_declaration", "type_identifier"}},
	{Query: "(class_definition (identifier) @name) @definition.class", Symbols: []string{"class_definition", "identifier"}},
	{Query: "(interface_declaration (identifier) @name) @definition.interface", Symbols: []string{"interface_declaration", "identifier"}},
	{Query: "(interface_declaration (type_identifier) @name) @definition.interface", Symbols: []string{"interface_declaration", "type_identifier"}},
	{Query: "(enum_declaration (identifier) @name) @definition.type", Symbols: []string{"enum_declaration", "identifier"}},
	{Query: "(enum_declaration (type_identifier) @name) @definition.type", Symbols: []string{"enum_declaration", "type_identifier"}},
	{Query: "(constructor_declaration (identifier) @name) @definition.constructor", Symbols: []string{"constructor_declaration", "identifier"}},
	{Query: "(type_definition (type_identifier) @name) @definition.type", Symbols: []string{"type_definition", "type_identifier"}},
	{Query: "(type_definition (identifier) @name) @definition.type", Symbols: []string{"type_definition", "identifier"}},
	{Query: "(type_declaration (type_spec (type_identifier) @name)) @definition.type", Symbols: []string{"type_declaration", "type_spec", "type_identifier"}},
	{Query: "(type_declaration (type_alias (type_identifier) @name)) @definition.type", Symbols: []string{"type_declaration", "type_alias", "type_identifier"}},
	{Query: "(function_item (identifier) @name) @definition.function", Symbols: []string{"function_item", "identifier"}},
	{Query: "(function_signature_item (identifier) @name) @definition.function", Symbols: []string{"function_signature_item", "identifier"}},
	{Query: "(struct_item (type_identifier) @name) @definition.type", Symbols: []string{"struct_item", "type_identifier"}},
	{Query: "(enum_item (type_identifier) @name) @definition.type", Symbols: []string{"enum_item", "type_identifier"}},
	{Query: "(trait_item (type_identifier) @name) @definition.type", Symbols: []string{"trait_item", "type_identifier"}},
	{Query: "(class_specifier (type_identifier) @name) @definition.class", Symbols: []string{"class_specifier", "type_identifier"}},
	{Query: "(struct_specifier (type_identifier) @name) @definition.type", Symbols: []string{"struct_specifier", "type_identifier"}},
	// A bare "type_alias" node (not wrapped in type_declaration, unlike Go's
	// shape above) whose first named child is the alias name. Verified
	// identical direct-child shape in Kotlin, Dart, and templ; the pattern
	// still fails closed for grammars that nest the name deeper (Gleam wraps
	// it in type_name, so it simply never matches there).
	{Query: "(type_alias (type_identifier) @name) @definition.type", Symbols: []string{"type_alias", "type_identifier"}},

	// Constants and variables.
	{Query: "(const_spec (identifier) @name) @definition.constant", Symbols: []string{"const_spec", "identifier"}},
	{Query: "(var_spec (identifier) @name) @definition.variable", Symbols: []string{"var_spec", "identifier"}},
	{Query: "(short_var_declaration (identifier) @name) @definition.variable", Symbols: []string{"short_var_declaration", "identifier"}},

	// Common call references.
	{Query: "(call_expression (identifier) @name) @reference.call", Symbols: []string{"call_expression", "identifier"}},
	{Query: "(call_expression (field_identifier) @name) @reference.call", Symbols: []string{"call_expression", "field_identifier"}},
	{Query: "(call_expression (property_identifier) @name) @reference.call", Symbols: []string{"call_expression", "property_identifier"}},
	{Query: "(call_expression (member_expression (property_identifier) @name)) @reference.call", Symbols: []string{"call_expression", "member_expression", "property_identifier"}},
	{Query: "(call_expression (selector_expression (field_identifier) @name)) @reference.call", Symbols: []string{"call_expression", "selector_expression", "field_identifier"}},
	{Query: "(call_expression (scoped_identifier (identifier) @name)) @reference.call", Symbols: []string{"call_expression", "scoped_identifier", "identifier"}},
	{Query: "(call (identifier) @name) @reference.call", Symbols: []string{"call", "identifier"}},
	{Query: "(call (attribute (identifier) @name)) @reference.call", Symbols: []string{"call", "attribute", "identifier"}},
	{Query: "(method_invocation (identifier) @name) @reference.call", Symbols: []string{"method_invocation", "identifier"}},
	{Query: "(macro_invocation (identifier) @name) @reference.call", Symbols: []string{"macro_invocation", "identifier"}},
}

func inferredTagsQuery(entry LangEntry) string {
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		return ""
	}

	if cached, ok := inferredTagsQueryCache.Load(name); ok {
		return cached.(string)
	}

	if query := inferredTagsQueryOverrides[name]; strings.TrimSpace(query) != "" {
		inferredTagsQueryCache.Store(name, query)
		return query
	}

	if entry.Language == nil {
		inferredTagsQueryCache.Store(name, "")
		return ""
	}
	lang := entry.Language()
	if lang == nil {
		inferredTagsQueryCache.Store(name, "")
		return ""
	}

	hasSymbol := func(symbol string) bool {
		_, ok := lang.SymbolByName(symbol)
		return ok
	}

	lines := make([]string, 0, 32)
	seen := make(map[string]struct{}, len(inferredTagsQueryPatterns))
	for _, pattern := range inferredTagsQueryPatterns {
		missing := false
		for _, symbol := range pattern.Symbols {
			if !hasSymbol(symbol) {
				missing = true
				break
			}
		}
		if missing {
			continue
		}
		if _, ok := seen[pattern.Query]; ok {
			continue
		}
		seen[pattern.Query] = struct{}{}
		lines = append(lines, pattern.Query)
	}

	query := strings.Join(lines, "\n")
	inferredTagsQueryCache.Store(name, query)
	return query
}
