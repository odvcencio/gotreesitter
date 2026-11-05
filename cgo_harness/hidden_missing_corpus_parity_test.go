//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Keep one grammar per process when selecting these regressions. A missing
// recovery leaf can disappear from public children while still affecting
// ancestor error flags, subsequent Tree.Edit invalidation and subtree reuse.
func TestCHiddenMissingCorpusFreshLockedC(t *testing.T) {
	testHiddenMissingCorpusLockedC(t, "c", false)
}

func TestCHiddenMissingCorpusFirstEditLockedC(t *testing.T) {
	testHiddenMissingCorpusLockedC(t, "c", true)
}

func TestEDSHiddenMissingCorpusFreshLockedC(t *testing.T) {
	testHiddenMissingCorpusLockedC(t, "eds", false)
}

func TestEDSHiddenMissingCorpusFirstEditLockedC(t *testing.T) {
	testHiddenMissingCorpusLockedC(t, "eds", true)
}

type hiddenMissingCorpusCounters struct {
	Tokens                    uint64 `json:"tokens"`
	NewNodes                  uint64 `json:"new_nodes"`
	MaxLiveVersions           uint64 `json:"max_live_versions"`
	MultiVersionTokenSharePPM uint64 `json:"multi_version_token_share_ppm"`
	ReusedBytes               uint64 `json:"reused_bytes"`
	BlockSplices              uint64 `json:"block_splices"`
	StopReason                string `json:"stop_reason"`
	RootEnd                   uint32 `json:"root_end"`
	InputBytes                int    `json:"input_bytes"`
	HasError                  bool   `json:"has_error"`
}

