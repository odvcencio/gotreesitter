//go:build linux && cgo && treesitter_c_parity

package cgoharness

/*
#include <stddef.h>
#include <stdint.h>

extern void ts_set_allocator(
  void *(*)(size_t), void *(*)(size_t, size_t),
  void *(*)(void *, size_t), void (*)(void *)
);
extern void *(*ts_current_malloc)(size_t);
extern void *(*ts_current_calloc)(size_t, size_t);
extern void *(*ts_current_realloc)(void *, size_t);
extern void (*ts_current_free)(void *);

typedef struct { uint64_t calls; uint64_t bytes; } q1_native_allocations;
static q1_native_allocations q1_allocations;
static void *(*q1_malloc_previous)(size_t);
static void *(*q1_calloc_previous)(size_t, size_t);
static void *(*q1_realloc_previous)(void *, size_t);
static void (*q1_free_previous)(void *);

static void *q1_malloc(size_t size) {
  void *result = q1_malloc_previous(size);
  if (result) { q1_allocations.calls++; q1_allocations.bytes += size; }
  return result;
}
static void *q1_calloc(size_t count, size_t size) {
  void *result = q1_calloc_previous(count, size);
  if (result) { q1_allocations.calls++; q1_allocations.bytes += count * size; }
  return result;
}
static void *q1_realloc(void *ptr, size_t size) {
  void *result = q1_realloc_previous(ptr, size);
  if (result) { q1_allocations.calls++; q1_allocations.bytes += size; }
  return result;
}
static void q1_allocations_begin(void) {
  q1_malloc_previous = ts_current_malloc;
  q1_calloc_previous = ts_current_calloc;
  q1_realloc_previous = ts_current_realloc;
  q1_free_previous = ts_current_free;
  q1_allocations = (q1_native_allocations){0, 0};
  ts_set_allocator(q1_malloc, q1_calloc, q1_realloc, q1_free_previous);
}
static q1_native_allocations q1_allocations_end(void) {
  ts_set_allocator(q1_malloc_previous, q1_calloc_previous,
                   q1_realloc_previous, q1_free_previous);
  return q1_allocations;
}
*/
import "C"

// These process-wide hooks run only in isolated, single-language measurements.
// They delegate to the existing allocator, preserving its failure policy and
// its ability to free objects created before the measurement. Bytes count
// successful allocation requests, including the full requested realloc size;
// calls include realloc even when libc can resize a block in place.
type q1NativeAllocations struct {
	Calls uint64
	Bytes uint64
}

func q1BeginNativeAllocations() { C.q1_allocations_begin() }

func q1EndNativeAllocations() q1NativeAllocations {
	counts := C.q1_allocations_end()
	return q1NativeAllocations{Calls: uint64(counts.calls), Bytes: uint64(counts.bytes)}
}
