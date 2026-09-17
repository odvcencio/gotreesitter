package grammarruntime

import (
	"sort"
	"strings"
	"sync"
)

// ExternalScannerSourceFile identifies an upstream scanner-facing source file.
// The hash is SHA-256 over the exact file bytes at ExternalScannerSpec.UpstreamCommit.
type ExternalScannerSourceFile struct {
	Path   string
	SHA256 string
}

// ExternalScannerSpec records the source contract for a hand-written external
// scanner port. It lets updater tooling distinguish grammar-only changes from
// external token or scanner-source changes that require scanner review.
type ExternalScannerSpec struct {
	Language       string
	UpstreamRepo   string
	UpstreamCommit string
	SourceFiles    []ExternalScannerSourceFile
	Externals      []string
}

var externalScannerSpecRegistry = struct {
	sync.RWMutex
	byName map[string]ExternalScannerSpec
}{
	byName: map[string]ExternalScannerSpec{},
}

// RegisterExternalScannerSpec records scanner-source metadata for a language.
func RegisterExternalScannerSpec(spec ExternalScannerSpec) {
	name := normalizeExternalScannerSpecName(spec.Language)
	if name == "" {
		return
	}
	spec.Language = name
	externalScannerSpecRegistry.Lock()
	defer externalScannerSpecRegistry.Unlock()
	externalScannerSpecRegistry.byName[name] = cloneExternalScannerSpec(spec)
}

// LookupExternalScannerSpec returns scanner-source metadata for a language.
func LookupExternalScannerSpec(name string) (ExternalScannerSpec, bool) {
	name = normalizeExternalScannerSpecName(name)
	if name == "" {
		return ExternalScannerSpec{}, false
	}
	externalScannerSpecRegistry.RLock()
	defer externalScannerSpecRegistry.RUnlock()
	spec, ok := externalScannerSpecRegistry.byName[name]
	if !ok {
		return ExternalScannerSpec{}, false
	}
	return cloneExternalScannerSpec(spec), true
}

// ExternalScannerSpecs returns all registered scanner-source specs, sorted by language.
func ExternalScannerSpecs() []ExternalScannerSpec {
	externalScannerSpecRegistry.RLock()
	defer externalScannerSpecRegistry.RUnlock()
	out := make([]ExternalScannerSpec, 0, len(externalScannerSpecRegistry.byName))
	for _, spec := range externalScannerSpecRegistry.byName {
		out = append(out, cloneExternalScannerSpec(spec))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Language < out[j].Language
	})
	return out
}

func cloneExternalScannerSpec(spec ExternalScannerSpec) ExternalScannerSpec {
	if len(spec.SourceFiles) > 0 {
		spec.SourceFiles = append([]ExternalScannerSourceFile(nil), spec.SourceFiles...)
	}
	if len(spec.Externals) > 0 {
		spec.Externals = append([]string(nil), spec.Externals...)
	}
	return spec
}

func normalizeExternalScannerSpecName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
