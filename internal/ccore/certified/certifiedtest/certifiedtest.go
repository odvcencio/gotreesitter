// Package certifiedtest installs the test certification. Only test files and the
// build with the tag gts_ccore_test_certify (internal/ccore/bind) import it; a
// production build of the module does not link it.
package certifiedtest

import "github.com/odvcencio/gotreesitter/internal/ccore/certified"

// Install certifies every language the core can bind for modes, except a language
// that sets a root-only field not in checked and the blobs in exclude. The returned
// function restores the previous certification.
func Install(modes uint8, checked []string, exclude [][32]byte) (restore func()) {
	return InstallProviders(modes, checked, exclude, nil)
}

// InstallProviders is Install that also grants the listed scan providers (by
// identity) to every blob it certifies. The grant lasts until restore runs.
func InstallProviders(modes uint8, checked []string, exclude [][32]byte, providers []string) (restore func()) {
	t := &certified.Test{Modes: modes, Checked: map[string]bool{}, Excluded: map[[32]byte]bool{}, Providers: append([]string(nil), providers...)}
	for _, h := range exclude {
		t.Excluded[h] = true
	}
	for _, f := range checked {
		t.Checked[f] = true
	}
	prev := certified.Swap(t)
	return func() { certified.Swap(prev) }
}
