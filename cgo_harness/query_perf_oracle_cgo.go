//go:build cgo && treesitter_c_parity

package cgoharness

/*
#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <regex.h>

// Public tree-sitter ABI declarations. The implementation is linked from the
// pinned go-tree-sitter module, just like COracleLanguage's parity transport.
typedef struct TSLanguage TSLanguage;
typedef struct TSParser TSParser;
typedef struct TSTree TSTree;
typedef struct TSQuery TSQuery;
typedef struct TSQueryCursor TSQueryCursor;
typedef struct { uint32_t context[4]; const void *id; const TSTree *tree; } TSNode;
typedef struct { TSNode node; uint32_t index; } TSQueryCapture;
typedef struct { uint32_t id; uint16_t pattern_index; uint16_t capture_count; const TSQueryCapture *captures; } TSQueryMatch;
typedef struct { int type; uint32_t value_id; } TSQueryPredicateStep;
extern TSParser *ts_parser_new(void);
extern void ts_parser_delete(TSParser *);
extern bool ts_parser_set_language(TSParser *, const TSLanguage *);
extern TSTree *ts_parser_parse_string(TSParser *, const TSTree *, const char *, uint32_t);
extern void ts_tree_delete(TSTree *);
extern TSNode ts_tree_root_node(const TSTree *);
extern uint32_t ts_node_start_byte(TSNode);
extern uint32_t ts_node_end_byte(TSNode);
extern TSQuery *ts_query_new(const TSLanguage *, const char *, uint32_t, uint32_t *, int *);
extern void ts_query_delete(TSQuery *);
extern uint32_t ts_query_pattern_count(const TSQuery *);
extern const TSQueryPredicateStep *ts_query_predicates_for_pattern(const TSQuery *, uint32_t, uint32_t *);
extern const char *ts_query_string_value_for_id(const TSQuery *, uint32_t, uint32_t *);
extern TSQueryCursor *ts_query_cursor_new(void);
extern void ts_query_cursor_delete(TSQueryCursor *);
extern void ts_query_cursor_exec(TSQueryCursor *, const TSQuery *, TSNode);
extern bool ts_query_cursor_next_match(TSQueryCursor *, TSQueryMatch *);
extern bool ts_query_cursor_set_byte_range(TSQueryCursor *, uint32_t, uint32_t);
extern bool ts_query_cursor_did_exceed_match_limit(const TSQueryCursor *);
extern void ts_set_allocator(void *(*)(size_t), void *(*)(size_t, size_t), void *(*)(void *, size_t), void (*)(void *));

static void queryPerfNativeAllocator(void) {
  // The binding's default allocator bounces every malloc/free through Go.
  // Use the same libc allocator directly for a native-runtime comparison.
  ts_set_allocator(malloc, calloc, realloc, free);
}

typedef struct { uint32_t start, end, pattern, capture; } QueryPerfCapture;
typedef struct {
  int kind; // 0 metadata, 1 equality, 2 regex, 3 any-of
  bool positive, any, right_capture;
  uint32_t left, right;
  const char *literal;
  uint32_t length;
  regex_t regex;
  const TSQueryPredicateStep *values;
  uint32_t value_count;
} QueryPerfPredicate;
typedef struct { QueryPerfPredicate *items; uint32_t count; } QueryPerfPattern;
typedef struct {
  TSQuery *query;
  TSTree *tree;
  char *source;
  QueryPerfPattern *patterns;
  uint32_t pattern_count;
} QueryPerfOracle;

static char *queryPerfRegex(const char *s, uint32_t n) {
  char *out = malloc(n * 16 + 1); if (!out) return NULL;
  uint32_t j = 0; bool in_class = false;
  for (uint32_t i = 0; i < n; i++) {
    if (s[i] == '\\' && i + 1 < n && strchr("dws", s[i + 1])) {
      const char *replacement = s[++i] == 'd' ? "0-9" : s[i] == 'w' ? "a-zA-Z0-9_" : "[:space:]";
      if (!in_class) out[j++] = '[';
      while (*replacement) out[j++] = *replacement++;
      if (!in_class) out[j++] = ']';
    } else {
      if (s[i] == '[') in_class = true;
      if (s[i] == ']') in_class = false;
      out[j++] = s[i];
    }
  }
  out[j] = 0; return out;
}

static void queryPerfDelete(QueryPerfOracle *o) {
  if (!o) return;
  for (uint32_t i = 0; i < o->pattern_count; i++) {
    for (uint32_t j = 0; j < o->patterns[i].count; j++)
      if (o->patterns[i].items[j].kind == 2) regfree(&o->patterns[i].items[j].regex);
    free(o->patterns[i].items);
  }
  free(o->patterns); if (o->query) ts_query_delete(o->query);
  if (o->tree) ts_tree_delete(o->tree); free(o->source); free(o);
}

static QueryPerfOracle *queryPerfNew(const void *language, const char *source, uint32_t length, const char *query, uint32_t query_length, int *error) {
  *error = 1;
  QueryPerfOracle *o = calloc(1, sizeof(*o)); if (!o) return NULL;
  uint32_t error_offset; int error_type;
  o->query = ts_query_new(language, query, query_length, &error_offset, &error_type);
  if (!o->query) goto fail;
  o->source = malloc(length + 1); if (!o->source) goto fail;
  memcpy(o->source, source, length); o->source[length] = 0;
  TSParser *parser = ts_parser_new();
  if (!parser) goto fail;
  bool valid = ts_parser_set_language(parser, language);
  if (valid) o->tree = ts_parser_parse_string(parser, NULL, source, length);
  ts_parser_delete(parser); if (!o->tree) goto fail;
  o->pattern_count = ts_query_pattern_count(o->query);
  o->patterns = calloc(o->pattern_count, sizeof(*o->patterns));
  if (!o->patterns) { o->pattern_count = 0; goto fail; }
  *error = 2;
  for (uint32_t pi = 0; pi < o->pattern_count; pi++) {
    uint32_t n; const TSQueryPredicateStep *steps = ts_query_predicates_for_pattern(o->query, pi, &n);
    QueryPerfPattern *pat = &o->patterns[pi];
    pat->items = calloc(n ? n : 1, sizeof(*pat->items)); if (!pat->items) goto fail;
    for (uint32_t i = 0; i < n;) {
      uint32_t end = i; while (end < n && steps[end].type != 0) end++;
      if (steps[i].type != 2) goto fail;
      uint32_t name_len; const char *name = ts_query_string_value_for_id(o->query, steps[i].value_id, &name_len);
      QueryPerfPredicate *p = &pat->items[pat->count];
      p->positive = strncmp(name, "not-", 4) != 0 && strncmp(name, "any-not-", 8) != 0;
      p->any = strncmp(name, "any-", 4) == 0;
      if (strcmp(name, "set!") == 0 || strcmp(name, "is?") == 0 || strcmp(name, "is-not?") == 0 || strcmp(name, "offset!") == 0) {
        i = end + 1; continue; // host metadata/directives do not change raw node spans
      }
      if (end < i + 3 || steps[i + 1].type != 1) goto fail;
      p->left = steps[i + 1].value_id;
      if (strcmp(name, "eq?") == 0 || strcmp(name, "not-eq?") == 0 || strcmp(name, "any-eq?") == 0 || strcmp(name, "any-not-eq?") == 0) {
        p->kind = 1; p->right_capture = steps[i + 2].type == 1; p->right = steps[i + 2].value_id;
        if (!p->right_capture) p->literal = ts_query_string_value_for_id(o->query, p->right, &p->length);
      } else if (strcmp(name, "match?") == 0 || strcmp(name, "not-match?") == 0 || strcmp(name, "any-match?") == 0 || strcmp(name, "any-not-match?") == 0) {
        if (steps[i + 2].type != 2) goto fail;
        const char *s = ts_query_string_value_for_id(o->query, steps[i + 2].value_id, &p->length);
        char *regex = queryPerfRegex(s, p->length); if (!regex) goto fail;
        int result = regcomp(&p->regex, regex, REG_EXTENDED | REG_NOSUB); free(regex);
        if (result) goto fail;
        p->kind = 2;
      } else if (strcmp(name, "any-of?") == 0 || strcmp(name, "not-any-of?") == 0) {
        p->kind = 3; p->any = false; p->values = steps + i + 2; p->value_count = end - i - 2;
        for (uint32_t j = 0; j < p->value_count; j++) if (p->values[j].type != 2) goto fail;
      } else goto fail;
      pat->count++; i = end + 1;
    }
  }
  *error = 0; return o;
fail:
  queryPerfDelete(o); return NULL;
}

static bool queryPerfPredicates(QueryPerfOracle *o, TSQueryMatch *m) {
  QueryPerfPattern *pat = &o->patterns[m->pattern_index];
  for (uint32_t i = 0; i < pat->count; i++) {
    QueryPerfPredicate *p = &pat->items[i]; bool found = false, satisfied = !p->any;
    uint32_t right_index = 0;
    for (uint32_t j = 0; j < m->capture_count; j++) {
      if (m->captures[j].index != p->left) continue;
      found = true; TSNode node = m->captures[j].node;
      uint32_t start = ts_node_start_byte(node), length = ts_node_end_byte(node) - start;
      const char *text = o->source + start; bool match = false;
      if (p->kind == 1) {
        const char *right = p->literal; uint32_t right_length = p->length;
        if (p->right_capture) {
          while (right_index < m->capture_count && m->captures[right_index].index != p->right) right_index++;
          if (right_index == m->capture_count) return false;
          TSNode r = m->captures[right_index++].node;
          right = o->source + ts_node_start_byte(r); right_length = ts_node_end_byte(r) - ts_node_start_byte(r);
        }
        match = length == right_length && memcmp(text, right, length) == 0;
      } else if (p->kind == 2) {
        char *terminated = malloc(length + 1); if (!terminated) return false;
        memcpy(terminated, text, length); terminated[length] = 0;
        match = regexec(&p->regex, terminated, 0, NULL, 0) == 0; free(terminated);
      } else {
        for (uint32_t k = 0; k < p->value_count && !match; k++) {
          uint32_t n; const char *v = ts_query_string_value_for_id(o->query, p->values[k].value_id, &n);
          match = length == n && memcmp(text, v, n) == 0;
        }
      }
      match = match == p->positive;
      if (!p->any && !match) { satisfied = false; break; }
      if (p->any && match) { satisfied = true; break; }
    }
    if (found && !satisfied) return false;
  }
  return true;
}

static int queryPerfCompare(const void *a, const void *b) {
  const QueryPerfCapture *x = a, *y = b;
  if (x->start != y->start) return x->start < y->start ? -1 : 1;
  if (x->end != y->end) return x->end < y->end ? -1 : 1;
  if (x->pattern != y->pattern) return x->pattern < y->pattern ? -1 : 1;
  return x->capture < y->capture ? -1 : x->capture > y->capture;
}

static QueryPerfCapture *queryPerfRun(QueryPerfOracle *o, uint32_t start, uint32_t end, uint32_t *count, bool *complete) {
  TSQueryCursor *cursor = ts_query_cursor_new();
  *count = 0; *complete = false; if (!cursor) return NULL;
  ts_query_cursor_set_byte_range(cursor, start, end);
  ts_query_cursor_exec(cursor, o->query, ts_tree_root_node(o->tree));
  uint32_t capacity = 0; QueryPerfCapture *rows = NULL; TSQueryMatch m;
  while (ts_query_cursor_next_match(cursor, &m)) {
    if (!queryPerfPredicates(o, &m)) continue;
    if (*count + m.capture_count > capacity) {
      capacity = (capacity + m.capture_count) * 2 + 32;
      QueryPerfCapture *next = realloc(rows, capacity * sizeof(*rows));
      if (!next) { free(rows); ts_query_cursor_delete(cursor); return NULL; }
      rows = next;
    }
    for (uint32_t i = 0; i < m.capture_count; i++) {
      TSQueryCapture cap = m.captures[i];
      rows[(*count)++] = (QueryPerfCapture){ts_node_start_byte(cap.node), ts_node_end_byte(cap.node), m.pattern_index, cap.index};
    }
  }
  *complete = !ts_query_cursor_did_exceed_match_limit(cursor);
  ts_query_cursor_delete(cursor);
  if (*count > 1) qsort(rows, *count, sizeof(*rows), queryPerfCompare);
  return rows;
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type queryPerfCapture struct{ start, end, pattern, capture uint32 }

type queryPerfNativeOracle struct{ inner *C.QueryPerfOracle }

func queryPerfUseNativeAllocator() { C.queryPerfNativeAllocator() }

func newQueryPerfNativeOracle(lang *sitter.Language, source []byte, query string) (*queryPerfNativeOracle, error) {
	src, scm := C.CBytes(source), C.CString(query)
	defer C.free(src)
	defer C.free(unsafe.Pointer(scm))
	var code C.int
	inner := C.queryPerfNew(unsafe.Pointer(lang.Inner), (*C.char)(src), C.uint32_t(len(source)), scm, C.uint32_t(len(query)), &code)
	if inner == nil {
		return nil, fmt.Errorf("native query oracle setup failed (code %d)", code)
	}
	return &queryPerfNativeOracle{inner}, nil
}

func (o *queryPerfNativeOracle) close() { C.queryPerfDelete(o.inner) }

// run includes cursor creation, matching, predicates, output allocation and
// ordering. The returned native buffer remains owned by the caller.
func (o *queryPerfNativeOracle) run(start, end uint32) ([]queryPerfCapture, func(), bool) {
	var count C.uint32_t
	var complete C.bool
	rows := C.queryPerfRun(o.inner, C.uint32_t(start), C.uint32_t(end), &count, &complete)
	return unsafe.Slice((*queryPerfCapture)(unsafe.Pointer(rows)), int(count)), func() { C.free(unsafe.Pointer(rows)) }, bool(complete)
}
