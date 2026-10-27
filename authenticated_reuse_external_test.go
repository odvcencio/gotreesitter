//go:build gts_parsercorephase0 || gts_workcount || gts_diag

package gotreesitter_test

import (
	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"testing"
)

func TestAuthenticatedNativeReuseWitness(t *testing.T) {
	gts.RunAuthenticatedNativeReuseWitness(t, grammars.GoLanguage())
}
