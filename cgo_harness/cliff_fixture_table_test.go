//go:build linux && cgo && treesitter_c_parity

package cgoharness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// The cliff report measures the seven R7 fixtures in cliffFixtures and the
// coverage fixtures below. The coverage fixtures bring the report to every
// top-20 language (the first 20 names in grammars/update_tier1_top50.txt), so
// each engine pull request can read a "before" column that the nightly run
// measured on main.
//
// Every fixture states its grammar, where its bytes come from, and its exact
// size. Every source is committed or generated in this repository, and the
// loader checks its SHA-256 digest before the first parse. A coverage fixture
// reuses bytes that another manifest already pins (editor-latency, the real
// corpus, or the generated set). TestCliffFixtureTable compares the two pins, so
// they cannot drift apart. Only the seven R7 archives have no second pin.

// cliffSourceKind says where a fixture gets its bytes.
type cliffSourceKind string

const (
	// cliffArchive is a gzip file in internal/benchfixtures/testdata/cliffs.
	// Only this package pins its digest. The seven R7 fixtures use it.
	cliffArchive cliffSourceKind = "archive"
	// cliffWholeFile is a gzip file in the same directory. It holds a whole
	// upstream file that cgo_harness/editor_latency/fixtures.json also pins by
	// repository, commit, path, size, and digest.
	cliffWholeFile cliffSourceKind = "whole_file"
	// cliffSample is the counter-ledger sample for the grammar. It lives under
	// internal/benchfixtures/testdata/real and real_corpus.json pins it.
	cliffSample cliffSourceKind = "sample"
	// cliffGenerated is benchfixtures.GeneratedSource at cliffGeneratedBytes.
	// generated.json pins the digest.
	cliffGenerated cliffSourceKind = "generated"
)

// cliffGeneratedBytes is the target size of every generated fixture, the 32 KiB
// tier of the v1 performance gate.
const cliffGeneratedBytes = 32 << 10

// cliffInput is one fixture with an explicit grammar, source, and size.
type cliffInput struct {
	// Name selects the fixture (GTS_CLIFF_LANGUAGE) and names its report file.
	Name string
	// Grammar is the grammars.DetectLanguageByName key for Go and the C oracle.
	Grammar string
	Kind    cliffSourceKind
	// Source is a file name in testdata/cliffs for the archive kinds, a path
	// under internal/benchfixtures for a sample, or a GeneratedSource shape.
	Source string
	// Bytes is the exact size of the decompressed source.
	Bytes  int
	SHA256 string
}

// cliffArchiveBytes lists the decompressed sizes of the seven R7 archives. They
// predate cliffInput, and cliffFixtures stays unchanged, so the sizes live here.
var cliffArchiveBytes = map[string]int{
	"c_sharp":  32809,
	"php":      144990,
	"go":       75688,
	"python":   32831,
	"elixir":   32770,
	"markdown": 54476,
	"html":     27268,
}

