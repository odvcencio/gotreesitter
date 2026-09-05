//go:build gts_parsercorephase0

package gotreesitter_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

func TestAdmissionCandidateGraphQLRootFieldsDirect(t *testing.T) {
	const (
		fixturePath = "testdata/admission_direct/graphql_root_fields.graphql"
		// This source comes from hasura/graphql-engine at commit
		// d769f172d6ba43ecb0254c6be84bb40498608b3d.
		sourceSHA256 = "12cb6d5c35d2f629cebc8c40738796173d86e93543e50c1138dc7f138c1ff061"
		wantDetail   = "digest a6a0c3dd54db"
	)

	source, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read GraphQL admission fixture: %v", err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != sourceSHA256 {
		t.Fatalf("GraphQL admission fixture SHA-256 = %s, want %s", got, sourceSHA256)
	}

	var graphql grammars.LangEntry
	for _, entry := range grammars.AllLanguages() {
		if entry.Name == "graphql" {
			graphql = entry
			break
		}
	}
	if graphql.Name == "" {
		t.Fatal("GraphQL grammar is not registered")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	row := runAdmissionScorecardSource(graphql, source)
	if row.status != scorecardPass {
		t.Fatalf("GraphQL RootFields compact route = %s: %s", row.status, row.detail)
	}
	if row.detail != wantDetail {
		t.Fatalf("GraphQL RootFields compact digest = %q, want %q", row.detail, wantDetail)
	}
}

func TestAdmissionCandidateSvelteButtonDirect(t *testing.T) {
	const (
		fixturePath = "testdata/admission_direct/svelte_button.svelte"
		// This source comes from themesberg/flowbite-svelte at commit
		// df3404b81aefbd91cb178dd68e4844a4e4e31066.
		// Its path is src/routes/docs-examples/typography/link/Button.svelte.
		// The locked Svelte grammar uses commit
		// ae5199db47757f785e43a14b332118a5474de1a2.
		sourceSHA256 = "fcbd956e6e07fbd1c2f52da59bcc0d35c9de495e425959a4cfddff2ea74ef7e4"
		wantDetail   = "digest 7697435b1072"
	)

	source, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read Svelte admission fixture: %v", err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != sourceSHA256 {
		t.Fatalf("Svelte admission fixture SHA-256 = %s, want %s", got, sourceSHA256)
	}

	var svelte grammars.LangEntry
	for _, entry := range grammars.AllLanguages() {
		if entry.Name == "svelte" {
			svelte = entry
			break
		}
	}
	if svelte.Name == "" {
		t.Fatal("Svelte grammar is not registered")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	row := runAdmissionScorecardSource(svelte, source)
	if row.status != scorecardPass {
		t.Fatalf("Svelte Button compact route = %s: %s", row.status, row.detail)
	}
	if row.detail != wantDetail {
		t.Fatalf("Svelte Button compact digest = %q, want %q", row.detail, wantDetail)
	}
}

func TestAdmissionCandidateElmHighlightDirect(t *testing.T) {
	const (
		fixturePath = "testdata/admission_direct/elm_highlight_basic.elm"
		// This source comes from elm-tooling/tree-sitter-elm at commit
		// 6d9511c28181db66daee4e883f811f6251220943.
		// Its path is test/highlight/basic.elm.
		sourceBytes  = 1231
		sourceSHA256 = "8fca87bd8cc2735e83704acd8d06ffbc6cf04e386505de45596218d7fb72642c"
		// TestAdmissionCandidateElmHighlightCOracle authenticates this digest.
		wantDetail = "digest f9e33776ce39"
	)

	source, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read Elm admission fixture: %v", err)
	}
	if len(source) != sourceBytes {
		t.Fatalf("Elm admission fixture bytes = %d, want %d", len(source), sourceBytes)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(source)); got != sourceSHA256 {
		t.Fatalf("Elm admission fixture SHA-256 = %s, want %s", got, sourceSHA256)
	}

	var elm grammars.LangEntry
	for _, entry := range grammars.AllLanguages() {
		if entry.Name == "elm" {
			elm = entry
			break
		}
	}
	if elm.Name == "" {
		t.Fatal("Elm grammar is not registered")
	}
	t.Cleanup(func() { grammars.PurgeEmbeddedLanguageCache() })

	row := runAdmissionScorecardSource(elm, source)
	if row.status != scorecardPass {
		t.Fatalf("Elm highlight compact route = %s: %s", row.status, row.detail)
	}
	if row.detail != wantDetail {
		t.Fatalf("Elm highlight compact digest = %q, want %q", row.detail, wantDetail)
	}
}
