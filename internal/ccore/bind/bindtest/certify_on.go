//go:build gts_ccore_test_certify

package bindtest

import "github.com/odvcencio/gotreesitter/internal/ccore/bind"

// CertifyForTests certifies every bound language for the test run (see
// bind.CertifyForTests). It is available only with the build tag
// gts_ccore_test_certify.
func CertifyForTests(modes uint8, checked []string, exclude [][32]byte) (func(), error) {
	return bind.CertifyForTests(modes, checked, exclude)
}

// CertifyForTestsProviders is CertifyForTests with scan providers granted to
// every certified language.
func CertifyForTestsProviders(modes uint8, checked []string, exclude [][32]byte, providers []string) (func(), error) {
	return bind.CertifyForTestsProviders(modes, checked, exclude, providers)
}
