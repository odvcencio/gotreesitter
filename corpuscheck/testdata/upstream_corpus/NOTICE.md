# Vendored upstream corpus fixtures

The files under this directory are `test/corpus`/`corpus` fixtures copied
verbatim from upstream tree-sitter grammar repositories, at the commits
pinned in `grammars/languages.lock`. They are grammar-author-written
test cases in tree-sitter's standard corpus format: source text paired
with the exact S-expression the grammar's own maintainers expect it to
parse to. `corpuscheck` uses them as a small, always-available parity
oracle so `go test ./corpuscheck/...` gets real signal in CI without
depending on a local `gts-corpora` checkout (see `census_test.go`).

All five source repositories are MIT-licensed.

| Directory | Upstream repo | Commit | License |
| --- | --- | --- | --- |
| `json/` | https://github.com/tree-sitter/tree-sitter-json | `001c28d7a29832b06b0e831ec77845553c89b56d` | MIT (c) Max Brunsfeld |
| `go/` | https://github.com/tree-sitter/tree-sitter-go | `2346a3ab1bb3857b48b29d779a1ef9799a248cd7` | MIT (c) Max Brunsfeld |
| `css/` | https://github.com/tree-sitter/tree-sitter-css | `dda5cfc5722c429eaba1c910ca32c2c0c5bb1a3f` | MIT (c) Max Brunsfeld |
| `html/` | https://github.com/tree-sitter/tree-sitter-html | `73a3947324f6efddf9e17c0ea58d454843590cc0` | MIT (c) Max Brunsfeld |
| `toml/` | https://github.com/tree-sitter/tree-sitter-toml | `342d9be207c2dba869b9967124c679b5e6fd0ebe` | MIT (c) Ika |

Selection notes:

- `css/` and `html/` are copied in full (36 KB and 12 KB): every case in
  both currently passes gotreesitter's strict comparison, so they act as
  a regression gate -- a real failure here means something broke, not
  that an upstream fixture predates a grammar feature.
- `json/main.txt` is copied in full (4 KB). Two of its six cases fail
  strict comparison because tree-sitter-json's own `pair` rule gained
  `key`/`value` fields in 2019 (upstream commit `fc89d8a`, "Add
  key/value fields to JSON") and its corpus fixture was never
  regenerated to match -- verified by walking the upstream repo's git
  history. `census_test.go` asserts the *field-lenient* pass rate here,
  documenting that gap rather than hiding it.
- `go/errors.txt` and `go/source_files.txt` (4 KB and 4 KB) are two of
  Go's seven corpus files, chosen small. `errors.txt`'s first case is a
  genuine gotreesitter defect, not a stale fixture: gotreesitter
  mis-recovers a dangling `selector_expression` (`a.` followed by a
  comment then an `if` statement) and produces a materially different,
  wrong tree, including losing track of the second function's closing
  brace. `census_test.go` pins this as a known-failing case with a
  comment pointing at the exact divergence rather than papering over it
  with a lenient comparison.
- `toml/` is copied in full (48 KB): a real, mixed-quality sample (about
  80% strict pass at the time this was captured) that exercises more of
  the comparator's category machinery (field, shape, type mismatches all
  occur) without being large.
