; Source: vlang/v-analyzer tree_sitter_v/queries/highlights.scm at 9cf6a37689f06b17d170dec644ace81eb8eab280 (MIT).
; Change: none.

(ERROR) @error

[
 (line_comment)
 (block_comment)
 (shebang)
] @comment

(identifier) @variable
(import_path) @variable

(parameter_declaration
  name: (identifier) @parameter)
(function_declaration
  name: (identifier) @function)
(function_declaration
  receiver: (receiver)
  name: (identifier) @method)

(short_lambda
  (reference_expression) @parameter)
(call_expression
  name: (selector_expression
    field: (reference_expression) @method))

(type_reference_expression) @type
(pointer_type) @type
(array_type) @type

(field_name) @property
(selector_expression
  field: (reference_expression) @property)

(int_literal) @number
(interpreted_string_literal) @string
(rune_literal) @string
(escape_sequence) @string.escape

[
 "as"
 "asm"
 "assert"
 ;"atomic"
 "break"
 "const"
 "continue"
 "defer"
 "else"
 "enum"
 "fn"
 "for"
 "$for"
 "go"
 "goto"
 "if"
 "$if"
 "implements"
 "import"
 "in"
 "!in"
 "interface"
 "is"
 "!is"
 "lock"
 "match"
 "module"
 "mut"
 "or"
 "pub"
 "return"
 "rlock"
 "select"
 "shared"
 "spawn"
 "static"
 "struct"
 "type"
 "union"
 "unsafe"
] @keyword

[
 (true)
 (false)
] @boolean

[
 "."
 ","
 ":"
 ";"
] @punctuation.delimiter

[
 "("
 ")"
 "{"
 "}"
 "["
 "]"
] @punctuation.bracket

(array_creation) @punctuation.bracket

[
 "++"
 "--"

 "+"
 "-"
 "*"
 "/"
 "%"

 "~"
 "&"
 "|"
 "^"

 "!"
 "&&"
 "||"
 "!="

 "<<"
 ">>"

 "<"
 ">"
 "<="
 ">="

 "+="
 "-="
 "*="
 "/="
 "&="
 "|="
 "^="
 "<<="
 ">>="

 "="
 ":="
 "=="

 "?"
 "<-"
 "$"
 ".."
 "..."
] @operator