// cliffCoverageFixtures adds the top-20 languages that the R7 fixtures miss,
// plus a real file for Python, C#, PHP, and Go, next to their R7 cliff
// fixtures. The Go file, elf.go, is a real file and also a Go cliff witness:
// when added, its legacy route failed the cliff rule on main (multi-stack share
// 99.48% against C's 1.46%). A language without a committed whole file gets its
// counter-ledger sample and a generated 32 KiB file. PowerShell is in the top
// 50 but not the top 20. Its whole file comes from the same editor-latency
// manifest.
//
// git's t/t0003-attributes.sh is the editor-latency Bash file. It is licensed
// GPL-2.0-only, so this table does not commit it. Bash uses the committed
// sample and a generated file instead.
var cliffCoverageFixtures = []cliffInput{
	{"typescript_extract_type", "typescript", cliffWholeFile, "typescript_extractType.ts.gz", 19950, "1c9771844ea117e1414365311fb181ca3a8fc58dcea06e761abd6d47c190a93f"},
	{"tsx_open_source_app_list", "tsx", cliffWholeFile, "tsx_openSourceAppList.tsx.gz", 11305, "db25d3e10b36b102ce31dbd3eeb8de972c743a931ed9f1c5830a87a6ad318ec8"},
	{"javascript_async_hooks", "javascript", cliffWholeFile, "javascript_async_hooks.js.gz", 20575, "c3ba587240b9a3bf529bf9f5b084dd8d210bb90e9627eaddc75d09bb36957ce9"},
	{"python_asyncio_locks", "python", cliffWholeFile, "python_locks.py.gz", 20508, "dd622639fdeeee94bd0164bb5dcd0d679a47a0a7f3bdb6094893834bc305f6b7"},
	{"java_jdbc_utils", "java", cliffWholeFile, "java_JdbcUtils.java.gz", 20455, "9b9efbdd7e6c1dd5c505bf3dd9e93d4247948a95833eb6f7edf5bbe897f25140"},
	{"c_sharp_resource_type_provider_tests", "c_sharp", cliffWholeFile, "c_sharp_AzResourceTypeProviderTests.cs.gz", 20247, "dbbae94b9dd80852bd58939f29492017d1b727c87b91f64bc3b362caaa6c9518"},
	{"php_editor_document", "php", cliffWholeFile, "php_EditorDocument.php.gz", 20597, "cd3f81810093827c3c5287c39e184250d45b55dce53801b3d4a0df000bfcc9c2"},
	{"go_debug_elf", "go", cliffWholeFile, "go_elf.go.gz", 140026, "ff2f811d29a56303bad96c3c7cff1a83814a974503a51cee918a314a96eb476e"},
	{"powershell_get_content_tests", "powershell", cliffWholeFile, "powershell_Get-Content.Tests.ps1.gz", 19441, "7210a67d7bc3039211067e0a5a9a3731cb340477673fed80b0531d2c0a94c3e2"},

	{"bash_sample", "bash", cliffSample, "testdata/real/bash", 2051, "ac333a366e8f2ac0b6d4437bc51c397ae8a6949c52b6453d85a86ba47389e618"},
	{"cpp_sample", "cpp", cliffSample, "testdata/real/cpp", 2321, "d0d1080c2d248595f039ddcb92073669109608f6db41ee99cf18310dc453404c"},
	{"css_sample", "css", cliffSample, "testdata/real/css", 2487, "fab8a4375ea7346fadbfd6741f1219721cab3c6021c7c0bc3d746a1dde8f4dd8"},
	{"sql_sample", "sql", cliffSample, "testdata/real/sql", 2049, "287954e902826960e9dec8f7e86bc0e39ca1e257fe25c0641f9f9fd5a4594e0d"},
	{"c_sample", "c", cliffSample, "testdata/real/c", 2049, "ccdf9a3671f4f291c653fce2ee409557cadddc893494c112ce49a1cf9c139917"},
	{"rust_sample", "rust", cliffSample, "testdata/real/rust", 2051, "a105a8715e68dfa5ea5bd82105f10a2d13f903bdc0f3b3424e257d70ba7ee6a3"},
	{"json_sample", "json", cliffSample, "testdata/real/json", 2026, "2a07c365e46b82ccf9d07d3dba72072e944edd3cbe1d573c4110c2bcc339131e"},
	{"ruby_sample", "ruby", cliffSample, "testdata/real/ruby", 2047, "260bd46e861522f8a8642c5d5934145d45f35058beeb087942b0c8ef38424011"},
	{"swift_sample", "swift", cliffSample, "testdata/real/swift", 2058, "d2d93b53efb2a837b3d103c01be947178863cbe5af5e872171ee1dbca0b47433"},
	{"kotlin_sample", "kotlin", cliffSample, "testdata/real/kotlin", 2051, "2e6076c2fa91622186aa0a325f8e5984ad26844f029fdae2802a4be4faf95c65"},
	{"dart_sample", "dart", cliffSample, "testdata/real/dart", 2047, "d9fdd1687d8d2fd2e3f9dc741ec3330def943ca3152b412168c5469d880c6d63"},

	{"bash_generated_32k", "bash", cliffGenerated, "sh", 32793, "afbf4ba466bbdec37d50dcfe308d39a70ab6d748a80d186df817ff1dcd4db6d3"},
	{"cpp_generated_32k", "cpp", cliffGenerated, "cpp", 32776, "e5cd10681da18d07c1c3201e9f02e5897cd7d062f35494a645da7a493a9df172"},
	{"css_generated_32k", "css", cliffGenerated, "css", 32770, "54a8ea8634538a0f132f96d1c579797663f87b4172dda6b1396a638260fd02e2"},
	{"sql_generated_32k", "sql", cliffGenerated, "sql", 32785, "686be8d924c2a931f31371517a72be713dca26c3ed4d92c33e142802267895da"},
	{"c_generated_32k", "c", cliffGenerated, "c", 32828, "2b1393bd58e419e5ca594d25eaa3cc3ede15d9750f03dde23641ecac631d9ba6"},
	{"rust_generated_32k", "rust", cliffGenerated, "rust", 32808, "fa727860dc29f7268b5df4a45370de844bf916c0662e4e49f7aa192d85a0b756"},
	{"json_generated_32k", "json", cliffGenerated, "json", 32796, "a7030fa0ae797e2de8bd1bb45a33ad3627edd68269cbdd396502ae4cd344a1eb"},
	{"ruby_generated_32k", "ruby", cliffGenerated, "ruby", 32798, "7fbac69a04e592ed746effc9602c2e9afa0695a06d6799b06d175277ae3d3aea"},
	{"swift_generated_32k", "swift", cliffGenerated, "swift", 32787, "076e9bb96bcebd7af246f27b726528cb3ad26ed42e234ca8b53d0695f60298e3"},
	{"kotlin_generated_32k", "kotlin", cliffGenerated, "kotlin", 32770, "d73ecd029cff54f9bf35daa973304cc60fb5483acd5e50dd868b11f309648137"},
	{"dart_generated_32k", "dart", cliffGenerated, "dart-report", 32842, "1758f045013ad5924e8cd0738b969e7175075f5cd86dce3bd6f59d91d1285e27"},
}

