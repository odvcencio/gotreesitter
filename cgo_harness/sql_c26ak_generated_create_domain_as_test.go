//go:build cgo && treesitter_c_parity

package cgoharness

import "testing"

func TestSQLC26akGeneratedCreateDomainAs(t *testing.T) {
	runSQLC26Witness(t, sqlC26Witness{
		label:             "CREATE DOMAIN AS",
		source:            "CREATE DOMAIN test AS text;",
		sourceSHA256:      "0cad93f1b70ffa192008df866587c05b206ee404cedd5a0025f542c39c6a504b",
		firstDivergence:   `root[0][1]: ChildCount go=1 c=0 (goType="CREATE_DOMAIN" cType="CREATE_DOMAIN" goBytes=[7-13] cBytes=[7-13])`,
		generatedDigest:   "67366d68529459613040eb60c5eef47b371fd3091dbe3283c82d7bcff287ea9c",
		generatedChildren: 2,
		generatedHasError: false,
		checkedInDigest:   "d0daa4279f00eb8c7278992cff6a870c98bf3deee2292b28f997adbe2f434916",
	})
}
