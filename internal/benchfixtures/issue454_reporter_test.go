package benchfixtures

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

type issue454FieldMatrix struct {
	Schema     string `json:"schema"`
	Spec       string `json:"spec"`
	G1Status   string `json:"g1_status"`
	Provenance struct {
		IssueURL        string `json:"issue_url"`
		IssueBodySHA256 string `json:"issue_body_sha256"`
		Reports         []struct {
			CommentID  int64  `json:"comment_id"`
			URL        string `json:"url"`
			BodySHA256 string `json:"body_sha256"`
		} `json:"reports"`
	} `json:"provenance"`
	Execution struct {
		GotreesitterRevision string `json:"gotreesitter_revision"`
		GrammarBlobReceipt   string `json:"grammar_blob_receipt"`
		SourceGrowth         string `json:"source_growth"`
		ProcessScope         string `json:"process_scope"`
		SeedProcessScope     string `json:"seed_process_scope"`
		BaseCandidateOrder   string `json:"base_candidate_order"`
		EditOrder            string `json:"edit_order"`
		ShuffleSeeds         []int  `json:"shuffle_seeds"`
	} `json:"execution"`
	Grammars []struct {
		Name       string `json:"name"`
		BlobSHA256 string `json:"blob_sha256"`
	} `json:"grammars"`
	Fixtures []struct {
		ID                string   `json:"id"`
		Languages         []string `json:"languages"`
		SourceStatus      string   `json:"source_status"`
		GeneratorFunction string   `json:"generator_function"`
		TemplateSHA256    string   `json:"template_sha256"`
		Marker            string   `json:"marker"`
		Site              *struct {
			Byte   int    `json:"byte"`
			Row    uint32 `json:"row"`
			Column uint32 `json:"column"`
		} `json:"site"`
		Targets []struct {
			TargetBytes  int    `json:"target_bytes"`
			SourceBytes  int    `json:"source_bytes"`
			SourceSHA256 string `json:"source_sha256"`
			Edits        map[string]struct {
				SourceBytes  int    `json:"source_bytes"`
				SourceSHA256 string `json:"source_sha256"`
			} `json:"edits"`
		} `json:"targets"`
		PendingFields    []string `json:"pending_fields"`
		InternalFixture  string   `json:"internal_fixture"`
		InternalRelation string   `json:"internal_relation"`
	} `json:"fixtures"`
	Rows []struct {
		ID               string   `json:"id"`
		Stage            string   `json:"stage"`
		CurrentStatus    string   `json:"current_status"`
		EditClasses      []string `json:"edit_classes"`
		Fixtures         []string `json:"fixtures"`
		ExpectedReceipts []string `json:"expected_receipts"`
	} `json:"rows"`
}

