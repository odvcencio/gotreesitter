//go:build cgo

package tree_sitter

/*
#cgo CFLAGS: -Iinclude -Isrc -std=c11 -D_POSIX_C_SOURCE=200112L -D_DEFAULT_SOURCE
#include <tree_sitter/api.h>
#include "lib.c" // <- This is needed to build the C library from the C source code, but cannot be included in files that have other declarations.

// Called synchronously from the parser's log callback.
static uint32_t gts_log_stack_version_count(const TSParser *parser) {
	return ts_stack_version_count(parser->stack);
}
*/
import "C"

// LogStackVersionCount returns the current number of resident stack versions.
// Call it only from a synchronous log callback on this parser.
func (p *Parser) LogStackVersionCount() uint32 {
	return uint32(C.gts_log_stack_version_count(p._inner))
}
