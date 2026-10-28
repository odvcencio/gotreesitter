//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestSQLC26ajGeneratedCreateDomain(t *testing.T) {
	runSQLC26Witness(t, sqlC26Witness{
		label:             "CREATE DOMAIN",
		source:            "CREATE DOMAIN test;",
		sourceSHA256:      "94c5d360b7205e2bd6e84fa28efc2cb3ee2cbc89aa6b759dd3349e578dd133c8",
		firstDivergence:   `root[0][1]: ChildCount go=1 c=0 (goType="CREATE_DOMAIN" cType="CREATE_DOMAIN" goBytes=[7-13] cBytes=[7-13])`,
		generatedDigest:   "f08d628fa30d83cf92352dbfcab4885b7422ac0fadde9c756e747e6c116dc044",
		generatedChildren: 2,
		generatedHasError: false,
		checkedInDigest:   "e72fb9fb57180d5db5be9b649969c7e722d20e1ea4040075260258ee293ce630",
	})
}
