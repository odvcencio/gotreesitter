//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

const (
	compactT3WitnessMinerSchema            = "compact-t3-witness-miner-v2"
	compactT3WitnessMinerAlgorithmVersion  = 2
	compactT3WitnessMinerContractSHA256    = "6bd2d3eb0de902c923e00ee9aad34f7e43a66f1227e58fc4f00d33fc6ba77150"
	compactT3WitnessMinerSelectionContract = "production_exact_c_compact_divergent_v1"
)

// Keep this alphabet ordered. Its order is part of the mutation contract.
var compactT3WitnessMinerAlphabet = []byte{'^', '<', '>', '/', '"', '\'', '=', ' ', '\n', '?', '!', '&', ';', ':'}

type compactT3WitnessMinerMutation struct {
	Kind           string `json:"kind"`
	Start          int    `json:"start"`
	OldEnd         int    `json:"old_end"`
	ReplacementHex string `json:"replacement_hex"`
}

type compactT3WitnessMinerCandidate struct {
	Mutation compactT3WitnessMinerMutation `json:"mutation"`
	Source   []byte                        `json:"-"`
	SHA256   string                        `json:"source_sha256"`
}

type compactT3WitnessMinerOracle struct {
	Language              string `json:"language"`
	Contract              string `json:"contract"`
	BindingCommit         string `json:"binding_commit"`
	RuntimeCommit         string `json:"runtime_commit"`
	GrammarRepository     string `json:"grammar_repository"`
	GrammarCommit         string `json:"grammar_commit"`
	CompilerPath          string `json:"compiler_path"`
	CompilerVersion       string `json:"compiler_version"`
	GrammarCompileFlags   string `json:"grammar_compile_flags"`
	GrammarArtifactSHA256 string `json:"grammar_artifact_sha256"`
}

type compactT3WitnessMinerFinding struct {
	ID                         string                        `json:"id"`
	DifferenceClass            string                        `json:"difference_class"`
	Language                   string                        `json:"language"`
	SeedID                     string                        `json:"seed_id"`
	Mutation                   compactT3WitnessMinerMutation `json:"mutation"`
	SourceUTF8                 string                        `json:"source_utf8"`
	SourceSHA256               string                        `json:"source_sha256"`
	CHasError                  bool                          `json:"c_has_error"`
	ProductionHasError         bool                          `json:"production_has_error"`
	CompactHasError            bool                          `json:"compact_has_error"`
	ProductionCDivergenceCount int                           `json:"production_c_divergence_count"`
	CompactCDivergenceCount    int                           `json:"compact_c_divergence_count"`
}

type compactT3WitnessMinerReport struct {
	Schema                              string                         `json:"schema"`
	AlgorithmVersion                    int                            `json:"algorithm_version"`
	ParityScope                         string                         `json:"parity_scope"`
	SelectionContract                   string                         `json:"selection_contract"`
	SourceManifestSHA256                string                         `json:"source_manifest_sha256"`
	MutationContractSHA                 string                         `json:"mutation_contract_sha256"`
	MaxMutationsPerSeed                 int                            `json:"max_mutations_per_seed"`
	SeedIDs                             []string                       `json:"seed_ids"`
	Languages                           []string                       `json:"languages"`
	GeneratedCandidates                 int                            `json:"generated_candidates"`
	SampledCandidates                   int                            `json:"sampled_candidates"`
	UniqueCandidates                    int                            `json:"unique_candidates"`
	ProductionOracleExactCandidates     int                            `json:"production_oracle_exact_candidates"`
	ProductionOracleDivergentCandidates int                            `json:"production_oracle_divergent_candidates"`
	Findings                            []compactT3WitnessMinerFinding `json:"findings"`
	Oracles                             []compactT3WitnessMinerOracle  `json:"oracles"`
}

func compactT3WitnessMinerEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("GTS_COMPACT_T3_WITNESS_MINER"))
	return raw == "1" || strings.EqualFold(raw, "true")
}

func compactT3WitnessMinerCSV(name string) map[string]struct{} {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	values := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values[part] = struct{}{}
		}
	}
	return values
}

func compactT3WitnessMinerMaxMutations(t *testing.T) int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("GTS_COMPACT_T3_WITNESS_MINER_MAX_MUTATIONS"))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		t.Fatalf("GTS_COMPACT_T3_WITNESS_MINER_MAX_MUTATIONS=%q is not a non-negative integer", raw)
	}
	return value
}