func TestIssue454FieldMatrixIdentity(t *testing.T) {
	manifest := loadIssue454FieldMatrix(t)
	if manifest.Schema != "gotreesitter-issue454-field-matrix/v1" {
		t.Fatalf("schema = %q", manifest.Schema)
	}
	if manifest.Spec != "spec.issue454-field-matrix.v1" {
		t.Fatalf("spec = %q", manifest.Spec)
	}
	if manifest.G1Status != "in_progress_missing_reporter_sources" {
		t.Fatalf("G1 status = %q", manifest.G1Status)
	}
	if manifest.Provenance.IssueURL != "https://github.com/odvcencio/gotreesitter/issues/454" {
		t.Fatalf("issue URL = %q", manifest.Provenance.IssueURL)
	}
	assertIssue454SHA256(t, "issue body", manifest.Provenance.IssueBodySHA256)
	wantReportIDs := map[int64]struct{}{
		5060588126: {},
		5152318747: {},
		5188399245: {},
		5190539569: {},
	}
	if len(manifest.Provenance.Reports) != len(wantReportIDs) {
		t.Fatalf("provenance report count = %d", len(manifest.Provenance.Reports))
	}
	for _, report := range manifest.Provenance.Reports {
		if _, exists := wantReportIDs[report.CommentID]; !exists {
			t.Errorf("unexpected provenance comment %d", report.CommentID)
		}
		if report.URL == "" {
			t.Errorf("provenance comment %d has no URL", report.CommentID)
		}
		assertIssue454SHA256(t, "report body", report.BodySHA256)
	}
	if manifest.Execution.GotreesitterRevision != "9036bbb47f2a2bc5c5ea8f5188913b4827571476" {
		t.Fatalf("gotreesitter revision = %q", manifest.Execution.GotreesitterRevision)
	}
	if manifest.Execution.GrammarBlobReceipt != "testdata/c4_table_shape_fleet.json" {
		t.Fatalf("grammar blob receipt = %q", manifest.Execution.GrammarBlobReceipt)
	}
	if manifest.Execution.SourceGrowth != "append_complete_records_while_len_is_below_target" {
		t.Fatalf("source growth = %q", manifest.Execution.SourceGrowth)
	}
	if manifest.Execution.ProcessScope != "one_language_per_process" {
		t.Fatalf("process scope = %q", manifest.Execution.ProcessScope)
	}
	if manifest.Execution.SeedProcessScope != "one_seed_per_process" {
		t.Fatalf("seed process scope = %q", manifest.Execution.SeedProcessScope)
	}
	if manifest.Execution.BaseCandidateOrder != "randomized_per_seed" {
		t.Fatalf("base and candidate order = %q", manifest.Execution.BaseCandidateOrder)
	}
	if manifest.Execution.EditOrder != "randomized_per_seed" {
		t.Fatalf("edit order = %q", manifest.Execution.EditOrder)
	}
	if len(manifest.Execution.ShuffleSeeds) != 20 {
		t.Fatalf("shuffle seed count = %d", len(manifest.Execution.ShuffleSeeds))
	}
	seedSet := make(map[int]struct{}, len(manifest.Execution.ShuffleSeeds))
	for _, seed := range manifest.Execution.ShuffleSeeds {
		if seed == 0 {
			t.Fatal("shuffle seed is zero")
		}
		if _, exists := seedSet[seed]; exists {
			t.Fatalf("duplicate shuffle seed %d", seed)
		}
		seedSet[seed] = struct{}{}
	}

	grammarNames := make(map[string]struct{}, len(manifest.Grammars))
	fleetDigests := loadIssue454FleetDigests(t)
	for _, grammar := range manifest.Grammars {
		if _, exists := grammarNames[grammar.Name]; exists {
			t.Fatalf("duplicate grammar %q", grammar.Name)
		}
		grammarNames[grammar.Name] = struct{}{}
		if digest, err := hex.DecodeString(grammar.BlobSHA256); err != nil || len(digest) != sha256.Size {
			t.Fatalf("grammar %q digest = %q", grammar.Name, grammar.BlobSHA256)
		}
		if got := fleetDigests[grammar.Name]; got != grammar.BlobSHA256 {
			t.Fatalf("grammar %q digest = %q, fleet receipt has %q", grammar.Name, grammar.BlobSHA256, got)
		}
	}

	fixtureIDs := make(map[string]struct{}, len(manifest.Fixtures))
	var pending []string
	for _, fixture := range manifest.Fixtures {
		if _, exists := fixtureIDs[fixture.ID]; exists {
			t.Fatalf("duplicate fixture %q", fixture.ID)
		}
		fixtureIDs[fixture.ID] = struct{}{}
		for _, language := range fixture.Languages {
			if _, exists := grammarNames[language]; !exists {
				t.Errorf("fixture %q uses unknown grammar %q", fixture.ID, language)
			}
		}
		switch fixture.SourceStatus {
		case "verbatim_public":
			if len(fixture.PendingFields) != 0 {
				t.Errorf("verbatim fixture %q has pending fields", fixture.ID)
			}
		case "reporter_source_pending":
			pending = append(pending, fixture.ID)
			if fixture.GeneratorFunction != "" || len(fixture.PendingFields) == 0 {
				t.Errorf("pending fixture %q does not identify its gap", fixture.ID)
			}
		default:
			t.Errorf("fixture %q source status = %q", fixture.ID, fixture.SourceStatus)
		}
	}
	sort.Strings(pending)
	wantPending := []string{
		"c-repeated-functions-v1",
		"csharp-repeated-classes-v1",
		"dart-repeated-declarations-v1",
		"flat-root-gamut-v1",
		"forest-reuse-gamut-v1",
		"groovy-command-blocks-v1",
		"haskell-repeated-source-v1",
		"php-repeated-functions-v1",
		"scanner-tail-gamut-v1",
	}
	if !reflect.DeepEqual(pending, wantPending) {
		t.Fatalf("pending fixtures = %q, want %q", pending, wantPending)
	}
	wantInternalApproximations := map[string]string{
		"c-repeated-functions-v1":    "different_template_and_exact_length_padding",
		"csharp-repeated-classes-v1": "different_namespace_and_member_template",
		"php-repeated-functions-v1":  "different_template_and_exact_length_padding",
	}
	for _, fixture := range manifest.Fixtures {
		wantRelation, exists := wantInternalApproximations[fixture.ID]
		if !exists {
			continue
		}
		if fixture.InternalFixture == "" || fixture.InternalRelation != wantRelation {
			t.Errorf(
				"fixture %q internal approximation = %q/%q",
				fixture.ID,
				fixture.InternalFixture,
				fixture.InternalRelation,
			)
		}
	}

	rowIDs := make(map[string]struct{}, len(manifest.Rows))
	if len(manifest.Rows) != 11 {
		t.Fatalf("row count = %d, want 11", len(manifest.Rows))
	}
	for _, row := range manifest.Rows {
		if _, exists := rowIDs[row.ID]; exists {
			t.Fatalf("duplicate row %q", row.ID)
		}
		rowIDs[row.ID] = struct{}{}
		if len(row.Fixtures) == 0 {
			t.Errorf("row %q has no fixtures", row.ID)
		}
		if row.Stage == "" || row.CurrentStatus == "" {
			t.Errorf("row %q lacks its stage or status", row.ID)
		}
		if len(row.EditClasses) == 0 || len(row.ExpectedReceipts) == 0 {
			t.Errorf("row %q lacks edit classes or expected receipts", row.ID)
		}
		for _, fixtureID := range row.Fixtures {
			if _, exists := fixtureIDs[fixtureID]; !exists {
				t.Errorf("row %q uses unknown fixture %q", row.ID, fixtureID)
			}
		}
	}
}

