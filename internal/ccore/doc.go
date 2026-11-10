// Package ccore is a function-by-function port of the parse loop of
// tree-sitter 0.27.1 (parser.c, stack.c, subtree.c, reusable_node.h,
// tree.c). Function comments name the C function and `C/<file>:<line>` of
// the pinned runtime. The package imports the standard library only. The
// root package supplies the language tables, the lexer kernels and the
// external scanner through Tables, LexKernel and ExtScanner.
//
// Go constructs that differ from C, semantics kept:
//   - Handle is an 8-byte value: bit 0 set = inline leaf with C's packed
//     fields (SubtreeInlineData); otherwise index<<1 into the Store's record
//     arena. Handle 0 is C's NULL_SUBTREE.
//   - Subtree reference counts are not kept: records live until the Store is
//     cleared by the next fresh parse. ts_subtree_make_mut therefore always
//     copies a heap leaf; ts_subtree_compress treats every ref_count as 1
//     except for records of an old tree (Parser.shared).
//   - Stack nodes keep C's reference counts in an index arena with a free list.
package ccore
