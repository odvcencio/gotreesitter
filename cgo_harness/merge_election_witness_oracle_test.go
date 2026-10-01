//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// TestMergeElectionWitnessOracle authenticates portable C expectations for
// unresolved merge-election cases. It does not assert that Go matches them.
// Run one grammar per process with GTS_MERGE_ELECTION_LANG.
func TestMergeElectionWitnessOracle(t *testing.T) {
	name := os.Getenv("GTS_MERGE_ELECTION_LANG")
	if name == "" {
		t.Skip("set GTS_MERGE_ELECTION_LANG to one witness grammar")
	}
	data, err := os.ReadFile("testdata/merge_election_witnesses.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version   string `json:"runtime_version"`
		Commit    string `json:"runtime_commit"`
		Witnesses []struct {
			Name      string `json:"name"`
			Language  string `json:"language"`
			Source    string `json:"source"`
			SourceSHA string `json:"source_sha256"`
			Digest    string `json:"c_digest"`
		} `json:"witnesses"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != COracleRuntimeVersion || fixture.Commit != COracleRuntimeCommit {
		t.Fatal("witness runtime differs from the locked C runtime")
	}
	count := 0
	for _, witness := range fixture.Witnesses {
		if witness.Language != name {
			continue
		}
		count++
		t.Run(witness.Name, func(t *testing.T) {
			source := []byte(witness.Source)
			if fmt.Sprintf("%x", sha256.Sum256(source)) != witness.SourceSHA {
				t.Fatal("witness source digest changed")
			}
			language, err := COracleLanguage(witness.Language)
			if err != nil {
				t.Fatal(err)
			}
			parser := sitter.NewParser()
			defer parser.Close()
			if err := parser.SetLanguage(language); err != nil {
				t.Fatal(err)
			}
			tree := parser.Parse(source, nil)
			if tree == nil {
				t.Fatal("C parse returned nil")
			}
			defer tree.Close()
			if tree.RootNode().HasError() || tree.RootNode().EndByte() != uint(len(source)) {
				t.Fatal("C witness has an error or incomplete coverage")
			}
			digest, err := COracleDeepDigest(tree)
			if err != nil {
				t.Fatal(err)
			}
			if digest != witness.Digest {
				t.Fatalf("C digest=%s, want %s", digest, witness.Digest)
			}
		})
	}
	if count == 0 {
		t.Fatalf("no witnesses for %s", name)
	}
}
