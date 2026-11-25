// Package cquery is the start of a function-by-function port of tree-sitter's
// query compiler, pattern analysis and query cursor (query.c in the pinned C
// runtime, tree-sitter 0.27.1). The port is compared with pinned C record by
// record, in five layers:
//
//   - L1, steps: capture names, capture quantifiers, patterns, the pattern
//     map, the compiled steps and the step offsets.
//   - L2, analysis: whether C accepts the query, the rooted and non-local
//     flags per pattern, the three analysis flags per step, and the repeat
//     symbols that rootless patterns need.
//   - L3, matches: the ordered matches, each with its pattern and captures.
//   - L4, captures: the ordered stream of single captures.
//   - L5, limits: the matches and the limit flag under a byte range, a
//     maximum start depth and a capture-list limit.
//
// This stage holds the record types, their stable text forms, the
// first-difference comparison, the tree digest and the reader and writer of
// corpus 1 (testdata/cases.jsonl), which stores pinned C's records for each
// case. The harness test TestCQueryCorpus in cgo_harness writes and checks the
// corpus against pinned C; roottest/query compares the current matcher with
// it.
//
// The package uses the standard library only and does not import the root
// package.
package cquery
