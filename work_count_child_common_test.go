package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	workCountFixtureIDEnv        = "GTS_WORK_COUNT_FIXTURE"
	workCountSourcePathEnv       = "GTS_WORK_COUNT_SOURCE"
	workCountResultPathEnv       = "GTS_WORK_COUNT_RESULT"
	workCountGoChildSchema       = "gts-work-count-go-child/v3"
	workCountTaggedGoChildSchema = "gts-work-count-go-child/v6"
	workCountAdmissionEngine     = "go-production-glr-untagged"
	workCountTaggedEngine        = "go-production-glr-tagged-diagnostic"
)

type workCountGoChildResult struct {
	Schema            string `json:"schema"`
	Engine            string `json:"engine"`
	Fixture           string `json:"fixture"`
	SourceSHA256      string `json:"source_sha256"`
	SourceBytes       uint32 `json:"source_bytes"`
	GrammarCommit     string `json:"grammar_commit"`
	GrammarBlobSHA256 string `json:"grammar_blob_sha256"`
	DigestFormat      string `json:"digest_format"`
	DeepTreeSHA256    string `json:"deep_tree_sha256"`
	RootEndByte       uint32 `json:"root_end_byte"`
	RootHasError      bool   `json:"root_has_error"`
	MaxStacksSeen     int    `json:"max_stacks_seen"`
	MultiStackIters   int    `json:"multi_stack_iterations"`
	MultiStackTokens  uint64 `json:"multi_stack_tokens"`
}

type workCountChildInput struct {
	fixture benchfixtures.LoadedFixture
	source  []byte
	lang    *gotreesitter.Language
	blobSHA string
}

func loadWorkCountChildInput() (workCountChildInput, error) {
	fixtureID := strings.TrimSpace(os.Getenv(workCountFixtureIDEnv))
	if fixtureID == "" {
		return workCountChildInput{}, fmt.Errorf("%s is empty", workCountFixtureIDEnv)
	}
	sourcePath := os.Getenv(workCountSourcePathEnv)
	if sourcePath == "" {
		return workCountChildInput{}, fmt.Errorf("%s is empty", workCountSourcePathEnv)
	}
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		return workCountChildInput{}, fmt.Errorf("load frozen fixtures: %w", err)
	}
	var fixture benchfixtures.LoadedFixture
	found := false
	for _, candidate := range fixtures {
		if candidate.Fixture.ID == fixtureID {
			fixture = candidate
			found = true
			break
		}
	}
	if !found {
		return workCountChildInput{}, fmt.Errorf("fixture %s is absent", fixtureID)
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return workCountChildInput{}, fmt.Errorf("read source: %w", err)
	}
	if err := fixture.Fixture.VerifySource(source); err != nil {
		return workCountChildInput{}, err
	}
	blob := grammars.BlobByName("go")
	if len(blob) == 0 {
		return workCountChildInput{}, fmt.Errorf("embedded Go grammar blob is empty")
	}
	blobHash := sha256.Sum256(blob)
	blobSHA := hex.EncodeToString(blobHash[:])
	if err := benchfixtures.VerifyGoGrammarIdentity(benchfixtures.GoGrammarCommit, blobSHA); err != nil {
		return workCountChildInput{}, err
	}
	lang := grammars.GoLanguage()
	if lang == nil {
		return workCountChildInput{}, fmt.Errorf("load embedded Go language: nil")
	}
	return workCountChildInput{fixture: fixture, source: source, lang: lang, blobSHA: blobSHA}, nil
}

