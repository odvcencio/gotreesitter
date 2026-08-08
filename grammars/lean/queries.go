package lean

// HighlightQuery provides editor captures for the native Lean grammar.
const HighlightQuery = `
[
  (line_comment)
  (block_comment)
  (doc_comment)
  (module_doc_comment)
] @comment

[
  "module"
  "prelude"
  "import"
  "namespace"
  "section"
  "end"
  "mutual"
  "def"
  "abbrev"
  "opaque"
  "theorem"
  "lemma"
  "axiom"
  "constant"
  "instance"
  "structure"
  "class"
  "inductive"
  "coinductive"
  "example"
  "deriving"
  "syntax"
  "macro"
  "macro_rules"
  "notation"
  "infix"
  "infixl"
  "infixr"
  "prefix"
  "postfix"
  "elab"
  "elab_rules"
  "open"
  "export"
  "variable"
  "variables"
  "include"
  "omit"
  "universe"
  "universes"
  "set_option"
  "attribute"
  "initialize"
  "builtin_initialize"
  "private"
  "public"
  "protected"
  "noncomputable"
  "partial"
  "unsafe"
  "meta"
  "nonrec"
  "local"
  "scoped"
] @keyword

(directive_name) @function.macro
(string_literal) @string
(character_literal) @character
(numeric_literal) @number
(operator) @operator

(definition_declaration name: (identifier) @function)
(theorem_declaration name: (identifier) @function)
(axiom_declaration name: (identifier) @constant)
(structure_declaration name: (identifier) @type)
(inductive_declaration name: (identifier) @type)
(namespace_declaration name: (identifier) @module)
(section_declaration name: (identifier) @module)
`

// TagsQuery extracts Lean declarations for outlines and symbol indexes.
const TagsQuery = `
(definition_declaration name: (identifier) @name) @definition.function
(theorem_declaration name: (identifier) @name) @definition.function
(axiom_declaration name: (identifier) @name) @definition.constant
(structure_declaration name: (identifier) @name) @definition.type
(inductive_declaration name: (identifier) @name) @definition.type
(namespace_declaration name: (identifier) @name) @definition.module
(section_declaration name: (identifier) @name) @definition.module
`
