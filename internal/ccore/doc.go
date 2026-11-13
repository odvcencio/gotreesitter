// Package ccore is a function-by-function port of the parse loop of
// tree-sitter 0.27.1 (parser.c, stack.c, subtree.c, reusable_node.h,
// tree.c). Function comments name the C function and `C/<file>:<line>` of
// the pinned runtime. The package imports the standard library only. The
// root package supplies the language tables, the lex automata and the
// external scanner through Tables and ExtScanner.
//
// Go constructs that differ from C, semantics kept:
//   - Handle is an 8-byte value: bit 0 set = inline leaf with C's packed
//     fields (SubtreeInlineData); otherwise index<<1 into the Store's record
//     pages. Handle 0 is C's NULL_SUBTREE.
//   - Records, child lists and scanner-state bytes live in fixed pages of a
//     Store that never move. Owners are counted in a side array, as C counts
//     them in each subtree, but only when a tree is published, edited, copied
//     or released; the parse loop counts nothing. Freed records, child lists
//     and state bytes are reused by the next parse of the same document
//     (store.go, lifetime.go, tree.go). ts_subtree_make_mut always copies a
//     heap leaf; the balance step rotates a node only if the running parse
//     allocated it (the new flag), which C decides by ref_count == 1.
//   - Reader contract, as C's for a TSTree: many goroutines may read one Tree
//     value at once; Edit and Release need exclusive use of that value; a
//     goroutine that reads while another edits takes its own Copy. See
//     Tree.Reader.
//   - Stack nodes keep C's reference counts in an index arena with a free list.
package ccore
