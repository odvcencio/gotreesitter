//go:build cgo && treesitter_c_parity && gts_merge_census

package cgoharness

import (
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

// Tuple and pair assignments previously reached head link-union three times
// apiece. C elects their convergent children without any head merge; the
// trailing singleton covers the same reduction with incomplete raw children.
func TestLegacyMergeElectionConvergentAssignments(t *testing.T) {
	oracle := mergeCensusOracleForTest(t)
	sources := []a3CertificationSweepSource{
		{Name: "triple", Source: []byte("x, y, z = 1, 2, 3\nxyz = x, y, z\n")},
		{Name: "pair", Source: []byte("a = 1\nb = 2\npair = a, b\n")},
		{Name: "single", Source: []byte("a = 1\nsingle = a,\n")},
	}
	cRows, err := mergeCensusRunC(oracle, "python", sources)
	if err != nil {
		t.Fatal(err)
	}
	for i, source := range sources {
		t.Run(source.Name, func(t *testing.T) {
			row := runMergeEventCensusRow("python", grammars.PythonLanguage(), source.Name, source.Source, cRows[i])
			mergeCensusLogRow(t, row)
			if row.GoParseError != "" {
				t.Fatal(row.GoParseError)
			}
			if row.Go.Successes != row.C.MergeSuccesses {
				t.Fatalf("head merges: Go=%d C=%d", row.Go.Successes, row.C.MergeSuccesses)
			}
		})
	}
}
