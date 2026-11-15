// Package certified holds the certificates of internal/ccore: which grammar blobs
// the core may serve by default, for which modes, and the test certification that a
// test program installs. It is a leaf package: it imports nothing of the module.
//
// The root package can read Blobs and Current. Nothing in this package or in a
// production build of the module writes the test certification: the writer is in
// package certifiedtest, which only test files and the build with the tag
// gts_ccore_test_certify import (TestNoProductionImportOfCertifiedtest checks the
// root package; the tagged file is in internal/ccore/bind).
package certified

import "sync/atomic"

// The mode bits of a certificate; internal/ccore/certgen uses the same numbers.
const (
	Fresh   uint8 = 1 << iota // a parse without an old tree
	Recover                   // error recovery
	Scan                      // an external scanner
	Incr                      // a parse with an old tree
	Ranges                    // included ranges
)

// Cert is one certificate of the generated list (gen.go).
type Cert struct {
	Modes   uint8    // bits of Fresh, Recover, Scan, Incr, Ranges
	Scanner string   // %T of the external scanner the blob was checked with ("" for none)
	Checked []string // root-only fields compared with pinned C
	Config  [32]byte // the effective-configuration hash of the blob's loaded language
}

// Test is the test certification: every language with a blob identity and a
// binding is certified for Modes, except a language that sets a root-only field
// that is not in Checked, and the blobs in Excluded.
type Test struct {
	Modes    uint8
	Checked  map[string]bool
	Excluded map[[32]byte]bool // blobs the test run does not certify (known differences from C)
}

var current atomic.Pointer[Test]

// Current returns the test certification, nil in production.
func Current() *Test { return current.Load() }

// Swap installs t (nil removes it) and returns the previous one. It is exported
// for package certifiedtest only; production code does not call it.
func Swap(t *Test) *Test { return current.Swap(t) }
