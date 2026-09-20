package grammars

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter"
	grammarruntime "github.com/odvcencio/gotreesitter/grammars/runtime"
)

// cRecoveryGateFleetRow mirrors cmd/crecoverygatefleet's gateRow. Keep both
// declarations in the same field order: json.MarshalIndent encodes struct
// fields in declaration order, and this test compares its own measurement
// against the committed receipt byte-for-byte.
type cRecoveryGateFleetRow struct {
	Grammar              string `json:"grammar"`
	Capable              bool   `json:"capable"`
	Default              bool   `json:"default"`
	Reason               string `json:"reason"`
	ExternalLexStateRows int    `json:"external_lex_state_rows"`
	ExternalLexStatesGen bool   `json:"external_lex_states_sidecar"`
}

type cRecoveryGateFleetReceipt struct {
	Schema   string                  `json:"schema"`
	Grammars []cRecoveryGateFleetRow `json:"grammars"`
}

const cRecoveryGateFleetSchema = "gts-c-recovery-gate-fleet/v1"

// TestCRecoveryGateFleetReceipt is task #71 item 2's fleet-wide receipt: for
// every shipped grammar blob, it pins whether the ported C-recovery
// cost-competition gate is capable, on by default, its diagnostic reason,
// its ExternalLexStates row count, and whether an
// *_external_lex_states_gen.go sidecar exists (grammars/runtime).
//
// generatedCRecoveryDefaultSafe (parser_recover_c.go) recomputes "default"
// from a scanner-bearing grammar's ExternalLexStates table, so a routine
// blob regeneration that restores a previously-empty table can silently
// flip a language's default recovery behavior (pine's 2026-09-21 doxygen
// diagnosis). This test re-measures every blob the same way production's
// embedded loader does (attach the registered scanner, then certify) and
// fails on any row that moved, printing the changed rows, instead of
// letting a regeneration's side effect ship unreviewed.
//
// Regenerate the receipt with:
//
//	go run ./cmd/crecoverygatefleet -out testdata/c_recovery_gate_fleet.json \
//	    grammars/grammar_blobs/*.bin
func TestCRecoveryGateFleetReceipt(t *testing.T) {
	const receiptPath = "../testdata/c_recovery_gate_fleet.json"
	committed, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("read committed receipt: %v", err)
	}

	paths, err := filepath.Glob("grammar_blobs/*.bin")
	if err != nil {
		t.Fatalf("glob grammar blobs: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no grammar blobs found under grammars/grammar_blobs")
	}
	sort.Strings(paths)

	measured := cRecoveryGateFleetReceipt{Schema: cRecoveryGateFleetSchema}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".bin")
		blob, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		lang, err := gotreesitter.LoadLanguage(blob)
		if err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if lang.Name == "" {
			lang.Name = name
		}
		// Attach the registered scanner exactly the way production's
		// embedded loader does, then certify explicitly so a scanner-less
		// language is also measured with the full table diagnostic
		// (AttachLanguageSupport short-circuits before certifying when a
		// language has no ExternalSymbols at all).
		grammarruntime.AttachLanguageSupport(name, lang)
		gotreesitter.CertifyCRecoveryCostCompetition(lang)
		diag := gotreesitter.DiagnoseCRecoveryGate(lang)
		_, sidecarErr := os.Stat(filepath.Join("runtime", name+"_external_lex_states_gen.go"))
		measured.Grammars = append(measured.Grammars, cRecoveryGateFleetRow{
			Grammar:              name,
			Capable:              lang.CRecoveryCostCompetitionCapable,
			Default:              lang.CRecoveryCostCompetitionEnabledByDefault,
			Reason:               diag.Reason,
			ExternalLexStateRows: len(lang.ExternalLexStates),
			ExternalLexStatesGen: sidecarErr == nil,
		})
	}

	encoded, err := json.MarshalIndent(measured, "", "  ")
	if err != nil {
		t.Fatalf("encode measured receipt: %v", err)
	}
	encoded = append(encoded, '\n')

	if bytes.Equal(committed, encoded) {
		return
	}

	reportCRecoveryGateFleetDiff(t, committed, encoded)
	t.Fatalf("fleet C-recovery gate receipt %s is stale; regenerate from the repo root with:\n"+
		"  go run ./cmd/crecoverygatefleet -out testdata/c_recovery_gate_fleet.json grammars/grammar_blobs/*.bin",
		receiptPath)
}

// reportCRecoveryGateFleetDiff prints the rows that moved between the
// committed receipt and the fresh measurement, so a regeneration's cause is
// obvious from the test log instead of a raw byte diff.
func reportCRecoveryGateFleetDiff(t *testing.T, committed, measured []byte) {
	t.Helper()
	var committedReceipt, measuredReceipt cRecoveryGateFleetReceipt
	if err := json.Unmarshal(committed, &committedReceipt); err != nil {
		t.Errorf("parse committed receipt: %v", err)
		return
	}
	if err := json.Unmarshal(measured, &measuredReceipt); err != nil {
		t.Errorf("parse measured receipt: %v", err)
		return
	}
	committedByName := make(map[string]cRecoveryGateFleetRow, len(committedReceipt.Grammars))
	for _, row := range committedReceipt.Grammars {
		committedByName[row.Grammar] = row
	}
	measuredByName := make(map[string]cRecoveryGateFleetRow, len(measuredReceipt.Grammars))
	for _, row := range measuredReceipt.Grammars {
		measuredByName[row.Grammar] = row
	}
	for name, want := range committedByName {
		got, ok := measuredByName[name]
		if !ok {
			t.Errorf("grammar %q dropped from the fleet", name)
			continue
		}
		if got != want {
			t.Errorf("grammar %q changed: committed=%+v measured=%+v", name, want, got)
		}
	}
	for name := range measuredByName {
		if _, ok := committedByName[name]; !ok {
			t.Errorf("grammar %q is new; add it to the receipt", name)
		}
	}
}
