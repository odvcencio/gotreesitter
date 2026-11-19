//go:build gts_ccore_test_certify

package bind

import "github.com/odvcencio/gotreesitter/internal/ccore/certified/certifiedtest"

// This file exists only in a build with the tag gts_ccore_test_certify, which
// the test programs outside the root package set (the equality script, the
// roottest suites that run with the route on). No build without the tag has
// a function that can add a certificate.

// CertifyForTests certifies every language the core can serve for the given
// mode bits (internal/ccore/certgen) so that a test program outside the root
// package can drive the public parse methods through the core. checked names
// the root-only language fields that the test has compared with pinned C;
// exclude lists blob hashes the test does not certify (known differences). The
// returned function undoes it.
func CertifyForTests(modes uint8, checked []string, exclude [][32]byte) (restore func(), err error) {
	if _, err := registered(); err != nil {
		return func() {}, err
	}
	return certifiedtest.Install(modes, checked, exclude), nil
}
