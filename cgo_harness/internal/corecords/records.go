//go:build cgo

// Package corecords hashes the subtree records of a tree built by the pinned C
// runtime, for the internal/ccore equality tests. The line format must stay
// identical to internal/ccore/records.go (recordLine).
//
// The helper reads private runtime headers (subtree.h, tree.h) and the tree's
// TSTree pointer. It lives here, beside the vendored binding in ../coracle and
// not inside it, so that the vendored sources stay byte-equal to the manifest
// in ../coracle/upstream.json (TestCOracleVendoredSources, refresh_sources.py
// --verify, and the oracle identity hash that scripts/editor_latency_gate.py
// pins). The pointer comes from the binding's unexported field _inner, through
// reflection. The runtime object code is the binding's: this package compiles
// only static inline functions and links nothing of its own.
package corecords

/*
#cgo CFLAGS: -I${SRCDIR}/../coracle/include -I${SRCDIR}/../coracle/src -std=c11 -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include "tree.h"

typedef struct { Subtree s; uint32_t d; } ccore_rec_item;

static int ccore_records(const TSTree *tree, const char *dump, uint64_t *hash, uint64_t *lines) {
  uint64_t h = 14695981039346656037ULL, n_lines = 0;
  FILE *f = NULL;
  if (dump && dump[0]) { f = fopen(dump, "wb"); if (!f) return 1; }
  size_t cap = 1024, len = 0;
  ccore_rec_item *stack = malloc(cap * sizeof(ccore_rec_item));
  stack[len++] = (ccore_rec_item){tree->root, 0};
  char buf[768];
  while (len > 0) {
    ccore_rec_item it = stack[--len];
    Subtree s = it.s;
    Length pad = ts_subtree_padding(s), size = ts_subtree_size(s);
    uint32_t n = ts_subtree_child_count(s);
    int o = snprintf(buf, sizeof buf,
      "d=%u inl=%d sym=%u vis=%d named=%d extra=%d chg=%d miss=%d kw=%d fl=%d fr=%d ext=%d extchg=%d col=%d st=%u pad=%u,%u,%u size=%u,%u,%u la=%u cost=%u n=%u",
      it.d, (int)s.data.is_inline, (unsigned)ts_subtree_symbol(s), (int)ts_subtree_visible(s), (int)ts_subtree_named(s),
      (int)ts_subtree_extra(s), (int)ts_subtree_has_changes(s), (int)ts_subtree_missing(s), (int)ts_subtree_is_keyword(s),
      (int)ts_subtree_fragile_left(s), (int)ts_subtree_fragile_right(s), (int)ts_subtree_has_external_tokens(s), (int)ts_subtree_has_external_scanner_state_change(s),
      (int)ts_subtree_depends_on_column(s), (unsigned)ts_subtree_parse_state(s),
      pad.bytes, pad.extent.row, pad.extent.column, size.bytes, size.extent.row, size.extent.column,
      ts_subtree_lookahead_bytes(s), ts_subtree_error_cost(s), n);
    if (n > 0) {
      o += snprintf(buf + o, sizeof buf - o, " vc=%u nc=%u vd=%u dp=%d rd=%u prod=%u fls=%u flst=%u",
        ts_subtree_visible_child_count(s), s.ptr->named_child_count, ts_subtree_visible_descendant_count(s),
        (int)ts_subtree_dynamic_precedence(s), ts_subtree_repeat_depth(s), (unsigned)ts_subtree_production_id(s),
        (unsigned)s.ptr->first_leaf.symbol, (unsigned)s.ptr->first_leaf.parse_state);
    }
    buf[o++] = '\n';
    for (int i = 0; i < o; i++) h = (h ^ (uint8_t)buf[i]) * 1099511628211ULL;
    n_lines++;
    if (f) fwrite(buf, 1, o, f);
    if (n > 0) {
      if (len + n > cap) { while (len + n > cap) cap *= 2; stack = realloc(stack, cap * sizeof(ccore_rec_item)); }
      Subtree *kids = ts_subtree_children(s);
      for (int64_t i = (int64_t)n - 1; i >= 0; i--) stack[len++] = (ccore_rec_item){kids[i], it.d + 1};
    }
  }
  free(stack);
  if (f) fclose(f);
  *hash = h; *lines = n_lines;
  return 0;
}
*/
import "C"

import (
	"fmt"
	"reflect"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Hash hashes every subtree record of the tree (pre-order, hidden nodes
// included); dump, when not empty, receives the lines. rc is not 0 when the
// dump file could not be created.
func Hash(tree *sitter.Tree, dump string) (hash uint64, lines uint64, rc int) {
	inner := reflect.ValueOf(tree).Elem().FieldByName("_inner")
	if !inner.IsValid() || inner.Kind() != reflect.Pointer {
		panic(fmt.Sprintf("corecords: the binding's Tree has no pointer field _inner (%v)", inner))
	}
	cs := C.CString(dump)
	defer C.free(unsafe.Pointer(cs))
	var h, n C.uint64_t
	rc = int(C.ccore_records((*C.TSTree)(inner.UnsafePointer()), cs, &h, &n))
	return uint64(h), uint64(n), rc
}
