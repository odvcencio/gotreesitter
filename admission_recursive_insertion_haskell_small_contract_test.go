//go:build gts_parsercorephase0 && gts_merge_census

package gotreesitter_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type recursiveInsertionProfileExpectation struct {
	grammarBlobSHA256            string
	convergedReductionSplitDrops bool
	eofAcceptNoActionSiblings    bool
	primaryAcceptanceDerivation  bool
	exactStackNodeEquivalence    bool
	strategy2ErrorRegion         bool
	missingTokenInsertion        bool
}

type recursiveInsertionRealCorpusRow struct {
	language     string
	bucket       string
	pathSuffix   string
	bytes        int
	sourceSHA256 string
	profile      recursiveInsertionProfileExpectation
}

var recursiveInsertionRealCorpusRows = []recursiveInsertionRealCorpusRow{
	{
		language:     "haskell",
		bucket:       "small",
		pathSuffix:   "corpus_real/haskell/small__Main.hs",
		bytes:        260,
		sourceSHA256: "c60b55de99836dacb00a0f2808835895132996a95d52304cefab548b8cbdef65",
		profile: recursiveInsertionProfileExpectation{
			grammarBlobSHA256:            "5e8ac7ac21ebd2e299fa471805498176fecc02c6ccc9eb3422609f0a075c0441",
			convergedReductionSplitDrops: true,
		},
	},
}

type recursiveInsertionProfileSnapshot struct {
	grammarBlobSHA256            [32]byte
	grammarBlobSHA256OK          bool
	convergedReductionSplitDrops bool
	eofAcceptNoActionSiblings    bool
	primaryAcceptanceDerivation  bool
	exactStackNodeEquivalence    bool
	strategy2ErrorRegion         bool
	missingTokenInsertion        bool
}

// TestAdmissionCandidateRecursiveInsertionHaskellSmall is a strict route contract.
// It requires generic core support and permits no new language grant.
func TestAdmissionCandidateRecursiveInsertionHaskellSmall(t *testing.T) {
	manifestPath := strings.TrimSpace(os.Getenv("GTS_ADMISSION_REAL_CORPUS_MANIFEST"))
	if manifestPath == "" {
		manifestPath = filepath.Join("cgo_harness", "corpus_real", "manifest.json")
	}
	manifestSource, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("canonical real-corpus manifest is unavailable at %s: %v", manifestPath, err)
		}
		t.Fatal(err)
	}
	var manifest admissionRealCorpusManifest
	if err := json.Unmarshal(manifestSource, &manifest); err != nil {
		t.Fatal(err)
	}

	languages := make(map[string]grammars.LangEntry)
	for _, entry := range grammars.AllLanguages() {
		languages[entry.Name] = entry
	}
	t.Cleanup(func() {
		gts.ResetAdmissionCandidateCountersForTest()
		grammars.PurgeEmbeddedLanguageCache()
	})

	for _, want := range recursiveInsertionRealCorpusRows {
		want := want
		t.Run(want.language+"/"+want.bucket+"/"+filepath.Base(want.pathSuffix), func(t *testing.T) {
			corpus, ok := findRecursiveInsertionRealCorpusEntry(manifest, want)
			if !ok {
				t.Fatalf("canonical manifest has no unique row for %s/%s/%s", want.language, want.bucket, want.pathSuffix)
			}
			sourcePath := admissionRealCorpusPath(manifestPath, corpus.OutputPath)
			source, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatalf("read canonical source %s: %v", sourcePath, err)
			}
			if len(source) != want.bytes {
				t.Fatalf("source bytes=%d, want %d", len(source), want.bytes)
			}
			sourceDigest := sha256.Sum256(source)
			if got := hex.EncodeToString(sourceDigest[:]); got != want.sourceSHA256 {
				t.Fatalf("source SHA-256=%s, want %s", got, want.sourceSHA256)
			}

			entry, ok := languages[want.language]
			if !ok {
				t.Fatalf("language %q is absent from the registry", want.language)
			}
			lang := entry.Language()
			if lang == nil {
				t.Fatalf("language %q returned nil", want.language)
			}
			beforeProfile := recursiveInsertionProfile(lang)
			requireRecursiveInsertionProfile(t, want, beforeProfile)

			gts.ResetAdmissionCandidateCountersForTest()
			production := gts.NewParser(lang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(source)
			if err != nil || productionTree == nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()
			productionRouted, productionFallback := gts.AdmissionCandidateCounters()
			if productionRouted != 0 || productionFallback != 0 {
				t.Fatalf("production route counters=%d/%d, want 0/0", productionRouted, productionFallback)
			}
			productionRoot := requireRecursiveInsertionCleanFullSpan(t, productionTree, len(source), "production")
			productionInspection, err := benchfixtures.InspectGoTree(productionRoot, lang)
			if err != nil {
				t.Fatalf("production deep digest: %v", err)
			}

			candidate := gts.NewParser(lang)
			candidate.SetAdmissionCandidateRoute(true)
			gts.MergeEventCensusReset()
			candidateTree, err := candidate.Parse(source)
			if err != nil || candidateTree == nil {
				t.Fatalf("compact candidate parse: %v; fallback reason=%q", err, gts.AdmissionCandidateLastFallbackReason())
			}
			defer candidateTree.Release()
			candidateRouted, candidateFallback := gts.AdmissionCandidateCounters()
			if candidateRouted-productionRouted != 1 || candidateFallback-productionFallback != 0 {
				t.Fatalf("compact route delta=%d/%d, want 1/0; reason=%q", candidateRouted-productionRouted, candidateFallback-productionFallback, gts.AdmissionCandidateLastFallbackReason())
			}
			work := gts.MergeEventCensusSnapshot()
			if work.CompactAcceptancesObserved != 1 || work.CompactLinkUnionRecursiveChanged == 0 {
				t.Fatalf("compact non-vacuity work receipt=%+v, want one acceptance and recursive link-union work", work)
			}
			candidateRoot := requireRecursiveInsertionCleanFullSpan(t, candidateTree, len(source), "compact")
			candidateInspection, err := benchfixtures.InspectGoTree(candidateRoot, lang)
			if err != nil {
				t.Fatalf("compact deep digest: %v", err)
			}
			if diff := firstAdmissionTreeDivergence(candidateRoot, productionRoot, lang, "root"); diff != "" {
				t.Fatalf("compact public tree diverges: %s", diff)
			}
			if candidateInspection.SHA256 != productionInspection.SHA256 {
				t.Fatalf("compact deep digest=%s, production=%s", candidateInspection.SHA256, productionInspection.SHA256)
			}

			afterProfile := recursiveInsertionProfile(lang)
			if afterProfile != beforeProfile {
				t.Fatalf("language profile or grammar digest changed during admission: before=%+v after=%+v", beforeProfile, afterProfile)
			}
			requireRecursiveInsertionProfile(t, want, afterProfile)
		})
	}
}

