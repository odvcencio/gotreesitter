package benchfixtures

// GoTagsQuery matches Go function, method, and type definitions and call references.
// Tagger benchmarks and fuzz tests share this exact query.
const GoTagsQuery = `
(function_declaration (identifier) @name) @definition.function
(method_declaration (field_identifier) @name) @definition.method
(call_expression (identifier) @name) @reference.call
(call_expression (selector_expression (field_identifier) @name)) @reference.call
(type_declaration (type_spec (type_identifier) @name)) @definition.type
`
