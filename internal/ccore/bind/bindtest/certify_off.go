//go:build !gts_ccore_test_certify

package bindtest

import "errors"

// CertifyForTests reports that the build has no certification hook: run the
// test program with -tags gts_ccore_test_certify.
func CertifyForTests(modes uint8, checked []string, exclude [][32]byte) (func(), error) {
	return func() {}, errors.New("bindtest: build with -tags gts_ccore_test_certify to certify languages for a test run")
}