// cliffInputs returns the seven R7 fixtures, unchanged and in order, followed
// by the coverage fixtures.
func cliffInputs() []cliffInput {
	inputs := make([]cliffInput, 0, len(cliffFixtures)+len(cliffCoverageFixtures))
	for _, fixture := range cliffFixtures {
		inputs = append(inputs, cliffInput{Name: fixture.Name, Grammar: fixture.Grammar, Kind: cliffArchive,
			Source: fixture.File, Bytes: cliffArchiveBytes[fixture.Name], SHA256: fixture.SHA256})
	}
	return append(inputs, cliffCoverageFixtures...)
}

// loadCliffInput returns the fixture's bytes after it checks the size and the
// SHA-256 digest. It stops the test when either differs.
func loadCliffInput(t *testing.T, input cliffInput) []byte {
	t.Helper()
	var source []byte
	switch input.Kind {
	case cliffArchive, cliffWholeFile:
		source = loadCliffSource(t, cliffFixture{Name: input.Name, Grammar: input.Grammar, File: input.Source, SHA256: input.SHA256})
	case cliffSample:
		var err error
		source, err = os.ReadFile(filepath.Join("..", "internal", "benchfixtures", input.Source))
		if err != nil {
			t.Fatal(err)
		}
	case cliffGenerated:
		var err error
		source, _, err = benchfixtures.GeneratedSource(input.Source, cliffGeneratedBytes)
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("fixture %s has unknown source kind %q", input.Name, input.Kind)
	}
	digest := sha256.Sum256(source)
	if got := hex.EncodeToString(digest[:]); got != input.SHA256 {
		t.Fatalf("fixture %s digest %s, want %s", input.Name, got, input.SHA256)
	}
	if len(source) != input.Bytes {
		t.Fatalf("fixture %s is %d bytes, want %d", input.Name, len(source), input.Bytes)
	}
	return source
}

// TestCliffFixtureNames prints one CLIFF_FIXTURE line per fixture, in table
// order. scripts/run_cliff_report_in_docker.sh reads these lines, so the table
// is the only list of fixtures. The test is silent unless GTS_CLIFF_LIST=1.
func TestCliffFixtureNames(t *testing.T) {
	if os.Getenv("GTS_CLIFF_LIST") != "1" {
		t.Skip("set GTS_CLIFF_LIST=1 to list the fixture names")
	}
	for _, input := range cliffInputs() {
		fmt.Printf("CLIFF_FIXTURE %s\n", input.Name)
	}
}

type cliffPin struct {
	bytes  int
	sha256 string
}

