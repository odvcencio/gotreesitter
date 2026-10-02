//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

/*
#cgo CFLAGS: -std=c11 -D_POSIX_C_SOURCE=200112L
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

// These declarations are the stable public tree-sitter ABI. The definitions
// come from the same locked runtime linked by go-tree-sitter in this harness.
typedef struct TSParser TSParser;
typedef struct TSTree TSTree;
typedef struct TSLanguage TSLanguage;
typedef struct { uint32_t row, column; } TSPoint;
typedef struct { uint32_t context[4]; const void *id; const TSTree *tree; } TSNode;
typedef struct {
  uint32_t start_byte, old_end_byte, new_end_byte;
  TSPoint start_point, old_end_point, new_end_point;
} TSInputEdit;
extern TSParser *ts_parser_new(void);
extern bool ts_parser_set_language(TSParser *, const TSLanguage *);
extern TSTree *ts_parser_parse_string(TSParser *, const TSTree *, const char *, uint32_t);
extern void ts_parser_delete(TSParser *);
extern void ts_tree_delete(TSTree *);
extern void ts_tree_edit(TSTree *, const TSInputEdit *);
extern TSNode ts_tree_root_node(const TSTree *);
extern uint32_t ts_node_end_byte(TSNode);
extern bool ts_node_has_error(TSNode);
extern void ts_set_allocator(void *(*)(size_t), void *(*)(size_t, size_t), void *(*)(void *, size_t), void (*)(void *));

typedef struct {
  TSParser *parser;
  TSTree *tree;
  char *source[2];
  uint32_t length[2];
  TSInputEdit edit[2];
  int incremental, current;
} ceiling_parser;
typedef struct { uint64_t ns, parse_ns, bytes, allocs; int failed; } ceiling_sample;
static uint64_t ceiling_bytes, ceiling_allocs;
static void *ceiling_malloc(size_t n) { ceiling_bytes += n; ceiling_allocs++; return malloc(n); }
static void *ceiling_calloc(size_t n, size_t m) { ceiling_bytes += n*m; ceiling_allocs++; return calloc(n,m); }
static void *ceiling_realloc(void *p, size_t n) { ceiling_bytes += n; ceiling_allocs++; return realloc(p,n); }
static uint64_t ceiling_now(void) {
  struct timespec t; clock_gettime(CLOCK_MONOTONIC, &t);
  return (uint64_t)t.tv_sec * 1000000000 + t.tv_nsec;
}
static ceiling_parser *ceiling_new(const void *language, const void *a, uint32_t an, const void *b, uint32_t bn, TSInputEdit forward, TSInputEdit backward, int incremental) {
  ceiling_parser *p = calloc(1, sizeof(*p));
  if (!p) return NULL;
  p->parser = ts_parser_new(); p->incremental = incremental;
  p->length[0] = an; p->length[1] = bn;
  p->source[0] = malloc(an); p->source[1] = malloc(bn);
  if (!p->parser || !p->source[0] || !p->source[1]) goto fail;
  memcpy(p->source[0], a, an); memcpy(p->source[1], b, bn);
  p->edit[0] = forward; p->edit[1] = backward;
  if (!ts_parser_set_language(p->parser, language)) goto fail;
  p->tree = ts_parser_parse_string(p->parser, NULL, p->source[0], an);
  if (!p->tree || ts_node_end_byte(ts_tree_root_node(p->tree)) != an) goto fail;
  if (!incremental) { ts_tree_delete(p->tree); p->tree = NULL; }
  return p;
fail:
  if (p->tree) ts_tree_delete(p->tree);
  if (p->parser) ts_parser_delete(p->parser);
  free(p->source[0]); free(p->source[1]); free(p); return NULL;
}
static ceiling_sample ceiling_batch(ceiling_parser *p, uint64_t n, int count) {
  ceiling_sample result = {0};
  if (count) { ceiling_bytes = ceiling_allocs = 0; ts_set_allocator(ceiling_malloc, ceiling_calloc, ceiling_realloc, free); }
  for (uint64_t i = 0; i < n; i++) {
    uint64_t begin = ceiling_now();
    if (p->incremental) { ts_tree_edit(p->tree, &p->edit[p->current]); p->current ^= 1; }
    uint64_t parse_begin = ceiling_now();
    TSTree *tree = ts_parser_parse_string(p->parser, p->tree, p->source[p->current], p->length[p->current]);
    uint64_t parse_end = ceiling_now();
    if (p->tree) ts_tree_delete(p->tree);
    if (p->incremental) p->tree = tree;
    else ts_tree_delete(tree);
    result.ns += ceiling_now() - begin;
    result.parse_ns += parse_end - parse_begin;
    if (!tree) { result.failed = 1; break; }
  }
  if (count) { result.bytes = ceiling_bytes; result.allocs = ceiling_allocs; ts_set_allocator(malloc, calloc, realloc, free); }
  return result;
}
static void ceiling_close(ceiling_parser *p) {
  if (p->tree) ts_tree_delete(p->tree);
  ts_parser_delete(p->parser); free(p->source[0]); free(p->source[1]); free(p);
}
static void ceiling_reset_direction(ceiling_parser *p) {
  if (p->incremental && p->current) ceiling_batch(p, 1, 0);
}
static uint64_t ceiling_noop(uint64_t n) { return n; }
*/
import "C"

import (
	gts "github.com/odvcencio/gotreesitter"
	"unsafe"
)

type ceilingCParser struct{ inner *C.ceiling_parser }
type ceilingCSample struct {
	nanos, parseNanos, bytes, allocs uint64
	failed                           bool
}

func ceilingCEdit(e gts.InputEdit) C.TSInputEdit {
	return C.TSInputEdit{start_byte: C.uint32_t(e.StartByte), old_end_byte: C.uint32_t(e.OldEndByte), new_end_byte: C.uint32_t(e.NewEndByte),
		start_point:   C.TSPoint{row: C.uint32_t(e.StartPoint.Row), column: C.uint32_t(e.StartPoint.Column)},
		old_end_point: C.TSPoint{row: C.uint32_t(e.OldEndPoint.Row), column: C.uint32_t(e.OldEndPoint.Column)},
		new_end_point: C.TSPoint{row: C.uint32_t(e.NewEndPoint.Row), column: C.uint32_t(e.NewEndPoint.Column)}}
}

func newCeilingCParser(language unsafe.Pointer, a, b []byte, forward, backward gts.InputEdit, incremental bool) *ceilingCParser {
	var inc C.int
	if incremental {
		inc = 1
	}
	p := C.ceiling_new(language, unsafe.Pointer(&a[0]), C.uint32_t(len(a)), unsafe.Pointer(&b[0]), C.uint32_t(len(b)), ceilingCEdit(forward), ceilingCEdit(backward), inc)
	if p == nil {
		return nil
	}
	return &ceilingCParser{inner: p}
}

func (p *ceilingCParser) batch(n int, count bool) ceilingCSample {
	var c C.int
	if count {
		c = 1
	}
	r := C.ceiling_batch(p.inner, C.uint64_t(n), c)
	return ceilingCSample{uint64(r.ns), uint64(r.parse_ns), uint64(r.bytes), uint64(r.allocs), r.failed != 0}
}
func (p *ceilingCParser) close()          { C.ceiling_close(p.inner) }
func (p *ceilingCParser) resetDirection() { C.ceiling_reset_direction(p.inner) }
func ceilingCGONoop(n uint64) uint64      { return uint64(C.ceiling_noop(C.uint64_t(n))) }

func (p *ceilingCParser) directionIsReset() bool { return p.inner.current == 0 }
