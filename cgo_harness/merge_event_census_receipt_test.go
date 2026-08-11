package cgoharness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestMergeEventCensusReceiptIsProvenanced checks the checked-in M0 evidence.
// It does not run a parser or claim a real-corpus measurement.
func TestMergeEventCensusReceiptIsProvenanced(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate the merge-event receipt test")
	}
	path := filepath.Join(filepath.Dir(sourceFile), "work_count", "merge_event_census_receipt_v1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read merge-event receipt: %v", err)
	}
	var receipt mergeEventCensusReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatalf("decode merge-event receipt: %v", err)
	}

	if receipt.Schema != "gts-merge-census-receipt/v1" {
		t.Fatalf("schema = %q", receipt.Schema)
	}
	if receipt.Spec != "spec.merge-time-election.v1#M0" {
		t.Fatalf("spec = %q", receipt.Spec)
	}
	if receipt.Result.StageM0 != "open_until_real_corpus_receipt" {
		t.Fatalf("stage M0 status = %q", receipt.Result.StageM0)
	}
	if receipt.Result.Instrument != "complete" || receipt.Result.Constructed != "complete" {
		t.Fatalf("instrument and constructed baseline must be complete: %+v", receipt.Result)
	}
	if receipt.Result.FooIntA != "complete" || receipt.Result.CompactFoldCount != "not_exposed" {
		t.Fatalf("M0 discriminator or compact-fold status drifted: %+v", receipt.Result)
	}
	if receipt.Result.RealCorpus != "not_measured" || receipt.RealCorpus.Measured {
		t.Fatalf("receipt must preserve the real-corpus gap: result=%+v corpus=%+v", receipt.Result, receipt.RealCorpus)
	}
	if receipt.RealCorpus.StatusInArchivedGate != "empty" || receipt.RealCorpus.SourceCountInArchivedGate != 0 {
		t.Fatalf("archived gate real-corpus state = %+v", receipt.RealCorpus)
	}

	if receipt.Provenance.HeadSHA != "f13d192e7e768639821c93a40a3f83b477ed66de" {
		t.Fatalf("head SHA = %q", receipt.Provenance.HeadSHA)
	}
	if receipt.Provenance.M0Commit != "596b9bd0eaec3303fc78cf75c5ddcf6d80bd1cac" {
		t.Fatalf("M0 commit = %q", receipt.Provenance.M0Commit)
	}
	if receipt.Provenance.Reference.Commit != "f5afe475deb7c0bae6407fb776c76824f717bb61" {
		t.Fatalf("reference commit = %q", receipt.Provenance.Reference.Commit)
	}
	for name, values := range map[string][2]string{
		"runtime tree": {receipt.Provenance.Reference.RuntimeTreeSHA256, "3b22a44edd412230a555dd3a78c14d45aafadd0033aa8f5d2516d016ab21dbb2"},
		"patch":        {receipt.Provenance.Instrumentation.PatchSHA256, "4b7cadd46ebbbb73b8883d6bd634f9bf6bd5b3001333dae9bfcbedd5fbea4bf1"},
		"driver":       {receipt.Provenance.Instrumentation.DriverSHA256, "7f93a00213c5d69d1bc9e8eca7da2237b2f7dd8780b51cdc3b5b8df0ae6aec37"},
	} {
		if len(values[0]) != 64 || values[0] != values[1] {
			t.Fatalf("%s SHA-256 = %q, want %q", name, values[0], values[1])
		}
	}

	if receipt.Constructed.SourceCount != 104 || receipt.Constructed.Totals.CMergeSuccesses != 191 || receipt.Constructed.Totals.GoMergeSuccesses != 15 {
		t.Fatalf("constructed baseline drifted: sources=%d C=%d Go=%d", receipt.Constructed.SourceCount, receipt.Constructed.Totals.CMergeSuccesses, receipt.Constructed.Totals.GoMergeSuccesses)
	}
	if receipt.Constructed.Totals.SourcesWhereGoOverMerges != 0 {
		t.Fatalf("M0 over-merge source count = %d", receipt.Constructed.Totals.SourcesWhereGoOverMerges)
	}
	if len(receipt.Constructed.Languages) != 5 {
		t.Fatalf("constructed language count = %d, want 5", len(receipt.Constructed.Languages))
	}
	if len(receipt.FooIntA.Witnesses) != 3 {
		t.Fatalf("Foo[int](a) witness count = %d, want 3", len(receipt.FooIntA.Witnesses))
	}
	for _, witness := range receipt.FooIntA.Witnesses {
		if witness.Verdict != "packed_elected_at_pop" {
			t.Fatalf("Foo[int](a) witness %q verdict = %q", witness.Name, witness.Verdict)
		}
	}
}

type mergeEventCensusReceipt struct {
	Schema string `json:"schema"`
	Spec   string `json:"spec"`
	Result struct {
		Instrument       string `json:"instrument"`
		Constructed      string `json:"constructed_census"`
		FooIntA          string `json:"foo_int_a_discriminator"`
		RealCorpus       string `json:"real_corpus"`
		CompactFoldCount string `json:"compact_fold_count"`
		StageM0          string `json:"stage_m0"`
	} `json:"result"`
	Provenance struct {
		HeadSHA   string `json:"head_sha"`
		M0Commit  string `json:"m0_commit"`
		Reference struct {
			Commit            string `json:"commit"`
			RuntimeTreeSHA256 string `json:"runtime_tree_sha256"`
		} `json:"reference"`
		Instrumentation struct {
			PatchSHA256  string `json:"patch_sha256"`
			DriverSHA256 string `json:"driver_sha256"`
		} `json:"instrumentation"`
	} `json:"provenance"`
	Constructed struct {
		SourceCount int                        `json:"source_count"`
		Totals      mergeEventCensusTotals     `json:"totals"`
		Languages   []mergeEventCensusLanguage `json:"languages"`
	} `json:"constructed"`
	RealCorpus struct {
		StatusInArchivedGate      string `json:"status_in_archived_gate"`
		SourceCountInArchivedGate int    `json:"source_count_in_archived_gate"`
		Measured                  bool   `json:"measured"`
	} `json:"real_corpus"`
	FooIntA struct {
		Witnesses []mergeEventCensusWitness `json:"witnesses"`
	} `json:"foo_int_a_discriminator"`
}

type mergeEventCensusTotals struct {
	CMergeSuccesses          uint64 `json:"c_merge_successes"`
	GoMergeSuccesses         uint64 `json:"go_merge_successes"`
	SourcesWhereGoOverMerges int    `json:"sources_where_go_over_merges"`
}

type mergeEventCensusLanguage struct {
	Name string `json:"name"`
}

type mergeEventCensusWitness struct {
	Name    string `json:"name"`
	Verdict string `json:"verdict"`
}