func findRecursiveInsertionRealCorpusEntry(manifest admissionRealCorpusManifest, want recursiveInsertionRealCorpusRow) (admissionRealCorpusEntry, bool) {
	var match admissionRealCorpusEntry
	found := 0
	for _, corpus := range manifest.Entries {
		if corpus.Language != want.language || corpus.Bucket != want.bucket || corpus.Bytes != want.bytes {
			continue
		}
		if !strings.HasSuffix(filepath.ToSlash(corpus.OutputPath), want.pathSuffix) {
			continue
		}
		match = corpus
		found++
	}
	return match, found == 1
}

func requireRecursiveInsertionCleanFullSpan(t testing.TB, tree *gts.Tree, sourceBytes int, label string) *gts.Node {
	t.Helper()
	root := tree.RootNode()
	if root == nil {
		t.Fatalf("%s tree has no root", label)
	}
	if root.IsError() || root.HasError() {
		t.Fatalf("%s tree has an error: %s", label, root.SExpr(tree.Language()))
	}
	if root.StartByte() != 0 || root.EndByte() != uint32(sourceBytes) {
		t.Fatalf("%s root span=[%d,%d), want [0,%d)", label, root.StartByte(), root.EndByte(), sourceBytes)
	}
	return root
}

func recursiveInsertionProfile(lang *gts.Language) recursiveInsertionProfileSnapshot {
	grammarBlobSHA256, grammarBlobSHA256OK := lang.GrammarBlobSHA256()
	return recursiveInsertionProfileSnapshot{
		grammarBlobSHA256:            grammarBlobSHA256,
		grammarBlobSHA256OK:          grammarBlobSHA256OK,
		convergedReductionSplitDrops: lang.CompactConvergedReductionSplitDropsCertified,
		eofAcceptNoActionSiblings:    lang.CompactEOFAcceptNoActionSiblingsCertified,
		primaryAcceptanceDerivation:  lang.CompactPrimaryAcceptanceDerivationCertified,
		exactStackNodeEquivalence:    lang.ExactStackNodeEquivalenceCertified,
		strategy2ErrorRegion:         lang.CompactStrategy2ErrorRegionCertified,
		missingTokenInsertion:        lang.CompactMissingTokenInsertionCertified,
	}
}

func requireRecursiveInsertionProfile(t testing.TB, want recursiveInsertionRealCorpusRow, got recursiveInsertionProfileSnapshot) {
	t.Helper()
	if !got.grammarBlobSHA256OK || hex.EncodeToString(got.grammarBlobSHA256[:]) != want.profile.grammarBlobSHA256 {
		t.Fatalf("grammar blob digest=%x (present=%t), want %s", got.grammarBlobSHA256, got.grammarBlobSHA256OK, want.profile.grammarBlobSHA256)
	}
	if got.convergedReductionSplitDrops != want.profile.convergedReductionSplitDrops ||
		got.eofAcceptNoActionSiblings != want.profile.eofAcceptNoActionSiblings ||
		got.primaryAcceptanceDerivation != want.profile.primaryAcceptanceDerivation ||
		got.exactStackNodeEquivalence != want.profile.exactStackNodeEquivalence ||
		got.strategy2ErrorRegion != want.profile.strategy2ErrorRegion ||
		got.missingTokenInsertion != want.profile.missingTokenInsertion {
		t.Fatalf("compact profile changed for %s: got=%+v want=%+v", want.language, got, want.profile)
	}
}
