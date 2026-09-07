package gotreesitter

import (
	"os"
	"testing"
)

// The shipping default routes fresh full parses to the production engine; see
// admissionCandidateEnvEnabled. The package tests opt in to the compact
// candidate route when GTS_ADMISSION_CANDIDATE is unset, so the certification,
// parity, and counter suites keep their established coverage and an explicit
// GTS_ADMISSION_CANDIDATE=0 still pins a test run to the production route.
func init() {
	if _, set := os.LookupEnv("GTS_ADMISSION_CANDIDATE"); !set {
		SetAdmissionCandidateRouteDefault(true)
	}
}

// TestPackageTestsOptIntoCompactRoute documents the test-binary opt-in above.
// TestAdmissionSwitchEnvVarContract locks the shipping default separately.
func TestPackageTestsOptIntoCompactRoute(t *testing.T) {
	if _, set := os.LookupEnv("GTS_ADMISSION_CANDIDATE"); set {
		t.Skip("GTS_ADMISSION_CANDIDATE is set; the environment owns the default")
	}
	if !AdmissionCandidateRouteDefault() {
		t.Fatal("package tests must opt in to the compact candidate route when GTS_ADMISSION_CANDIDATE is unset")
	}
}
