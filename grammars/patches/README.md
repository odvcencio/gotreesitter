# Upstream grammar patches

These patches are narrow, pinned overlays applied by `cmd/ts2go` before it
extracts a grammar table. They exist only when an upstream grammar has a
confirmed correctness gap that must be shipped before its next release.

`tree-sitter-typescript-import-type.patch` closes confirmed gaps. It:

- adds the `import_type` production, including qualified generic imports;
- adds TypeScript variance annotations from upstream pull request 361;
- separates adjacent generic call signatures at a newline;
- permits a contextual `in` property after a newline in an object type;
- accepts predefined type keywords and `keyof` as required and optional tuple
  labels; and
- accepts `unique` as an expression identifier while preserving `unique symbol`.

Required and optional tuple names use the identifier token, single-word
predefined type keywords, and `keyof`.
The conflicts preserve labeled `[symbol?: string]` and unlabeled `[symbol?]`.
Contextual names such as `get` and `async` stay identifiers in bare tuple
elements, including `[get?]`. The `keyof` operator still works in
`[keyof Shape]` and `[(keyof Shape)?]`. Regression coverage is tracked in
issues #1429 and #1431.

The call-signature rule uses its dedicated automatic-semicolon token. It does
not change the generic automatic-semicolon rule.

The patch applies to the TypeScript commit pinned in `../languages.lock`.
Regeneration fails if upstream changes the surrounding grammar shape. Remove
each source change after an upstream release includes the same behavior. Then,
refresh both TypeScript and TSX blobs and their parity fixtures.

`tree-sitter-yaml-multiline-quoted-scalars.patch` repairs quoted scalar
continuation and termination after a line break. It applies to the YAML commit
pinned in `../languages.lock`. The YAML Go DSL and Go scanner contain the same
behavior. Keep the C oracle patch until the pinned upstream revision includes
the correction.
