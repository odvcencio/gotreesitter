#include <stdint.h>
#include <stdlib.h>

typedef struct { uint64_t bytes; uint64_t allocations; } query_alloc_stats;
extern void ts_set_allocator(void *(*)(size_t), void *(*)(size_t, size_t),
                             void *(*)(void *, size_t), void (*)(void *));
static query_alloc_stats stats;

// Public TSQueryMatch layout, shared by the locked runtime and Go binding.
typedef struct {
  uint32_t id;
  uint16_t pattern_index;
  uint16_t capture_count;
  const void *captures;
} query_match_record;

uint64_t query_alloc_match_record_bytes(void) { return sizeof(query_match_record); }

static void *query_malloc(size_t size) {
  stats.bytes += size;
  stats.allocations++;
  return malloc(size);
}

static void *query_calloc(size_t count, size_t size) {
  stats.bytes += count * size;
  stats.allocations++;
  return calloc(count, size);
}

static void *query_realloc(void *pointer, size_t size) {
  stats.bytes += size;
  stats.allocations++;
  return realloc(pointer, size);
}

void query_alloc_start(void) {
  stats = (query_alloc_stats){0};
  ts_set_allocator(query_malloc, query_calloc, query_realloc, free);
}

query_alloc_stats query_alloc_stop(void) {
  ts_set_allocator(NULL, NULL, NULL, NULL);
  return stats;
}