// TestCliffFixtureTable checks the fixture table without the C oracle. It
// loads every source, compares its size and digest with the manifest that pins
// the same bytes, and requires every top-20 language to have a fixture.
func TestCliffFixtureTable(t *testing.T) {
	var editor struct {
		Fixtures []struct {
			Language string `json:"language"`
			Bytes    int    `json:"bytes"`
			SHA256   string `json:"sha256"`
		} `json:"fixtures"`
	}
	readCliffManifest(t, filepath.Join("editor_latency", "fixtures.json"), &editor)
	wholeFiles := map[string]cliffPin{}
	for _, fixture := range editor.Fixtures {
		wholeFiles[fixture.Language] = cliffPin{fixture.Bytes, fixture.SHA256}
	}

	var realCorpus struct {
		Entries []struct {
			Language      string `json:"language"`
			Role          string `json:"role"`
			CommittedPath string `json:"committed_path"`
			Bytes         int    `json:"bytes"`
			SHA256        string `json:"sha256"`
		} `json:"entries"`
	}
	readCliffManifest(t, filepath.Join("..", "internal", "benchfixtures", "real_corpus.json"), &realCorpus)
	samples := map[string]cliffPin{}
	samplePaths := map[string]string{}
	for _, entry := range realCorpus.Entries {
		if entry.Role == "sample" && entry.CommittedPath != "" {
			samples[entry.Language] = cliffPin{entry.Bytes, entry.SHA256}
			samplePaths[entry.Language] = entry.CommittedPath
		}
	}

	var generated struct {
		Entries []struct {
			Language    string `json:"language"`
			TargetBytes int    `json:"target_bytes"`
			SourceBytes int    `json:"source_bytes"`
			SHA256      string `json:"sha256"`
		} `json:"entries"`
	}
	readCliffManifest(t, filepath.Join("..", "internal", "benchfixtures", "generated.json"), &generated)
	shapes := map[string]cliffPin{}
	for _, entry := range generated.Entries {
		if entry.TargetBytes == cliffGeneratedBytes {
			shapes[entry.Language] = cliffPin{entry.SourceBytes, entry.SHA256}
		}
	}

	names := map[string]bool{}
	covered := map[string]bool{}
	namePattern := regexp.MustCompile(`^[a-z0-9_]+$`)
	inputs := cliffInputs()
	if len(inputs) != len(cliffFixtures)+len(cliffCoverageFixtures) {
		t.Fatalf("cliffInputs returned %d fixtures", len(inputs))
	}
	for index, input := range inputs {
		if index < len(cliffFixtures) && input.Name != cliffFixtures[index].Name {
			t.Fatalf("fixture %d is %s, want the R7 fixture %s first and in order", index, input.Name, cliffFixtures[index].Name)
		}
		if !namePattern.MatchString(input.Name) {
			t.Errorf("fixture name %q must use lowercase letters, digits, and underscores", input.Name)
		}
		if names[input.Name] {
			t.Errorf("duplicate fixture name %q", input.Name)
		}
		names[input.Name] = true
		covered[input.Grammar] = true
		t.Run(input.Name, func(t *testing.T) {
			if entry := grammars.DetectLanguageByName(input.Grammar); entry == nil {
				t.Fatalf("Go grammar %q unavailable", input.Grammar)
			}
			if input.Bytes <= 0 {
				t.Fatalf("fixture %s must state its size", input.Name)
			}
			loadCliffInput(t, input)
			var pin cliffPin
			var pinned bool
			switch input.Kind {
			case cliffWholeFile:
				pin, pinned = wholeFiles[input.Grammar]
			case cliffSample:
				pin, pinned = samples[input.Grammar]
				if pinned && samplePaths[input.Grammar] != input.Source {
					t.Fatalf("sample path %s, manifest names %s", input.Source, samplePaths[input.Grammar])
				}
			case cliffGenerated:
				pin, pinned = shapes[input.Source]
			case cliffArchive:
				return
			}
			if !pinned {
				t.Fatalf("no manifest pins %s source %s", input.Kind, input.Source)
			}
			if pin.bytes != input.Bytes || pin.sha256 != input.SHA256 {
				t.Fatalf("manifest pins %d bytes %s, table has %d bytes %s", pin.bytes, pin.sha256, input.Bytes, input.SHA256)
			}
		})
	}

	// Tier 1 is the first 20 names in the top-50 list (docs/v1-design.md, P1).
	list, err := os.ReadFile(filepath.Join("..", "grammars", "update_tier1_top50.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var top20 []string
	for _, line := range strings.Split(string(list), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			top20 = append(top20, line)
		}
	}
	if len(top20) < 20 {
		t.Fatalf("the top-50 list names only %d languages", len(top20))
	}
	for _, name := range top20[:20] {
		if !covered[name] {
			t.Errorf("top-20 language %s has no cliff fixture", name)
		}
	}
}

func readCliffManifest(t *testing.T, path string, into any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
