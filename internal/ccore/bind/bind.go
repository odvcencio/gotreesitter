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
	// CheckTables compares the core's tables of a *Language with the root
	// package's own lookups for every state and symbol. It returns the number
	// of pairs checked and the number that differ.
	CheckTables func(lang any) (checked, differ int, err error)
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
	if a.Config == nil || a.CheckTables == nil {
		panic("bind: Register needs Config and CheckTables")
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

// FuncScanner is a ccore.ExtScanner made of two functions. The root adapter
// uses it so that its scanner type has no exported method (the root's API
// snapshots list exported methods of every type).
type FuncScanner struct {
	ScanFn  func(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool
	CloseFn func()
}

// Scan calls ScanFn.
func (f *FuncScanner) Scan(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool {
	return f.ScanFn(src, pos, row, col, prev, valid, buf, out)
}

// Close calls CloseFn.
func (f *FuncScanner) Close() { f.CloseFn() }