func TestIssue454VerbatimReporterGenerators(t *testing.T) {
	manifest := loadIssue454FieldMatrix(t)
	type exactGenerator struct {
		function string
		template string
		generate func(int) []byte
	}
	exact := map[string]exactGenerator{
		"python-repeated-functions-v1": {
			function: "Issue454ReporterPythonSource",
			template: issue454ReporterPythonTemplate,
			generate: Issue454ReporterPythonSource,
		},
		"javascript-repeated-functions-v1": {
			function: "Issue454ReporterJavaScriptSource",
			template: issue454ReporterJavaScriptTemplate,
			generate: Issue454ReporterJavaScriptSource,
		},
		"typescript-repeated-functions-v1": {
			function: "Issue454ReporterTypeScriptSource",
			template: issue454ReporterTypeScriptTemplate,
			generate: Issue454ReporterTypeScriptSource,
		},
	}
	seen := make(map[string]struct{}, len(exact))
	for _, fixture := range manifest.Fixtures {
		generator, ok := exact[fixture.ID]
		if !ok {
			continue
		}
		seen[fixture.ID] = struct{}{}
		if fixture.SourceStatus != "verbatim_public" {
			t.Errorf("fixture %q status = %q", fixture.ID, fixture.SourceStatus)
		}
		if fixture.GeneratorFunction != generator.function {
			t.Errorf("fixture %q function = %q", fixture.ID, fixture.GeneratorFunction)
		}
		if got := sha256Hex([]byte(generator.template)); got != fixture.TemplateSHA256 {
			t.Errorf("fixture %q template sha256 = %q, want %q", fixture.ID, got, fixture.TemplateSHA256)
		}
		if fixture.Site == nil {
			t.Errorf("fixture %q has no edit site", fixture.ID)
			continue
		}
		for _, target := range fixture.Targets {
			source := generator.generate(target.TargetBytes)
			if len(source) != target.SourceBytes {
				t.Errorf("fixture %q target %d source bytes = %d, want %d", fixture.ID, target.TargetBytes, len(source), target.SourceBytes)
			}
			if got := sha256Hex(source); got != target.SourceSHA256 {
				t.Errorf("fixture %q target %d source sha256 = %q, want %q", fixture.ID, target.TargetBytes, got, target.SourceSHA256)
			}
			for _, class := range []Issue454ReporterEditClass{
				Issue454ReporterReplace,
				Issue454ReporterInsert,
				Issue454ReporterDelete,
			} {
				edit, err := BuildIssue454ReporterEdit(source, fixture.Marker, class)
				if err != nil {
					t.Errorf("fixture %q target %d %s edit: %v", fixture.ID, target.TargetBytes, class, err)
					continue
				}
				if int(edit.StartByte) != fixture.Site.Byte || edit.StartPoint.Row != fixture.Site.Row || edit.StartPoint.Column != fixture.Site.Column {
					t.Errorf("fixture %q site = byte %d row %d column %d", fixture.ID, edit.StartByte, edit.StartPoint.Row, edit.StartPoint.Column)
				}
				expected, exists := target.Edits[string(class)]
				if !exists {
					t.Errorf("fixture %q target %d lacks %s receipt", fixture.ID, target.TargetBytes, class)
					continue
				}
				if len(edit.Source) != expected.SourceBytes {
					t.Errorf("fixture %q target %d %s bytes = %d, want %d", fixture.ID, target.TargetBytes, class, len(edit.Source), expected.SourceBytes)
				}
				if got := sha256Hex(edit.Source); got != expected.SourceSHA256 {
					t.Errorf("fixture %q target %d %s sha256 = %q, want %q", fixture.ID, target.TargetBytes, class, got, expected.SourceSHA256)
				}
			}
		}
	}
	if len(seen) != len(exact) {
		t.Fatalf("verbatim generators found = %d, want %d", len(seen), len(exact))
	}
}

func loadIssue454FieldMatrix(t *testing.T) issue454FieldMatrix {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "issue454", "field_matrix_v1.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest issue454FieldMatrix
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func loadIssue454FleetDigests(t *testing.T) map[string]string {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "c4_table_shape_fleet.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Grammars []struct {
			Name       string `json:"grammar"`
			BlobSHA256 string `json:"blob_sha256"`
		} `json:"grammars"`
	}
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	digests := make(map[string]string, len(receipt.Grammars))
	for _, grammar := range receipt.Grammars {
		digests[grammar.Name] = grammar.BlobSHA256
	}
	return digests
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func assertIssue454SHA256(t *testing.T, label, value string) {
	t.Helper()
	digest, err := hex.DecodeString(value)
	if err != nil || len(digest) != sha256.Size {
		t.Fatalf("%s sha256 = %q", label, value)
	}
}
