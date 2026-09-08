// Package benchfixtures provides immutable, content-addressed benchmark inputs.
//
// The fixtures are snapshots rather than reads from the working tree so benchmark
// results do not silently change when the source files used to select them move.
package benchfixtures

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"fmt"
	"io"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

const (
	// GoFixtureOriginRevision is the gotreesitter revision from which the human
	// source snapshots were selected.
	GoFixtureOriginRevision = "a5df0aa5b3c5b20ce12bb21250bd166b9f47bd68"
	// GoGrammarCommit is the tree-sitter-go revision pinned by languages.lock.
	GoGrammarCommit = "2346a3ab1bb3857b48b29d779a1ef9799a248cd7"
	// GoGrammarBlobSHA256 identifies the exact gotreesitter Go grammar blob.
	GoGrammarBlobSHA256 = "a0287eb2011072c1fac90a7c1cdf21f8d7e923a7df8bfc9a287588fd2e9b1d58"
)

var goFixtureRequiredNodeKinds = [...]string{
	"import_declaration",
	"comment",
	"method_declaration",
	"selector_expression",
	"parameter_declaration",
	"composite_literal",
	"index_expression",
	"call_expression",
}

var goSuiteRequiredNodeKinds = [...]string{
	"type_arguments",
	"generic_type",
	"qualified_type",
}

var goSuiteRequiredAnyNodeKinds = [...]string{
	"expression_switch_statement",
	"select_statement",
}

// WorkloadIdentity defines the minimum GLR regime a fixture must retain to
// qualify for the canonical full-parse matrix.
type WorkloadIdentity struct {
	MinMaxStacksSeen        int
	MinMultiStackIterations int
	MinMultiStackTokens     uint64
}

// Fixture describes one frozen human-authored Go full-parse input.
type Fixture struct {
	ID                  string
	Origin              string
	OriginRevision      string
	UncompressedBytes   int
	SHA256              string
	CompressedSHA256    string
	DeepTreeSHA256      string
	WorkloadIdentity    WorkloadIdentity
	compressedAssetPath string
}

// LoadedFixture pairs verified metadata with source bytes decompressed outside
// the timed benchmark region.
type LoadedFixture struct {
	Fixture Fixture
	Source  []byte
}

//go:embed testdata/*.gz
var fixtureFS embed.FS

var goFullParseFixtures = [...]Fixture{
	{
		ID:                  "query_compile",
		Origin:              "query_compile.go",
		OriginRevision:      GoFixtureOriginRevision,
		UncompressedBytes:   20168,
		SHA256:              "b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894",
		CompressedSHA256:    "8811a477187606ab2a0be003924e2750c8d10cb5e6ef29e8f0f673c5bd5ccfa4",
		DeepTreeSHA256:      "ecc090a83a4343a1c7c2afbad63277f5b4d60c42d8d94a2af2a9b16e46f2ccb5",
		WorkloadIdentity:    WorkloadIdentity{MinMaxStacksSeen: 8, MinMultiStackIterations: 500, MinMultiStackTokens: 400},
		compressedAssetPath: "testdata/query_compile.go.gz",
	},
	{
		ID:                  "rewrite",
		Origin:              "rewrite.go",
		OriginRevision:      GoFixtureOriginRevision,
		UncompressedBytes:   5116,
		SHA256:              "74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097",
		CompressedSHA256:    "e8c92713adda73b661c1e41dae09d4f76f1447b3f8ddd5b3348b67ce655ee243",
		DeepTreeSHA256:      "b3f9814b65763642d4eac58b9065018048ea13e6f10d56afb28a0479bf5a68a1",
		WorkloadIdentity:    WorkloadIdentity{MinMaxStacksSeen: 8, MinMultiStackIterations: 500, MinMultiStackTokens: 400},
		compressedAssetPath: "testdata/rewrite.go.gz",
	},
	{
		ID:                  "language",
		Origin:              "language.go",
		OriginRevision:      GoFixtureOriginRevision,
		UncompressedBytes:   41387,
		SHA256:              "009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a",
		CompressedSHA256:    "a064d45748fd555209971728d9740510c2ca7449870b54acb861909afea2c8c9",
		DeepTreeSHA256:      "583df223904fe414c33bba3b474c6557ecdb20e7f47e304b9a09bfcc2da44539",
		WorkloadIdentity:    WorkloadIdentity{MinMaxStacksSeen: 8, MinMultiStackIterations: 500, MinMultiStackTokens: 400},
		compressedAssetPath: "testdata/language.go.gz",
	},
	{
		ID:                  "grammargen_lr",
		Origin:              "grammargen/lr.go",
		OriginRevision:      GoFixtureOriginRevision,
		UncompressedBytes:   235626,
		SHA256:              "a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2",
		CompressedSHA256:    "7d64368d4dbffca1b3cc472ff3397871c23e082efb1a2bdba5f9670564cc7b22",
		DeepTreeSHA256:      "1472cfd9a014d4034dbc1456afd12c282630ef787c3543cf0cecb73619883ad2",
		WorkloadIdentity:    WorkloadIdentity{MinMaxStacksSeen: 8, MinMultiStackIterations: 500, MinMultiStackTokens: 400},
		compressedAssetPath: "testdata/grammargen_lr.go.gz",
	},
}