func compactT3WitnessMinerMutations(source []byte) []compactT3WitnessMinerCandidate {
	candidates := make([]compactT3WitnessMinerCandidate, 0, (len(source)+1)*len(compactT3WitnessMinerAlphabet)*2)
	seen := make(map[string]struct{})
	appendCandidate := func(kind string, start, oldEnd int, replacement []byte) {
		mutated := make([]byte, 0, len(source)-(oldEnd-start)+len(replacement))
		mutated = append(mutated, source[:start]...)
		mutated = append(mutated, replacement...)
		mutated = append(mutated, source[oldEnd:]...)
		if bytes.Equal(mutated, source) || !utf8.Valid(mutated) {
			return
		}
		sum := sha256.Sum256(mutated)
		digest := hex.EncodeToString(sum[:])
		if _, ok := seen[digest]; ok {
			return
		}
		seen[digest] = struct{}{}
		candidates = append(candidates, compactT3WitnessMinerCandidate{
			Mutation: compactT3WitnessMinerMutation{
				Kind:           kind,
				Start:          start,
				OldEnd:         oldEnd,
				ReplacementHex: hex.EncodeToString(replacement),
			},
			Source: mutated,
			SHA256: digest,
		})
	}

	for offset := 0; offset <= len(source); offset++ {
		for _, replacement := range compactT3WitnessMinerAlphabet {
			appendCandidate("insert", offset, offset, []byte{replacement})
		}
		if offset == len(source) {
			continue
		}
		appendCandidate("delete", offset, offset+1, nil)
		for _, replacement := range compactT3WitnessMinerAlphabet {
			appendCandidate("replace", offset, offset+1, []byte{replacement})
		}
	}
	return candidates
}

func compactT3WitnessMinerSample(candidates []compactT3WitnessMinerCandidate, max int) []compactT3WitnessMinerCandidate {
	if max <= 0 || len(candidates) <= max {
		return candidates
	}
	if max == 1 {
		return candidates[:1]
	}
	selected := make([]compactT3WitnessMinerCandidate, 0, max)
	for i := 0; i < max; i++ {
		index := i * (len(candidates) - 1) / (max - 1)
		selected = append(selected, candidates[index])
	}
	return selected
}

func compactT3WitnessMinerContractBytes() []byte {
	candidates := compactT3WitnessMinerMutations([]byte("ab"))
	contract := make([]struct {
		Mutation compactT3WitnessMinerMutation `json:"mutation"`
		SHA256   string                        `json:"source_sha256"`
	}, 0, len(candidates))
	for _, candidate := range candidates {
		contract = append(contract, struct {
			Mutation compactT3WitnessMinerMutation `json:"mutation"`
			SHA256   string                        `json:"source_sha256"`
		}{Mutation: candidate.Mutation, SHA256: candidate.SHA256})
	}
	raw, err := json.Marshal(contract)
	if err != nil {
		panic(err)
	}
	return raw
}

func compactT3WitnessMinerContractSHA() string {
	sum := sha256.Sum256(compactT3WitnessMinerContractBytes())
	return hex.EncodeToString(sum[:])
}

func assertCompactT3WitnessMinerMutationContract(t *testing.T) {
	t.Helper()
	candidates := compactT3WitnessMinerMutations([]byte("ab"))
	if got, want := len(candidates), 72; got != want {
		t.Fatalf("compact T3 witness miner contract candidate count=%d, want %d", got, want)
	}
	if got := compactT3WitnessMinerContractSHA(); got != compactT3WitnessMinerContractSHA256 {
		t.Fatalf("compact T3 witness miner contract SHA-256=%s, want %s", got, compactT3WitnessMinerContractSHA256)
	}
}

func compactT3WitnessMinerDifferenceClass(
	productionDivergences, compactDivergences int,
	cHasError, compactHasError bool,
) (string, bool) {
	if productionDivergences != 0 || compactDivergences == 0 {
		return "", false
	}
	if cHasError != compactHasError {
		return "root_has_error", true
	}
	return "full_structural", true
}

