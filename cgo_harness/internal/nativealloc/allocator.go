//go:build cgo && treesitter_c_parity

// Package nativealloc measures successful native allocation requests. Install
// its functions with the oracle's allocator hook only in a dedicated receipt
// process. It measures requested bytes, not retained memory or process RSS.
package nativealloc

/*
#include <stdlib.h>
#include <stdint.h>
#include <stdatomic.h>

static _Atomic uint64_t accounting_bytes;
static _Atomic uint64_t accounting_allocs;

static void accounting_note(void *ptr, size_t size) {
 if (ptr && size) {
  atomic_fetch_add(&accounting_bytes, size);
  atomic_fetch_add(&accounting_allocs, 1);
 }
}
static void *accounting_malloc(size_t size) {
 void *ptr = malloc(size);
 accounting_note(ptr, size);
 return ptr;
}
static void *accounting_calloc(size_t num, size_t size) {
 void *ptr = calloc(num, size);
 accounting_note(ptr, num * size);
 return ptr;
}
static void *accounting_realloc(void *ptr, size_t size) {
 void *next = realloc(ptr, size);
 accounting_note(next, size);
 return next;
}
static void accounting_reset(void) {
 atomic_store(&accounting_bytes, 0);
 atomic_store(&accounting_allocs, 0);
}
static uint64_t accounting_total_bytes(void) { return atomic_load(&accounting_bytes); }
static uint64_t accounting_total_allocs(void) { return atomic_load(&accounting_allocs); }
*/
import "C"

import "unsafe"

type Counts struct{ Bytes, Allocations uint64 }

func Malloc(size uint) unsafe.Pointer      { return C.accounting_malloc(C.size_t(size)) }
func Calloc(num, size uint) unsafe.Pointer { return C.accounting_calloc(C.size_t(num), C.size_t(size)) }
func Realloc(ptr unsafe.Pointer, size uint) unsafe.Pointer {
	return C.accounting_realloc(ptr, C.size_t(size))
}
func Free(ptr unsafe.Pointer) { C.free(ptr) }
func Reset()                  { C.accounting_reset() }
func Snapshot() Counts {
	return Counts{uint64(C.accounting_total_bytes()), uint64(C.accounting_total_allocs())}
}
