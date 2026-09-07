package cgoharness

import (
	"os"

	"github.com/odvcencio/gotreesitter"
)

// The shipping default routes fresh full parses to the production engine. These
// tests opt in to the compact candidate route when GTS_ADMISSION_CANDIDATE is
// unset, so route receipts and counter assertions keep their established
// coverage. An explicit GTS_ADMISSION_CANDIDATE=0 still pins a run to the
// production route. See admission_default_test.go in the root package.
func init() {
	if _, set := os.LookupEnv("GTS_ADMISSION_CANDIDATE"); !set {
		gotreesitter.SetAdmissionCandidateRouteDefault(true)
	}
}
