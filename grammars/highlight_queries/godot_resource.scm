; Source: nvim-treesitter/nvim-treesitter runtime/queries/godot_resource/highlights.scm at a2f694478be07f89c2d03cc17858a8745a400c4b (Apache-2.0)
; Change: none.

(identifier) @variable

(section
  (identifier) @tag)

(section
  [
    "["
    "]"
  ] @tag.delimiter)

(attribute
  (identifier) @tag.attribute)

(property
  (path) @property)

(constructor
  (identifier) @constructor)

(string) @string

(integer) @number

(float) @number.float

[
  (true)
  (false)
] @boolean

(null) @constant.builtin

(array
  [
    "["
    "]"
  ] @punctuation.bracket)

[
  "("
  ")"
  "{"
  "}"
] @punctuation.bracket

"=" @operator

(comment) @comment @spell
