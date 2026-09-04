package grammars

import (
	"os"
	"regexp"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

var scannerMatrixRowRE = regexp.MustCompile("(?m)^\\| `([^`]+)` \\| ([^|]+) \\|$")

func TestExternalScannerDocumentationCertificationsMatchAttachedRuntimeRegistry(t *testing.T) {
	certifications := documentedExternalScannerCertifications(t)
	if len(certifications) != len(externalScannerRegistry) {
		t.Fatalf("scanner matrix has %d rows, runtime registry has %d languages", len(certifications), len(externalScannerRegistry))
	}

	ordinaryRegistry := make(map[string]LangEntry)
	for _, entry := range AllLanguages() {
		ordinaryRegistry[entry.Name] = entry
	}
	for name := range externalScannerRegistry {
		entry, ok := ordinaryRegistry[name]
		if !ok || entry.Language == nil {
			t.Errorf("runtime external scanner %q has no ordinary language-registry loader", name)
			continue
		}

		probe := &gts.Language{Name: name}
		if !attachExternalScannerForLanguage(name, probe) || probe.ExternalScanner == nil {
			t.Errorf("runtime external scanner %q did not attach through the embedded-loader path", name)
			continue
		}
		lang := entry.Language()
		if lang == nil || lang.ExternalScanner == nil {
			t.Errorf("runtime external scanner %q did not attach to its ordinary language", name)
			continue
		}
		want := externalScannerCertification(lang.ExternalScanner)
		got, ok := certifications[name]
		if !ok {
			t.Errorf("scanner matrix is missing runtime language %q", name)
			continue
		}
		if got != want {
			t.Errorf("scanner matrix certification for %q = %q, want %q", name, got, want)
		}
	}
	for name := range certifications {
		if _, ok := externalScannerRegistry[name]; !ok {
			t.Errorf("scanner matrix contains language %q with no runtime scanner registration", name)
		}
	}
}

func documentedExternalScannerCertifications(t *testing.T) map[string]string {
	t.Helper()
	content, err := os.ReadFile("../docs/external-scanners.md")
	if err != nil {
		t.Fatal(err)
	}
	const startHeading = "### Incremental-reuse certification matrix"
	const endHeading = "## Hard-learned behavioral contracts"
	start := strings.Index(string(content), startHeading)
	end := strings.Index(string(content), endHeading)
	if start < 0 || end < 0 || end <= start {
		t.Fatalf("external-scanner matrix section is missing or malformed")
	}
	matches := scannerMatrixRowRE.FindAllStringSubmatch(string(content[start:end]), -1)
	certifications := make(map[string]string, len(matches))
	for _, match := range matches {
		name := match[1]
		if _, exists := certifications[name]; exists {
			t.Fatalf("scanner matrix contains duplicate language %q", name)
		}
		contract := strings.TrimSpace(match[2])
		if strings.HasPrefix(contract, "bounded reuse") {
			// Bounded reuse is scanner-certified reuse plus a parser-level source
			// guard. The root runtime test checks the exact bound against its
			// production constant; this registry test checks scanner certification.
			contract = "certified reuse"
		}
		certifications[name] = contract
	}
	return certifications
}

func externalScannerCertification(scanner gts.ExternalScanner) string {
	reusable, certified := scanner.(gts.IncrementalReuseExternalScanner)
	if !certified {
		return "fallback (uncertified)"
	}
	if !reusable.SupportsIncrementalReuse() {
		return "fallback (explicit opt-out)"
	}
	return "certified reuse"
}
