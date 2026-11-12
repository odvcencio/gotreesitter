//go:build cgo

// Package scandiff drives the compiled C external scanner of a grammar with the
// C runtime's own lexer, for the scanner differential test in cgo_harness.
//
// One Call does what ts_parser__lex does around an external scan
// (C/parser.c:536-545): position the lexer (ts_lexer_reset), ts_lexer_start,
// the scanner's deserialize, its scan, ts_lexer_finish and, when the scanner
// found a token, its serialize into the lexer's 1,024-byte buffer. The lexer is
// the vendored runtime's (ts_lexer_init, ts_lexer_set_input, ts_lexer_reset,
// ts_lexer_start, ts_lexer_finish): UTF-8 decoding, the -1 lookahead on a
// decode error, columns, the byte order mark and eof are C's own. Nothing here
// re-implements a lexer.
//
// Like ../corecords, the package reads private runtime headers without touching
// the vendored sources; the runtime object code is the binding's. The scanner
// entry points come from the TSLanguage struct of the loaded grammar, or, to
// measure branch coverage, from a second shared object that holds the same
// scanner source built with --coverage.
//
// A scanner can read out of bounds on a damaged saved state. Every call is
// guarded: a fatal signal inside the scanner returns to the caller as a crash
// record instead of ending the process.
package scandiff

