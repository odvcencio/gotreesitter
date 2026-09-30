//go:build cgo && treesitter_c_parity && !gts_incr_census

package cgoharness

import (
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/diag/incrcensus"
)

func reuseCensusObserve(old *gts.Tree, run func() (*gts.Tree, error)) (*gts.Tree, incrcensus.Report, error) {
	tree, err := run()
	return tree, incrcensus.Report{}, err
}