// GoFullParseFixtures returns the frozen fixture metadata in stable order.
func GoFullParseFixtures() []Fixture {
	fixtures := make([]Fixture, len(goFullParseFixtures))
	copy(fixtures, goFullParseFixtures[:])
	return fixtures
}

// LoadGoFullParseFixtures loads and authenticates every frozen fixture.
func LoadGoFullParseFixtures() ([]LoadedFixture, error) {
	loaded := make([]LoadedFixture, 0, len(goFullParseFixtures))
	for _, fixture := range goFullParseFixtures {
		source, err := fixture.Load()
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, LoadedFixture{Fixture: fixture, Source: source})
	}
	return loaded, nil
}

// Load decompresses and authenticates a frozen fixture. Both the compressed
// asset and uncompressed source must match their recorded identities.
func (f Fixture) Load() ([]byte, error) {
	if f.ID == "" || f.Origin == "" || f.OriginRevision == "" || f.compressedAssetPath == "" || f.UncompressedBytes <= 0 || f.SHA256 == "" || f.CompressedSHA256 == "" || f.DeepTreeSHA256 == "" {
		return nil, fmt.Errorf("benchmark fixture metadata is incomplete for %q", f.ID)
	}
	if f.WorkloadIdentity.MinMaxStacksSeen <= 1 || f.WorkloadIdentity.MinMultiStackIterations <= 0 || f.WorkloadIdentity.MinMultiStackTokens == 0 {
		return nil, fmt.Errorf("benchmark fixture workload identity is incomplete for %q", f.ID)
	}
	compressed, err := fixtureFS.ReadFile(f.compressedAssetPath)
	if err != nil {
		return nil, fmt.Errorf("load benchmark fixture %s: %w", f.ID, err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(compressed)); got != f.CompressedSHA256 {
		return nil, fmt.Errorf("benchmark fixture %s compressed sha256=%s want=%s", f.ID, got, f.CompressedSHA256)
	}

	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("open benchmark fixture %s: %w", f.ID, err)
	}
	source, readErr := io.ReadAll(io.LimitReader(reader, int64(f.UncompressedBytes)+1))
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("decompress benchmark fixture %s: %w", f.ID, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close benchmark fixture %s: %w", f.ID, closeErr)
	}
	if err := f.VerifySource(source); err != nil {
		return nil, err
	}
	return source, nil
}

