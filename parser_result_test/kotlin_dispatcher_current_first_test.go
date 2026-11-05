package parserresult_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

const (
	kotlinDispatcherWitnessSHA256    = "efc3157ca94245c59ff5d093ca456c319f2518882f4b9a28303baa25182ec4fc"
	kotlinDispatcherRawDigest        = "3f89c1a3d1cc592c1c607a8bceeac417c4849e87e39de005efeb46e14ba629db"
	kotlinDispatcherProductionDigest = "3f89c1a3d1cc592c1c607a8bceeac417c4849e87e39de005efeb46e14ba629db"
)

// TestKotlinDispatcherCurrentFirstGate checks that the already-correct raw tree needs no rewrite.
func TestKotlinDispatcherCurrentFirstGate(t *testing.T) {
	t.Setenv("GTS_DISPATCHER_CENSUS", "1")
	lang := grammars.KotlinLanguage()
	if lang == nil || lang.Name != "kotlin" {
		t.Fatalf("language=%v, want kotlin", lang)
	}
	source := []byte(`tasks.named<KotlinCompile>("compile") {}`)
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != kotlinDispatcherWitnessSHA256 {
		t.Fatalf("source SHA-256=%s, want %s", got, kotlinDispatcherWitnessSHA256)
	}

	rawParser := gotreesitter.NewParser(lang)
	rawParser.SetAdmissionCandidateRoute(false)
	raw, err := rawParser.ParseNoResultCompatibilityBenchmarkOnly(source)
	if err != nil {
		t.Fatalf("raw parse: %v", err)
	}
	defer raw.Release()
	rawInspection, err := benchfixtures.InspectGoTree(raw.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect raw tree: %v", err)
	}
	if rawInspection.SHA256 != kotlinDispatcherRawDigest {
		t.Fatalf("raw digest=%s, want %s", rawInspection.SHA256, kotlinDispatcherRawDigest)
	}
	if pass, ok := kotlinDispatcherPass(raw); ok {
		t.Fatalf("raw dispatch.kotlin pass=%+v, want no compatibility pass", pass)
	}

	productionParser := gotreesitter.NewParser(lang)
	productionParser.SetAdmissionCandidateRoute(false)
	production, err := productionParser.Parse(source)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer production.Release()
	productionInspection, err := benchfixtures.InspectGoTree(production.RootNode(), lang)
	if err != nil {
		t.Fatalf("inspect production tree: %v", err)
	}
	if productionInspection.SHA256 != kotlinDispatcherProductionDigest {
		t.Fatalf("production digest=%s, want %s", productionInspection.SHA256, kotlinDispatcherProductionDigest)
	}
	if rawInspection.SHA256 != productionInspection.SHA256 {
		t.Fatal("raw and production digests differ; an already-correct tree was rewritten")
	}
	pass, ok := kotlinDispatcherPass(production)
	if !ok {
		t.Fatal("production did not record dispatch.kotlin")
	}
	if pass.Checked != 1 || pass.Run != 1 || pass.NodesVisited != 24 || pass.NodesRewritten != 0 {
		t.Fatalf("production dispatch.kotlin=%+v, want checked=1 run=1 visited=24 rewritten=0", pass)
	}
	recovered, ok := kotlinDispatcherSubpass(production, "dispatch.kotlin.recovered-source-file-root")
	if !ok {
		t.Fatal("production did not record dispatch.kotlin.recovered-source-file-root")
	} else if recovered.Checked != 1 || recovered.Run != 1 || recovered.NodesVisited != 24 || recovered.NodesRewritten != 0 {
		t.Fatalf("production recovered-root pass=%+v, want checked=1 run=1 visited=24 rewritten=0", recovered)
	}
	t.Logf("source_sha256=%s raw_digest=%s production_digest=%s dispatch.kotlin=%d/%d/%d/%d recovered_root=%d/%d/%d/%d", kotlinDispatcherWitnessSHA256, rawInspection.SHA256, productionInspection.SHA256, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten, recovered.Checked, recovered.Run, recovered.NodesVisited, recovered.NodesRewritten)
}

func kotlinDispatcherPass(tree *gotreesitter.Tree) (gotreesitter.NormalizationPassRuntime, bool) {
	return kotlinDispatcherSubpass(tree, "dispatch.kotlin")
}

func kotlinDispatcherSubpass(tree *gotreesitter.Tree, name string) (gotreesitter.NormalizationPassRuntime, bool) {
	if tree == nil || tree.ParseRuntime().NormalizationPasses == nil {
		return gotreesitter.NormalizationPassRuntime{}, false
	}
	for _, pass := range *tree.ParseRuntime().NormalizationPasses {
		if pass.Name == name {
			return pass, true
		}
	}
	return gotreesitter.NormalizationPassRuntime{}, false
}
