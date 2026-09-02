package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeStrictRejectsDuplicateKeysAtEveryDepth(t *testing.T) {
	for _, raw := range []string{
		`{"schema":"one","schema":"two"}`,
		`{"outer":{"value":1,"value":2}}`,
		`[{"value":1,"value":2}]`,
	} {
		var target any
		if err := decodeStrict([]byte(raw), &target); err == nil || !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("decodeStrict(%s) error = %v, want duplicate-key rejection", raw, err)
		}
	}
}

func TestParseSampleRejectsIterationCapOverflow(t *testing.T) {
	raw := strings.Join([]string{
		"schema=" + sampleSchema, "status=ok", "implementation=c",
		"lane=public_validated", "fixture=rewrite",
		"source_sha256=74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097",
		"source_bytes=5116", "clock=clock-monotonic", "warmups=1",
		"iterations=1000000001", "elapsed_ns=750000000", "visited_nodes=0",
		"traversal_checksum=0000000000000000", "",
	}, "\n")
	if _, err := parseSample([]byte(raw)); err == nil || !strings.Contains(err.Error(), "1e9") {
		t.Fatalf("parseSample error = %v, want iteration-cap rejection", err)
	}
}

func TestExpandCommandTemplateBindsLaneFixtureAndFD(t *testing.T) {
	template := []string{"go_timing_oracle_c", "--bench", "--lane", "{lane}", "--fixture", "{fixture}", "--source-fd", "{source_fd}"}
	want := []string{"go_timing_oracle_c", "--bench", "--lane", "selected_consumer", "--fixture", "language", "--source-fd", "9"}
	if got := expandCommandTemplate(template, "selected_consumer", "language", 9); !equalStrings(got, want) {
		t.Fatalf("expanded argv = %v, want %v", got, want)
	}
}

func TestVerifySignedPayloadAuthenticatesExactRawPayload(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload := json.RawMessage(`{"schema":"test/v1","value":"bound"}`)
	envelope := signedEnvelope{
		Schema: "gts-signed-receipt/v1", Payload: payload,
		PublicKey: base64.StdEncoding.EncodeToString(publicKey),
		Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload)),
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "receipt.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Schema string `json:"schema"`
		Value  string `json:"value"`
	}
	_, err = verifySignedPayload(dir, fileBinding{Path: "receipt.json", SHA256: hashBytes(raw)}, envelope.PublicKey, &decoded)
	if err != nil || decoded.Schema != "test/v1" || decoded.Value != "bound" {
		t.Fatalf("verifySignedPayload = %#v, %v", decoded, err)
	}
}

func TestReadNoFollowRejectsFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("sealed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readNoFollow(link); err == nil {
		t.Fatal("readNoFollow accepted a final-component symlink")
	}
}