/*
#cgo CFLAGS: -I${SRCDIR}/../coracle/include -I${SRCDIR}/../coracle/src -std=gnu11 -D_POSIX_C_SOURCE=200809L -D_DEFAULT_SOURCE -O1
#cgo linux LDFLAGS: -ldl
#include <stdio.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
#include <signal.h>
#include <setjmp.h>
#include <dlfcn.h>
#include "lexer.h"
#include "parser.h"
#include "language.h"

typedef void *(*sd_create_fn)(void);
typedef void (*sd_destroy_fn)(void *);
typedef bool (*sd_scan_fn)(void *, TSLexer *, const bool *);
typedef unsigned (*sd_serialize_fn)(void *, char *);
typedef void (*sd_deserialize_fn)(void *, const char *, unsigned);

typedef struct {
  const TSLanguage *lang;
  sd_create_fn create;
  sd_destroy_fn destroy;
  sd_scan_fn scan;
  sd_serialize_fn serialize;
  sd_deserialize_fn deserialize;
  void *dl;
  void *payload;
  char *src;
  uint32_t len;
  Lexer lexer;
  int lexer_live;
  int payload_made;
  int rg_set;
  TSRange rg;
  // The scanner gets copies of the mask and the saved state in zero-filled buffers, so a read past their end
  // sees zeros: the same on every run, not whatever follows a Go slice in memory.
  bool mbuf[512];
  char stbuf[2 * TREE_SITTER_SERIALIZATION_BUFFER_SIZE];
  char dbg[TREE_SITTER_SERIALIZATION_BUFFER_SIZE]; // the lexer's serialization buffer, kept across calls as in a parse
} sd_driver;

typedef struct {
  int crash;         // 0 none; else the signal number
  int phase;         // 1 create, 2 deserialize, 3 scan, 4 serialize (where the crash happened)
  int found;
  uint32_t sym_index;  // result_symbol: index of the external token
  uint32_t symbol;     // symbol_map[result_symbol]
  uint32_t start, end;
  uint32_t srow, scol, erow, ecol;
  uint32_t look_end;
  int did_get_column;
  uint32_t state_len;  // the length the scanner reported, not clamped
  unsigned char state[TREE_SITTER_SERIALIZATION_BUFFER_SIZE];
} sd_out;

static __thread sigjmp_buf *sd_jmp;
static __thread volatile int sd_phase;

static void sd_handler(int sig) {
  if (sd_jmp) siglongjmp(*sd_jmp, sig);
  signal(sig, SIG_DFL);
  raise(sig);
}

static const int sd_signals[] = {SIGSEGV, SIGBUS, SIGABRT, SIGFPE, SIGILL, SIGTRAP};
#define SD_NSIG ((int)(sizeof sd_signals / sizeof sd_signals[0]))

static void sd_guard_on(struct sigaction *old) {
  struct sigaction sa;
  memset(&sa, 0, sizeof sa);
  sa.sa_handler = sd_handler;
  sigemptyset(&sa.sa_mask);
  sa.sa_flags = SA_ONSTACK | SA_NODEFER;
  for (int i = 0; i < SD_NSIG; i++) sigaction(sd_signals[i], &sa, &old[i]);
}

static void sd_guard_off(struct sigaction *old) {
  for (int i = 0; i < SD_NSIG; i++) sigaction(sd_signals[i], &old[i], NULL);
}

static const char *sd_read(void *payload, uint32_t byte, TSPoint pos, uint32_t *bytes_read) {
  (void)pos;
  sd_driver *d = (sd_driver *)payload;
  if (byte >= d->len) { *bytes_read = 0; return ""; }
  *bytes_read = d->len - byte;
  return d->src + byte;
}

static sd_driver *sd_new(const void *lang) {
  sd_driver *d = calloc(1, sizeof *d);
  d->lang = (const TSLanguage *)lang;
  d->create = d->lang->external_scanner.create;
  d->destroy = d->lang->external_scanner.destroy;
  d->scan = d->lang->external_scanner.scan;
  d->serialize = d->lang->external_scanner.serialize;
  d->deserialize = d->lang->external_scanner.deserialize;
  return d;
}

// sd_use_so takes the five entry points from a shared object by their exported
// names (tree_sitter_<name>_external_scanner_*). It returns NULL on success.
static const char *sd_use_so(sd_driver *d, const char *path, const char *name) {
  d->dl = dlopen(path, RTLD_NOW | RTLD_LOCAL);
  if (!d->dl) return dlerror();
  char sym[256];
#define SD_LOAD(field, suffix) \
  snprintf(sym, sizeof sym, "tree_sitter_%s_external_scanner_" suffix, name); \
  *(void **)(&d->field) = dlsym(d->dl, sym); \
  if (!d->field) return "missing symbol: " suffix;
  SD_LOAD(create, "create")
  SD_LOAD(destroy, "destroy")
  SD_LOAD(scan, "scan")
  SD_LOAD(serialize, "serialize")
  SD_LOAD(deserialize, "deserialize")
#undef SD_LOAD
  return NULL;
}

static void sd_set_source(sd_driver *d, const char *src, uint32_t len) {
  free(d->src);
  d->src = malloc(len ? len : 1);
  if (len) memcpy(d->src, src, len);
  d->len = len;
}

static void sd_drop_payload(sd_driver *d) {
  d->payload = NULL; // leaked on purpose: after a crash the payload may be damaged
  d->payload_made = 0;
}

static void sd_set_range(sd_driver *d, uint32_t sb, uint32_t sr, uint32_t sc, uint32_t eb, uint32_t er, uint32_t ec) {
  d->rg_set = 1;
  d->rg.start_byte = sb; d->rg.start_point.row = sr; d->rg.start_point.column = sc;
  d->rg.end_byte = eb; d->rg.end_point.row = er; d->rg.end_point.column = ec;
}
static void sd_clear_range(sd_driver *d) { d->rg_set = 0; }

// sd_reset_all gives the driver a fresh scanner payload, a fresh lexer and a zeroed serialization buffer.
static void sd_reset_all(sd_driver *d) {
  if (d->payload_made && d->destroy) d->destroy(d->payload);
  d->payload = NULL;
  d->payload_made = 0;
  if (d->lexer_live) { ts_lexer_delete(&d->lexer); d->lexer_live = 0; }
  memset(d->dbg, 0, sizeof d->dbg);
}

// sd_new_parse ends a parse: the payload is destroyed and the lexer dropped, the serialization buffer is kept
// (the core destroys the payload after a parse and keeps its buffer).
static void sd_new_parse(sd_driver *d) {
  if (d->payload_made && d->destroy) d->destroy(d->payload);
  d->payload = NULL;
  d->payload_made = 0;
  if (d->lexer_live) { memcpy(d->dbg, d->lexer.debug_buffer, sizeof d->dbg); ts_lexer_delete(&d->lexer); d->lexer_live = 0; }
}

static void sd_close_payload(sd_driver *d) {
  if (d->payload_made && d->destroy) d->destroy(d->payload);
  d->payload = NULL;
  d->payload_made = 0;
}

// sd_call is one external lex step. mask has external_token_count entries.
static void sd_call(sd_driver *d, uint32_t off, uint32_t row, uint32_t col,
                    const bool *mask, const char *state, uint32_t state_len, sd_out *out) {
  memset(out, 0, sizeof *out);
  if (d->lexer_live) {
    memcpy(d->dbg, d->lexer.debug_buffer, sizeof d->dbg);
    ts_lexer_delete(&d->lexer);
  }
  ts_lexer_init(&d->lexer);
  memcpy(d->lexer.debug_buffer, d->dbg, sizeof d->dbg);
  d->lexer_live = 1;
  TSInput in = {.payload = d, .read = sd_read, .encoding = TSInputEncodingUTF8, .decode = NULL};
  ts_lexer_set_input(&d->lexer, in);
  if (d->rg_set) ts_lexer_set_included_ranges(&d->lexer, &d->rg, 1);
  Length pos = {off, {row, col}};
  ts_lexer_reset(&d->lexer, pos);

  memset(d->mbuf, 0, sizeof d->mbuf);
  memcpy(d->mbuf, mask, d->lang->external_token_count < sizeof d->mbuf ? d->lang->external_token_count : sizeof d->mbuf);
  memset(d->stbuf, 0, sizeof d->stbuf);
  if (state_len > 0) memcpy(d->stbuf, state, state_len < sizeof d->stbuf ? state_len : sizeof d->stbuf);
  sigjmp_buf env;
  struct sigaction old[SD_NSIG];
  sd_jmp = &env;
  sd_guard_on(old);
  int sig = sigsetjmp(env, 1);
  if (sig != 0) {
    sd_guard_off(old);
    sd_jmp = NULL;
    out->crash = sig;
    out->phase = sd_phase;
    sd_drop_payload(d);
    return;
  }
  sd_phase = 1;
  if (!d->payload_made) { d->payload = d->create(); d->payload_made = 1; }
  ts_lexer_start(&d->lexer);
  sd_phase = 2;
  // C passes data=NULL, length=0 when there is no previous external token.
  d->deserialize(d->payload, state_len ? d->stbuf : NULL, state_len);
  sd_phase = 3;
  bool found = d->scan(d->payload, &d->lexer.data, d->mbuf);
  out->did_get_column = d->lexer.did_get_column;
  uint32_t look = 0;
  ts_lexer_finish(&d->lexer, &look);
  out->look_end = look;
  out->found = found;
  if (found) {
    out->sym_index = d->lexer.data.result_symbol;
    out->symbol = d->lang->external_scanner.symbol_map[d->lexer.data.result_symbol];
    out->start = d->lexer.token_start_position.bytes;
    out->srow = d->lexer.token_start_position.extent.row;
    out->scol = d->lexer.token_start_position.extent.column;
    out->end = d->lexer.token_end_position.bytes;
    out->erow = d->lexer.token_end_position.extent.row;
    out->ecol = d->lexer.token_end_position.extent.column;
    sd_phase = 4;
    unsigned n = d->serialize(d->payload, d->lexer.debug_buffer);
    out->state_len = n;
    memcpy(out->state, d->lexer.debug_buffer, n > TREE_SITTER_SERIALIZATION_BUFFER_SIZE ? TREE_SITTER_SERIALIZATION_BUFFER_SIZE : n);
  }
  sd_guard_off(old);
  sd_jmp = NULL;
}

// sd_row_count is one more than the largest external lex state any parse state uses.
static uint32_t sd_row_count(sd_driver *d) {
  uint32_t max = 0;
  for (uint32_t s = 0; s < d->lang->state_count; s++) {
    TSLexerMode m = ts_language_lex_mode_for_state(d->lang, s);
    if (m.external_lex_state > max) max = m.external_lex_state;
  }
  return max + 1;
}
static uint32_t sd_external_count(sd_driver *d) { return d->lang->external_token_count; }
static const bool *sd_states(sd_driver *d) { return d->lang->external_scanner.states; }
static const char *sd_lang_name(sd_driver *d) { return d->lang->name; }
static int sd_has_scanner(sd_driver *d) { return d->create && d->scan && d->serialize && d->deserialize && d->lang->external_scanner.states; }

static void sd_free(sd_driver *d) {
  sd_close_payload(d);
  if (d->lexer_live) ts_lexer_delete(&d->lexer);
  free(d->src);
  if (d->dl) dlclose(d->dl);
  free(d);
}

// Coverage helpers (libgcov in the coverage shared object).
static int sd_gcov_flush(sd_driver *d) {
  if (!d->dl) return 1;
  void (*hook)(void) = (void (*)(void))dlsym(d->dl, "sd_gcov_hook");
  if (!hook) return 2;
  hook();
  return 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// Record is what one scan call returns, from either side.
type Record struct {
	Crash      int // fatal signal inside the scanner (C side only), 0 when none
	CrashPhase int
	Found      bool
	SymIndex   uint32 // external token index the scanner set (C side only)
	Symbol     uint32 // symbol of the language (symbol_map applied on the C side)
	Start, End uint32 // token bytes
	StartRow   uint32
	StartCol   uint32
	EndRow     uint32
	EndCol     uint32
	LookEnd    uint32 // farthest byte the lexer reached plus one (ts_lexer_finish)
	DidGetCol  bool
	State      []byte // serialized state, valid when Found
	StateLen   int64  // the length the scanner reported, signed and unconverted; below 0 or above 1,024 is an overflow
}

// Driver drives the C scanner of one language.
type Driver struct {
	d *C.sd_driver
}

// CrashPhaseName names a crash phase.
func CrashPhaseName(p int) string {
	switch p {
	case 1:
		return "create"
	case 2:
		return "deserialize"
	case 3:
		return "scan"
	case 4:
		return "serialize"
	}
	return "?"
}

// New returns a driver for a loaded TSLanguage (the raw pointer of
// sitter.Language.Inner). When coverSO is not empty, the scanner entry points
// are taken from that shared object (built from the same scanner source with
// --coverage) under the exported name tree_sitter_<name>_external_scanner_*.
func New(lang unsafe.Pointer, coverSO, coverName string) (*Driver, error) {
	d := C.sd_new(lang)
	if C.sd_has_scanner(d) == 0 && coverSO == "" {
		C.sd_free(d)
		return nil, errors.New("the language has no external scanner")
	}
	if coverSO != "" {
		cs, cn := C.CString(coverSO), C.CString(coverName)
		defer C.free(unsafe.Pointer(cs))
		defer C.free(unsafe.Pointer(cn))
		if msg := C.sd_use_so(d, cs, cn); msg != nil {
			err := fmt.Errorf("coverage object %s: %s", coverSO, C.GoString(msg))
			C.sd_free(d)
			return nil, err
		}
	}
	return &Driver{d: d}, nil
}

// LangName is the name the grammar reports (ABI 15), used for the exported symbols.
func (x *Driver) LangName() string {
	p := C.sd_lang_name(x.d)
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

// RowCount is the number of external lex states of the loaded language (rows of C's table, row 0 included).
func (x *Driver) RowCount() int { return int(C.sd_row_count(x.d)) }

// ExternalCount is the number of external tokens.
func (x *Driver) ExternalCount() int { return int(C.sd_external_count(x.d)) }

// StateRow returns row i of C's external-lex-state table.
func (x *Driver) StateRow(i int) []bool {
	n := x.ExternalCount()
	base := unsafe.Pointer(C.sd_states(x.d))
	out := make([]bool, n)
	for j := 0; j < n; j++ {
		out[j] = *(*bool)(unsafe.Add(base, i*n+j))
	}
	return out
}

// SetSource gives the lexer's input (copied into C memory).
func (x *Driver) SetSource(src []byte) {
	if len(src) == 0 {
		C.sd_set_source(x.d, nil, 0)
		return
	}
	C.sd_set_source(x.d, (*C.char)(unsafe.Pointer(&src[0])), C.uint32_t(len(src)))
}

// Call runs the external lex step at byte offset off (row and column of that
// offset), with the valid-symbol mask and the saved state.
func (x *Driver) Call(off int, row, col uint32, mask []bool, state []byte) Record {
	var out C.sd_out
	var mp *C.bool
	if len(mask) > 0 {
		mp = (*C.bool)(unsafe.Pointer(&mask[0]))
	}
	var sp *C.char
	if len(state) > 0 {
		sp = (*C.char)(unsafe.Pointer(&state[0]))
	}
	C.sd_call(x.d, C.uint32_t(off), C.uint32_t(row), C.uint32_t(col), mp, sp, C.uint32_t(len(state)), &out)
	r := Record{
		Crash:      int(out.crash),
		CrashPhase: int(out.phase),
		Found:      out.found != 0,
		SymIndex:   uint32(out.sym_index),
		Symbol:     uint32(out.symbol),
		Start:      uint32(out.start),
		End:        uint32(out.end),
		StartRow:   uint32(out.srow),
		StartCol:   uint32(out.scol),
		EndRow:     uint32(out.erow),
		EndCol:     uint32(out.ecol),
		LookEnd:    uint32(out.look_end),
		DidGetCol:  out.did_get_column != 0,
		StateLen:   int64(out.state_len),
	}
	if r.Found {
		n := uint32(out.state_len)
		if n > 1024 {
			n = 1024
		}
		r.State = C.GoBytes(unsafe.Pointer(&out.state[0]), C.int(n))
	}
	return r
}

// Reset gives the driver a fresh payload, a fresh lexer and a zeroed serialization buffer.
func (x *Driver) Reset() { C.sd_reset_all(x.d) }

// NewParse ends a parse: fresh payload and lexer, the serialization buffer kept.
func (x *Driver) NewParse() { C.sd_new_parse(x.d) }

// SetRange gives the next calls one included range (bytes and points); ClearRange removes it.
func (x *Driver) SetRange(startByte, startRow, startCol, endByte, endRow, endCol uint32) {
	C.sd_set_range(x.d, C.uint32_t(startByte), C.uint32_t(startRow), C.uint32_t(startCol), C.uint32_t(endByte), C.uint32_t(endRow), C.uint32_t(endCol))
}

// ClearRange returns to the default range (the whole input).
func (x *Driver) ClearRange() { C.sd_clear_range(x.d) }

// GcovFlush writes the coverage counters of the coverage object to its .gcda
// files and resets them (0 on success).
func (x *Driver) GcovFlush() int { return int(C.sd_gcov_flush(x.d)) }

// Close destroys the scanner payload and frees the driver.
func (x *Driver) Close() {
	if x.d != nil {
		C.sd_free(x.d)
		x.d = nil
	}
}