func authenticateWorkCountTree(input workCountChildInput, tree *gotreesitter.Tree) (workCountGoChildResult, error) {
	if tree == nil || tree.RootNode() == nil {
		return workCountGoChildResult{}, fmt.Errorf("parse returned no root")
	}
	root := tree.RootNode()
	if root.EndByte() != uint32(len(input.source)) || root.HasError() {
		return workCountGoChildResult{}, fmt.Errorf("tree span/error: end=%d bytes=%d error=%v", root.EndByte(), len(input.source), root.HasError())
	}
	inspection, err := benchfixtures.InspectGoTree(root, input.lang)
	if err != nil {
		return workCountGoChildResult{}, fmt.Errorf("deep tree digest: %w", err)
	}
	if err := input.fixture.Fixture.VerifyDeepTreeDigest(inspection.SHA256); err != nil {
		return workCountGoChildResult{}, err
	}
	runtime := tree.ParseRuntime()
	if err := input.fixture.Fixture.VerifyWorkloadIdentity(runtime, inspection.NodeKinds); err != nil {
		return workCountGoChildResult{}, err
	}
	return workCountGoChildResult{
		Schema:            workCountGoChildSchema,
		Fixture:           input.fixture.Fixture.ID,
		SourceSHA256:      input.fixture.Fixture.SHA256,
		SourceBytes:       uint32(len(input.source)),
		GrammarCommit:     benchfixtures.GoGrammarCommit,
		GrammarBlobSHA256: input.blobSHA,
		DigestFormat:      benchfixtures.DeepTreeDigestVersion,
		DeepTreeSHA256:    inspection.SHA256,
		RootEndByte:       root.EndByte(),
		RootHasError:      root.HasError(),
		MaxStacksSeen:     runtime.MaxStacksSeen,
		MultiStackIters:   runtime.MultiStackIterations,
		MultiStackTokens:  runtime.MultiStackTokens,
	}, nil
}

func writeWorkCountChildResult(value any) error {
	resultPath := os.Getenv(workCountResultPathEnv)
	if resultPath == "" {
		return fmt.Errorf("%s is empty", workCountResultPathEnv)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(resultPath, data, 0o600); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}

func TestLoadWorkCountChildInputSelectsEveryCanonicalFixture(t *testing.T) {
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.Fixture.ID, func(t *testing.T) {
			sourcePath := filepath.Join(t.TempDir(), fixture.Fixture.ID+".go")
			if err := os.WriteFile(sourcePath, fixture.Source, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv(workCountFixtureIDEnv, fixture.Fixture.ID)
			t.Setenv(workCountSourcePathEnv, sourcePath)
			input, err := loadWorkCountChildInput()
			if err != nil {
				t.Fatal(err)
			}
			if input.fixture.Fixture.ID != fixture.Fixture.ID || input.blobSHA != benchfixtures.GoGrammarBlobSHA256 {
				t.Fatalf("selected fixture/grammar=%q/%s want=%q/%s", input.fixture.Fixture.ID, input.blobSHA, fixture.Fixture.ID, benchfixtures.GoGrammarBlobSHA256)
			}
		})
	}
}

func runWorkCountProductionRouteRegression(t *testing.T, child func(*testing.T), schema, engine string) {
	t.Helper()
	fixtures, err := benchfixtures.LoadGoFullParseFixtures()
	if err != nil {
		t.Fatal(err)
	}
	var fixture benchfixtures.LoadedFixture
	for _, candidate := range fixtures {
		if candidate.Fixture.ID == "rewrite" {
			fixture = candidate
			break
		}
	}
	if fixture.Fixture.ID == "" {
		t.Fatal("rewrite fixture is absent")
	}
	root := t.TempDir()
	sourcePath := filepath.Join(root, "rewrite.go")
	resultPath := filepath.Join(root, "result.json")
	if err := os.WriteFile(sourcePath, fixture.Source, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(workCountFixtureIDEnv, fixture.Fixture.ID)
	t.Setenv(workCountSourcePathEnv, sourcePath)
	t.Setenv(workCountResultPathEnv, resultPath)
	previousDefault := gotreesitter.AdmissionCandidateRouteDefault()
	gotreesitter.SetAdmissionCandidateRouteDefault(true)
	t.Cleanup(func() { gotreesitter.SetAdmissionCandidateRouteDefault(previousDefault) })
	child(t)
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result workCountGoChildResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Schema != schema || result.Engine != engine || result.DeepTreeSHA256 != fixture.Fixture.DeepTreeSHA256 {
		t.Fatalf("production child identity=%q/%q tree=%s want=%q/%q/%s", result.Schema, result.Engine, result.DeepTreeSHA256, schema, engine, fixture.Fixture.DeepTreeSHA256)
	}
}
