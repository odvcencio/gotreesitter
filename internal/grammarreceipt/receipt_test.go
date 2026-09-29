package grammarreceipt

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReceiptJSONRoundTripV1(t *testing.T) {
	want := Receipt{
		Schema:             SchemaV1,
		GeneratedAt:        time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		Grammar:            GrammarIdentity{Name: "go", BlobSHA256: strings.Repeat("a", 64)},
		GotreesitterCommit: strings.Repeat("b", 40),
		COracle: COracleIdentity{
			RuntimeVersion: "0.25.1",
			RuntimeCommit:  strings.Repeat("c", 40),
			Grammar: CGrammarArtifact{
				Repository: "https://github.com/tree-sitter/tree-sitter-go",
				Commit:     strings.Repeat("d", 40),
				SHA256:     strings.Repeat("e", 64),
			},
			BindingModule:  "github.com/tree-sitter/go-tree-sitter",
			BindingVersion: "v0.25.0",
			BindingCommit:  strings.Repeat("f", 40),
		},
		Cohort: "1a",
		Route: CompactRoute{
			Status: RouteAccepted, SampledFiles: 1, AcceptedFiles: 1,
			Probe: "grammars.ParseSmokeSample", ProbeSHA256: strings.Repeat("5", 64),
		},
		Corpus: CorpusIdentity{
			LockPath:       "corpus_sources.lock",
			LockSHA256:     strings.Repeat("1", 64),
			ManifestSHA256: strings.Repeat("2", 64),
			Selection:      CorpusSelection{Order: "largest", MaxFiles: 4, MaxFileBytes: 1 << 10},
			Files:          []CorpusFile{{Path: "src/example.go", Revision: strings.Repeat("3", 40), Bytes: 12, SHA256: strings.Repeat("4", 64)}},
		},
		FreshParity: ParityResult{
			Status: ResultPass, Cases: 1, Matched: 1,
			Steps: []StepResult{{Step: 1, InvariantPass: true, Pass: true}},
		},
		IncrementalParity: ParityResult{Status: ResultPass, Cases: 24, Matched: 24},
		InvariantGate: InvariantResult{
			Status: ResultPass, SessionSteps: 24, StepsPassed: 24,
			NoEditReparseAllocsPerRun: 0, NoEditReparsePass: true,
		},
	}

	if err := want.Validate(); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Receipt
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("round-tripped receipt rejected: %v", err)
	}
	if got.Schema != SchemaV1 || got.Grammar.Name != want.Grammar.Name || got.COracle.RuntimeCommit != want.COracle.RuntimeCommit || got.Route.Status != RouteAccepted || !got.FreshParity.Steps[0].InvariantPass {
		t.Fatalf("round trip lost schema identity: got %#v", got)
	}
}

func TestValidateRequiresReasonForDecline(t *testing.T) {
	receipt := minimalValidReceipt()
	receipt.Route = CompactRoute{Status: RouteDeclined, Probe: "smoke", ProbeSHA256: strings.Repeat("5", 64)}
	if err := receipt.Validate(); err == nil || !strings.Contains(err.Error(), "decline_reason") {
		t.Fatalf("decline validation error = %v, want decline_reason error", err)
	}
}

func TestValidateRejectsUnknownVersionAndCohort(t *testing.T) {
	for name, mutate := range map[string]func(*Receipt){
		"version": func(r *Receipt) { r.Schema = "gts-grammar-receipt/v2" },
		"cohort":  func(r *Receipt) { r.Cohort = "5" },
	} {
		t.Run(name, func(t *testing.T) {
			receipt := minimalValidReceipt()
			mutate(&receipt)
			if err := receipt.Validate(); err == nil {
				t.Fatal("invalid receipt accepted")
			}
		})
	}
}

func TestTimeoutExecutionRequiresExplicitLimitAndStatuses(t *testing.T) {
	receipt := minimalValidReceipt()
	receipt.Execution = &ExecutionResult{Status: ResultTimeout, TimeLimit: "90m", Details: "wall timeout"}
	receipt.FreshParity.Status = ResultTimeout
	receipt.IncrementalParity.Status = ResultTimeout
	receipt.InvariantGate.Status = ResultTimeout
	if err := receipt.Validate(); err != nil {
		t.Fatalf("valid timeout receipt rejected: %v", err)
	}

	receipt.Execution.TimeLimit = ""
	if err := receipt.Validate(); err == nil || !strings.Contains(err.Error(), "time_limit") {
		t.Fatalf("timeout without a time limit error = %v, want time_limit error", err)
	}
}

func minimalValidReceipt() Receipt {
	return Receipt{
		Schema: SchemaV1, GeneratedAt: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC),
		Grammar:            GrammarIdentity{Name: "lean", BlobSHA256: strings.Repeat("a", 64)},
		GotreesitterCommit: strings.Repeat("b", 40),
		COracle: COracleIdentity{
			RuntimeVersion: "0.25.1", RuntimeCommit: strings.Repeat("c", 40),
			Grammar: CGrammarArtifact{Repository: "https://example.invalid/grammar", Commit: strings.Repeat("d", 40), SHA256: strings.Repeat("e", 64)},
		},
		Cohort: "Lean 4", Route: CompactRoute{Status: RouteForest, ForestFiles: 1, Probe: "smoke", ProbeSHA256: strings.Repeat("5", 64)},
		Corpus:            CorpusIdentity{LockPath: "lock", LockSHA256: strings.Repeat("1", 64), ManifestSHA256: strings.Repeat("2", 64)},
		FreshParity:       ParityResult{Status: ResultUnavailable},
		IncrementalParity: ParityResult{Status: ResultNotRun},
		InvariantGate:     InvariantResult{Status: ResultPass, NoEditReparsePass: true},
	}
}