func testHiddenMissingCorpusLockedC(t *testing.T, name string, firstEdit bool) {
	t.Helper()
	var sourceBytes int
	var sourceSHA, sessionSHA, firstEditSHA string
	switch name {
	case "c":
		sourceBytes = 2049
		sourceSHA = "ccdf9a3671f4f291c653fce2ee409557cadddc893494c112ce49a1cf9c139917"
		sessionSHA = "b9cefc3950abd10a96cc068ecfc6f42c591268d5c67371c8bc6b1e2258f3eb73"
		firstEditSHA = "924d402a1ec6a7bcebdc247a1012a377e46b56f7065e76e93d6f59ed75962011"
	case "eds":
		sourceBytes = 28828
		sourceSHA = "e874774d9088e4605cffedaf3af14d98aa1a68f76e24748194468b5fe3300226"
		sessionSHA = "980d7054da1d543ca7d179783642dc6c7280b77ab3f400ad3f0993566fef4404"
		firstEditSHA = "27522c62a07fb03b73adcc3daf3d541bc0553e809169efbe08afa99d93581e16"
	default:
		t.Fatalf("unsupported regression grammar %q", name)
	}
	source, err := os.ReadFile("../internal/benchfixtures/testdata/real/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != sourceBytes || fmt.Sprintf("%x", sha256.Sum256(source)) != sourceSHA || benchfixtures.EditingSessionSHA256(source) != sessionSHA {
		t.Fatal("frozen corpus source or complete editing session changed")
	}
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		t.Fatal("Go grammar unavailable")
	}
	language := entry.Language()
	if language == nil {
		t.Fatal("Go grammar unavailable")
	}
	// Match the default ledger's Parse/root-counter/Edit/profile sequence.
	// Do not inspect old children: lazy materialization before Tree.Edit can
	// change invalidation/reuse and would test a different operation.
	parser := gts.NewParser(language)
	parser.SetAdmissionCandidateRoute(false)
	gts.ResetAdmissionCandidateCounters()
	old, err := parser.Parse(source)
	if err != nil || old == nil || old.RootNode() == nil {
		t.Fatalf("initial parse: tree=%t err=%v", old != nil, err)
	}
	defer old.Release()
	runtime := old.ParseRuntime()
	full := hiddenMissingCorpusCounters{
		Tokens: runtime.TokensConsumed, NewNodes: uint64(max(0, runtime.NodesAllocated)),
		MaxLiveVersions:           uint64(max(0, runtime.MaxStacksSeen)),
		MultiVersionTokenSharePPM: hiddenMissingCorpusShare(runtime.MultiStackTokens, runtime.TokensConsumed),
		StopReason:                string(runtime.StopReason), RootEnd: old.RootNode().EndByte(), InputBytes: len(source), HasError: old.RootNode().HasError(),
	}
	fullRouted, fullFallback := gts.AdmissionCandidateCounters()
	var inc *gts.Tree
	var profile gts.IncrementalParseProfile
	var edited *hiddenMissingCorpusCounters
	var step benchfixtures.EditStep
	if firstEdit {
		step = benchfixtures.EditingSession(source)[0]
		old.Edit(step.Edit)
		inc, profile, err = parser.ParseIncrementalProfiled(step.Source, old)
		if err != nil || inc == nil || inc.RootNode() == nil {
			t.Fatalf("first edit: tree=%t err=%v", inc != nil, err)
		}
		defer inc.Release()
		edited = &hiddenMissingCorpusCounters{
			Tokens: profile.TokensConsumed, NewNodes: profile.NewNodesAllocated,
			MaxLiveVersions:           uint64(max(0, profile.MaxStacksSeen)),
			MultiVersionTokenSharePPM: hiddenMissingCorpusShare(profile.MultiStackTokens, profile.TokensConsumed),
			ReusedBytes:               profile.ReusedBytes, BlockSplices: profile.BlockSpliceSteps,
			StopReason: string(profile.StopReason), RootEnd: inc.RootNode().EndByte(), InputBytes: len(step.Source), HasError: inc.RootNode().HasError(),
		}
	}
	// Counter capture is complete. C setup, deep tree walks, logs and extra
	// Go parses are safe from this point onward.
	routed, fallback := gts.AdmissionCandidateCounters()
	if firstEdit && (len(step.Source) != sourceBytes+1 || fmt.Sprintf("%x", sha256.Sum256(step.Source)) != firstEditSHA) {
		t.Fatal("frozen first-edit source changed")
	}
	record, err := json.Marshal(map[string]any{
		"language": name, "route": "default", "first_edit": firstEdit, "full": full, "edit": edited, "profile": profile,
		"source_sha256": sourceSHA, "session_sha256": sessionSHA,
		"actual_initial_compact_routes": fullRouted, "actual_initial_compact_fallbacks": fullFallback,
		"actual_edit_compact_routes": routed - fullRouted, "actual_edit_compact_fallbacks": fallback - fullFallback,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("hidden_missing_corpus_counters=%s", record)
	if firstEdit {
		t.Logf("first_edit=%+v after_bytes=%d after_sha256=%s", step.Edit, len(step.Source), firstEditSHA)
	}
	cLanguage, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := COracleIdentity(name)
	if err != nil {
		t.Fatal(err)
	}
	identityJSON, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("COracle=%s", identityJSON)
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	cOld := cParser.Parse(source, nil)
	if cOld == nil || cOld.RootNode() == nil {
		t.Fatal("cold C parse returned no root")
	}
	defer cOld.Close()
	if !firstEdit {
		hiddenMissingCorpusRequireAccepted(t, old, source)
		if name == "eds" {
			hiddenMissingCorpusRequireEDSSectionError(t, old, language, cOld)
		}
		assertLockedCTreeExactWithErrors(t, name+" fresh Go/fresh C", old, language, cOld)
		return
	}
	cEdit := realCorpusCInputEdit(step.Edit)
	cOld.Edit(&cEdit)
	cInc := cParser.Parse(step.Source, cOld)
	if cInc == nil || cInc.RootNode() == nil {
		t.Fatal("incremental C returned no root")
	}
	defer cInc.Close()
	freshCParser := sitter.NewParser()
	defer freshCParser.Close()
	if err := freshCParser.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	freshC := freshCParser.Parse(step.Source, nil)
	if freshC == nil || freshC.RootNode() == nil {
		t.Fatal("fresh C returned no root")
	}
	defer freshC.Close()
	freshParser := gts.NewParser(language)
	freshParser.SetAdmissionCandidateRoute(false)
	fresh, err := freshParser.Parse(step.Source)
	if err != nil || fresh == nil || fresh.RootNode() == nil {
		t.Fatalf("independent fresh Go: tree=%t err=%v", fresh != nil, err)
	}
	defer fresh.Release()
	incInspection, err := benchfixtures.InspectGoTree(inc.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	freshInspection, err := benchfixtures.InspectGoTree(fresh.RootNode(), language)
	if err != nil {
		t.Fatal(err)
	}
	freshCDigest, err := COracleDeepDigest(freshC)
	if err != nil {
		t.Fatal(err)
	}
	incCDigest, err := COracleDeepDigest(cInc)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("incremental_go_digest=%s fresh_go_digest=%s incremental_c_digest=%s fresh_c_digest=%s", incInspection.SHA256, freshInspection.SHA256, incCDigest, freshCDigest)
	t.Run("D8-incremental-go-fresh-go", func(t *testing.T) {
		hiddenMissingCorpusRequireAccepted(t, inc, step.Source)
		hiddenMissingCorpusRequireAccepted(t, fresh, step.Source)
		if incInspection.SHA256 != freshInspection.SHA256 {
			t.Fatalf("D8 all-axis digest differs: incremental Go=%s fresh Go=%s", incInspection.SHA256, freshInspection.SHA256)
		}
	})
	t.Run("incremental-go-fresh-c", func(t *testing.T) {
		assertLockedCTreeExactWithErrors(t, name+" first-edit incremental Go/fresh C", inc, language, freshC)
	})
	t.Run("fresh-go-fresh-c", func(t *testing.T) {
		assertLockedCTreeExactWithErrors(t, name+" first-edit fresh Go/fresh C", fresh, language, freshC)
	})
	t.Run("incremental-go-incremental-c", func(t *testing.T) {
		assertLockedCTreeExactWithErrors(t, name+" first-edit incremental Go/incremental C", inc, language, cInc)
	})
	t.Run("incremental-c-fresh-c", func(t *testing.T) {
		if incCDigest != freshCDigest {
			t.Fatalf("C all-axis digest differs: incremental=%s fresh=%s", incCDigest, freshCDigest)
		}
	})
	if name == "eds" {
		t.Run("section-retains-hidden-missing-error", func(t *testing.T) {
			hiddenMissingCorpusRequireEDSSectionError(t, fresh, language, freshC)
			hiddenMissingCorpusRequireEDSSectionError(t, inc, language, cInc)
		})
	}
}

func hiddenMissingCorpusShare(part, total uint64) uint64 {
	if total == 0 {
		return 0
	}
	return part * 1_000_000 / total
}

func hiddenMissingCorpusRequireAccepted(t *testing.T, tree *gts.Tree, source []byte) {
	t.Helper()
	root, runtime := tree.RootNode(), tree.ParseRuntime()
	if tree.ParseStoppedEarly() || runtime.StopReason != gts.ParseStopAccepted || root.StartByte() != 0 || root.EndByte() != uint32(len(source)) || (root.IsError() && !root.HasError()) {
		t.Fatalf("invariant failure: runtime=%s span=%d:%d bytes=%d IsError=%t HasError=%t", runtime.Summary(), root.StartByte(), root.EndByte(), len(source), root.IsError(), root.HasError())
	}
}

func hiddenMissingCorpusRequireEDSSectionError(t *testing.T, goTree *gts.Tree, language *gts.Language, cTree *sitter.Tree) {
	t.Helper()
	goSection, cSection := goTree.RootNode().Child(0), cTree.RootNode().Child(0)
	if goSection == nil || cSection == nil || goSection.Type(language) != "section" || cSection.Kind() != "section" {
		t.Fatal("frozen EDS first section missing")
	}
	if !cSection.HasError() || !goSection.HasError() {
		t.Fatalf("hidden MISSING must mark the first EDS section: Go.HasError=%t C.HasError=%t", goSection.HasError(), cSection.HasError())
	}
}