func TestFrozenRecipeParsesAndPassesStaticPolicy(t *testing.T) {
	raw, err := os.ReadFile("../../go_c_timing/static_build_recipe_v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var recipe staticBuildRecipe
	if err := decodeStrict(raw, &recipe); err != nil {
		t.Fatalf("decode recipe: %v", err)
	}
	if err := verifyRecipe(&recipe); err != nil {
		t.Fatalf("verify recipe: %v", err)
	}
	if err := verifyDriverSourceClosure(filepath.Join("..", "..", ".."), recipe.Driver); err != nil {
		t.Fatalf("verify recipe source closure: %v", err)
	}
	driver, err := os.ReadFile("../../pure_c/go_timing_oracle.c")
	if err != nil {
		t.Fatal(err)
	}
	if got := hashBytes(driver); got != recipe.Driver.SHA256 {
		t.Fatalf("recipe driver hash = %s, source hash = %s", recipe.Driver.SHA256, got)
	}
}

func TestTimingOracleTenSecondWrapperSharesCanonicalImplementation(t *testing.T) {
	canonical, err := os.ReadFile("../../pure_c/go_timing_oracle.c")
	if err != nil {
		t.Fatal(err)
	}
	tenSecond, err := os.ReadFile("../../pure_c/go_timing_oracle_10s.c")
	if err != nil {
		t.Fatal(err)
	}
	canonicalText := string(canonical)
	tenSecondText := string(tenSecond)
	const durationMacro = "GTS_TIMING_ORACLE_MIN_ELAPSED_NS"
	if !strings.Contains(canonicalText, "#define "+durationMacro+" UINT64_C(750000000)") {
		t.Fatal("canonical timing oracle lost its 750-millisecond default")
	}
	if !strings.Contains(canonicalText, "k_min_elapsed_ns = "+durationMacro) {
		t.Fatal("canonical timing oracle does not use the configurable duration")
	}
	if !strings.Contains(tenSecondText, "#define "+durationMacro+" UINT64_C(10000000000)") {
		t.Fatal("ten-second timing oracle lost its ten-second duration")
	}
	if !strings.Contains(tenSecondText, "#include \"go_timing_oracle.c\"") {
		t.Fatal("ten-second timing oracle does not include the canonical implementation")
	}
	for _, marker := range []string{"static const fixture_t k_fixtures[]", "int main("} {
		if strings.Contains(tenSecondText, marker) {
			t.Fatalf("ten-second timing oracle contains duplicated implementation marker %q", marker)
		}
	}

	raw, err := os.ReadFile("../../go_c_timing/static_build_recipe_v3_enclave_10s.json")
	if err != nil {
		t.Fatal(err)
	}
	var recipe staticBuildRecipe
	if err := decodeStrict(raw, &recipe); err != nil {
		t.Fatalf("decode ten-second recipe: %v", err)
	}
	if err := verifyRecipe(&recipe); err != nil {
		t.Fatalf("verify ten-second recipe: %v", err)
	}
	if err := verifyDriverSourceClosure(filepath.Join("..", "..", ".."), recipe.Driver); err != nil {
		t.Fatalf("verify ten-second source closure: %v", err)
	}
	if recipe.Driver.Path != "cgo_harness/pure_c/go_timing_oracle_10s.c" {
		t.Fatalf("ten-second recipe driver path = %q", recipe.Driver.Path)
	}
	if got := hashBytes(tenSecond); got != recipe.Driver.SHA256 {
		t.Fatalf("ten-second recipe driver hash = %s, source hash = %s", recipe.Driver.SHA256, got)
	}
}

func TestTimingOracleSourceClosureRejectsIncludedSourceMutation(t *testing.T) {
	raw, err := os.ReadFile("../../go_c_timing/static_build_recipe_v3_enclave_10s.json")
	if err != nil {
		t.Fatal(err)
	}
	var recipe staticBuildRecipe
	if err := decodeStrict(raw, &recipe); err != nil {
		t.Fatalf("decode ten-second recipe: %v", err)
	}
	sourceBase := t.TempDir()
	repoRoot := filepath.Join("..", "..", "..")
	for _, binding := range recipe.Driver.SourceClosure {
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(binding.Path)))
		if err != nil {
			t.Fatalf("read %s: %v", binding.Path, err)
		}
		path := filepath.Join(sourceBase, filepath.FromSlash(binding.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	canonicalPath := filepath.Join(sourceBase, filepath.FromSlash("cgo_harness/pure_c/go_timing_oracle.c"))
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	canonical = append(canonical, '\n')
	if err := os.WriteFile(canonicalPath, canonical, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyDriverSourceClosure(sourceBase, recipe.Driver); err == nil {
		t.Fatal("source closure accepted a mutation to the included canonical source")
	}
}

func TestTimingOracleSourceClosureRejectsClosureHashMutation(t *testing.T) {
	raw, err := os.ReadFile("../../go_c_timing/static_build_recipe_v3_enclave_10s.json")
	if err != nil {
		t.Fatal(err)
	}
	var recipe staticBuildRecipe
	if err := decodeStrict(raw, &recipe); err != nil {
		t.Fatalf("decode ten-second recipe: %v", err)
	}
	recipe.Driver.SourceClosureSHA256 = strings.Repeat("a", 64)
	if err := verifyRecipe(&recipe); err == nil {
		t.Fatal("recipe accepted a mutated source closure hash")
	}
}

func TestArtifactSourceClosureRejectsClosureHashMutation(t *testing.T) {
	raw, err := os.ReadFile("../../go_c_timing/static_build_recipe_v3_enclave_10s.json")
	if err != nil {
		t.Fatal(err)
	}
	var recipe staticBuildRecipe
	if err := decodeStrict(raw, &recipe); err != nil {
		t.Fatalf("decode ten-second recipe: %v", err)
	}
	inputs := buildInputs{
		SourceClosure:       recipe.Driver.SourceClosure,
		SourceClosureSHA256: recipe.Driver.SourceClosureSHA256,
	}
	inputs.SourceClosureSHA256 = strings.Repeat("b", 64)
	if err := verifyBuildSourceClosure(inputs, recipe.Driver); err == nil {
		t.Fatal("artifact inputs accepted a mutated source closure hash")
	}
}

func TestRecipeRejectsMissingSourceClosure(t *testing.T) {
	raw, err := os.ReadFile("../../go_c_timing/static_build_recipe_v3.json")
	if err != nil {
		t.Fatal(err)
	}
	var recipe staticBuildRecipe
	if err := decodeStrict(raw, &recipe); err != nil {
		t.Fatalf("decode recipe: %v", err)
	}
	recipe.Driver.SourceClosure = nil
	recipe.Driver.SourceClosureSHA256 = ""
	if err := verifyRecipe(&recipe); err == nil {
		t.Fatal("recipe accepted missing source closure binding")
	}
}