// VerifySource fails closed unless source is the exact frozen input.
func (f Fixture) VerifySource(source []byte) error {
	if len(source) != f.UncompressedBytes {
		return fmt.Errorf("benchmark fixture %s bytes=%d want=%d", f.ID, len(source), f.UncompressedBytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != f.SHA256 {
		return fmt.Errorf("benchmark fixture %s sha256=%s want=%s", f.ID, got, f.SHA256)
	}
	return nil
}

// VerifyDeepTreeDigest fails closed unless a parser produced the frozen deep
// structural identity for this fixture.
func (f Fixture) VerifyDeepTreeDigest(digest string) error {
	if digest != f.DeepTreeSHA256 {
		return fmt.Errorf("benchmark fixture %s deep tree sha256=%s want=%s", f.ID, digest, f.DeepTreeSHA256)
	}
	return nil
}

// VerifyRuntimeIdentity fails closed when the parser no longer enters the GLR
// regime that qualified this fixture for headline use.
func (f Fixture) VerifyRuntimeIdentity(runtime gotreesitter.ParseRuntime) error {
	expect := f.WorkloadIdentity
	if runtime.MaxStacksSeen < expect.MinMaxStacksSeen {
		return fmt.Errorf("benchmark fixture %s max stacks=%d want >=%d", f.ID, runtime.MaxStacksSeen, expect.MinMaxStacksSeen)
	}
	if runtime.MultiStackIterations < expect.MinMultiStackIterations {
		return fmt.Errorf("benchmark fixture %s multi-stack iterations=%d want >=%d", f.ID, runtime.MultiStackIterations, expect.MinMultiStackIterations)
	}
	if runtime.MultiStackTokens < expect.MinMultiStackTokens {
		return fmt.Errorf("benchmark fixture %s multi-stack tokens=%d want >=%d", f.ID, runtime.MultiStackTokens, expect.MinMultiStackTokens)
	}
	return nil
}

// VerifyNodeKindCoverage fails closed when a fixture loses the real-code syntax
// features that distinguish it from the historical generated control.
func (f Fixture) VerifyNodeKindCoverage(coverage NodeKindCoverage) error {
	for _, kind := range goFixtureRequiredNodeKinds {
		if coverage[kind] == 0 {
			return fmt.Errorf("benchmark fixture %s has no %s node", f.ID, kind)
		}
	}
	return nil
}

// VerifyWorkloadIdentity validates both the parser regime and untimed syntax
// coverage for one fixture.
func (f Fixture) VerifyWorkloadIdentity(runtime gotreesitter.ParseRuntime, coverage NodeKindCoverage) error {
	if err := f.VerifyRuntimeIdentity(runtime); err != nil {
		return err
	}
	return f.VerifyNodeKindCoverage(coverage)
}

// VerifyGoFullParseSuiteNodeKindCoverage validates syntax features that only
// need to be represented once across the complete canonical fixture matrix.
func VerifyGoFullParseSuiteNodeKindCoverage(coverage NodeKindCoverage) error {
	for _, kind := range goSuiteRequiredNodeKinds {
		if coverage[kind] == 0 {
			return fmt.Errorf("canonical Go benchmark suite has no %s node", kind)
		}
	}
	for _, kind := range goSuiteRequiredAnyNodeKinds {
		if coverage[kind] > 0 {
			return nil
		}
	}
	return fmt.Errorf("canonical Go benchmark suite has neither expression_switch_statement nor select_statement nodes")
}

// VerifyGoGrammarIdentity fails closed unless both Go parser implementations
// are tied to the same pinned grammar revision and exact gotreesitter blob.
// The C parity harness calls this before it performs deep tree parity and starts
// either timer.
func VerifyGoGrammarIdentity(commit, blobSHA256 string) error {
	if commit != GoGrammarCommit {
		return fmt.Errorf("Go grammar commit=%q want=%q", commit, GoGrammarCommit)
	}
	if blobSHA256 != GoGrammarBlobSHA256 {
		return fmt.Errorf("Go grammar blob sha256=%q want=%q", blobSHA256, GoGrammarBlobSHA256)
	}
	return nil
}
