// Command crecoverygatefleet is the C-recovery cost-competition gate's
// fleet receipt generator (task #71, item 2).
//
// generatedCRecoveryDefaultSafe (parser_recover_c.go) decides, per language,
// whether the ported C-recovery cost-competition gate turns on by default. A
// grammar blob regeneration can silently flip that decision the moment a
// scanner-bearing grammar's ExternalLexStates table goes from empty to
// populated (pine's 2026-09-21 doxygen diagnosis). This command measures the
// live gate state — capable, default, reason, ExternalLexStates row count,
// and sidecar presence — for every shipped grammar blob, so the committed
// receipt at testdata/c_recovery_gate_fleet.json pins today's fleet and a
// regeneration that moves a row shows up as a diff instead of a silent
// behavior change.
//
// Regenerate the receipt with:
//
//	go run ./cmd/crecoverygatefleet -out testdata/c_recovery_gate_fleet.json \
//	    grammars/grammar_blobs/*.bin
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gotreesitter "github.com/odvcencio/gotreesitter"
	_ "github.com/odvcencio/gotreesitter/grammars"
	grammarruntime "github.com/odvcencio/gotreesitter/grammars/runtime"
)

// gateRow is one committed fleet row. Field names are stable, snake_case
// JSON keys so the receipt reads the same from Go or any other tool.
type gateRow struct {
	Grammar              string `json:"grammar"`
	Capable              bool   `json:"capable"`
	Default              bool   `json:"default"`
	Reason               string `json:"reason"`
	ExternalLexStateRows int    `json:"external_lex_state_rows"`
	ExternalLexStatesGen bool   `json:"external_lex_states_sidecar"`
}

type receipt struct {
	Schema   string    `json:"schema"`
	Grammars []gateRow `json:"grammars"`
}

const receiptSchema = "gts-c-recovery-gate-fleet/v1"

func main() {
	out := flag.String("out", "", "write the receipt to this path (default: stdout)")
	sidecarDir := flag.String("sidecar-dir", "grammars/runtime", "directory containing *_external_lex_states_gen.go sidecars")
	flag.Parse()

	paths := flag.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "crecoverygatefleet: give at least one grammar blob path")
		os.Exit(2)
	}
	sort.Strings(paths)

	measured := receipt{Schema: receiptSchema}
	for _, path := range paths {
		row, err := measure(path, *sidecarDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "crecoverygatefleet: %s: %v\n", path, err)
			os.Exit(1)
		}
		measured.Grammars = append(measured.Grammars, row)
	}

	encoded, err := json.MarshalIndent(measured, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "crecoverygatefleet: encode receipt: %v\n", err)
		os.Exit(1)
	}
	encoded = append(encoded, '\n')

	if *out == "" {
		os.Stdout.Write(encoded)
		return
	}
	if err := os.WriteFile(*out, encoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "crecoverygatefleet: write receipt: %v\n", err)
		os.Exit(1)
	}
}

// measure decodes one shipped grammar blob through
// grammarruntime.DecodeAndCertifyLanguageBlob — the same production decode
// and certification path the embedded loader uses (table repairs, then
// scanner and ExternalLexStates attachment, which certifies the gate) — and
// reports the resulting gate state. It does not reassemble the steps
// itself, so a future decode-time repair cannot silently drift this
// measurement away from the language a parser actually uses.
func measure(path, sidecarDir string) (gateRow, error) {
	name := strings.TrimSuffix(filepath.Base(path), ".bin")
	blob, err := os.ReadFile(path)
	if err != nil {
		return gateRow{}, err
	}
	lang, err := grammarruntime.DecodeAndCertifyLanguageBlob(name, blob)
	if err != nil {
		return gateRow{}, fmt.Errorf("decode blob: %w", err)
	}
	diag := gotreesitter.DiagnoseCRecoveryGate(lang)

	_, sidecarErr := os.Stat(filepath.Join(sidecarDir, name+"_external_lex_states_gen.go"))

	return gateRow{
		Grammar:              name,
		Capable:              lang.CRecoveryCostCompetitionCapable,
		Default:              lang.CRecoveryCostCompetitionEnabledByDefault,
		Reason:               diag.Reason,
		ExternalLexStateRows: len(lang.ExternalLexStates),
		ExternalLexStatesGen: sidecarErr == nil,
	}, nil
}
