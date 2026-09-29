//go:build linux && cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"encoding/json"
	"fmt"
	"testing"
)

// C logging is kept out of every timed region. This uses the same frontier
// accounting as the R7 cliff detector and emits one row per fresh fixture.
func TestEngineCeilingCWork(t *testing.T) {
	language, err := COracleLanguage(ceilingLanguage(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range ceilingInputs(t) {
		if input.mode != "fresh" {
			continue
		}
		row := measureCliffC(t, language, input.source[0])
		encoded, err := json.Marshal(map[string]any{"language": input.language, "size": input.size, "bytes": len(input.source[0]), "runtime_commit": COracleRuntimeCommit, "c": row})
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("CEILING_C_WORK %s\n", encoded)
	}
}
