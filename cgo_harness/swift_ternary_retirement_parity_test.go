//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

const (
	swiftTernaryRetirementManifestDigest = "49f57837686d0ea7e070cd08e14ab05dc1b7c128a540ae5aa6c155435a6e18e9"
	swiftTernaryPositiveControlCommit    = "685ef7f885aa697e8a842de56de414d2a25cf0d8"
	swiftTernaryProducerFixCommit        = "71180718521aa6cf53fa4122a50998a7a2ef8020"
)

type swiftTernaryRetirementManifest struct {
	Schema string                       `json:"schema"`
	Cases  []swiftTernaryRetirementCase `json:"cases"`
}

type swiftTernaryRetirementCase struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
	Source string `json:"source"`
}

func TestSwiftTernaryRetirementMatchesLockedC(t *testing.T) {
	manifest := loadSwiftTernaryRetirementManifest(t)
	for _, test := range manifest.Cases {
		t.Run(test.Name, func(t *testing.T) {
			source := []byte(test.Source)
			cLang, err := ParityCLanguage("swift")
			if err != nil {
				t.Fatalf("load locked Swift C parser: %v", err)
			}
			cParser := sitter.NewParser()
			t.Cleanup(cParser.Close)
			if err := cParser.SetLanguage(cLang); err != nil {
				t.Fatalf("set locked Swift C language: %v", err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("locked Swift C parser returned no tree")
			}
			t.Cleanup(cTree.Close)

			goTree, goLang, err := parseWithGo(
				parityCase{name: "swift", source: test.Source},
				source,
				nil,
			)
			if err != nil {
				t.Fatalf("parse Swift with Go: %v", err)
			}
			t.Cleanup(func() { releaseGoTree(goTree) })
			assertSwiftConditionTreeExact(t, goTree, goLang, cTree)
		})
	}
}

func loadSwiftTernaryRetirementManifest(t *testing.T) swiftTernaryRetirementManifest {
	t.Helper()
	path := filepath.Join("..", "testdata", "swift_ternary_retirement_cases_v1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if got := fmt.Sprintf("%x", sum); got != swiftTernaryRetirementManifestDigest {
		t.Fatalf("manifest digest = %s, want %s", got, swiftTernaryRetirementManifestDigest)
	}
	var manifest swiftTernaryRetirementManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Schema != "swift-ternary-retirement-cases-v1" || len(manifest.Cases) != 16 {
		t.Fatalf("manifest schema/cases = %q/%d", manifest.Schema, len(manifest.Cases))
	}
	requireSwiftTernaryManifestIdentity(t, manifest)
	return manifest
}

func requireSwiftTernaryManifestIdentity(t *testing.T, manifest swiftTernaryRetirementManifest) {
	t.Helper()
	names := make(map[string]bool, len(manifest.Cases))
	sources := make(map[string]bool, len(manifest.Cases))
	origins := make(map[string]int, 2)
	for _, test := range manifest.Cases {
		if test.Name == "" || names[test.Name] {
			t.Fatalf("manifest has an empty or duplicate name %q", test.Name)
		}
		if test.Source == "" || sources[test.Source] {
			t.Fatalf("manifest has an empty or duplicate source for %q", test.Name)
		}
		names[test.Name] = true
		sources[test.Source] = true
		origins[test.Origin]++
	}
	if len(origins) != 2 || origins[swiftTernaryPositiveControlCommit] != 11 || origins[swiftTernaryProducerFixCommit] != 5 {
		t.Fatalf("manifest origin counts = %+v, want 11 positive controls and 5 producer controls", origins)
	}
}
