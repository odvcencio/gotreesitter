//go:build cgo

package corecords

/*
#cgo CFLAGS: -I${SRCDIR}/../coracle/include -I${SRCDIR}/../coracle/src -std=c11 -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE
#include <stdbool.h>
#include "tree_cursor.h"
*/
import "C"

import (
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Supertypes returns the supertype symbols that C's ts_tree_cursor_current_status
// (C/tree_cursor.c:506) reports for the cursor's current node: the hidden
// supertype nodes between the node and its visible parent. The cursor is the
// binding's TreeCursor, whose only field is the C cursor; like Hash, this
// links nothing of its own.
func Supertypes(cursor *sitter.TreeCursor) []uint16 {
	var field C.TSFieldId
	var later, laterNamed, sameField C.bool
	var buf [64]C.TSSymbol
	n := C.uint(len(buf))
	C.ts_tree_cursor_current_status((*C.TSTreeCursor)(unsafe.Pointer(cursor)), &field, &later, &laterNamed, &sameField, &buf[0], &n)
	out := make([]uint16, n)
	for i := range out {
		out[i] = uint16(buf[i])
	}
	return out
}
