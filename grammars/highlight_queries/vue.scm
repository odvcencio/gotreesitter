; inherits: html
; Source: tree-sitter-grammars/tree-sitter-vue queries/vue/highlights.scm at ce8011a414fdf8091f4e4071752efc376f4afb08 (MIT).
; Change: the parent html_tags is the registry's html query here; the (template_element) pattern that only held a (#set! ...) directive was removed.

[
  "["
  "]"
] @punctuation.bracket

[
  ":"
  "."
] @character.special

[
  (interpolation)
  "@"
] @punctuation.special

(interpolation
  (raw_text) @none)

(dynamic_directive_inner_value) @variable

(directive_name) @tag.attribute

; Accessing a component object's field
(":"
  .
  (directive_value) @variable.member)

("."
  .
  (directive_value) @property)

; @click is like onclick for HTML
("@"
  .
  (directive_value) @function.method)

; Used in v-slot, declaring position the element should be put in
("#"
  .
  (directive_value) @variable)

(directive_attribute
  (quoted_attribute_value) @punctuation.special)

(directive_attribute
  (quoted_attribute_value
    (attribute_value) @none))

(directive_modifier) @function.method
