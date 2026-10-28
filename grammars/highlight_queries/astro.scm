; inherits: html
; Source: nvim-treesitter/nvim-treesitter runtime/queries/astro/highlights.scm at a2f694478be07f89c2d03cc17858a8745a400c4b (Apache-2.0)
; Change: the parent html_tags is the registry's html query here.

(doctype) @constant

"<!" @tag.delimiter

"---" @punctuation.delimiter

[
  "{"
  "}"
] @punctuation.special

; custom components get `@type` highlighting
((start_tag
  (tag_name) @type)
  (#lua-match? @type "^[A-Z]"))

((end_tag
  (tag_name) @type)
  (#lua-match? @type "^[A-Z]"))

((self_closing_tag
  (tag_name) @type)
  (#lua-match? @type "^[A-Z]"))

((erroneous_end_tag
  (erroneous_end_tag_name) @type)
  (#lua-match? @type "^[A-Z]"))