func assertCompactT3WitnessMinerSelectionContract(t *testing.T) {
	t.Helper()
	tests := []struct {
		name                  string
		productionDivergences int
		compactDivergences    int
		cHasError             bool
		compactHasError       bool
		wantClass             string
		wantFinding           bool
	}{
		{name: "exact_all_routes"},
		{name: "production_diverges", productionDivergences: 1, compactDivergences: 1},
		{name: "root_error", compactDivergences: 1, cHasError: true, wantClass: "root_has_error", wantFinding: true},
		{name: "full_structural", compactDivergences: 1, wantClass: "full_structural", wantFinding: true},
	}
	for _, test := range tests {
		gotClass, gotFinding := compactT3WitnessMinerDifferenceClass(
			test.productionDivergences,
			test.compactDivergences,
			test.cHasError,
			test.compactHasError,
		)
		if gotClass != test.wantClass || gotFinding != test.wantFinding {
			t.Errorf("%s classification=(%q,%t), want (%q,%t)", test.name, gotClass, gotFinding, test.wantClass, test.wantFinding)
		}
	}
}

func compactT3WitnessMinerSeeds(manifest compactT3WitnessManifest, languages, seedIDs map[string]struct{}) []compactT3Witness {
	seeds := make([]compactT3Witness, 0, len(manifest.Witnesses))
	for _, witness := range manifest.Witnesses {
		if languages != nil {
			if _, ok := languages[witness.Language]; !ok {
				continue
			}
		}
		if seedIDs != nil {
			if _, ok := seedIDs[witness.ID]; !ok {
				continue
			}
		}
		seeds = append(seeds, witness)
	}
	sort.Slice(seeds, func(i, j int) bool {
		if seeds[i].Language != seeds[j].Language {
			return seeds[i].Language < seeds[j].Language
		}
		return seeds[i].ID < seeds[j].ID
	})
	return seeds
}

func compactT3WitnessMinerOracleIdentity(language string, identity COracleBuildIdentity) compactT3WitnessMinerOracle {
	return compactT3WitnessMinerOracle{
		Language:              language,
		Contract:              identity.Contract,
		BindingCommit:         identity.BindingCommit,
		RuntimeCommit:         identity.RuntimeCommit,
		GrammarRepository:     identity.GrammarRepo,
		GrammarCommit:         identity.GrammarCommit,
		CompilerPath:          identity.CompilerPath,
		CompilerVersion:       identity.CompilerVersion,
		GrammarCompileFlags:   identity.GrammarCompileFlags,
		GrammarArtifactSHA256: identity.GrammarArtifactSHA256,
	}
}

func compactT3WitnessMinerParseGo(t *testing.T, parser *gotreesitter.Parser, source []byte) *gotreesitter.Tree {
	t.Helper()
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("parse Go candidate: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("Go candidate parse returned no root")
	}
	return tree
}

