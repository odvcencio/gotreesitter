// Complete-operation counterpart to BenchmarkTreeNavigationComplete.
// Link with the locked C runtime, then load its locked grammar artifact.
#include <dlfcn.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <tree_sitter/api.h>

static uint64_t allocated_bytes, allocation_calls, visits, wrong_parents;
static void *count_malloc(size_t n) {
  allocated_bytes += n; allocation_calls++; return malloc(n);
}
static void *count_calloc(size_t n, size_t size) {
  allocated_bytes += n * size; allocation_calls++; return calloc(n, size);
}
static void *count_realloc(void *p, size_t n) {
  allocated_bytes += n; allocation_calls++; return realloc(p, n);
}
static uint64_t now_ns(void) {
  struct timespec t;
  clock_gettime(CLOCK_MONOTONIC, &t);
  return (uint64_t)t.tv_sec * 1000000000ull + t.tv_nsec;
}
static void walk(TSNode parent) {
  uint32_t count = ts_node_child_count(parent);
  for (uint32_t i = 0; i < count; i++) {
    TSNode child = ts_node_child(parent, i);
    if (!ts_node_eq(ts_node_parent(child), parent)) wrong_parents++;
    visits++;
    walk(child);
  }
}
static char *read_source(const char *path, uint32_t *length) {
  FILE *f = fopen(path, "rb");
  if (!f || fseek(f, 0, SEEK_END)) return NULL;
  long size = ftell(f);
  if (size < 0 || (uint64_t)size > UINT32_MAX || fseek(f, 0, SEEK_SET)) {
    fclose(f); return NULL;
  }
  char *source = malloc((size_t)size + 1);
  if (!source || fread(source, 1, size, f) != (size_t)size) {
    fclose(f); free(source); return NULL;
  }
  fclose(f);
  source[size] = 0;
  *length = (uint32_t)size;
  return source;
}
int main(int argc, char **argv) {
  if (argc != 7) {
    fprintf(stderr, "usage: %s grammar.so language symbol source Full|Edit duration_ms\n", argv[0]);
    return 2;
  }
  uint32_t length;
  char *source = read_source(argv[4], &length);
  void *grammar = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
  const TSLanguage *(*language)(void) = grammar ? dlsym(grammar, argv[3]) : NULL;
  uint64_t duration = strtoull(argv[6], NULL, 10) * 1000000ull;
  int editing = !strcmp(argv[5], "Edit");
  if (!source || !language || !duration || (!editing && strcmp(argv[5], "Full"))) return 2;
  ts_set_allocator(count_malloc, count_calloc, count_realloc, free);
  TSParser *parser = ts_parser_new();
  if (!parser || !ts_parser_set_language(parser, language())) return 2;
  TSTree *old = ts_parser_parse_string(parser, NULL, source, length);
  if (!old || ts_node_has_error(ts_tree_root_node(old)) || ts_node_end_byte(ts_tree_root_node(old)) != length) return 3;
  walk(ts_tree_root_node(old));
  char *marker = NULL;
  for (char *p = source; (p = strstr(p, "a + b")); p++) marker = p;
  if (!marker) return 2;
  uint32_t at = (uint32_t)(marker - source) + 2;
  TSPoint start = {0, 0};
  for (uint32_t i = 0; i < at; i++) {
    if (source[i] == '\n') { start.row++; start.column = 0; }
    else start.column++;
  }
  TSPoint end = {start.row, start.column + 1};
  TSInputEdit edit = {at, at + 1, at + 1, start, end, end};
  if (!editing) { ts_tree_delete(old); old = NULL; }
  allocated_bytes = allocation_calls = visits = wrong_parents = 0;
  uint64_t repetitions = 0, begin = now_ns(), elapsed;
  do {
    if (editing) {
      source[at] = source[at] == '+' ? '-' : '+';
      ts_tree_edit(old, &edit);
    }
    TSTree *next = ts_parser_parse_string(parser, old, source, length);
    if (!next) return 3;
    walk(ts_tree_root_node(next));
    if (editing) {
      walk(ts_tree_root_node(old));
      ts_tree_delete(old);
      old = next;
    } else ts_tree_delete(next);
    repetitions++;
    elapsed = now_ns() - begin;
  } while (elapsed < duration);
  printf("BenchmarkLockedCNavigation/%s/%s %llu %.3f ns/op %.3f B/op %.3f allocs/op %.3f wrong-parent/op\n",
    argv[2], argv[5], (unsigned long long)repetitions,
    (double)elapsed / repetitions, (double)allocated_bytes / repetitions,
    (double)allocation_calls / repetitions, (double)wrong_parents / repetitions);
  if (old) ts_tree_delete(old);
  ts_parser_delete(parser);
  free(source);
  dlclose(grammar);
  return wrong_parents ? 4 : 0;
}
