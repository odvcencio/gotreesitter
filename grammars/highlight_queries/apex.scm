; Source: aheber/tree-sitter-sfapex apex/queries/highlights.scm at da568eee10ce4724a40f368b2b7e795dacb4f1ba (MIT).
; Change: none.

;; attempting to match concepts represented here:
;; https://code.visualstudio.com/api/language-extensions/semantic-highlight-guide

[
  "["
  "]"
  "{"
  "}"
  "?"
  ";"
  "@"
] @punctuation

(identifier) @variable

;; Methods

(method_declaration
  name: (identifier) @method.declaration)
(method_declaration
  type: (type_identifier) @type)

(method_invocation
  name: (identifier) @method)
(argument_list
  (identifier) @variable)
(super) @function.defaultLibrary

(explicit_constructor_invocation
  arguments: (argument_list
    (identifier) @variable ))

;; Annotations

(annotation
  name: (identifier) @decorator)

(annotation_key_value
  (identifier) @variable)

;; because itendifying it when declared doesn't carry to use
;; leans on the convention that "screaming snake case" is a const
((identifier) @variable.readonly
  (#match? @variable.readonly "^_*[A-Z][A-Z\\d_]+$"))

(enum_declaration
  name: (identifier) @enum.declaration)
(enum_body
  (enum_constant
    name: (identifier) @enumMember.declaration))

;; Types

(interface_declaration
  name: (identifier) @interface.declaration)
(class_declaration
  name: (identifier) @class.declaration)
(class_declaration
  (superclass) @class)

(interfaces
  (type_list
    (type_identifier) @interface ))

(local_variable_declaration
  (type_identifier) @type )

(type_arguments "<" @punctuation)
(type_arguments ">" @punctuation)

;; (identifier) @variable
(variable_declarator (identifier) @variable.declaration)

((field_access
  object: (identifier) @type)) ;; don't know what type of thing it is

(generic_type
  (type_identifier) @type)
(type_arguments (type_identifier) @type)

(field_access
  field: (identifier) @property)

((scoped_identifier
  scope: (identifier) @type)
 (#match? @type "^[A-Z]"))
((method_invocation
  object: (identifier) @type)
 (#match? @type "^[A-Z]"))


(field_declaration
  type: (type_identifier) @type)

(formal_parameter
  type: (type_identifier) @type)

(formal_parameter
  name: (identifier) @parameter.declaration)

(enhanced_for_statement
  type: (type_identifier) @type)

(enhanced_for_statement
  value: (identifier) @variable)

(enhanced_for_statement
  name: (identifier) @variable.declaration)

(object_creation_expression
  type: (type_identifier) @type)

(array_creation_expression
  type: (type_identifier) @type)

(array_type
  element: (type_identifier) @type)

(return_statement
  (identifier) @variable)

(for_statement
  condition: (binary_expression
    (identifier) @variable))

(for_statement
  update: (update_expression
    (identifier) @variable))

(constructor_declaration
  name: (identifier) @class)

(dml_type) @function.defaultLibrary

(bound_apex_expression
  (identifier) @variable)

(assignment_operator) @operator

(update_operator) @operator

(instanceof_expression
  left: (identifier) @variable
  right: (type_identifier) @type )

(cast_expression
  type: (type_identifier) @type
  value: (identifier) @variable)

(switch_expression
  condition: (identifier) @variable)

(switch_rule
  (switch_label
    (identifier) @enumMember ))

(when_sobject_type
  (type_identifier) @type
  (identifier) @variable.declaration )

(trigger_declaration
  name: (identifier) @type.declaration
  object: (identifier) @type
  (trigger_event) @keyword
  ("," (trigger_event) @keyword)*)

(binary_expression
  operator: [
    ">"
    "<"
    ">="
    "<="
    "=="
    "==="
    "!="
    "!=="
    "&&"
    "||"
    "+"
    "-"
    "*"
    "/"
    "&"
    "|"
    "^"
    "%"
    "<<"
    ">>"
    ">>>"] @operator)

(binary_expression
  (identifier) @variable)

(unary_expression
  operator: [
    "+"
    "-"
    "!"
    "~"
  ]) @operator

("=>" @operator)

[
  (boolean_type)
  (void_type)
] @type.defaultLibrary

; Variables

(field_declaration (variable_declarator
  (identifier) @property.declaration))

(field_declaration
  (modifiers (modifier [(final) (static)])(modifier [(final) (static)]))
  (variable_declarator
    name: (identifier) @variable.declaration.readonly))

(this) @variable.defaultLibrary

; Literals

[
  (int)
] @number

[
  (string_literal)
  (multi_line_string_literal)
] @string

[
  (line_comment)
  (block_comment)
] @comment

;; Keywords

[
  (abstract)
  (all_rows_clause)
  "break"
  "catch"
  "class"
  "continue"
  "do"
  "else"
  "enum"
  "extends"
  (final)
  "finally"
  "for"
  "get"
  (global)
  "if"
  "implements"
  "instanceof"
  "interface"
  "new"
  "on"
  (override)
  (private)
  (protected)
  (public)
  "return"
  "set"
  (static)
  "switch"
  (testMethod)
  (webservice)
  "throw"
  (transient)
  "try"
  "trigger"
  (virtual)
  "when"
  "while"
  (with_sharing)
  (without_sharing)
  (inherited_sharing)
] @keyword

(assignment_expression
  left: (identifier) @variable)

; (type_identifier) @type ;; not respecting precedence...
;; I don't love this but couldn't break them up right now
;; can't figure out how to let that be special without conflicting
;; in the grammar
"System.runAs" @method.defaultLibrary

(scoped_type_identifier
  (type_identifier) @type)

;; (identifier) @variable
