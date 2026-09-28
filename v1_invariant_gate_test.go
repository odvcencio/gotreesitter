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

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

type v1InvariantCorpusManifest struct {
	Entries []struct {
		Language      string `json:"language"`
		Role          string `json:"role"`
		CommittedPath string `json:"committed_path"`
		Bytes         int    `json:"bytes"`
		SHA256        string `json:"sha256"`
	} `json:"entries"`
}

// TestV1InvariantGateR4EditSession checks the design invariants for one R4
// grammar sample. Languages without a committed R4 sample use their grammar
// smoke sample. Run it once per language with GOTREESITTER_V1_INVARIANT_LANGUAGE
// set so each heavy grammar has its own process and memory lifetime.
func TestV1InvariantGateR4EditSession(t *testing.T) {
	languageName := strings.TrimSpace(os.Getenv("GOTREESITTER_V1_INVARIANT_LANGUAGE"))
	if languageName == "" {
		t.Skip("set GOTREESITTER_V1_INVARIANT_LANGUAGE to one R4 language")
	}
	raw, err := os.ReadFile(filepath.Join("internal", "benchfixtures", "real_corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest v1InvariantCorpusManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	var sample *v1InvariantCorpusManifestEntry
	for _, row := range manifest.Entries {
		if row.Language == languageName && row.Role == "sample" && row.CommittedPath != "" {
			copy := v1InvariantCorpusManifestEntry{CommittedPath: row.CommittedPath, Bytes: row.Bytes, SHA256: row.SHA256}
			sample = &copy
			break
		}
	}
	entry := grammars.DetectLanguageByName(languageName)
	if entry == nil || entry.Language() == nil {
		t.Fatalf("Go grammar %q unavailable", languageName)
	}
	lang := entry.Language()
	fixture := "r4"
	var source []byte
	if sample == nil {
		fixture = "grammar-smoke"
		source = []byte(grammars.ParseSmokeSample(languageName))
	} else {
		source, err = os.ReadFile(filepath.Join("internal", "benchfixtures", sample.CommittedPath))
		if err != nil {
			t.Fatal(err)
		}
		if len(source) != sample.Bytes {
			t.Fatalf("R4 sample bytes=%d, want %d", len(source), sample.Bytes)
		}
		digest := sha256.Sum256(source)
		if got := hex.EncodeToString(digest[:]); got != sample.SHA256 {
			t.Fatalf("R4 sample digest=%s, want %s", got, sample.SHA256)
		}
	}

	parser := gts.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	if tree == nil {
		t.Fatal("initial parse returned a nil tree")
	}
	defer func() {
		if tree != nil {
			tree.Release()
		}
	}()
	assertV1InvariantTree(t, "initial", tree, lang, source)

	var noEditErr error
	allocs := testing.AllocsPerRun(5, func() {
		next, parseErr := parser.ParseIncremental(source, tree)
		if parseErr != nil {
			noEditErr = parseErr
			return
		}
		if next == nil {
			noEditErr = fmt.Errorf("no-edit parse returned a nil tree")
			return
		}
		next.Release()
	})
	if noEditErr != nil {
		t.Fatalf("no-edit reparse: %v", noEditErr)
	}
	if allocs != 0 {
		t.Fatalf("no-edit reparse allocations=%g, want 0", allocs)
	}

	for stepIndex, step := range benchfixtures.EditingSession(source) {
		previous := tree
		previous.Edit(step.Edit)
		incremental, err := parser.ParseIncremental(step.Source, previous)
		if err != nil {
			t.Fatalf("step %d incremental parse: %v", stepIndex+1, err)
		}
		if incremental == nil {
			t.Fatalf("step %d incremental parse returned nil tree", stepIndex+1)
		}
		freshParser := gts.NewParser(lang)
		fresh, err := freshParser.Parse(step.Source)
		if err != nil {
			t.Fatalf("step %d fresh parse: %v", stepIndex+1, err)
		}
		if fresh == nil {
			t.Fatalf("step %d fresh parse returned nil tree", stepIndex+1)
		}
		assertV1InvariantTree(t, fmt.Sprintf("step %d incremental", stepIndex+1), incremental, lang, step.Source)
		assertV1InvariantTree(t, fmt.Sprintf("step %d fresh", stepIndex+1), fresh, lang, step.Source)
		incrementalDigest, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
		if err != nil {
			t.Fatalf("step %d incremental tree digest: %v", stepIndex+1, err)
		}
		freshDigest, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
		if err != nil {
			t.Fatalf("step %d fresh tree digest: %v", stepIndex+1, err)
		}
		if incrementalDigest.SHA256 != freshDigest.SHA256 {
			incrementalRuntime := incremental.ParseRuntime()
			freshRuntime := fresh.ParseRuntime()
			t.Fatalf("step %d incremental tree differs from fresh parse: incremental=%s fresh=%s\nincremental runtime: %s old_tree_reuse=%t compact_reuse=%t\nfresh runtime: %s\nincremental tree: %s\nfresh tree: %s", stepIndex+1, incrementalDigest.SHA256, freshDigest.SHA256, incrementalRuntime.Summary(), incrementalRuntime.IncrementalOldTreeReuseRoute, incrementalRuntime.CompactIncrementalReuseRoute, freshRuntime.Summary(), incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
		}
		if previous != incremental {
			previous.Release()
		}
		fresh.Release()
		tree = incremental
	}
	t.Logf("language=%s fixture=%s sample_bytes=%d session_steps=%d no_edit_allocs=%g", languageName, fixture, len(source), benchfixtures.EditSessionSteps, allocs)
}

type v1InvariantCorpusManifestEntry struct {
	CommittedPath string
	Bytes         int
	SHA256        string
}

func assertV1InvariantTree(t *testing.T, phase string, tree *gts.Tree, lang *gts.Language, source []byte) {
	t.Helper()
	if tree == nil || tree.RootNode() == nil {
		t.Fatalf("%s returned no root", phase)
	}
	root := tree.RootNode()
	if root.Type(lang) == "ERROR" && !root.HasError() {
		t.Fatalf("%s has an ERROR root without HasError()", phase)
	}
	if root.EndByte() < uint32(len(source)) {
		stop := tree.ParseRuntime().StopReason
		if stop == "" || stop == gts.ParseStopAccepted {
			t.Fatalf("%s root ends at %d of %d bytes without an explanatory stop reason (%q)", phase, root.EndByte(), len(source), stop)
		}
	}
}