func TestCompactT3WitnessMiner(t *testing.T) {
	if !compactT3WitnessMinerEnabled() {
		t.Skip("set GTS_COMPACT_T3_WITNESS_MINER=1 to enable deterministic witness mining")
	}
	assertCompactT3WitnessMinerMutationContract(t)

	manifestRaw, err := os.ReadFile(compactT3WitnessManifestPath)
	if err != nil {
		t.Fatalf("read witness manifest: %v", err)
	}
	manifestSum := sha256.Sum256(manifestRaw)
	manifest := loadCompactT3WitnessManifest(t)
	seeds := compactT3WitnessMinerSeeds(
		manifest,
		compactT3WitnessMinerCSV("GTS_COMPACT_T3_WITNESS_MINER_LANGS"),
		compactT3WitnessMinerCSV("GTS_COMPACT_T3_WITNESS_MINER_SEEDS"),
	)
	if len(seeds) == 0 {
		t.Fatal("witness miner selected no seeds")
	}

	report := compactT3WitnessMinerReport{
		Schema:               compactT3WitnessMinerSchema,
		AlgorithmVersion:     compactT3WitnessMinerAlgorithmVersion,
		ParityScope:          compactT3WitnessParityScope,
		SelectionContract:    compactT3WitnessMinerSelectionContract,
		SourceManifestSHA256: hex.EncodeToString(manifestSum[:]),
		MutationContractSHA:  compactT3WitnessMinerContractSHA256,
		Findings:             []compactT3WitnessMinerFinding{},
	}
	maxMutations := compactT3WitnessMinerMaxMutations(t)
	report.MaxMutationsPerSeed = maxMutations
	seenCandidates := make(map[string]struct{})
	seenLanguages := make(map[string]struct{})

	for _, seed := range seeds {
		report.SeedIDs = append(report.SeedIDs, seed.ID)
		if _, ok := seenLanguages[seed.Language]; !ok {
			report.Languages = append(report.Languages, seed.Language)
			seenLanguages[seed.Language] = struct{}{}
		}
	}
	sort.Strings(report.Languages)

	for _, language := range report.Languages {
		cLanguage, err := ParityCLanguage(language)
		if err != nil {
			t.Fatalf("load C oracle for %s: %v", language, err)
		}
		identity, err := COracleIdentity(language)
		if err != nil {
			t.Fatalf("read C oracle identity for %s: %v", language, err)
		}
		report.Oracles = append(report.Oracles, compactT3WitnessMinerOracleIdentity(language, identity))
		goLanguage, ok := compactT3GoLanguage(language)
		if !ok {
			t.Fatalf("witness miner does not support language %q", language)
		}

		cParser := sitter.NewParser()
		if err := cParser.SetLanguage(cLanguage); err != nil {
			cParser.Close()
			t.Fatalf("set C language %s: %v", language, err)
		}
		productionParser := gotreesitter.NewParser(goLanguage)
		productionParser.SetAdmissionCandidateRoute(false)
		compactParser := gotreesitter.NewParser(goLanguage)
		compactParser.SetAdmissionCandidateRoute(true)

		for _, seed := range seeds {
			if seed.Language != language {
				continue
			}
			candidates := compactT3WitnessMinerMutations([]byte(seed.SourceUTF8))
			report.GeneratedCandidates += len(candidates)
			candidates = compactT3WitnessMinerSample(candidates, maxMutations)
			report.SampledCandidates += len(candidates)
			for _, candidate := range candidates {
				candidateKey := language + "\x00" + candidate.SHA256
				if _, ok := seenCandidates[candidateKey]; ok {
					continue
				}
				seenCandidates[candidateKey] = struct{}{}
				report.UniqueCandidates++

				cTree := cParser.Parse(candidate.Source, nil)
				if cTree == nil || cTree.RootNode() == nil {
					t.Fatalf("C candidate parse returned no root for seed %q", seed.ID)
				}
				productionTree := compactT3WitnessMinerParseGo(t, productionParser, candidate.Source)
				compactTree := compactT3WitnessMinerParseGo(t, compactParser, candidate.Source)

				cRoot := cTree.RootNode()
				productionRoot := productionTree.RootNode()
				compactRoot := compactTree.RootNode()
				cHasError := cRoot.HasError()
				productionHasError := productionRoot.HasError()
				compactHasError := compactRoot.HasError()
				productionDivergences := compactT3StructuralDivergences(productionRoot, goLanguage, cRoot)
				compactDivergences := compactT3StructuralDivergences(compactRoot, goLanguage, cRoot)
				if len(productionDivergences) == 0 {
					report.ProductionOracleExactCandidates++
				} else {
					report.ProductionOracleDivergentCandidates++
				}
				differenceClass, finding := compactT3WitnessMinerDifferenceClass(
					len(productionDivergences),
					len(compactDivergences),
					cHasError,
					compactHasError,
				)
				if finding {
					report.Findings = append(report.Findings, compactT3WitnessMinerFinding{
						ID:                         fmt.Sprintf("miner_v2_%s_%s", language, candidate.SHA256[:16]),
						DifferenceClass:            differenceClass,
						Language:                   language,
						SeedID:                     seed.ID,
						Mutation:                   candidate.Mutation,
						SourceUTF8:                 string(candidate.Source),
						SourceSHA256:               candidate.SHA256,
						CHasError:                  cHasError,
						ProductionHasError:         productionHasError,
						CompactHasError:            compactHasError,
						ProductionCDivergenceCount: len(productionDivergences),
						CompactCDivergenceCount:    len(compactDivergences),
					})
				}
				compactTree.Release()
				productionTree.Release()
				cTree.Close()
			}
		}
		cParser.Close()
	}

	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Language != report.Findings[j].Language {
			return report.Findings[i].Language < report.Findings[j].Language
		}
		return report.Findings[i].SourceSHA256 < report.Findings[j].SourceSHA256
	})
	if got := report.ProductionOracleExactCandidates + report.ProductionOracleDivergentCandidates; got != report.UniqueCandidates {
		t.Fatalf("classified candidates=%d, want %d", got, report.UniqueCandidates)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("encode witness miner report: %v", err)
	}
	reportSum := sha256.Sum256(raw)
	t.Logf("COMPACT_T3_WITNESS_MINER_REPORT_SHA256=%s", hex.EncodeToString(reportSum[:]))
	t.Logf("COMPACT_T3_WITNESS_MINER_REPORT=%s", raw)
}
