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
	Forest                    // fresh native forest epochs
	ErrReuse                  // reuse of an old tree that has an error, and an incremental result that has one
)

// ShortcutPolicy controls the token-stable incremental shortcut.
type ShortcutPolicy uint8

const (
	ShortcutDefault     ShortcutPolicy = iota // the historical policy for this set of modes
	ShortcutOrdinary                          // use ordinary native incremental reuse
	ShortcutTokenStable                       // the certificate covers the token-stable shortcut
)

// Cert is one certificate of the generated list (gen.go).
type Cert struct {
	Shortcut ShortcutPolicy
	Modes    uint8    // bits of Fresh, Recover, Scan, Incr, Ranges
	Scanner  string   // %T of the external scanner the blob was checked with ("" for none)
	Checked  []string // root-only fields compared with pinned C
	Config   [32]byte // the effective-configuration hash of the blob's loaded language
	// Providers lists the scan providers whose answers were compared with pinned C
	// for this blob, by identity. Empty means none.
	Providers []string
}

// HasProvider reports whether the certificate lists the provider.
func (c Cert) HasProvider(id string) bool {
	for _, p := range c.Providers {
		if p == id {
			return true
		}
	}
	return false
}

// blobEntry keeps generated certificates in data. The number of entries
// does not change the code emitted for package initialization.
type blobEntry struct {
	blob [32]byte
	cert Cert
}

func makeBlobMap(entries []blobEntry) map[[32]byte]Cert {
	blobs := make(map[[32]byte]Cert, len(entries))
	for _, entry := range entries {
		blobs[entry.blob] = entry.cert
	}
	return blobs
}

// Test is the test certification: every language with a blob identity and a
// binding is certified for Modes, except a language that sets a root-only field
// that is not in Checked, and the blobs in Excluded.
type Test struct {
	Modes    uint8
	Checked  map[string]bool
	Excluded map[[32]byte]bool // blobs the test run does not certify (known differences from C)
	// Providers lists the scan providers the test grants to every blob it
	// certifies. Empty grants none.
	Providers []string
}

// HasProvider reports whether the test certification grants the provider.
func (t *Test) HasProvider(id string) bool {
	for _, p := range t.Providers {
		if p == id {
			return true
		}
	}
	return false
}

var current atomic.Pointer[Test]

// everActive is set once a certificate can have applied in this process: the
// generated list is not empty, or a test certification was installed. It is
// never cleared.
var everActive atomic.Bool

// everTest is set once a test certification was installed in this process. It
// is never cleared.
var everTest atomic.Bool

func init() {
	if len(Blobs) != 0 {
		everActive.Store(true)
	}
}

// EverActive reports whether any certificate has existed in this process. While
// it is false no parse was served by the core and no tree has a core link, so
// the hot paths of the root package ask this one flag and nothing else.
func EverActive() bool { return everActive.Load() }

// EverTest reports whether a test certification was ever installed in this
// process. It is false in production. While it is false the core serves only
// the blobs of the generated list, so a language whose blob is not in that list
// has no tree the core built.
func EverTest() bool { return everTest.Load() }

// Current returns the test certification, nil in production.
func Current() *Test { return current.Load() }

// Swap installs t (nil removes it) and returns the previous one. It is exported
// for package certifiedtest only; production code does not call it.
func Swap(t *Test) *Test {
	if t != nil {
		everActive.Store(true)
		everTest.Store(true)
	}
	return current.Swap(t)
}
