//go:build cgo

// Package queryalloc measures native C query allocation requests in isolated
// harness processes. The runtime allocator hooks are process global.
package queryalloc

/*
#include <stdint.h>
typedef struct { uint64_t bytes; uint64_t allocations; } query_alloc_stats;
void query_alloc_start(void);
query_alloc_stats query_alloc_stop(void);
uint64_t query_alloc_match_record_bytes(void);
*/
import "C"

type Stats struct{ Bytes, Allocations uint64 }

// Start replaces the C runtime's allocator until Stop. Call only while no
// other C operations run; it does not replace Go's allocator.
func Start() { C.query_alloc_start() }

// Stop restores the runtime's default allocator and returns requested bytes
// and allocation calls, including reallocations. Native allocator rounding
// and grammar scanner allocations outside the runtime hooks are excluded.
func Stop() Stats {
	s := C.query_alloc_stop()
	return Stats{uint64(s.bytes), uint64(s.allocations)}
}

// MatchRecordBytes is the public TSQueryMatch ABI size. The Go C binding
// allocates one such libc record per Matches.Next call, outside runtime hooks.
func MatchRecordBytes() uint64 { return uint64(C.query_alloc_match_record_bytes()) }
