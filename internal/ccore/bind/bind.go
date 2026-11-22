// Package bind is where the root package's adapters meet the code that drives
// internal/ccore for a language.
//
// The root package builds ccore tables and the external-scanner hook for a
// *Language (ccore_bind.go, ccore_scanner.go) and registers them here in an
// init function. It exports nothing for this. The parse route and the
// cgo_harness equality tests (through internal/ccore/bind/bindtest) ask this
// package for a configured parser for a language passed as any. The root
// package imports this package in every build, so it imports internal/ccore
// and the standard library only: no test support, and it never imports the
// root package.
package bind

import (
	"errors"
	"fmt"
	"sync"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// Adapter is what the root package registers.
type Adapter struct {
	// Config returns the core configuration of a *Language (passed as any), or
	// an error that says why the core cannot serve it.
	Config func(lang any) (ccore.Config, error)
	// ConfigHash returns the canonical configuration hash of a *Language.
	ConfigHash func(lang any) ([32]byte, error)
	// CheckTables compares the core's tables of a *Language with the root
	// package's own lookups for every state and symbol. It returns the number
	// of pairs checked and the number that differ.
	CheckTables func(lang any) (checked, differ int, err error)
	// ViewsOn switches lazy public nodes and returns the previous setting.
	ViewsOn func(bool) bool
	// ViewCounters reports views and conversions since startup.
	ViewCounters func() (int64, int64, map[string]int64)
	// RouteCounters is optional: the route's decline counts by reason, and the
	// number of fresh parses and reparses the core served. Test programs read it.
	RouteCounters func() (declines map[string]int64, servedFresh, servedIncr int64)
	// IncrFallbacks returns fresh-reparse counts by reason.
	IncrFallbacks func() map[string]int64
	// RouteNanos is optional: nanoseconds the route spent in the core parse, in
	// the conversion to the public tree, in the core's tree edit and in the check
	// of the recorded edits against the bytes, since the
	// last RouteNanos(true) (reset true zeroes them).
	RouteNanos func(reset bool) (parse, convert, edit, check int64)
	// RouteLimits is optional: the soft memory budget and the hard ceiling a
	// parser of the language would apply to a request of sourceLen bytes, and
	// whether the route would install its progress hook for it. A timing test
	// reads it to prove which arm it measures.
	RouteLimits func(lang any, sourceLen int, softOff bool) (budget, ceiling int64, hooked bool)
	// RootOnlyFields is optional: the root-only fields a language sets.
	RootOnlyFields func(lang any) []string
}

// ErrNoAdapter is returned when nothing has registered: the root package is
// not linked into the program.
var ErrNoAdapter = errors.New("bind: no adapter registered (the root package is not linked)")

var (
	mu      sync.Mutex
	adapter *Adapter
)

// Register installs the adapter. It panics when called twice or with a nil
// function, because two adapters would make the answer depend on init order.
func Register(a Adapter) {
	if a.Config == nil || a.ConfigHash == nil || a.CheckTables == nil {
		panic("bind: Register needs Config, ConfigHash and CheckTables")
	}
	mu.Lock()
	defer mu.Unlock()
	if adapter != nil {
		panic("bind: Register called twice")
	}
	adapter = &a
}

func registered() (*Adapter, error) {
	mu.Lock()
	defer mu.Unlock()
	if adapter == nil {
		return nil, ErrNoAdapter
	}
	return adapter, nil
}

// ViewsOn changes the optional lazy-tree switch.
func ViewsOn(on bool) bool {
	a, err := registered()
	if err != nil || a.ViewsOn == nil {
		return false
	}
	return a.ViewsOn(on)
}

// ViewCounters reports lazy-tree counters when the adapter provides them.
func ViewCounters() (int64, int64, map[string]int64) {
	a, err := registered()
	if err != nil || a.ViewCounters == nil {
		return 0, 0, nil
	}
	return a.ViewCounters()
}

// Config returns the core configuration of a language. The configuration's
// scanner holds one payload and one lexer, so two parsers built from one Config
// value would share scanner state: use one parser per Config value. New builds
// a fresh configuration for every parser and is the call to use.
func Config(lang any) (ccore.Config, error) {
	a, err := registered()
	if err != nil {
		return ccore.Config{}, err
	}
	cfg, err := a.Config(lang)
	if err != nil {
		return ccore.Config{}, err
	}
	return cfg, nil
}

// ConfigHash returns the canonical configuration hash of a language.
func ConfigHash(lang any) ([32]byte, error) {
	a, err := registered()
	if err != nil {
		return [32]byte{}, err
	}
	return a.ConfigHash(lang)
}

// New returns a parser for a language: the adapter's configuration, checked by
// ccore.NewParser. Each call gets its own external-scanner hook, so parsers do
// not share scanner state.
func New(lang any) (*ccore.Parser, error) {
	cfg, err := Config(lang)
	if err != nil {
		return nil, err
	}
	p, err := ccore.NewParser(cfg)
	if err != nil {
		return nil, fmt.Errorf("bind: %w", err)
	}
	return p, nil
}

// RouteLimits reads the limits of a request (see Adapter.RouteLimits); softOff
// asks for a parser whose soft budget is switched off.
func RouteLimits(lang any, sourceLen int, softOff bool) (budget, ceiling int64, hooked bool, err error) {
	a, err := registered()
	if err != nil {
		return 0, 0, false, err
	}
	if a.RouteLimits == nil {
		return 0, 0, false, errors.New("bind: the adapter has no RouteLimits")
	}
	budget, ceiling, hooked = a.RouteLimits(lang, sourceLen, softOff)
	return budget, ceiling, hooked, nil
}

// RouteCounters reads the route's counters (see Adapter.RouteCounters).
func RouteCounters() (declines map[string]int64, servedFresh, servedIncr int64) {
	a, err := registered()
	if err != nil || a.RouteCounters == nil {
		return nil, 0, 0
	}
	return a.RouteCounters()
}

// IncrFallbacks reads the reasons for fresh reparses.
func IncrFallbacks() map[string]int64 {
	a, err := registered()
	if err != nil || a.IncrFallbacks == nil {
		return nil
	}
	return a.IncrFallbacks()
}

// RouteNanos reads (and with reset zeroes) the route's time counters (see Adapter.RouteNanos).
func RouteNanos(reset bool) (parse, convert, edit, check int64) {
	a, err := registered()
	if err != nil || a.RouteNanos == nil {
		return 0, 0, 0, 0
	}
	return a.RouteNanos(reset)
}

// RootOnlyFields names the root-only fields a language sets (see Adapter.RootOnlyFields).
func RootOnlyFields(lang any) []string {
	a, err := registered()
	if err != nil || a.RootOnlyFields == nil {
		return nil
	}
	return a.RootOnlyFields(lang)
}

// CheckTables compares the core's tables of a language with the root
// package's lookups. It returns the number of (state, symbol) pairs checked and
// the number that differ.
func CheckTables(lang any) (checked, differ int, err error) {
	a, err := registered()
	if err != nil {
		return 0, 0, err
	}
	return a.CheckTables(lang)
}

// NewFuncScanner constructs a scanner whose callbacks cannot be replaced by its
// holders. Both callbacks must be non-nil.
func NewFuncScanner(scan func(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool, close func()) ccore.ExtScanner {
	return funcScanner{state: &funcScannerState{scan: scan, close: close}}
}

type funcScanner struct {
	state *funcScannerState
}

type funcScannerState struct {
	scan         func(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool
	scanIncluded func(src []byte, cursor ccore.ScannerCursor, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool
	close        func()
}

func (f funcScanner) Scan(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool {
	return f.state.scan(src, pos, row, col, prev, valid, buf, out)
}
func (f funcScanner) Close() { f.state.close() }

// NewRangedFuncScanner constructs a scanner with an included-range hook.
func NewRangedFuncScanner(
	scan func(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool,
	scanIncluded func(src []byte, cursor ccore.ScannerCursor, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool,
	close func(),
) ccore.RangedExtScanner {
	// Keep function fields behind the state pointer so each hook has a
	// comparable identity, including when it is held as an ExtScanner.
	state := &funcScannerState{scan: scan, scanIncluded: scanIncluded, close: close}
	return rangedFuncScanner{funcScanner{state: state}}
}

type rangedFuncScanner struct{ funcScanner }

func (f rangedFuncScanner) ScanIncluded(src []byte, cursor ccore.ScannerCursor, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool {
	return f.state.scanIncluded(src, cursor, prev, valid, buf, out)
}
